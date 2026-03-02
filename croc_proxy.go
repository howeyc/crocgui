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
	"golang.org/x/net/webdav"
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

// readLoggingReader обертка для логирования чтения из тела ответа
type readLoggingReader struct {
	reader io.ReadCloser
	id     uint64
}

func (r *readLoggingReader) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	if n > 0 {
		log.Debugf("readLoggingReader: Read %d bytes for id=%d, err=%v", n, r.id, err)
	} else if err != nil {
		log.Debugf("readLoggingReader: Read error for id=%d: %v", r.id, err)
	}
	return
}

func (r *readLoggingReader) Close() error {
	log.Debugf("readLoggingReader: Closing reader for id=%d", r.id)
	return r.reader.Close()
}

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
	writeWg    sync.WaitGroup // Для отслеживания активных goroutine записи
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

	// Для потоковой обработки тел запросов (на отправителе)
	pendingRequestBodys sync.Map // map[uint64]*pipeWriterWithMutex

	// Для сохранения состояния запросов с телом (на отправителе)
	pendingRequests sync.Map // map[uint64]*pendingRequestState
	mu              sync.RWMutex
}

// pipeWriterWithMutex защищает PipeWriter с mutex для предотвращения race condition
type pipeWriterWithMutex struct {
	pw     *io.PipeWriter
	mu     sync.Mutex
	closed bool
}

func newPipeWriterWithMutex(pw *io.PipeWriter) *pipeWriterWithMutex {
	return &pipeWriterWithMutex{
		pw:     pw,
		mu:     sync.Mutex{},
		closed: false,
	}
}

func (pwm *pipeWriterWithMutex) Write(data []byte) (int, error) {
	pwm.mu.Lock()
	defer pwm.mu.Unlock()

	if pwm.closed {
		return 0, io.ErrClosedPipe
	}
	return pwm.pw.Write(data)
}

func (pwm *pipeWriterWithMutex) Close() error {
	pwm.mu.Lock()
	defer pwm.mu.Unlock()

	if pwm.closed {
		return nil // Уже закрыт
	}
	pwm.closed = true
	return pwm.pw.Close()
}

// pendingRequestState хранит состояние запроса с телом
type pendingRequestState struct {
	req      *http.Request
	handler  http.Handler
	writer   *StreamResponseWriter
	response bool           // true если ответ уже отправлен
	wg       sync.WaitGroup // Для синхронизации завершения handler
	doneChan chan struct{}  // Для сигнализации о завершении handler
}

func newPendingRequestState(req *http.Request, handler http.Handler, writer *StreamResponseWriter) *pendingRequestState {
	return &pendingRequestState{
		req:      req,
		handler:  handler,
		writer:   writer,
		response: false,
		wg:       sync.WaitGroup{},
		doneChan: make(chan struct{}),
	}
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

	// Очищаем все pending request body pipes
	p.pendingRequestBodys.Range(func(key, value interface{}) bool {
		if pw, ok := value.(*io.PipeWriter); ok {
			pw.Close()
		}
		return true
	})

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
		log.Debugf("RoundTrip: Received reader for id=%d, status=%d, Content-Length=%s",
			requestID, reader.statusCode, reader.header.Get("Content-Length"))
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

		// Создаем обертку для логирования чтения из body
		log.Debugf("RoundTrip: Returning response for id=%d with body reader", requestID)
		return &http.Response{
			StatusCode: reader.statusCode,
			Status:     reader.status,
			Header:     reader.header,
			Body:       &readLoggingReader{reader: reader.body, id: requestID},
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

	log.Debugf("sendRequest: %s %s (id=%d, ContentLength=%d)",
		req.Method, req.URL.Path, id, req.ContentLength)

	// Если есть тело (ContentLength > 0), отправляем чанками
	if req.Body != nil && req.ContentLength > 0 {
		buffer := make([]byte, ChunkSize)
		totalBytes := 0
		for {
			n, err := req.Body.Read(buffer)
			if n > 0 {
				totalBytes += n
				log.Debugf("sendRequest: sending chunk %d bytes (total %d/%d) for id=%d",
					n, totalBytes, req.ContentLength, id)
				if err := p.sendMessage(id, StreamMsgData, buffer[:n]); err != nil {
					req.Body.Close()
					return err
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Errorf("sendRequest: error reading body for id=%d: %v", id, err)
				req.Body.Close()
				return err
			}
		}
		req.Body.Close()
		log.Debugf("sendRequest: sent total %d bytes for id=%d", totalBytes, id)
	} else if req.Body != nil && req.ContentLength == 0 {
		// Тело есть, но ContentLength=0 (например, POST без данных)
		// Просто читаем чтобы убедиться что EOF
		buffer := make([]byte, 1)
		_, err := req.Body.Read(buffer)
		if err != nil && err != io.EOF {
			log.Errorf("sendRequest: unexpected error reading zero-length body for id=%d: %v", id, err)
			req.Body.Close()
			return err
		}
		req.Body.Close()
		log.Debugf("sendRequest: zero-length body handled for id=%d", id)
	}

	// ВАЖНО: Отправляем StreamMsgDone чтобы закрыть pipe на отправителе!
	log.Debugf("sendRequest: sending StreamMsgDone for id=%d", id)
	if err := p.sendMessage(id, StreamMsgDone, nil); err != nil {
		return err
	}

	return nil
}

// sendMessage отправляет зашифрованное бинарное сообщение
func (p *StreamCrocProxy) sendMessage(id uint64, msgType byte, data []byte) error {
	packet := make([]byte, 13+len(data))
	binary.LittleEndian.PutUint64(packet[0:8], id)
	packet[8] = msgType
	binary.LittleEndian.PutUint32(packet[9:13], uint32(len(data)))
	copy(packet[13:], data)

	log.Debugf("sendMessage: id=%d, msgType=0x%02x, dataLen=%d, packetLen=%d",
		id, msgType, len(data), len(packet))

	encrypted, err := crypt.Encrypt(packet, p.key)
	if err != nil {
		log.Errorf("sendMessage: encryption failed for id=%d: %v", id, err)
		return err
	}

	log.Debugf("sendMessage: encrypted packet size=%d for id=%d, sending...", len(encrypted), id)
	err = p.controlConn.Send(encrypted)
	if err != nil {
		log.Errorf("sendMessage: Send() failed for id=%d: %v", id, err)
		return err
	}
	log.Debugf("sendMessage: Send() succeeded for id=%d", id)
	return nil
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
			log.Debugf("senderLoop: stop signal received, exiting")
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("senderLoop: connection error: %v", result.err)
				// Интерпретируем разрыв соединения как неявный StreamMsgDone для всех pending запросов
				p.handleConnectionError()
				return
			}
			log.Debugf("senderLoop: received %d bytes from controlConn", len(result.data))
			p.handleSenderMessage(result.data)
		}
	}
}

func (p *StreamCrocProxy) handleSenderMessage(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic in handleSenderMessage: %v", r)
		}
	}()

	log.Debugf("handleSenderMessage: received %d encrypted bytes", len(data))

	decrypted, err := crypt.Decrypt(data, p.key)
	if err != nil {
		log.Errorf("handleSenderMessage: Failed to decrypt %d bytes: %v", len(data), err)
		return
	}

	log.Debugf("handleSenderMessage: decrypted to %d bytes", len(decrypted))

	if len(decrypted) < 13 {
		log.Errorf("handleSenderMessage: Received too short message: %d bytes", len(decrypted))
		return
	}

	id := binary.LittleEndian.Uint64(decrypted[0:8])
	msgType := decrypted[8]
	dataLen := binary.LittleEndian.Uint32(decrypted[9:13])

	var payload []byte
	if dataLen > 0 && len(decrypted) >= 13+int(dataLen) {
		payload = decrypted[13 : 13+dataLen]
	}

	log.Debugf("handleSenderMessage: id=%d, msgType=0x%02x, dataLen=%d, payloadLen=%d",
		id, msgType, dataLen, len(payload))

	switch msgType {
	case StreamMsgRequest:
		log.Debugf("Sender received StreamMsgRequest id=%d, payload=%d bytes", id, len(payload))
		p.handleRequest(id, payload)
	case StreamMsgData:
		log.Debugf("Sender received StreamMsgData id=%d, data=%d bytes", id, len(payload))
		p.handleData(id, payload)
	case StreamMsgDone:
		log.Debugf("Sender received StreamMsgDone id=%d, payload=%d bytes", id, len(payload))
		p.handleDone(id)
	default:
		log.Errorf("Unknown message type 0x%02x for id=%d", msgType, id)
	}
}

// handleRequest - отправитель обрабатывает запрос от получателя
func (p *StreamCrocProxy) handleRequest(id uint64, data []byte) {
	log.Debugf("handleRequest: ENTER id=%d, payload=%d bytes", id, len(data))

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic in handleRequest: %v", r)
			p.sendMessage(id, StreamMsgError, []byte("internal server error"))
			// Очищаем pendingRequestBody если есть
			if pw, ok := p.pendingRequestBodys.LoadAndDelete(id); ok {
				if writer, ok := pw.(*io.PipeWriter); ok {
					writer.Close()
				}
			}
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

	// Парсим HTTP запрос (только заголовки, body уже отрезан)
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		log.Errorf("Failed to parse request: %v", err)
		p.sendMessage(id, StreamMsgError, []byte("invalid request"))
		return
	}

	log.Debugf("Sender handling request: %s %s (id=%d, ContentLength=%d, Headers=%+v)",
		req.Method, req.URL.Path, id, req.ContentLength, req.Header)

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

	// Если есть тело (ContentLength != 0), создаем pipe для потокового чтения
	// Это включает: ContentLength > 0 (известный размер) и ContentLength < 0 (chunked encoding)
	if req.ContentLength != 0 {
		pr, pw := io.Pipe()
		req.Body = pr
		// Используем pipeWriterWithMutex для защиты от race condition
		pwm := newPipeWriterWithMutex(pw)
		p.pendingRequestBodys.Store(id, pwm)
		log.Debugf("Created pipe for request body (id=%d, expected %d bytes)", id, req.ContentLength)

		// Verify that pendingRequestBodys was stored
		_, ok := p.pendingRequestBodys.Load(id)
		if !ok {
			log.Errorf("handleRequest: FAILED to verify pendingRequestBodys.Store for id=%d", id)
		} else {
			log.Debugf("handleRequest: Verified pendingRequestBodys.Store for id=%d", id)
		}

		// ВАЖНО! Создаем состояние и запускаем handler НЕМЕДЛЕННО в goroutine
		// Handler будет читать из pipe, данные будут поступать через handleData
		writer := NewStreamResponseWriter(p, id)
		state := newPendingRequestState(req, handler, writer)
		p.pendingRequests.Store(id, state)
		log.Debugf("handleRequest: saved pending state for id=%d", id)

		// Verify that pendingRequests was stored
		_, ok = p.pendingRequests.Load(id)
		if !ok {
			log.Errorf("handleRequest: FAILED to verify pendingRequests.Store for id=%d", id)
		} else {
			log.Debugf("handleRequest: Verified pendingRequests.Store for id=%d", id)
		}

		// Запускаем handler в goroutine немедленно
		state.wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("handleRequest: PANIC in handler goroutine for id=%d: %v", id, r)
					if !state.writer.headerSent {
						state.writer.WriteHeader(http.StatusInternalServerError)
						state.writer.Write([]byte(fmt.Sprintf("internal server error: %v", r)))
					}
					state.writer.Close()
				}
				state.wg.Done()
				close(state.doneChan)
			}()

			log.Debugf("handleRequest: About to call handler.ServeHTTP for id=%d (in goroutine)", id)
			state.handler.ServeHTTP(state.writer, state.req)
			log.Debugf("handleRequest: handler.ServeHTTP returned for id=%d, headerSent=%v", id, state.writer.headerSent)

			// Если заголовки не были отправлены, отправляем с кодом 200
			if !state.writer.headerSent {
				log.Debugf("handleRequest: Sending default 200 status for id=%d", id)
				state.writer.WriteHeader(http.StatusOK)
			}
		}()

		log.Debugf("handleRequest: EXIT id=%d (handler running in goroutine, waiting for StreamMsgDone)", id)
		return
	} else {
		// Тела нет - используем http.NoBody вместо nil, чтобы WebDAV handler не паниковал
		req.Body = http.NoBody
		log.Debugf("No body for request (id=%d), using http.NoBody", id)

		// Создаем writer и сразу вызываем handler
		writer := NewStreamResponseWriter(p, id)

		log.Debugf("handleRequest: calling handler.ServeHTTP for id=%d (no body)", id)
		// Обрабатываем запрос - все Write() будут отправлять сразу
		handler.ServeHTTP(writer, req)

		log.Debugf("handleRequest: handler.ServeHTTP completed for id=%d, headerSent=%v", id, writer.headerSent)

		// Если заголовки не были отправлены, отправляем с кодом 200
		if !writer.headerSent {
			writer.WriteHeader(http.StatusOK)
		}

		// Закрываем writer
		log.Debugf("handleRequest: closing writer for id=%d", id)
		if err := writer.Close(); err != nil {
			log.Errorf("Failed to close writer: %v", err)
		}
		log.Debugf("handleRequest: EXIT id=%d", id)
	}
}

// handleData - обработка данных от получателя (для отправителя)
func (p *StreamCrocProxy) handleData(id uint64, data []byte) {
	log.Debugf("handleData: ENTER for id=%d, dataLen=%d", id, len(data))
	defer log.Debugf("handleData: EXIT for id=%d", id)

	// Находим pipeWriter для этого запроса
	val, ok := p.pendingRequestBodys.Load(id)
	if !ok {
		log.Debugf("No pending request body for id=%d, ignoring data", id)
		return
	}

	pwm, ok := val.(*pipeWriterWithMutex)
	if !ok {
		log.Errorf("Invalid type in pendingRequestBodys for id=%d", id)
		// ВАЖНО! При ошибке всё равно вызываем handleDone для завершения запроса
		p.handleDone(id)
		return
	}

	log.Debugf("Sender received data for request id=%d: %d bytes", id, len(data))

	// Пишем данные в pipe в отдельной goroutine, чтобы не блокировать senderLoop
	// pipeWriterWithMutex.Write() блокируется пока данные не будут прочитаны или pipe закрыт
	log.Debugf("handleData: Starting goroutine to write %d bytes to pipe for id=%d", len(data), id)
	go func() {
		log.Debugf("handleData: Goroutine started for id=%d, about to write %d bytes", id, len(data))
		n, err := pwm.Write(data)
		log.Debugf("handleData: Goroutine Write() returned for id=%d, n=%d, err=%v", id, n, err)
		if err != nil {
			log.Errorf("Failed to write to request body pipe (id=%d): %v", id, err)
			// ВАЖНО! При ошибке записи в pipe всё равно вызываем handleDone
			// чтобы завершить pending запрос и вызвать handler
			log.Debugf("handleData: Calling handleDone due to pipe write error for id=%d", id)
			p.handleDone(id)
			return
		}
		log.Debugf("handleData: Successfully wrote %d bytes to pipe for id=%d", n, id)
	}()
}

// handleDone - обработка завершения от получателя
func (p *StreamCrocProxy) handleDone(id uint64) {
	log.Debugf("handleDone: ENTER for request id=%d", id)

	// Log the current state of pendingRequestBodys for debugging
	var pendingBodyIds []uint64
	p.pendingRequestBodys.Range(func(key, value interface{}) bool {
		pendingBodyIds = append(pendingBodyIds, key.(uint64))
		return true
	})
	log.Debugf("handleDone: Current pendingRequestBodys IDs: %v", pendingBodyIds)

	// Log the current state of pendingRequests for debugging
	var pendingRequestIds []uint64
	p.pendingRequests.Range(func(key, value interface{}) bool {
		pendingRequestIds = append(pendingRequestIds, key.(uint64))
		return true
	})
	log.Debugf("handleDone: Current pendingRequests IDs: %v", pendingRequestIds)

	defer log.Debugf("handleDone: EXIT for request id=%d", id)

	// Проверяем есть ли pending запрос (с телом)
	stateVal, ok := p.pendingRequests.LoadAndDelete(id)
	if ok {
		log.Debugf("handleDone: Found pending request for id=%d", id)
		state, ok := stateVal.(*pendingRequestState)
		if !ok {
			log.Errorf("handleDone: Invalid type in pendingRequests for id=%d", id)
			return
		}

		// ВАЖНО! Закрываем pipeWriter - это сигнализирует handler что тело закончилось
		// Handler уже запущен в goroutine и читает из pipe
		val, ok := p.pendingRequestBodys.LoadAndDelete(id)
		if ok {
			if pwm, ok := val.(*pipeWriterWithMutex); ok {
				pwm.Close()
				log.Debugf("handleDone: Closed pipeWriter for request id=%d, waiting for handler to complete", id)
			}
		} else {
			log.Debugf("handleDone: No pendingRequestBody for id=%d", id)
		}

		// Ждем завершения handler через WaitGroup
		// Handler был запущен в goroutine в handleRequest()
		state.wg.Wait()
		log.Debugf("handleDone: Handler completed for id=%d", id)

		// Если заголовки не были отправлены, отправляем с кодом 200
		if !state.writer.headerSent {
			log.Debugf("handleDone: Sending default 200 status for id=%d", id)
			state.writer.WriteHeader(http.StatusOK)
		}

		// Закрываем writer - это отправит StreamMsgDone клиенту
		log.Debugf("handleDone: Closing writer for id=%d", id)
		if err := state.writer.Close(); err != nil {
			log.Errorf("handleDone: Failed to close writer: %v", err)
		}
	} else {
		log.Debugf("handleDone: No pending request found for id=%d (request without body or already processed)", id)

		// Если нет pending запроса, но есть pendingRequestBody, закрываем его
		val, ok := p.pendingRequestBodys.LoadAndDelete(id)
		if ok {
			if pwm, ok := val.(*pipeWriterWithMutex); ok {
				pwm.Close()
				log.Debugf("handleDone: Closed orphaned pipeWriter for request id=%d", id)
			}
		}
	}
}

// handleConnectionError обрабатывает разрыв соединения как неявный StreamMsgDone для всех pending запросов
func (p *StreamCrocProxy) handleConnectionError() {
	log.Debugf("handleConnectionError: processing all pending requests due to connection error")

	// Обрабатываем все pending request bodies (закрываем pipes)
	p.pendingRequestBodys.Range(func(key, value interface{}) bool {
		id := key.(uint64)
		if pwm, ok := value.(*pipeWriterWithMutex); ok {
			pwm.Close()
			log.Debugf("handleConnectionError: Closed pipeWriter for request id=%d", id)
		}
		return true
	})
	p.pendingRequestBodys.Clear()

	// Обрабатываем все pending запросы (вызываем handler)
	p.pendingRequests.Range(func(key, value interface{}) bool {
		id := key.(uint64)
		state, ok := value.(*pendingRequestState)
		if !ok {
			log.Errorf("handleConnectionError: Invalid type in pendingRequests for id=%d", id)
			return true
		}

		log.Debugf("handleConnectionError: calling handler.ServeHTTP for id=%d (connection closed)", id)

		// ВАЖНО! Добавляем panic recovery
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("handleConnectionError: PANIC for id=%d: %v", id, r)
			}
		}()

		// Обрабатываем запрос
		log.Debugf("handleConnectionError: About to call handler.ServeHTTP for id=%d", id)
		state.handler.ServeHTTP(state.writer, state.req)
		log.Debugf("handleConnectionError: handler.ServeHTTP returned for id=%d, headerSent=%v", id, state.writer.headerSent)

		// Если заголовки не были отправлены, отправляем с кодом 200
		if !state.writer.headerSent {
			log.Debugf("handleConnectionError: Sending default 200 status for id=%d", id)
			state.writer.WriteHeader(http.StatusOK)
		}

		// Закрываем writer
		log.Debugf("handleConnectionError: Closing writer for id=%d", id)
		if err := state.writer.Close(); err != nil {
			log.Errorf("handleConnectionError: Failed to close writer: %v", err)
		}
		return true
	})
	p.pendingRequests.Clear()

	log.Debugf("handleConnectionError: completed processing all pending requests")
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
			log.Debugf("receiverLoop: stop signal received, exiting")
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("receiverLoop: connection error: %v", result.err)
				// Интерпретируем разрыв соединения как завершение всех pending ответов
				p.handleReceiverConnectionError()
				return
			}
			log.Debugf("receiverLoop: received %d bytes from controlConn", len(result.data))
			p.handleReceiverMessage(result.data)
		}
	}
}

func (p *StreamCrocProxy) handleReceiverMessage(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("handleReceiverMessage: Recovered from panic: %v", r)
		}
	}()

	log.Debugf("handleReceiverMessage: received %d encrypted bytes", len(data))

	decrypted, err := crypt.Decrypt(data, p.key)
	if err != nil {
		log.Errorf("handleReceiverMessage: Failed to decrypt %d bytes: %v", len(data), err)
		return
	}

	log.Debugf("handleReceiverMessage: decrypted to %d bytes", len(decrypted))

	if len(decrypted) < 13 {
		log.Errorf("handleReceiverMessage: Received too short message: %d bytes", len(decrypted))
		return
	}

	id := binary.LittleEndian.Uint64(decrypted[0:8])
	msgType := decrypted[8]
	dataLen := binary.LittleEndian.Uint32(decrypted[9:13])

	var payload []byte
	if dataLen > 0 && len(decrypted) >= 13+int(dataLen) {
		payload = decrypted[13 : 13+dataLen]
	}

	log.Debugf("handleReceiverMessage: id=%d, msgType=0x%02x, dataLen=%d, payloadLen=%d",
		id, msgType, dataLen, len(payload))

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
	log.Debugf("handleResponse: Stored reader in pending for id=%d", id)

	// Отправляем reader в канал
	ch := p.requestMgr.Get(id)
	if ch != nil {
		log.Debugf("handleResponse: Sending reader to channel for id=%d", id)
		ch <- reader
		log.Debugf("handleResponse: Reader sent to channel for id=%d", id)
	} else {
		log.Errorf("handleResponse: No channel found for id=%d - response will be lost!", id)
	}
}

func (p *StreamCrocProxy) handleResponseData(id uint64, data []byte) {
	log.Debugf("handleResponseData: Received %d bytes for id=%d", len(data), id)
	if val, ok := p.pending.Load(id); ok {
		if reader, ok := val.(*StreamResponseReader); ok {
			// Пишем данные в reader в отдельной goroutine, чтобы не блокировать receiverLoop
			// io.PipeWriter.Write() блокируется пока данные не будут прочитаны
			reader.writeWg.Add(1)
			go func() {
				defer reader.writeWg.Done()
				log.Debugf("handleResponseData: Writing %d bytes to pipe for id=%d", len(data), id)
				if _, err := reader.Write(data); err != nil {
					// Если pipe уже закрыт (StreamMsgDone уже получен), это не ошибка
					// Просто игнорируем эти данные, так как ответ уже завершён
					if err == io.ErrClosedPipe {
						log.Warnf("handleResponseData: Pipe already closed for id=%d, ignoring %d bytes - THIS MAY CAUSE INCOMPLETE RESPONSE!", id, len(data))
					} else {
						log.Errorf("Failed to write data to reader (id=%d): %v", id, err)
						// ВАЖНО! При ошибке записи всё равно вызываем handleResponseDone
						// для корректного завершения
						log.Debugf("handleResponseData: Calling handleResponseDone due to write error for id=%d", id)
						p.handleResponseDone(id)
					}
				} else {
					log.Debugf("handleResponseData: Successfully wrote %d bytes to pipe for id=%d", len(data), id)
				}
			}()
		} else {
			log.Warnf("handleResponseData: Found pending entry for id=%d but it's not a StreamResponseReader", id)
		}
	} else {
		log.Warnf("handleResponseData: No pending entry found for id=%d - data may be lost!", id)
	}
}

func (p *StreamCrocProxy) handleResponseDone(id uint64) {
	log.Debugf("handleResponseDone: Waiting for all write goroutines to complete for id=%d", id)

	if val, ok := p.pending.Load(id); ok {
		if reader, ok := val.(*StreamResponseReader); ok {
			// Ждем завершения всех goroutine записи перед закрытием pipe
			reader.writeWg.Wait()
			log.Debugf("handleResponseDone: All write goroutines completed for id=%d", id)
		}
	}

	log.Warnf("handleResponseDone: Closing pipe for id=%d - NO MORE DATA WILL BE ACCEPTED!", id)

	if val, ok := p.pending.LoadAndDelete(id); ok {
		if reader, ok := val.(*StreamResponseReader); ok {
			reader.Close()
			log.Debugf("handleResponseDone: Pipe closed for id=%d", id)
		} else {
			log.Warnf("handleResponseDone: Found pending entry for id=%d but it's not a StreamResponseReader", id)
		}
	} else {
		log.Warnf("handleResponseDone: No pending entry found for id=%d - response may already be closed!", id)
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

// handleReceiverConnectionError обрабатывает разрыв соединения на стороне получателя
func (p *StreamCrocProxy) handleReceiverConnectionError() {
	log.Debugf("handleReceiverConnectionError: processing all pending responses due to connection error")

	// Закрываем все pending response readers
	p.pending.Range(func(key, value interface{}) bool {
		id := key.(uint64)
		if reader, ok := value.(*StreamResponseReader); ok {
			log.Debugf("handleReceiverConnectionError: Closing reader for id=%d", id)
			reader.Close()
		}
		return true
	})
	p.pending.Clear()

	// Также сигнализируем всем ожидающим RoundTrip вызовам через requestMgr
	// Это нужно для запросов, которые еще не получили StreamMsgResponse
	// Мы не можем сделать это напрямую, так как requestMgr.Get() удаляет запись
	// Вместо этого мы просто логируем, что соединение закрыто
	log.Debugf("handleReceiverConnectionError: connection closed, pending requests will timeout")
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
		log.Debugf("StreamResponseWriter.Write: id=%d, len=%d bytes", w.id, len(p))
		if err := w.proxy.sendMessage(w.id, StreamMsgData, p); err != nil {
			log.Errorf("StreamResponseWriter.Write: failed to send data for id=%d: %v", w.id, err)
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

		log.Debugf("StreamResponseWriter.WriteHeader: id=%d, statusCode=%d, status=%s", w.id, statusCode, statusText)

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
		w.header.Write(&buf)
		buf.WriteString("\r\n")

		log.Debugf("StreamResponseWriter.WriteHeader: sending headers for id=%d, buf size=%d", w.id, buf.Len())

		// Отправляем заголовки
		if err := w.proxy.sendMessage(w.id, StreamMsgResponse, buf.Bytes()); err != nil {
			log.Errorf("StreamResponseWriter.WriteHeader: failed to send headers for id=%d: %v", w.id, err)
		} else {
			log.Debugf("StreamResponseWriter.WriteHeader: headers sent successfully for id=%d", w.id)
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

// RestartStreamProxy перезапускает потоковый прокси с новым адресом
func (s *WebDAVServer) RestartStreamProxy(addr string) error {
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

	log.Infof("RestartStreamProxy: restarting proxy from %s to %s", s.addr, addr)

	// Сохраняем соединение croc
	streamProxy, ok := s.proxy.(*StreamCrocProxy)
	if !ok {
		return fmt.Errorf("proxy is not a StreamCrocProxy")
	}

	conn := streamProxy.controlConn
	isSender := streamProxy.isSender
	key := streamProxy.key

	// Останавливаем старый прокси
	s.proxy.Stop()
	s.proxy = nil

	// Создаем новый прокси на новом адресе
	newProxy := NewStreamCrocProxy(conn, isSender, key)

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
		if err := newProxy.StartReceiver(); err != nil {
			return fmt.Errorf("failed to restart receiver proxy: %w", err)
		}
	}

	// Меняем handler на прокси-версию
	proxyHandler := s.createStreamProxyHandler(newProxy)
	s.currentHandler = proxyHandler

	// Обновляем адрес в структуре
	s.addr = addr
	s.proxy = newProxy
	log.Infof("RestartStreamProxy: proxy restarted successfully")
	return nil
}
