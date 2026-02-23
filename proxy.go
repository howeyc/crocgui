// proxy.go
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
func NewCrocProxy(conn *comm.Comm, isSender bool) *CrocProxy {
	return &CrocProxy{
		controlConn: conn,
		isSender:    isSender,
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

	encoded, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	if err := p.controlConn.Send(encoded); err != nil {
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

		data, err := p.controlConn.Receive()
		if err != nil {
			log.Errorf("sender receive error: %v", err)
			continue
		}

		var msg message.Message
		if err := json.Unmarshal(data, &msg); err != nil {
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

		data, err := p.controlConn.Receive()
		if err != nil {
			log.Errorf("receiver receive error: %v", err)
			continue
		}

		var msg message.Message
		if err := json.Unmarshal(data, &msg); err != nil {
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

	encoded, err := json.Marshal(responseMsg)
	if err != nil {
		log.Errorf("failed to encode response: %v", err)
		return
	}

	p.controlConn.Send(encoded)
}

// EnableWebDAVProxy активирует прокси-режим для WebDAV сервера
func (s *WebDAVServer) EnableProxy(client *croc.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Уже есть прокси?
	if s.proxy != nil {
		return nil
	}

	conns, err := Conns(client)
	if err != nil || len(conns) == 0 {
		return fmt.Errorf("%v", err)
	}

	// Создаём прокси
	proxy := NewCrocProxy(conns[0], client.Options.IsSender)

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
			return err
		}
	} else {

		if err := proxy.StartReceiver(s.addr); err != nil {
			return fmt.Errorf("failed to start receiver proxy: %v", err)
		}
	}

	s.proxy = proxy
	return nil
}

// DisableProxy отключает прокси-режим для WebDAV сервера
func (s *WebDAVServer) DisableProxy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.proxy != nil {
		s.proxy.Stop()
		s.proxy = nil
		log.Info("WebDAV proxy disabled")
	}
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

func (s *WebDAVServer) IsProxyActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proxy != nil
}

func (s *WebDAVServer) GetProxy() *CrocProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proxy
}
