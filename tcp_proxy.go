// tcp_proxy.go
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/crypt"
	"github.com/schollz/croc/v10/src/tcp"
	log "github.com/schollz/logger"
)

// Константы для TCP-over-Croc
const (
	TCPCommandConnect = 0x01
	TCPCommandData    = 0x02
	TCPCommandClose   = 0x03
)

// TCPCrocProxy реализует TCP-over-Croc прокси
type TCPCrocProxy struct {
	controlConn *comm.Comm
	key         []byte
	isSender    bool

	// TCP сервер
	listener net.Listener
	port     int

	// Управление соединениями
	connections map[uint64]*TCPConnection
	connMu      sync.RWMutex

	// Состояние
	active   bool
	stopChan chan struct{}

	// Для отслеживания активных TCP соединений
	wg sync.WaitGroup
}

// TCPConnection представляет активное TCP соединение
type TCPConnection struct {
	id         uint64
	localConn  net.Conn
	remoteAddr string
	active     bool
	mu         sync.RWMutex
}

// NewTCPCrocProxy создает новый TCP-over-Croc прокси
func NewTCPCrocProxy(conn *comm.Comm, isSender bool, key []byte) *TCPCrocProxy {
	return &TCPCrocProxy{
		controlConn: conn,
		key:         key,
		isSender:    isSender,
		connections: make(map[uint64]*TCPConnection),
		stopChan:    make(chan struct{}),
	}
}

// StartTCPServer запускает TCP сервер для проксирования
func (p *TCPCrocProxy) StartTCPServer(port int) error {
	p.port = port
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("failed to start TCP server on port %d: %w", port, err)
	}
	p.listener = listener
	p.active = true

	log.Infof("TCP-over-Croc proxy started on 127.0.0.1:%d", port)

	if p.isSender {
		// Отправитель: слушаем входящие TCP соединения и перенаправляем через croc
		go p.senderTCPLoop()
	} else {
		// Получатель: слушаем croc и перенаправляем в TCP
		go p.receiverTCPLoop()
	}

	// Запускаем обработку входящих TCP соединений
	go p.acceptTCPLoop()

	return nil
}

// Stop останавливает TCP прокси
func (p *TCPCrocProxy) Stop() error {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	if !p.active {
		return nil
	}

	p.active = false

	// Закрываем все активные TCP соединения
	for _, conn := range p.connections {
		conn.Close()
	}
	p.connections = make(map[uint64]*TCPConnection)

	// Закрываем listener
	if p.listener != nil {
		p.listener.Close()
	}

	// Закрываем канал остановки
	select {
	case <-p.stopChan:
	default:
		close(p.stopChan)
	}

	// Ждем завершения всех горутин
	p.wg.Wait()

	log.Info("TCP-over-Croc proxy stopped")
	return nil
}

// IsActive возвращает true если прокси активен
func (p *TCPCrocProxy) IsActive() bool {
	p.connMu.RLock()
	defer p.connMu.RUnlock()
	return p.active
}

// acceptTCPLoop обрабатывает входящие TCP соединения
func (p *TCPCrocProxy) acceptTCPLoop() {
	defer p.wg.Done()
	p.wg.Add(1)

	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		conn, err := p.listener.Accept()
		if err != nil {
			if !p.active {
				return
			}
			log.Errorf("TCP accept error: %v", err)
			continue
		}

		// Генерируем уникальный ID для соединения
		connID := p.generateConnectionID()

		// Создаем новое TCP соединение
		tcpConn := &TCPConnection{
			id:        connID,
			localConn: conn,
			active:    true,
		}

		// Сохраняем соединение
		p.connMu.Lock()
		p.connections[connID] = tcpConn
		p.connMu.Unlock()

		// Запускаем обработку соединения
		go p.handleTCPConnection(tcpConn)
	}
}

// handleTCPConnection обрабатывает TCP соединение
func (p *TCPCrocProxy) handleTCPConnection(tcpConn *TCPConnection) {
	defer func() {
		// Удаляем соединение из списка
		p.connMu.Lock()
		delete(p.connections, tcpConn.id)
		p.connMu.Unlock()

		// Закрываем локальное соединение
		if tcpConn.localConn != nil {
			tcpConn.localConn.Close()
		}

		p.wg.Done()
	}()
	p.wg.Add(1)

	if p.isSender {
		p.handleSenderTCPConnection(tcpConn)
	} else {
		p.handleReceiverTCPConnection(tcpConn)
	}
}

// handleSenderTCPConnection обрабатывает TCP соединение на стороне отправителя
func (p *TCPCrocProxy) handleSenderTCPConnection(tcpConn *TCPConnection) {
	// Отправляем CONNECT сообщение
	targetAddr := tcpConn.localConn.RemoteAddr().String()
	if err := p.sendTCPMessage(tcpConn.id, TCPCommandConnect, []byte(targetAddr)); err != nil {
		log.Errorf("Failed to send CONNECT: %v", err)
		return
	}

	// Читаем данные из TCP и отправляем через croc
	buffer := make([]byte, 8192)
	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		n, err := tcpConn.localConn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Errorf("TCP read error: %v", err)
			}
			// Отправляем CLOSE сообщение
			p.sendTCPMessage(tcpConn.id, TCPCommandClose, nil)
			return
		}

		// Отправляем данные через croc
		if err := p.sendTCPMessage(tcpConn.id, TCPCommandData, buffer[:n]); err != nil {
			log.Errorf("Failed to send DATA: %v", err)
			return
		}
	}
}

// handleReceiverTCPConnection обрабатывает TCP соединение на стороне получателя
func (p *TCPCrocProxy) handleReceiverTCPConnection(tcpConn *TCPConnection) {
	// Ждем CONNECT сообщение
	select {
	case <-p.stopChan:
		return
	case <-time.After(30 * time.Second):
		log.Errorf("Timeout waiting for CONNECT message")
		return
	}
}

// senderTCPLoop обрабатывает входящие сообщения от получателя (режим отправителя)
func (p *TCPCrocProxy) senderTCPLoop() {
	defer p.wg.Done()
	p.wg.Add(1)

	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		// Читаем сообщение от получателя
		data, err := p.controlConn.Receive()
		if err != nil {
			log.Errorf("Failed to receive message: %v", err)
			continue
		}

		// Расшифровываем сообщение
		decrypted, err := crypt.Decrypt(data, p.key)
		if err != nil {
			log.Errorf("Failed to decrypt message: %v", err)
			continue
		}

		// Обрабатываем сообщение
		if err := p.handleTCPMessage(decrypted); err != nil {
			log.Errorf("Failed to handle TCP message: %v", err)
		}
	}
}

// receiverTCPLoop обрабатывает входящие сообщения от отправителя (режим получателя)
func (p *TCPCrocProxy) receiverTCPLoop() {
	defer p.wg.Done()
	p.wg.Add(1)

	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		// Читаем сообщение от отправителя
		data, err := p.controlConn.Receive()
		if err != nil {
			log.Errorf("Failed to receive message: %v", err)
			continue
		}

		// Расшифровываем сообщение
		decrypted, err := crypt.Decrypt(data, p.key)
		if err != nil {
			log.Errorf("Failed to decrypt message: %v", err)
			continue
		}

		// Обрабатываем сообщение
		if err := p.handleTCPMessage(decrypted); err != nil {
			log.Errorf("Failed to handle TCP message: %v", err)
		}
	}
}

// handleTCPMessage обрабатывает TCP сообщение
func (p *TCPCrocProxy) handleTCPMessage(data []byte) error {
	if len(data) < 13 { // connection-id(8) + command(1) + data-length(4)
		return fmt.Errorf("invalid message length: %d", len(data))
	}

	// Извлекаем connection-id
	connID := binary.LittleEndian.Uint64(data[0:8])

	// Извлекаем команду
	command := data[8]

	// Извлекаем длину данных
	dataLen := binary.LittleEndian.Uint32(data[9:13])

	// Извлекаем данные
	var messageData []byte
	if dataLen > 0 {
		if len(data) < 13+int(dataLen) {
			return fmt.Errorf("invalid message length: expected %d, got %d", 13+dataLen, len(data))
		}
		messageData = data[13 : 13+dataLen]
	}

	switch command {
	case TCPCommandConnect:
		return p.handleTCPConnect(connID, string(messageData))
	case TCPCommandData:
		return p.handleTCPData(connID, messageData)
	case TCPCommandClose:
		return p.handleTCPClose(connID)
	default:
		return fmt.Errorf("unknown command: 0x%02x", command)
	}
}

// handleTCPConnect обрабатывает CONNECT сообщение
func (p *TCPCrocProxy) handleTCPConnect(connID uint64, targetAddr string) error {
	log.Debugf("TCP CONNECT: connection %d to %s", connID, targetAddr)

	if p.isSender {
		// На стороне отправителя CONNECT не должен приходить
		return fmt.Errorf("unexpected CONNECT message on sender side")
	}

	// На стороне получателя: создаем TCP соединение с целевым сервером
	remoteConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Errorf("Failed to connect to %s: %v", targetAddr, err)
		// Отправляем CLOSE сообщение
		p.sendTCPMessage(connID, TCPCommandClose, nil)
		return err
	}

	// Создаем новое TCP соединение
	tcpConn := &TCPConnection{
		id:         connID,
		localConn:  remoteConn,
		remoteAddr: targetAddr,
		active:     true,
	}

	// Сохраняем соединение
	p.connMu.Lock()
	p.connections[connID] = tcpConn
	p.connMu.Unlock()

	// Запускаем обработку соединения
	go p.handleTCPConnection(tcpConn)

	return nil
}

// handleTCPData обрабатывает DATA сообщение
func (p *TCPCrocProxy) handleTCPData(connID uint64, data []byte) error {
	p.connMu.RLock()
	tcpConn, exists := p.connections[connID]
	p.connMu.RUnlock()

	if !exists || !tcpConn.active {
		return fmt.Errorf("connection %d not found or inactive", connID)
	}

	// Записываем данные в TCP соединение
	_, err := tcpConn.localConn.Write(data)
	if err != nil {
		log.Errorf("Failed to write to TCP connection %d: %v", connID, err)
		// Закрываем соединение
		tcpConn.Close()
		return err
	}

	return nil
}

// handleTCPClose обрабатывает CLOSE сообщение
func (p *TCPCrocProxy) handleTCPClose(connID uint64) error {
	log.Debugf("TCP CLOSE: connection %d", connID)

	p.connMu.Lock()
	tcpConn, exists := p.connections[connID]
	if exists {
		delete(p.connections, connID)
	}
	p.connMu.Unlock()

	if exists && tcpConn.active {
		tcpConn.Close()
	}

	return nil
}

// sendTCPMessage отправляет TCP сообщение через croc
func (p *TCPCrocProxy) sendTCPMessage(connID uint64, command byte, data []byte) error {
	// Формируем пакет
	packet := make([]byte, 13+len(data))

	// connection-id (8 байт)
	binary.LittleEndian.PutUint64(packet[0:8], connID)

	// command (1 байт)
	packet[8] = command

	// data-length (4 байта)
	binary.LittleEndian.PutUint32(packet[9:13], uint32(len(data)))

	// data
	if len(data) > 0 {
		copy(packet[13:], data)
	}

	// Шифруем пакет
	encrypted, err := crypt.Encrypt(packet, p.key)
	if err != nil {
		return fmt.Errorf("failed to encrypt packet: %w", err)
	}

	// Отправляем через croc
	return p.controlConn.Send(encrypted)
}

// generateConnectionID генерирует уникальный ID для TCP соединения
func (p *TCPCrocProxy) generateConnectionID() uint64 {
	var id [8]byte
	rand.Read(id[:])
	return binary.LittleEndian.Uint64(id[:])
}

// Close закрывает TCP соединение
func (c *TCPConnection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active && c.localConn != nil {
		c.localConn.Close()
		c.active = false
	}
}

// IsActive возвращает true если соединение активно
func (c *TCPConnection) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

// EnableTCPProxy активирует TCP-over-Croc туннель
func (s *WebDAVServer) EnableTCPProxy(client *croc.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, не активен ли уже TCP прокси
	if s.proxy != nil && s.proxy.IsActive() {
		return fmt.Errorf("TCP proxy already active")
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

	// Параметры TCP туннеля
	roomSuffix := 2                      // используем другой порт, чтобы не конфликтовать с WebDAV
	tcpPort := basePort + roomSuffix + 1 // basePort + 3 = 9012
	tcpRoom := fmt.Sprintf("%s-%d", client.Options.RoomName, roomSuffix)
	tcpAddr := net.JoinHostPort(host, strconv.Itoa(tcpPort))

	log.Infof("TCP-over-Croc tunnel: establishing connection to %s (room: %s)",
		tcpAddr, tcpRoom)

	// Устанавливаем соединение для TCP
	tcpConn, banner, externalIP, err := tcp.ConnectToTCPServer(
		tcpAddr,
		client.Options.RelayPassword,
		tcpRoom,
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to establish TCP tunnel: %w", err)
	}

	log.Debugf("TCP tunnel connected: banner=%s, externalIP=%s", banner, externalIP)

	// Создаем TCP прокси с этим соединением
	tcpProxy := NewTCPCrocProxy(tcpConn, client.Options.IsSender, client.Key)

	if client.Options.IsSender {
		// Отправитель: запускаем TCP сервер
		if err := tcpProxy.StartTCPServer(8080); err != nil {
			tcpConn.Close()
			return fmt.Errorf("failed to start TCP server: %w", err)
		}
		log.Infof("TCP-over-Croc sender proxy started on 127.0.0.1:8080 (port %d, room %s)",
			tcpPort, tcpRoom)

	} else {
		// Получатель: запускаем TCP сервер
		if err := tcpProxy.StartTCPServer(8080); err != nil {
			tcpConn.Close()
			return fmt.Errorf("failed to start TCP server: %w", err)
		}

		// Создаем TCP handler
		tcpHandler := s.createTCPHandler(tcpProxy)

		// Меняем текущий handler на TCP
		if s.localHandler == nil {
			s.localHandler = s.currentHandler
		}
		s.currentHandler = tcpHandler

		log.Infof("TCP-over-Croc proxy handler activated on %s (port %d, room %s)",
			s.addr, tcpPort, tcpRoom)

		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(true)
			s.remote = true
		}
	}

	// Сохраняем прокси
	s.proxy = tcpProxy
	log.Infof("TCP-over-Croc tunnel successfully enabled on port %d (room %s)",
		tcpPort, tcpRoom)

	return nil
}

// createTCPHandler создает handler для TCP прокси
func (s *WebDAVServer) createTCPHandler(tcpProxy *TCPCrocProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalURL := r.URL.String()
		log.Debugf("TCP proxy handler received %s %s", r.Method, originalURL)

		// Создаем TCP клиент для подключения к локальному TCP серверу
		tcpConn, err := net.Dial("tcp", "127.0.0.1:8080")
		if err != nil {
			log.Errorf("Failed to connect to TCP proxy: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer tcpConn.Close()

		// Отправляем HTTP запрос через TCP
		requestLine := fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL.RequestURI(), r.Proto)
		headers := ""
		for name, values := range r.Header {
			for _, value := range values {
				headers += fmt.Sprintf("%s: %s\r\n", name, value)
			}
		}
		httpRequest := requestLine + headers + "\r\n"

		_, err = tcpConn.Write([]byte(httpRequest))
		if err != nil {
			log.Errorf("Failed to write to TCP proxy: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		// Если есть тело запроса, отправляем его
		if r.Body != nil {
			_, err = io.Copy(tcpConn, r.Body)
			if err != nil {
				log.Errorf("Failed to copy request body: %v", err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		}

		// Читаем ответ
		response, err := http.ReadResponse(bufio.NewReader(tcpConn), r)
		if err != nil {
			log.Errorf("Failed to read response: %v", err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()

		// Копируем заголовки ответа
		for name, values := range response.Header {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}

		// Устанавливаем статус-код
		w.WriteHeader(response.StatusCode)

		// Копируем тело ответа
		_, err = io.Copy(w, response.Body)
		if err != nil {
			log.Errorf("Failed to copy response body: %v", err)
		}
	})
}

// DisableTCPProxy отключает TCP-over-Croc туннель
func (s *WebDAVServer) DisableTCPProxy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.proxy != nil {
		log.Info("Disabling TCP-over-Croc tunnel...")

		// Останавливаем TCP прокси
		if tcpProxy, ok := s.proxy.(*TCPCrocProxy); ok {
			tcpProxy.Stop()
		}
		s.proxy = nil

		// Возвращаем локальный handler
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

// IsTCPProxyActive возвращает true если TCP прокси активен
func (s *WebDAVServer) IsTCPProxyActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if tcpProxy, ok := s.proxy.(*TCPCrocProxy); ok {
		return tcpProxy.IsActive()
	}
	return false
}
