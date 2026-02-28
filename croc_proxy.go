// croc_proxy.go
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
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
	"github.com/schollz/croc/v10/src/crypt"
	"github.com/schollz/croc/v10/src/tcp"
	log "github.com/schollz/logger"
)

// Константы для потоковой передачи
const (
	ChunkSize     = 64 * 1024 // 64KB
	StreamTimeout = 30 * time.Second
)

// Бинарные типы сообщений
const (
	StreamMsgRequest  = 0x01
	StreamMsgResponse = 0x02
	StreamMsgData     = 0x03
	StreamMsgDone     = 0x04
	StreamMsgError    = 0x05
)

// StreamRequestManager управляет ожидающими запросами
type StreamRequestManager struct {
	pending map[uint64]chan *StreamResponseReader
	mu      sync.RWMutex
}

func NewStreamRequestManager() *StreamRequestManager {
	return &StreamRequestManager{
		pending: make(map[uint64]chan *StreamResponseReader),
	}
}

func (m *StreamRequestManager) Add(id uint64) chan *StreamResponseReader {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *StreamResponseReader, 1)
	m.pending[id] = ch
	return ch
}

func (m *StreamRequestManager) Get(id uint64) chan *StreamResponseReader {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.pending[id]; ok {
		delete(m.pending, id)
		return ch
	}
	return nil
}

// StreamResponseReader для потокового чтения ответа
type StreamResponseReader struct {
	header     http.Header
	statusCode int
	status     string
	body       *io.PipeReader
	bodyWriter *io.PipeWriter
	once       sync.Once
}

func NewStreamResponseReader() *StreamResponseReader {
	pr, pw := io.Pipe()
	return &StreamResponseReader{
		header:     make(http.Header),
		body:       pr,
		bodyWriter: pw,
	}
}

func (r *StreamResponseReader) WriteHeader(statusCode int, status string) {
	r.statusCode = statusCode
	r.status = status
}

func (r *StreamResponseReader) Write(data []byte) (int, error) {
	if r.bodyWriter == nil {
		return 0, io.ErrClosedPipe
	}
	return r.bodyWriter.Write(data)
}

func (r *StreamResponseReader) Close() {
	r.once.Do(func() {
		if r.bodyWriter != nil {
			r.bodyWriter.Close()
		}
	})
}

// StreamCrocProxy - потоковый бинарный аналог CrocProxy
type StreamCrocProxy struct {
	controlConn *comm.Comm
	isSender    bool
	key         []byte

	requestMgr *StreamRequestManager
	pending    sync.Map // map[uint64]*StreamResponseReader
	active     bool
	stopChan   chan struct{}

	// Для режима отправителя
	handler http.Handler

	mu sync.RWMutex
}

func NewStreamCrocProxy(conn *comm.Comm, isSender bool, key []byte) *StreamCrocProxy {
	if key == nil {
		panic("key cannot be nil")
	}
	return &StreamCrocProxy{
		controlConn: conn,
		isSender:    isSender,
		key:         key,
		requestMgr:  NewStreamRequestManager(),
		stopChan:    make(chan struct{}),
	}
}

func (p *StreamCrocProxy) SetHandler(handler http.Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = handler
}

func (p *StreamCrocProxy) StartSender() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return nil
	}
	if !p.isSender {
		return fmt.Errorf("StartSender called on receiver proxy")
	}
	if p.handler == nil {
		return fmt.Errorf("no handler set")
	}

	p.active = true
	go p.senderLoop()
	log.Info("Stream proxy sender started")
	return nil
}

func (p *StreamCrocProxy) StartReceiver() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return nil
	}
	if p.isSender {
		return fmt.Errorf("StartReceiver called on sender proxy")
	}

	p.active = true
	go p.receiverLoop()
	log.Info("Stream proxy receiver started")
	return nil
}

func (p *StreamCrocProxy) Stop() error {
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

	log.Info("Stream proxy stopped")
	return nil
}

func (p *StreamCrocProxy) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// RoundTrip реализует http.RoundTripper для получателя
func (p *StreamCrocProxy) RoundTrip(req *http.Request) (*http.Response, error) {
	if p.isSender {
		return nil, fmt.Errorf("RoundTrip called on sender")
	}

	requestID := p.generateID()
	respChan := p.requestMgr.Add(requestID)

	log.Debugf("RoundTrip: sending request %s %s (id=%d)", req.Method, req.URL.Path, requestID)

	// Отправляем запрос (без StreamMsgDone!)
	if err := p.sendRequest(requestID, req); err != nil {
		p.requestMgr.Get(requestID)
		return nil, err
	}

	// Ждем ответ
	select {
	case reader := <-respChan:
		p.requestMgr.Get(requestID)
		if reader == nil {
			return nil, fmt.Errorf("connection closed")
		}

		// Проверяем статус код как в proxy.go
		if reader.statusCode == 0 {
			log.Errorf("RoundTrip: BUG! Response status code is 0 for %s %s",
				req.Method, req.URL.Path)
			reader.statusCode = 500
			reader.status = "500 Internal Server Error (proxy fix)"
		}

		return &http.Response{
			StatusCode: reader.statusCode,
			Status:     reader.status,
			Header:     reader.header,
			Body:       reader.body,
		}, nil

	case <-time.After(StreamTimeout):
		p.requestMgr.Get(requestID)
		log.Errorf("RoundTrip: timeout for request %d", requestID)
		return nil, fmt.Errorf("request timeout")

	case <-p.stopChan:
		p.requestMgr.Get(requestID)
		return nil, fmt.Errorf("proxy stopped")
	}
}

// sendRequest отправляет HTTP запрос (без завершения)
func (p *StreamCrocProxy) sendRequest(id uint64, req *http.Request) error {
	// Буфер только для заголовков
	var headerBuf bytes.Buffer

	// Пишем request line
	fmt.Fprintf(&headerBuf, "%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto)

	// Пишем заголовки
	req.Header.Write(&headerBuf)
	headerBuf.WriteString("\r\n")

	// Отправляем метаданные запроса
	if err := p.sendMessage(id, StreamMsgRequest, headerBuf.Bytes()); err != nil {
		return err
	}

	// Если есть тело, отправляем чанками
	if req.Body != nil {
		buffer := make([]byte, ChunkSize)
		for {
			n, err := req.Body.Read(buffer)
			if n > 0 {
				if err := p.sendMessage(id, StreamMsgData, buffer[:n]); err != nil {
					req.Body.Close()
					return err
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				req.Body.Close()
				return err
			}
		}
		req.Body.Close()
	}

	// НЕ отправляем StreamMsgDone здесь!
	return nil
}

// sendMessage отправляет зашифрованное бинарное сообщение
func (p *StreamCrocProxy) sendMessage(id uint64, msgType byte, data []byte) error {
	packet := make([]byte, 13+len(data))
	binary.LittleEndian.PutUint64(packet[0:8], id)
	packet[8] = msgType
	binary.LittleEndian.PutUint32(packet[9:13], uint32(len(data)))
	copy(packet[13:], data)

	encrypted, err := crypt.Encrypt(packet, p.key)
	if err != nil {
		return err
	}
	return p.controlConn.Send(encrypted)
}

// senderLoop - отправитель ждёт запросы от получателя
func (p *StreamCrocProxy) senderLoop() {
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	go func() {
		for {
			data, err := p.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-p.stopChan:
				return
			}
		}
	}()

	for {
		select {
		case <-p.stopChan:
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("senderLoop error: %v", result.err)
				return
			}
			p.handleSenderMessage(result.data)
		}
	}
}

func (p *StreamCrocProxy) handleSenderMessage(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic: %v", r)
		}
	}()

	decrypted, err := crypt.Decrypt(data, p.key)
	if err != nil {
		log.Errorf("Failed to decrypt: %v", err)
		return
	}

	if len(decrypted) < 13 {
		return
	}

	id := binary.LittleEndian.Uint64(decrypted[0:8])
	msgType := decrypted[8]
	dataLen := binary.LittleEndian.Uint32(decrypted[9:13])

	var payload []byte
	if dataLen > 0 && len(decrypted) >= 13+int(dataLen) {
		payload = decrypted[13 : 13+dataLen]
	}

	switch msgType {
	case StreamMsgRequest:
		p.handleRequest(id, payload)
	case StreamMsgData:
		p.handleData(id, payload)
	case StreamMsgDone:
		p.handleDone(id)
	}
}

// handleRequest - отправитель обрабатывает запрос от получателя
func (p *StreamCrocProxy) handleRequest(id uint64, data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic in handleRequest: %v", r)
			p.sendMessage(id, StreamMsgError, []byte("internal server error"))
		}
	}()

	p.mu.RLock()
	handler := p.handler
	p.mu.RUnlock()

	if handler == nil {
		log.Error("received request but no handler set")
		p.sendMessage(id, StreamMsgError, []byte("no handler"))
		return
	}

	// Парсим HTTP запрос
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		log.Errorf("Failed to parse request: %v", err)
		p.sendMessage(id, StreamMsgError, []byte("invalid request"))
		return
	}

	log.Debugf("Sender handling request: %s %s (id=%d)", req.Method, req.URL.Path, id)

	// Обработка WebDAV методов (как в proxy.go)
	if req.Method == "MOVE" || req.Method == "COPY" {
		if dest := req.Header.Get("Destination"); dest != "" {
			log.Debugf("Original Destination: %s", dest)
			destURL, err := url.Parse(dest)
			if err == nil {
				targetPath := destURL.Path
				if destURL.RawPath != "" {
					targetPath = destURL.RawPath
				}
				if destURL.RawQuery != "" {
					targetPath += "?" + destURL.RawQuery
				}
				if !strings.HasPrefix(targetPath, "/") {
					targetPath = "/" + targetPath
				}
				req.Header.Set("Destination", targetPath)
				log.Debugf("Fixed Destination: %s", targetPath)
			}
		}
	}

	// Создаем истинно потоковый writer (без буфера)
	writer := NewStreamResponseWriter(p, id)

	// Обрабатываем запрос - все Write() будут отправлять сразу
	handler.ServeHTTP(writer, req)

	// Если заголовки не были отправлены, отправляем с кодом 200
	if !writer.headerSent {
		writer.WriteHeader(http.StatusOK)
	}

	// Закрываем writer
	if err := writer.Close(); err != nil {
		log.Errorf("Failed to close writer: %v", err)
	}
}

// handleData - обработка данных от получателя (для отправителя)
func (p *StreamCrocProxy) handleData(id uint64, data []byte) {
	log.Debugf("Sender received data for id=%d: %d bytes", id, len(data))
	// На отправителе данные от клиента не ожидаются, но могут быть для WebSocket
}

// handleDone - обработка завершения от получателя
func (p *StreamCrocProxy) handleDone(id uint64) {
	log.Debugf("Sender received done for id=%d", id)
	// Получатель закрыл соединение
}

// receiverLoop - получатель ждёт ответы от отправителя
func (p *StreamCrocProxy) receiverLoop() {
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	go func() {
		for {
			data, err := p.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-p.stopChan:
				return
			}
		}
	}()

	for {
		select {
		case <-p.stopChan:
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("receiverLoop error: %v", result.err)
				return
			}
			p.handleReceiverMessage(result.data)
		}
	}
}

func (p *StreamCrocProxy) handleReceiverMessage(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic: %v", r)
		}
	}()

	decrypted, err := crypt.Decrypt(data, p.key)
	if err != nil {
		log.Errorf("Failed to decrypt: %v", err)
		return
	}

	if len(decrypted) < 13 {
		return
	}

	id := binary.LittleEndian.Uint64(decrypted[0:8])
	msgType := decrypted[8]
	dataLen := binary.LittleEndian.Uint32(decrypted[9:13])

	var payload []byte
	if dataLen > 0 && len(decrypted) >= 13+int(dataLen) {
		payload = decrypted[13 : 13+dataLen]
	}

	switch msgType {
	case StreamMsgResponse:
		p.handleResponse(id, payload)
	case StreamMsgData:
		p.handleResponseData(id, payload)
	case StreamMsgDone:
		p.handleResponseDone(id)
	case StreamMsgError:
		p.handleResponseError(id, payload)
	}
}

func (p *StreamCrocProxy) handleResponse(id uint64, data []byte) {
	log.Debugf("Received response metadata for id=%d: %d bytes", id, len(data))

	// Парсим HTTP ответ
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		log.Errorf("Failed to parse response: %v", err)
		return
	}

	// Создаем потоковый reader
	reader := NewStreamResponseReader()
	reader.WriteHeader(resp.StatusCode, resp.Status)
	for k, v := range resp.Header {
		reader.header[k] = v
	}

	// Сохраняем в pending
	p.pending.Store(id, reader)

	// Отправляем reader в канал
	ch := p.requestMgr.Get(id)
	if ch != nil {
		ch <- reader
	}
}

func (p *StreamCrocProxy) handleResponseData(id uint64, data []byte) {
	if val, ok := p.pending.Load(id); ok {
		if reader, ok := val.(*StreamResponseReader); ok {
			if _, err := reader.Write(data); err != nil {
				log.Errorf("Failed to write data to reader: %v", err)
			}
		}
	}
}

func (p *StreamCrocProxy) handleResponseDone(id uint64) {
	log.Debugf("Receiver handleResponseDone id=%d", id)

	if val, ok := p.pending.LoadAndDelete(id); ok {
		if reader, ok := val.(*StreamResponseReader); ok {
			reader.Close()
		}
	}
}

func (p *StreamCrocProxy) handleResponseError(id uint64, data []byte) {
	log.Errorf("Received error for id=%d: %s", id, string(data))

	ch := p.requestMgr.Get(id)
	if ch != nil {
		errReader := NewStreamResponseReader()
		errReader.WriteHeader(500, "500 Internal Server Error")
		errReader.Write(data)
		errReader.Close()
		ch <- errReader
	}
}

func (p *StreamCrocProxy) generateID() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// StreamResponseWriter - истинно потоковый writer без буферизации
type StreamResponseWriter struct {
	header     http.Header
	statusCode int
	proxy      *StreamCrocProxy
	id         uint64
	headerSent bool
	once       sync.Once
}

func NewStreamResponseWriter(proxy *StreamCrocProxy, id uint64) *StreamResponseWriter {
	return &StreamResponseWriter{
		header: make(http.Header),
		proxy:  proxy,
		id:     id,
	}
}

func (w *StreamResponseWriter) Header() http.Header {
	return w.header
}

func (w *StreamResponseWriter) Write(p []byte) (int, error) {
	if !w.headerSent {
		w.WriteHeader(http.StatusOK)
	}

	// Отправляем данные сразу, без буферизации!
	if len(p) > 0 {
		if err := w.proxy.sendMessage(w.id, StreamMsgData, p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *StreamResponseWriter) WriteHeader(statusCode int) {
	w.once.Do(func() {
		w.statusCode = statusCode
		w.headerSent = true

		statusText := http.StatusText(statusCode)
		if statusText == "" {
			statusText = "Unknown"
		}

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
		w.header.Write(&buf)
		buf.WriteString("\r\n")

		// Отправляем заголовки
		if err := w.proxy.sendMessage(w.id, StreamMsgResponse, buf.Bytes()); err != nil {
			log.Errorf("Failed to send response headers: %v", err)
		}
	})
}

// Close отправляет сигнал о завершении
func (w *StreamResponseWriter) Close() error {
	log.Debugf("Sending done for id=%d", w.id)
	return w.proxy.sendMessage(w.id, StreamMsgDone, nil)
}

// ================ ИНТЕГРАЦИЯ С WEBDAV SERVER ================

// EnableStreamProxy активирует потоковый прокси
func (s *WebDAVServer) EnableStreamProxy(client *croc.Client) error {
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

	// Параметры туннеля
	roomSuffix := 1
	relayPort := basePort + roomSuffix + 1
	roomName := fmt.Sprintf("%s-%d", client.Options.RoomName, roomSuffix)
	relayAddrFull := net.JoinHostPort(host, strconv.Itoa(relayPort))

	log.Infof("Stream tunnel: establishing connection to %s (room: %s)", relayAddrFull, roomName)

	// Устанавливаем соединение
	conn, banner, externalIP, err := tcp.ConnectToTCPServer(
		relayAddrFull,
		client.Options.RelayPassword,
		roomName,
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to establish tunnel: %w", err)
	}

	log.Debugf("Stream tunnel connected: banner=%s, externalIP=%s", banner, externalIP)

	// Создаем потоковый прокси
	streamProxy := NewStreamCrocProxy(conn, client.Options.IsSender, client.Key)

	if client.Options.IsSender {
		// Отправитель: используем локальный handler
		streamProxy.SetHandler(s.currentHandler)

		if err := streamProxy.StartSender(); err != nil {
			conn.Close()
			return fmt.Errorf("failed to start stream sender: %w", err)
		}
		log.Infof("Stream sender proxy started (relay port %d, room %s)", relayPort, roomName)

	} else {
		// Получатель: запускаем приём ответов
		if err := streamProxy.StartReceiver(); err != nil {
			conn.Close()
			return fmt.Errorf("failed to start stream receiver: %w", err)
		}

		// Создаем HTTP handler
		streamHandler := s.createStreamProxyHandler(streamProxy)

		// Сохраняем локальный handler
		if s.localHandler == nil {
			s.localHandler = s.currentHandler
		}
		s.currentHandler = streamHandler

		log.Infof("Stream proxy handler activated on %s (relay port %d, room %s)",
			s.addr, relayPort, roomName)

		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(true)
			s.remote = true
		}
	}

	// Сохраняем прокси
	s.proxy = streamProxy
	log.Infof("Stream tunnel successfully enabled on relay port %d (room %s)", relayPort, roomName)

	return nil
}

// createStreamProxyHandler создает handler для получателя
func (s *WebDAVServer) createStreamProxyHandler(proxy *StreamCrocProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("Stream proxy handler: %s %s", r.Method, r.URL.Path)

		// Используем прокси как RoundTripper
		resp, err := proxy.RoundTrip(r)
		if err != nil {
			log.Errorf("RoundTrip failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Копируем заголовки
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Устанавливаем статус
		w.WriteHeader(resp.StatusCode)

		// Копируем тело (потоково)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Errorf("Failed to copy response body: %v", err)
		}
	})
}

// DisableStreamProxy отключает потоковый прокси
func (s *WebDAVServer) DisableStreamProxy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.proxy != nil {
		log.Info("Disabling stream tunnel...")

		s.proxy.Stop()
		s.proxy = nil

		if s.localHandler != nil {
			s.currentHandler = s.localHandler
			s.localHandler = nil
			log.Info("WebDAV local handler restored")
		}

		s.remote = false

		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(false)
		}
	}
}

// IsStreamProxyActive возвращает true если прокси активен
func (s *WebDAVServer) IsStreamProxyActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.proxy != nil && s.proxy.IsActive()
}

// GetStreamProxy возвращает текущий прокси
func (s *WebDAVServer) GetStreamProxy() *StreamCrocProxy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if proxy, ok := s.proxy.(*StreamCrocProxy); ok {
		return proxy
	}
	return nil
}
