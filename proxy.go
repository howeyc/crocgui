// proxy.go
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/crypt"
	log "github.com/schollz/logger"
	"github.com/schollz/pake/v3"
)

// Константы для прокси
const (
	ProxyHTTPRequest  = "PROXY-REQ"
	ProxyHTTPResponse = "PROXY-RESP"
	ProxyHTTPChunk    = "PROXY-CHUNK"
	ProxyPing         = "PROXY-PING"
	ProxyClose        = "PROXY-CLOSE"

	defaultProxyPort = "8080"
	requestTimeout   = 30 * time.Second
)

// weakKey для PAKE (как в croc)
var weakKey = []byte{1, 2, 3}

// TunnelRoom представляет туннельное соединение
type TunnelRoom struct {
	ControlConn *comm.Comm
	DataConns   []*comm.Comm
	Key         []byte
	RoomName    string
	IsSender    bool
	closed      bool
	mu          sync.RWMutex
}

// RequestManager управляет ожидающими HTTP запросами
type RequestManager struct {
	pending map[string]chan *http.Response
	mu      sync.RWMutex
}

// CrocProxy реализует интерфейс ProxyHandler
type CrocProxy struct {
	opts        *croc.Options
	tunnel      *TunnelRoom
	requestMgr  *RequestManager
	active      bool
	mu          sync.RWMutex
	proxyServer *http.Server // для клиентского режима
	stopChan    chan struct{}
	webdavURL   string // URL локального WebDAV сервера (для sender)
}

// NewRequestManager создает новый менеджер запросов
func NewRequestManager() *RequestManager {
	return &RequestManager{
		pending: make(map[string]chan *http.Response),
	}
}

// Add добавляет канал для ожидания ответа
func (rm *RequestManager) Add(id string) chan *http.Response {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	ch := make(chan *http.Response, 1)
	rm.pending[id] = ch
	return ch
}

// Get получает и удаляет канал для ответа
func (rm *RequestManager) Get(id string) chan *http.Response {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if ch, ok := rm.pending[id]; ok {
		delete(rm.pending, id)
		return ch
	}
	return nil
}

// NewCrocProxy создает новый экземпляр прокси
func NewCrocProxy(opts *croc.Options) *CrocProxy {
	return &CrocProxy{
		opts:       opts,
		requestMgr: NewRequestManager(),
		stopChan:   make(chan struct{}),
	}
}

// SetWebDAVURL устанавливает URL локального WebDAV сервера (для sender)
func (p *CrocProxy) SetWebDAVURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.webdavURL = url
}

// Wrap implements ProxyHandler interface
func (p *CrocProxy) Wrap(next http.Handler) http.Handler {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.opts.IsSender {
		// Серверный режим - создаем туннель и обрабатываем запросы
		go p.runServerMode(next)
	}

	// Возвращаем handler который решает, что делать с запросом
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.RLock()
		active := p.active
		tunnel := p.tunnel
		p.mu.RUnlock()

		if !active || tunnel == nil {
			// Прокси не активен - передаем дальше
			next.ServeHTTP(w, r)
			return
		}

		if p.opts.IsSender {
			// Сервер: обрабатываем запрос локально
			next.ServeHTTP(w, r)
		} else {
			// Клиент: отправляем запрос через туннель
			p.handleClientRequest(w, r, next)
		}
	})
}

// IsActive implements ProxyHandler interface
func (p *CrocProxy) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// Stop implements Stoppable interface
func (p *CrocProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return nil
	}

	p.active = false

	// Безопасно закрываем stopChan - если уже закрыт, игнорируем
	select {
	case <-p.stopChan:
		// Канал уже закрыт
	default:
		close(p.stopChan)
	}

	// Останавливаем прокси сервер если есть
	if p.proxyServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.proxyServer.Shutdown(ctx); err != nil {
			log.Warnf("Error stopping proxy server: %v", err)
		}
		p.proxyServer = nil
	}

	// Закрываем туннель
	if p.tunnel != nil {
		p.tunnel.mu.Lock()
		p.tunnel.closed = true
		p.tunnel.ControlConn.Close()
		for _, conn := range p.tunnel.DataConns {
			conn.Close()
		}
		p.tunnel.mu.Unlock()
		p.tunnel = nil
	}

	log.Info("Croc proxy stopped")
	return nil
}

// StartProxyClient запускает клиентский режим (отдельно от Wrap)
func (p *CrocProxy) StartProxyClient(listenAddr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.opts.IsSender {
		return fmt.Errorf("StartProxyClient called with IsSender=true")
	}

	// Создаем туннель
	tunnel, err := p.connectToRelay()
	if err != nil {
		return err
	}

	p.tunnel = tunnel
	p.active = true

	// Создаем ReverseProxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "croc-tunnel"
		},
		Transport: p,
	}

	// Запускаем сервер
	p.proxyServer = &http.Server{
		Addr:    listenAddr,
		Handler: proxy,
	}

	log.Infof("Croc proxy client listening on %s", listenAddr)
	go func() {
		if err := p.proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Proxy server error: %v", err)
		}
	}()

	// Запускаем обработчик входящих сообщений
	go p.receiveLoop()

	return nil
}

// RoundTrip implements http.RoundTripper для клиентского режима
func (p *CrocProxy) RoundTrip(req *http.Request) (*http.Response, error) {
	requestID := generateRequestID()
	respChan := p.requestMgr.Add(requestID)
	defer p.requestMgr.Get(requestID)

	// Сериализуем запрос
	reqData, err := serializeRequest(req)
	if err != nil {
		return nil, err
	}

	p.mu.RLock()
	tunnel := p.tunnel
	p.mu.RUnlock()

	if tunnel == nil {
		return nil, fmt.Errorf("tunnel not established")
	}

	// Создаем сообщение в формате croc
	msg := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Bytes   []byte `json:"bytes"`
		Bytes2  []byte `json:"bytes2"`
		Num     int    `json:"num"`
	}{
		Type:    "message",
		Message: ProxyHTTPRequest,
		Bytes:   reqData,
		Bytes2:  []byte(requestID),
		Num:     0,
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	// Шифруем если есть ключ
	if tunnel.Key != nil {
		encoded, err = crypt.Encrypt(encoded, tunnel.Key)
		if err != nil {
			return nil, err
		}
	}

	if err := tunnel.ControlConn.Send(encoded); err != nil {
		return nil, err
	}

	// Ждем ответ
	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(requestTimeout):
		return nil, fmt.Errorf("request timeout")
	case <-p.stopChan:
		return nil, fmt.Errorf("proxy stopped")
	}
}

// receiveLoop обрабатывает входящие сообщения от сервера
func (p *CrocProxy) receiveLoop() {
	for {
		select {
		case <-p.stopChan:
			log.Debug("receiveLoop stopped")
			return
		default:
		}

		p.mu.RLock()
		tunnel := p.tunnel
		active := p.active
		p.mu.RUnlock()

		if !active || tunnel == nil {
			return
		}

		data, err := tunnel.ControlConn.Receive()
		if err != nil {
			log.Errorf("Error in receive loop: %v", err)
			break
		}

		// Расшифровываем если есть ключ
		if tunnel.Key != nil {
			data, err = crypt.Decrypt(data, tunnel.Key)
			if err != nil {
				log.Errorf("Failed to decrypt message: %v", err)
				continue
			}
		}

		// Декодируем сообщение
		var msg struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Bytes   []byte `json:"bytes"`
			Bytes2  []byte `json:"bytes2"`
			Num     int    `json:"num"`
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			log.Errorf("Failed to decode message: %v", err)
			continue
		}

		if msg.Type == "message" && msg.Message == ProxyHTTPResponse {
			// Это ответ на наш запрос
			requestID := string(msg.Bytes2)
			respChan := p.requestMgr.Get(requestID)
			if respChan != nil {
				resp, err := deserializeResponse(msg.Bytes, msg.Num)
				if err != nil {
					log.Errorf("Failed to deserialize response: %v", err)
					continue
				}
				respChan <- resp
			}
		}
	}
}

// runServerMode запускает серверный режим (ожидает запросы от клиента)
func (p *CrocProxy) runServerMode(next http.Handler) {
	// Создаем туннель
	tunnel, err := p.connectToRelay()
	if err != nil {
		log.Errorf("Failed to create tunnel: %v", err)
		return
	}

	p.mu.Lock()
	p.tunnel = tunnel
	p.active = true
	p.mu.Unlock()

	log.Info("Proxy server mode active, waiting for client requests")

	// Обрабатываем входящие запросы от клиента
	for {
		select {
		case <-p.stopChan:
			log.Debug("runServerMode stopped")
			return
		default:
		}

		// Получаем сообщение от клиента
		data, err := tunnel.ControlConn.Receive()
		if err != nil {
			log.Errorf("Error receiving from control conn: %v", err)
			break
		}

		// Расшифровываем если есть ключ
		if tunnel.Key != nil {
			data, err = crypt.Decrypt(data, tunnel.Key)
			if err != nil {
				log.Errorf("Failed to decrypt message: %v", err)
				continue
			}
		}

		// Декодируем сообщение
		var msg struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Bytes   []byte `json:"bytes"`
			Bytes2  []byte `json:"bytes2"`
			Num     int    `json:"num"`
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			log.Errorf("Failed to decode message: %v", err)
			continue
		}

		if msg.Type == "message" && msg.Message == ProxyHTTPRequest {
			// Это HTTP запрос от клиента
			go p.processClientRequest(msg, next)
		}
	}
}

// processClientRequest обрабатывает запрос от клиента
func (p *CrocProxy) processClientRequest(msg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Bytes   []byte `json:"bytes"`
	Bytes2  []byte `json:"bytes2"`
	Num     int    `json:"num"`
}, next http.Handler) {
	requestID := string(msg.Bytes2)

	// Десериализуем запрос
	req, err := deserializeRequest(msg.Bytes)
	if err != nil {
		log.Errorf("Failed to deserialize request: %v", err)
		return
	}

	// Создаем ResponseRecorder для захвата ответа
	recorder := NewResponseRecorder()

	// Обрабатываем запрос
	next.ServeHTTP(recorder, req)

	// Получаем ответ
	resp := recorder.Result()
	defer resp.Body.Close()

	// Читаем тело
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Failed to read response body: %v", err)
		return
	}

	// Сериализуем ответ
	respData, err := serializeResponse(resp, body)
	if err != nil {
		log.Errorf("Failed to serialize response: %v", err)
		return
	}

	p.mu.RLock()
	tunnel := p.tunnel
	p.mu.RUnlock()

	if tunnel == nil {
		log.Error("Tunnel is nil")
		return
	}

	// Отправляем ответ клиенту
	responseMsg := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Bytes   []byte `json:"bytes"`
		Bytes2  []byte `json:"bytes2"`
		Num     int    `json:"num"`
	}{
		Type:    "message",
		Message: ProxyHTTPResponse,
		Bytes:   respData,
		Bytes2:  []byte(requestID),
		Num:     resp.StatusCode,
	}

	encoded, err := json.Marshal(responseMsg)
	if err != nil {
		log.Errorf("Failed to encode response: %v", err)
		return
	}

	if tunnel.Key != nil {
		encoded, err = crypt.Encrypt(encoded, tunnel.Key)
		if err != nil {
			log.Errorf("Failed to encrypt response: %v", err)
			return
		}
	}

	tunnel.ControlConn.Send(encoded)
}

// handleClientRequest обрабатывает запрос на стороне клиента
func (p *CrocProxy) handleClientRequest(w http.ResponseWriter, r *http.Request, next http.Handler) {
	// Генерируем ID запроса
	requestID := generateRequestID()

	// Создаем канал для ответа
	respChan := p.requestMgr.Add(requestID)
	defer p.requestMgr.Get(requestID) // очищаем на всякий случай

	// Сериализуем запрос
	reqData, err := serializeRequest(r)
	if err != nil {
		http.Error(w, "Failed to serialize request", http.StatusInternalServerError)
		return
	}

	p.mu.RLock()
	tunnel := p.tunnel
	p.mu.RUnlock()

	if tunnel == nil {
		http.Error(w, "Tunnel not established", http.StatusServiceUnavailable)
		return
	}

	// Отправляем запрос через туннель
	msg := struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Bytes   []byte `json:"bytes"`
		Bytes2  []byte `json:"bytes2"`
		Num     int    `json:"num"`
	}{
		Type:    "message",
		Message: ProxyHTTPRequest,
		Bytes:   reqData,
		Bytes2:  []byte(requestID),
		Num:     0,
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		http.Error(w, "Failed to encode request", http.StatusInternalServerError)
		return
	}

	if tunnel.Key != nil {
		encoded, err = crypt.Encrypt(encoded, tunnel.Key)
		if err != nil {
			http.Error(w, "Failed to encrypt request", http.StatusInternalServerError)
			return
		}
	}

	if err := tunnel.ControlConn.Send(encoded); err != nil {
		http.Error(w, "Failed to send request", http.StatusInternalServerError)
		return
	}

	// Ждем ответ
	select {
	case resp := <-respChan:
		// Копируем заголовки
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		// Копируем тело
		body, _ := io.ReadAll(resp.Body)
		w.Write(body)
		resp.Body.Close()

	case <-time.After(requestTimeout):
		http.Error(w, "Request timeout", http.StatusGatewayTimeout)
	case <-p.stopChan:
		http.Error(w, "Proxy stopped", http.StatusServiceUnavailable)
	}
}

// connectToRelay подключается к relay и создает туннель
func (p *CrocProxy) connectToRelay() (*TunnelRoom, error) {
	roomName := generateRoomName(p.opts.SharedSecret)

	// Подключаемся к relay
	conn, banner, externalIP, err := ConnectToTCPServer(
		p.opts.RelayAddress,
		p.opts.RelayPassword,
		roomName,
		30*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to relay: %w", err)
	}

	log.Debugf("Connected to relay, banner: %s, external IP: %s", banner, externalIP)

	// Проходим PAKE аутентификацию
	var pakeInstance *pake.Pake
	if p.opts.IsSender {
		pakeInstance, err = pake.InitCurve([]byte(p.opts.SharedSecret[5:]), 1, p.opts.Curve)
	} else {
		pakeInstance, err = pake.InitCurve([]byte(p.opts.SharedSecret[5:]), 0, p.opts.Curve)
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("PAKE init failed: %w", err)
	}

	// Отправляем PAKE данные
	if !p.opts.IsSender {
		// Получатель инициирует PAKE
		err = conn.Send(pakeInstance.Bytes())
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	// Получаем PAKE данные от другой стороны
	data, err := conn.Receive()
	if err != nil {
		conn.Close()
		return nil, err
	}

	err = pakeInstance.Update(data)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("PAKE update failed: %w", err)
	}

	// Отправляем свои PAKE данные если мы sender
	if p.opts.IsSender {
		err = conn.Send(pakeInstance.Bytes())
		if err != nil {
			conn.Close()
			return nil, err
		}
	}

	// Получаем session key
	sessionKey, err := pakeInstance.SessionKey()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to get session key: %w", err)
	}

	// Генерируем ключ шифрования
	encryptionKey, salt, err := crypt.New(sessionKey, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Отправляем salt
	err = conn.Send(salt)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Получаем salt от другой стороны
	data, err = conn.Receive()
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Обновляем ключ с полученной солью
	encryptionKey, _, err = crypt.New(sessionKey, data)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Отправляем handshake
	err = conn.Send([]byte("handshake"))
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Ждем подтверждения
	data, err = conn.Receive()
	if err != nil || !bytes.Equal(data, []byte("handshake")) {
		conn.Close()
		return nil, fmt.Errorf("handshake failed")
	}

	// Парсим порты для данных
	ports := strings.Split(banner, ",")
	dataConns := make([]*comm.Comm, 0, len(ports)-1)

	// Подключаемся к дополнительным портам
	host := strings.Split(p.opts.RelayAddress, ":")[0]
	for i, port := range ports[1:] {
		addr := net.JoinHostPort(host, port)
		dataConn, err := comm.NewConnection(addr, 10*time.Second)
		if err != nil {
			log.Warnf("Failed to connect to data port %d: %v", i, err)
			continue
		}

		// Проходим PAKE для data канала
		dataConn.Send(pakeInstance.Bytes())
		dataConn.Receive()
		dataConn.Send(salt)
		dataConn.Receive()
		dataConn.Send([]byte("handshake"))
		dataConn.Receive()

		dataConns = append(dataConns, dataConn)
	}

	log.Infof("Tunnel established with %d data channels", len(dataConns))

	return &TunnelRoom{
		ControlConn: conn,
		DataConns:   dataConns,
		Key:         encryptionKey,
		RoomName:    roomName,
		IsSender:    p.opts.IsSender,
	}, nil
}

// GetTunnelInfo возвращает информацию о туннеле
func (p *CrocProxy) GetTunnelInfo() (roomName string, isSender bool, clientAddr string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.tunnel != nil {
		roomName = p.tunnel.RoomName
		isSender = p.tunnel.IsSender
		if p.tunnel.ControlConn != nil {
			clientAddr = p.tunnel.ControlConn.Connection().RemoteAddr().String()
		}
	}
	return
}

// WaitForConnection блокируется до подключения клиента или таймаута
func (p *CrocProxy) WaitForConnection(timeout time.Duration) error {
	p.mu.RLock()
	if p.active && p.tunnel != nil {
		p.mu.RUnlock()
		return nil // уже подключен
	}
	p.mu.RUnlock()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.RLock()
			active := p.active && p.tunnel != nil
			p.mu.RUnlock()
			if active {
				return nil
			}
		case <-timeoutTimer.C:
			return fmt.Errorf("timeout waiting for client connection")
		case <-p.stopChan:
			return fmt.Errorf("proxy stopped")
		case <-done:
			return ErrApplicationShutdown
		}
	}
}

// ConnectToTCPServer устанавливает соединение с TCP сервером
func ConnectToTCPServer(address, password, room string, timeout time.Duration) (*comm.Comm, string, string, error) {
	c, err := comm.NewConnection(address, timeout)
	if err != nil {
		return nil, "", "", err
	}

	// get PAKE connection with server to establish strong key to transfer info
	A, err := pake.InitCurve(weakKey, 0, "siec")
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	err = c.Send(A.Bytes())
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	Bbytes, err := c.Receive()
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	err = A.Update(Bbytes)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	strongKey, err := A.SessionKey()
	if err != nil {
		c.Close()
		return nil, "", "", err
	}

	strongKeyForEncryption, salt, err := crypt.New(strongKey, nil)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	// send salt
	err = c.Send(salt)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}

	// send password
	bSend, err := crypt.Encrypt([]byte(password), strongKeyForEncryption)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	err = c.Send(bSend)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}

	// wait for first ok
	enc, err := c.Receive()
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	data, err := crypt.Decrypt(enc, strongKeyForEncryption)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	if !strings.Contains(string(data), "|||") {
		err = fmt.Errorf("bad response: %s", string(data))
		c.Close()
		return nil, "", "", err
	}
	banner := strings.Split(string(data), "|||")[0]
	ipaddr := strings.Split(string(data), "|||")[1]

	// send room
	bSend, err = crypt.Encrypt([]byte(room), strongKeyForEncryption)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	err = c.Send(bSend)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}

	// wait for room confirmation
	enc, err = c.Receive()
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	data, err = crypt.Decrypt(enc, strongKeyForEncryption)
	if err != nil {
		c.Close()
		return nil, "", "", err
	}
	if !bytes.Equal(data, []byte("ok")) {
		err = fmt.Errorf("got bad response: %s", data)
		c.Close()
		return nil, "", "", err
	}

	return c, banner, ipaddr, nil
}

// Вспомогательные функции

func generateRoomName(secret string) string {
	hash := sha256.Sum256([]byte(secret[:4] + "croc"))
	return hex.EncodeToString(hash[:])
}

func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Serialization formats
type SerializableRequest struct {
	Method  string
	URL     string
	Headers map[string][]string
	Body    []byte
}

type SerializableResponse struct {
	Status  string
	Headers map[string][]string
	Body    []byte
}

func serializeRequest(req *http.Request) ([]byte, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()

	sReq := SerializableRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: req.Header,
		Body:    body,
	}

	return json.Marshal(sReq)
}

func deserializeRequest(data []byte) (*http.Request, error) {
	var sReq SerializableRequest
	if err := json.Unmarshal(data, &sReq); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(sReq.Method, sReq.URL, bytes.NewReader(sReq.Body))
	if err != nil {
		return nil, err
	}

	req.Header = sReq.Headers
	return req, nil
}

func serializeResponse(resp *http.Response, body []byte) ([]byte, error) {
	sResp := SerializableResponse{
		Status:  resp.Status,
		Headers: resp.Header,
		Body:    body,
	}

	return json.Marshal(sResp)
}

func deserializeResponse(data []byte, statusCode int) (*http.Response, error) {
	var sResp SerializableResponse
	if err := json.Unmarshal(data, &sResp); err != nil {
		return nil, err
	}

	resp := &http.Response{
		Status:     sResp.Status,
		StatusCode: statusCode,
		Header:     sResp.Headers,
		Body:       io.NopCloser(bytes.NewReader(sResp.Body)),
	}

	return resp, nil
}

// ResponseRecorder для захвата HTTP ответов
type ResponseRecorder struct {
	statusCode int
	header     http.Header
	body       *bytes.Buffer
}

func NewResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		header: make(http.Header),
		body:   &bytes.Buffer{},
	}
}

func (r *ResponseRecorder) Header() http.Header {
	return r.header
}

func (r *ResponseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *ResponseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *ResponseRecorder) Result() *http.Response {
	return &http.Response{
		StatusCode: r.statusCode,
		Header:     r.header,
		Body:       io.NopCloser(bytes.NewReader(r.body.Bytes())),
	}
}
