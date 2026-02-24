// proxy.go
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/message"
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

	// Для режима получателя
	proxyServer *http.Server

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

// StartReceiver запускает режим получателя (HTTP прокси на указанном адресе)
func (p *CrocProxy) StartReceiver(addr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return nil
	}
	if p.isSender {
		return fmt.Errorf("StartReceiver called on sender proxy")
	}

	// Создаём reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "croc-proxy"
		},
		Transport: p,
	}

	p.proxyServer = &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	p.active = true

	// Запускаем HTTP сервер
	go func() {
		log.Infof("Croc proxy receiver listening on %s", addr)
		if err := p.proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Proxy server error: %v", err)
		}
	}()

	// Запускаем приём ответов от отправителя
	go p.receiverReceiveLoop()

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

	if p.proxyServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.proxyServer.Shutdown(ctx); err != nil {
			log.Warnf("Error stopping proxy server: %v", err)
		}
		p.proxyServer = nil
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
		return resp, nil
	case <-time.After(requestTimeout):
		return nil, fmt.Errorf("request timeout")
	case <-p.stopChan:
		return nil, fmt.Errorf("proxy stopped")
	}
}

// senderReceiveLoop - отправитель ждёт запросы от получателя
func (p *CrocProxy) senderReceiveLoop() {
	for {
		select {
		case <-p.stopChan:
			log.Debug("senderReceiveLoop stopped")
			return
		default:
		}

		// Устанавливаем короткий дедлайн для чтения
		p.controlConn.Connection().SetReadDeadline(time.Now().Add(1 * time.Second))

		data, err := p.controlConn.Receive()
		if err != nil {
			// Проверяем на тайм-аут - это нормально, проверяем stopChan
			if err.Error() == "i/o timeout" {
				continue
			}
			// Другая ошибка - возможно закрытие соединения
			return
		}

		// Сбрасываем дедлайн после успешного чтения
		p.controlConn.Connection().SetReadDeadline(time.Time{})

		msg, err := message.Decode(p.key, data)
		if err != nil {
			log.Errorf("failed to decode message: %v", err)
			continue
		}

		if msg.Type == "proxy-request" {
			go p.handleSenderRequest(msg)
		}
	}
}

// receiverReceiveLoop - получатель ждёт ответы от отправителя
func (p *CrocProxy) receiverReceiveLoop() {
	for {
		select {
		case <-p.stopChan:
			log.Debug("receiverReceiveLoop stopped")
			return
		default:
		}

		// Устанавливаем короткий дедлайн для чтения
		p.controlConn.Connection().SetReadDeadline(time.Now().Add(1 * time.Second))

		data, err := p.controlConn.Receive()
		if err != nil {
			// Проверяем на тайм-аут - это нормально, проверяем stopChan
			if err.Error() == "i/o timeout" {
				continue
			}
			// Другая ошибка - возможно закрытие соединения
			return
		}

		// Сбрасываем дедлайн после успешного чтения
		p.controlConn.Connection().SetReadDeadline(time.Time{})

		msg, err := message.Decode(p.key, data)
		if err != nil {
			log.Errorf("failed to decode message: %v", err)
			continue
		}

		if msg.Type == "proxy-response" {
			requestID := string(msg.Bytes)
			ch := p.requestMgr.Get(requestID)
			if ch != nil {
				resp, err := deserializeResponse([]byte(msg.Message), msg.Num)
				if err != nil {
					log.Errorf("failed to deserialize response: %v", err)
					continue
				}
				ch <- resp
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

	recorder := NewResponseRecorder()
	handler.ServeHTTP(recorder, req)

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

// EnableWebDAVProxy активирует прокси-режим для WebDAV сервера
func (s *WebDAVServer) EnableProxy(client *croc.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debugf("EnableProxy: creating proxy (sender=%v)", client.Options.IsSender)

	conns, err := Conns(client)
	if err != nil {
		return fmt.Errorf("get conns: %w", err)
	}
	if len(conns) == 0 {
		return errors.New("no active connections")
	}
	if conns[0] == nil || conns[0].Connection() == nil {
		return errors.New("first connection is nil or closed")
	}
	log.Debugf("EnableProxy: connection found, local=%v, remote=%v",
		conns[0].Connection().LocalAddr(), conns[0].Connection().RemoteAddr())

	// Создаём прокси
	proxy := NewCrocProxy(conns[0], client.Options.IsSender, client.Key)

	if client.Options.IsSender {
		// Отправитель: создаём WebDAV handler и устанавливаем его в прокси
		var fs webdav.FileSystem
		if base := filepath.Base(s.root); !CanCreateSymlinks() && base == SEND {
			fs = &ResolvingFileSystem{root: s.root}
		} else {
			fs = webdav.Dir(s.root)
		}

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

		proxy.SetHandler(webdavHandler)
		if err := proxy.StartSender(); err != nil {
			return fmt.Errorf("StartSender: %w", err)
		}
	} else {
		// Получатель: останавливаем WebDAV и запускаем прокси на том же адресе
		if s.active {
			// Сохраняем настройки WebDAV для последующего восстановления
			s.savedAddr = s.addr
			s.savedRoot = s.root
			s.savedUseTLS = s.useTLS
			s.savedAddrs = s.tlsAddrs
			s.webDAVStopped = true // Помечаем, что мы остановили WebDAV
			log.Debugf("EnableProxy: saved WebDAV settings (addr=%v, root=%v, useTLS=%v)",
				s.savedAddr, s.savedRoot, s.savedUseTLS)
		}

		// Останавливаем WebDAV сервер
		if err := s.stopLocked(); err != nil {
			log.Errorf("EnableProxy: failed to stop WebDAV server: %v", err)
		}

		log.Debugf("EnableProxy: starting receiver on addr=%v", s.addr)
		if err := proxy.StartReceiver(s.addr); err != nil {
			return fmt.Errorf("failed to start receiver proxy: %w", err)
		}
	}

	s.proxy = proxy
	log.Infof("Croc proxy enabled (sender: %v)", client.Options.IsSender)
	return nil
}

// DisableProxy отключает прокси-режим для WebDAV сервера
func (s *WebDAVServer) DisableProxy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.proxy != nil {
		// Проверяем, был ли прокси в режиме получателя
		isReceiver := !s.proxy.isSender
		s.proxy.Stop()
		s.proxy = nil
		log.Info("WebDAV proxy disabled")

		// Если это был режим получателя и WebDAV был остановлен, восстанавливаем WebDAV сервер
		if isReceiver && s.savedAddr != "" && s.webDAVStopped {
			log.Debugf("DisableProxy: restoring WebDAV server (addr=%v, root=%v, useTLS=%v)",
				s.savedAddr, s.savedRoot, s.savedUseTLS)

			// Запускаем WebDAV сервер с сохранёнными настройками
			s.active = false // Сбрасываем флаг перед запуском
			if err := s.Start(s.savedAddr, s.savedRoot, s.savedUseTLS, s.savedAddrs...); err != nil {
				log.Errorf("DisableProxy: failed to restore WebDAV server: %v", err)
			} else {
				log.Info("WebDAV server restored")
			}

			// Очищаем сохранённые настройки
			s.savedAddr = ""
			s.savedRoot = ""
			s.savedUseTLS = false
			s.savedAddrs = nil
			s.webDAVStopped = false
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

	// Для режима получателя восстанавливаем WebDAV на старом адресе
	if !isSender && s.savedAddr != "" {
		s.active = false
		if err := s.Start(s.savedAddr, s.savedRoot, s.savedUseTLS, s.savedAddrs...); err != nil {
			log.Errorf("RestartProxy: failed to restore WebDAV: %v", err)
		}
	}

	// Создаём новый прокси на новом адресе
	newProxy := NewCrocProxy(conn, isSender, key)

	if isSender {
		// Отправитель: восстанавливаем handler
		var fs webdav.FileSystem
		if base := filepath.Base(s.root); !CanCreateSymlinks() && base == SEND {
			fs = &ResolvingFileSystem{root: s.root}
		} else {
			fs = webdav.Dir(s.root)
		}

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
		// Получатель: сохраняем настройки, останавливаем WebDAV, запускаем прокси на новом адресе
		if s.active {
			s.savedAddr = s.addr
			s.savedRoot = s.root
			s.savedUseTLS = s.useTLS
			s.savedAddrs = s.tlsAddrs
			s.webDAVStopped = true // Помечаем, что мы остановили WebDAV
		}

		if err := s.stopLocked(); err != nil {
			log.Errorf("RestartProxy: failed to stop WebDAV: %v", err)
		}

		if err := newProxy.StartReceiver(addr); err != nil {
			log.Errorf("failed to restart receiver proxy: %v", err)
		}
	}

	// Обновляем адрес в структуре
	s.addr = addr
	s.proxy = newProxy
	log.Infof("RestartProxy: proxy restarted successfully")
	return nil
}

// ================ ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ================

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
	Status  string
	Headers map[string][]string
	Body    []byte
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
