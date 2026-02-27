// proxy.go
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/message"
	"github.com/schollz/croc/v10/src/models"
	"github.com/schollz/croc/v10/src/tcp"
	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
)

// Константы для прокси
const (
	requestTimeout = 30 * time.Second
)

// RequestManager управляет ожидающими HTTP запросами
type RequestManager struct {
	pending map[string]chan *http.Response
	mu      sync.RWMutex
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

// CrocProxy реализует проксирование HTTP через существующее croc соединение
type CrocProxy struct {
	controlConn *comm.Comm
	isSender    bool
	key         []byte

	requestMgr *RequestManager
	active     bool
	stopChan   chan struct{}

	// Для режима отправителя - handler, который будет обрабатывать запросы
	handler http.Handler

	mu sync.RWMutex
}

// NewCrocProxy создает новый экземпляр прокси с существующим соединением
func NewCrocProxy(conn *comm.Comm, isSender bool, key []byte) *CrocProxy {
	return &CrocProxy{
		controlConn: conn,
		isSender:    isSender,
		key:         key,
		requestMgr:  NewRequestManager(),
		stopChan:    make(chan struct{}),
	}
}

// SetHandler устанавливает HTTP handler для режима отправителя
func (p *CrocProxy) SetHandler(handler http.Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = handler
}

// StartSender запускает режим отправителя (принимает запросы от получателя)
func (p *CrocProxy) StartSender() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return nil
	}
	if !p.isSender {
		return fmt.Errorf("StartSender called on receiver proxy")
	}
	if p.handler == nil {
		return fmt.Errorf("no handler set for sender proxy")
	}

	p.active = true
	go p.senderReceiveLoop()
	log.Info("Proxy sender started on existing croc connection")
	return nil
}

// StartReceiverLoop запускает приём ответов от отправителя (для режима получателя)
func (p *CrocProxy) StartReceiverLoop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return nil
	}
	if p.isSender {
		return fmt.Errorf("StartReceiverLoop called on sender proxy")
	}

	p.active = true
	go p.receiverReceiveLoop()
	log.Info("Proxy receiver loop started")
	return nil
}

// Stop останавливает прокси
func (p *CrocProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return nil
	}
	p.active = false

	select {
	case <-p.stopChan:
	default:
		close(p.stopChan)
	}

	log.Info("Croc proxy stopped")
	return nil
}

// IsActive возвращает true если прокси активен
func (p *CrocProxy) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// RoundTrip реализует http.RoundTripper для получателя
func (p *CrocProxy) RoundTrip(req *http.Request) (*http.Response, error) {
	requestID := generateRequestID()
	ch := p.requestMgr.Add(requestID)
	defer p.requestMgr.Get(requestID)

	log.Debugf("RoundTrip: sending request %s %s (id=%s)",
		req.Method, req.URL.Path, requestID)

	// Сериализуем запрос
	reqData, err := serializeRequest(req)
	if err != nil {
		return nil, err
	}

	// Отправляем через croc
	msg := message.Message{
		Type:    "proxy-request",
		Message: string(reqData),
		Bytes:   []byte(requestID),
		Num:     0,
	}

	if err := message.Send(p.controlConn, p.key, msg); err != nil {
		return nil, err
	}

	// Ждём ответ
	select {
	case resp := <-ch:
		log.Debugf("RoundTrip: received response (id=%s, status=%d, statusText=%s)",
			requestID, resp.StatusCode, resp.Status)

		// КРИТИЧЕСКАЯ ПРОВЕРКА!
		if resp.StatusCode == 0 {
			log.Errorf("RoundTrip: BUG! Response status code is 0 for %s %s",
				req.Method, req.URL.Path)

			// Исправляем, чтобы избежать паники
			resp.StatusCode = 500
			resp.Status = "500 Internal Server Error (proxy fix)"
		}
		return resp, nil

	case <-time.After(requestTimeout):
		log.Errorf("RoundTrip: timeout for request %s", requestID)
		return nil, fmt.Errorf("request timeout")

	case <-p.stopChan:
		return nil, fmt.Errorf("proxy stopped")
	case <-done:
		return nil, fmt.Errorf("app stopped")
	}
}

// senderReceiveLoop - отправитель ждёт запросы от получателя
func (p *CrocProxy) senderReceiveLoop() {
	// Создаем канал для результата чтения
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	// Запускаем горутину для чтения
	go func() {
		for {
			data, err := p.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-p.stopChan:
				log.Debug("senderReceiveLoop stopped")
				return
			case <-done:
				log.Debug("app stopped")
				return
			}
		}
	}()

	for {
		select {
		case <-p.stopChan:
			log.Debug("senderReceiveLoop stopped")
			return
		case <-done:
			log.Debug("app stopped")
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("senderReceiveLoop error: %v", result.err)
				return
			}

			if len(result.data) == 0 {
				continue
			}

			msg, err := message.Decode(p.key, result.data)
			if err != nil {
				log.Errorf("failed to decode message: %v", err)
				continue
			}

			if msg.Type == "proxy-request" {
				go p.handleSenderRequest(msg)
			}
		}
	}
}

// receiverReceiveLoop - получатель ждёт ответы от отправителя
func (p *CrocProxy) receiverReceiveLoop() {
	// Создаем канал для результата чтения
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	// Запускаем горутину для чтения
	go func() {
		for {
			data, err := p.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-p.stopChan:
				log.Debug("receiverReceiveLoop stopped")
				return
			case <-done:
				log.Debug("app stopped")
				return
			}
		}
	}()

	for {
		select {
		case <-p.stopChan:
			log.Debug("receiverReceiveLoop stopped")
			return
		case <-done:
			log.Debug("app stopped")
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("receiverReceiveLoop error: %v", result.err)
				return
			}

			if len(result.data) == 0 {
				continue
			}

			msg, err := message.Decode(p.key, result.data)
			if err != nil {
				log.Errorf("failed to decode message: %v", err)
				continue
			}

			if msg.Type == "proxy-response" {
				requestID := string(msg.Bytes)
				ch := p.requestMgr.Get(requestID)
				if ch != nil {
					resp, err := deserializeResponse([]byte(msg.Message))
					if err != nil {
						log.Errorf("failed to deserialize response: %v", err)
						continue
					}
					ch <- resp
				}
			}
		}
	}
}

// handleSenderRequest - отправитель обрабатывает запрос от получателя
func (p *CrocProxy) handleSenderRequest(msg message.Message) {
	p.mu.RLock()
	handler := p.handler
	p.mu.RUnlock()

	if handler == nil {
		log.Error("received proxy request but no handler set")
		return
	}

	requestID := string(msg.Bytes)

	req, err := deserializeRequest([]byte(msg.Message))
	if err != nil {
		log.Errorf("failed to deserialize request: %v", err)
		return
	}

	// ВАЖНО: Правим Destination ЗДЕСЬ, до вызова handler'а
	if req.Method == "MOVE" || req.Method == "COPY" {
		if dest := req.Header.Get("Destination"); dest != "" {
			log.Debugf("Original Destination in request: %s", dest)

			// Парсим URL
			destURL, err := url.Parse(dest)
			if err != nil {
				log.Errorf("Failed to parse Destination: %v", err)
			} else {
				// Извлекаем только путь
				targetPath := destURL.Path
				if destURL.RawPath != "" {
					// Сохраняем URL-encoded символы (для русских букв и пробелов)
					targetPath = destURL.RawPath
				}

				// Добавляем query string если есть
				if destURL.RawQuery != "" {
					targetPath += "?" + destURL.RawQuery
				}

				// Убеждаемся, что путь начинается с /
				if !strings.HasPrefix(targetPath, "/") {
					targetPath = "/" + targetPath
				}

				log.Debugf("Setting Destination to path only: %s", targetPath)
				req.Header.Set("Destination", targetPath)
			}
		}
	}

	recorder := NewResponseRecorder()
	handler.ServeHTTP(recorder, req)

	// Проверяем, что статус-код установлен
	if recorder.statusCode == 0 {
		log.Warnf("Handler for %s %s didn't call WriteHeader, defaulting to 200",
			req.Method, req.URL.Path)
		recorder.statusCode = http.StatusOK
	}

	resp := recorder.Result()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("failed to read response body: %v", err)
		return
	}

	respData, err := serializeResponse(resp, body)
	if err != nil {
		log.Errorf("failed to serialize response: %v", err)
		return
	}

	responseMsg := message.Message{
		Type:    "proxy-response",
		Message: string(respData),
		Bytes:   []byte(requestID),
		Num:     resp.StatusCode,
	}

	if err := message.Send(p.controlConn, p.key, responseMsg); err != nil {
		log.Errorf("failed to send response: %v", err)
		return
	}
}

// ================ ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ================

func ConvertFileToHTTPURL(fileURL string) (string, error) {
	if !strings.HasPrefix(fileURL, "file://") {
		return fileURL, nil
	}

	// Временно заменяем @ на : для парсинга
	tempURL := strings.Replace(fileURL, "@", ":", 1)

	parsed, err := url.Parse(tempURL)
	if err != nil {
		return "", err
	}

	parsed.Scheme = "http"
	parsed.Host = strings.Replace(parsed.Host, ":", "@", 1) // Возвращаем @ обратно
	parsed.Path = strings.TrimPrefix(parsed.Path, "/DavWWWRoot")
	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String(), nil
}

func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// SerializableRequest для сериализации HTTP запроса
type SerializableRequest struct {
	Method  string
	URL     string
	Headers map[string][]string
	Body    []byte
}

// SerializableResponse для сериализации HTTP ответа
type SerializableResponse struct {
	Status     string
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

func serializeRequest(req *http.Request) ([]byte, error) {
	var body []byte
	var err error

	// Проверяем, что Body не nil перед чтением
	if req.Body != nil {
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body.Close()
	}

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
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}

	return json.Marshal(sResp)
}

func deserializeResponse(data []byte) (*http.Response, error) {
	var sResp SerializableResponse
	if err := json.Unmarshal(data, &sResp); err != nil {
		return nil, err
	}

	resp := &http.Response{
		Status:     sResp.Status,
		StatusCode: sResp.StatusCode,
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
	if statusCode == 0 {
		log.Warnf("WriteHeader called with status code 0! Defaulting to 200")
		statusCode = http.StatusOK
	}
	r.statusCode = statusCode
}

func (r *ResponseRecorder) Result() *http.Response {
	statusCode := r.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
		log.Debugf("ResponseRecorder: status code was 0, defaulting to %d %s",
			statusCode, http.StatusText(statusCode))
	}

	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     r.header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(r.body.Bytes())),
	}
}

// ================ ИНТЕГРАЦИЯ С WEBDAV SERVER ================

// EnableProxy активирует WebDAV туннель через отдельное соединение
func (s *WebDAVServer) EnableProxy(client *croc.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, не активен ли уже прокси
	if s.proxy != nil && s.proxy.IsActive() {
		return fmt.Errorf("proxy already active")
	}

	// Получаем базовый адрес ретранслятора
	relayAddr := client.Options.RelayAddress
	if relayAddr == "" {
		return fmt.Errorf("no relay address configured")
	}

	// Парсим хост и порт
	host, portStr, _ := defAddress(relayAddr)
	basePort, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port number %s: %w", portStr, err)
	}

	// Параметры WebDAV туннеля
	roomSuffix := 1
	webdavPort := basePort + roomSuffix + 1 // basePort + 2 = 9011
	webdavRoom := fmt.Sprintf("%s-%d", client.Options.RoomName, roomSuffix)
	webdavAddr := net.JoinHostPort(host, strconv.Itoa(webdavPort))

	log.Infof("WebDAV tunnel: establishing connection to %s (room: %s)",
		webdavAddr, webdavRoom)

	// Устанавливаем соединение для WebDAV
	webdavConn, banner, externalIP, err := tcp.ConnectToTCPServer(
		webdavAddr,
		client.Options.RelayPassword,
		webdavRoom,
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to establish WebDAV tunnel: %w", err)
	}

	log.Debugf("WebDAV tunnel connected: banner=%s, externalIP=%s", banner, externalIP)

	// Создаем прокси с этим соединением
	proxy := NewCrocProxy(webdavConn, client.Options.IsSender, client.Key)

	if client.Options.IsSender {
		// Отправитель: используем локальный handler из webdav.go
		proxy.SetHandler(s.localHandler)

		if err := proxy.StartSender(); err != nil {
			webdavConn.Close()
			return fmt.Errorf("failed to start WebDAV sender: %w", err)
		}
		log.Infof("WebDAV sender proxy started (port %d, room %s)",
			webdavPort, webdavRoom)

	} else {
		// Получатель: запускаем приём ответов от отправителя
		if err := proxy.StartReceiverLoop(); err != nil {
			webdavConn.Close()
			return fmt.Errorf("failed to start WebDAV receiver loop: %w", err)
		}

		// Создаем прокси-HANDLER (НЕ сервер!)
		proxyHandler := s.createProxyHandler(proxy)

		// Просто меняем текущий handler!
		// Сохраняем старый handler если нужно
		if s.localHandler == nil {
			s.localHandler = s.currentHandler
		}
		s.currentHandler = proxyHandler

		log.Infof("WebDAV proxy handler activated on %s (port %d, room %s)",
			s.addr, webdavPort, webdavRoom)

		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(true)
			s.remote = true
		}
	}

	// Сохраняем прокси
	s.proxy = proxy
	log.Infof("WebDAV tunnel successfully enabled on port %d (room %s)",
		webdavPort, webdavRoom)

	return nil
}

// createProxyHandler создает handler, который перенаправляет запросы через прокси
func (s *WebDAVServer) createProxyHandler(proxy *CrocProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalURL := r.URL.String()
		log.Debugf("Proxy handler received %s %s", r.Method, originalURL)

		// Создаем новый запрос
		proxyReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			log.Errorf("Failed to create proxy request: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Копируем все заголовки
		proxyReq.Header = r.Header.Clone()

		// Отправляем через прокси
		resp, err := proxy.RoundTrip(proxyReq)
		if err != nil {
			log.Errorf("RoundTrip failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Копируем заголовки ответа
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Устанавливаем статус-код
		statusCode := resp.StatusCode
		if statusCode == 0 {
			log.Warnf("Response status code is 0 for %s %s, defaulting to 200",
				r.Method, originalURL)
			statusCode = http.StatusOK
		}
		w.WriteHeader(statusCode)

		// Копируем тело ответа
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Errorf("Failed to copy response body: %v", err)
		}
	})
}

// DisableProxy отключает WebDAV туннель и восстанавливает локальный handler
func (s *WebDAVServer) DisableProxy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.proxy != nil {
		log.Info("Disabling WebDAV tunnel...")

		// Останавливаем прокси
		s.proxy.Stop()
		s.proxy = nil

		// Возвращаем локальный handler!
		if s.localHandler != nil {
			s.currentHandler = s.localHandler
			log.Info("WebDAV local handler restored")
		}

		// Вызываем callback для уведомления о выключении прокси
		if s.remote && s.onProxyStateChanged != nil {
			s.onProxyStateChanged(false)
		}
	}
}

// RestartProxy перезапускает прокси с новым адресом
func (s *WebDAVServer) RestartProxy(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Если прокси не активен, ничего не делаем
	if s.proxy == nil || !s.proxy.IsActive() {
		return nil
	}

	// Если адрес тот же, перезапуск не нужен
	if s.addr == addr {
		return nil
	}

	log.Infof("RestartProxy: restarting proxy from %s to %s", s.addr, addr)

	// Сохраняем соединение croc
	conn := s.proxy.controlConn
	isSender := s.proxy.isSender
	key := s.proxy.key

	// Останавливаем старый прокси
	s.proxy.Stop()
	s.proxy = nil

	// Создаем новый прокси на новом адресе
	newProxy := NewCrocProxy(conn, isSender, key)

	if isSender {
		// Отправитель: восстанавливаем handler
		fs := createFileSystem(s.root)
		webdavHandler := &webdav.Handler{
			FileSystem: fs,
			LockSystem: webdav.NewMemLS(),
			Logger: func(r *http.Request, err error) {
				if err != nil {
					log.Errorf("Proxy WebDAV request %s %s: %v", r.Method, r.URL.Path, err)
				} else {
					log.Debugf("Proxy WebDAV request %s %s", r.Method, r.URL.Path)
				}
			},
		}

		newProxy.SetHandler(webdavHandler)
		if err := newProxy.StartSender(); err != nil {
			return fmt.Errorf("failed to restart sender proxy: %w", err)
		}
	} else {
		// Получатель: запускаем приём ответов от отправителя
		if err := newProxy.StartReceiverLoop(); err != nil {
			return fmt.Errorf("failed to restart receiver proxy: %w", err)
		}
	}

	// Меняем handler на прокси-версию
	proxyHandler := s.createProxyHandler(newProxy)
	s.currentHandler = proxyHandler

	// Обновляем адрес в структуре
	s.addr = addr
	s.proxy = newProxy
	log.Infof("RestartProxy: proxy restarted successfully")
	return nil
}

func (s *WebDAVServer) IsProxyActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proxy != nil && s.proxy.IsActive()
}

func (s *WebDAVServer) GetProxy() *CrocProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proxy
}

func defAddress(hp string, ports ...string) (host, port, address string) {
	var err error
	host, port, err = net.SplitHostPort(hp)
	// Default port to :9009
	if err != nil {
		host = hp
		port = models.DEFAULT_PORT
		for _, p := range ports {
			port = p
			break
		}
	}
	log.Debugf("got host '%v' and port '%v'", host, port)
	address = net.JoinHostPort(host, port)
	return
}
