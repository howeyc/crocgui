// tcp_forwarder.go
package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/crypt"
	log "github.com/schollz/logger"
)

// Константы для TCP форвардинга
const (
	ForwardBufferSize = 32 * 1024 // 32KB - оптимальный размер для большинства сценариев
	ForwardTimeout    = 30 * time.Second
)

// Пул буферов для уменьшения GC давления
var forwardBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, ForwardBufferSize)
	},
}

// ForwarderConnID уникальный идентификатор forwarded connection
type ForwarderConnID uint64

// ForwardMessage типы сообщений для портфорвардинга
const (
	ForwardMsgOpen  = 0x01 // Открытие нового соединения
	ForwardMsgData  = 0x02 // Данные соединения
	ForwardMsgClose = 0x03 // Закрытие соединения
	ForwardMsgError = 0x04 // Ошибка соединения
)

// TCPForwarder управляет TCP форвардингом через croc туннель
type TCPForwarder struct {
	controlConn *comm.Comm
	key         []byte
	isSender    bool

	// На стороне отправителя: map[connID]net.Conn - активные forwarded connections
	// На стороне получателя: map[connID]chan []byte - каналы для отправки данных
	connections sync.Map

	// На стороне отправителя: адрес локального сервера для подключения
	localServerAddr string

	active   bool
	stopChan chan struct{}
	mu       sync.RWMutex
}

// NewTCPForwarder создает новый TCP форвардер
// localServerAddr - адрес локального сервера для подключения на стороне отправителя (например, "127.0.0.1:9009")
func NewTCPForwarder(conn *comm.Comm, isSender bool, key []byte, localServerAddr string) *TCPForwarder {
	return &TCPForwarder{
		controlConn:     conn,
		isSender:        isSender,
		key:             key,
		localServerAddr: localServerAddr,
		stopChan:        make(chan struct{}),
	}
}

// Start запускает форвардер
func (f *TCPForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.active {
		return nil
	}

	f.active = true

	if f.isSender {
		go f.senderLoop()
		log.Info("TCP forwarder sender started")
	} else {
		go f.receiverLoop()
		log.Info("TCP forwarder receiver started")
	}

	return nil
}

// Stop останавливает форвардер
func (f *TCPForwarder) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.active {
		return nil
	}

	f.active = false

	select {
	case <-f.stopChan:
	default:
		close(f.stopChan)
	}

	// Закрываем все активные соединения
	log.Debugf("TCP forwarder: Stop() - closing all active connections")
	f.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(net.Conn); ok {
			log.Debugf("TCP forwarder: Stop() - closing net.Conn for connID=%v", key)
			conn.Close()
		} else if ch, ok := value.(chan []byte); ok {
			log.Debugf("TCP forwarder: Stop() - closing channel for connID=%v", key)
			close(ch)
		}
		return true
	})
	log.Debugf("TCP forwarder: Stop() - all connections closed")

	log.Info("TCP forwarder stopped")
	return nil
}

// IsActive возвращает true если форвардер активен
func (f *TCPForwarder) IsActive() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.active
}

// ForwardConnection пробрасывает локальное соединение через croc туннель (на стороне получателя)
func (f *TCPForwarder) ForwardConnection(localConn net.Conn) error {
	if f.isSender {
		return io.ErrClosedPipe // Не для отправителя
	}

	// Генерируем уникальный ID для соединения
	connID := f.generateID()

	log.Infof("TCP forwarder receiver: ForwardConnection called - connID=%d, localConn.RemoteAddr=%s, localConn.LocalAddr=%s",
		connID, localConn.RemoteAddr(), localConn.LocalAddr())

	// Создаем два канала: для отправки данных в туннель и для получения данных от сервера
	sendDataChan := make(chan []byte, 10)
	receiveDataChan := make(chan []byte, 10)
	f.connections.Store(connID, receiveDataChan)

	log.Debugf("TCP forwarder receiver: stored receiveDataChan in connections map for connID=%d", connID)

	// Отправляем сообщение об открытии соединения
	log.Debugf("TCP forwarder receiver: sending ForwardMsgOpen for connID=%d", connID)
	if err := f.sendMessage(connID, ForwardMsgOpen, nil); err != nil {
		f.connections.Delete(connID)
		return err
	}

	log.Debugf("TCP forwarder receiver: sent ForwardMsgOpen for connID=%d", connID)
	log.Infof("TCP forwarder: opened connection %d", connID)

	// Запускаем горутину для чтения из локального соединения и отправки в туннель
	go func() {
		defer localConn.Close()
		defer close(sendDataChan)
		defer f.connections.Delete(connID)

		log.Debugf("TCP forwarder receiver: starting read goroutine for connID=%d", connID)

		buffer := forwardBufferPool.Get().([]byte)
		defer forwardBufferPool.Put(buffer)

		totalRead := 0
		for {
			n, err := localConn.Read(buffer)
			if n > 0 {
				totalRead += n
				// Отправляем данные в туннель через канал
				data := make([]byte, n)
				copy(data, buffer[:n])
				log.Debugf("TCP forwarder receiver: read %d bytes from localConn (total=%d) for connID=%d", n, totalRead, connID)
				log.Debugf("TCP forwarder receiver: sending %d bytes to sendDataChan for connID=%d", n, connID)
				select {
				case sendDataChan <- data:
					log.Debugf("TCP forwarder receiver: sent %d bytes to sendDataChan for connID=%d", n, connID)
				case <-time.After(ForwardTimeout):
					log.Warnf("TCP forwarder: timeout sending data for connection %d", connID)
					return
				case <-f.stopChan:
					return
				case <-done:
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Debugf("TCP forwarder: read error for connection %d: %v", connID, err)
				}
				break
			}
		}

		// Отправляем сообщение о закрытии соединения
		log.Debugf("TCP forwarder receiver: sending ForwardMsgClose for connID=%d", connID)
		_ = f.sendMessage(connID, ForwardMsgClose, nil)
		log.Debugf("TCP forwarder: closed connection %d (read %d total bytes)", connID, totalRead)
	}()

	// Запускаем горутину для отправки данных из sendDataChan в туннель
	go func() {
		defer localConn.Close()
		defer func() {
			log.Debugf("TCP forwarder receiver: attempting to close receiveDataChan for connID=%d (goroutine send-to-tunnel)", connID)
			if r := recover(); r != nil {
				log.Debugf("TCP forwarder receiver: recovered from panic in defer for connID=%d: %v", connID, r)
			}
			close(receiveDataChan)
			log.Debugf("TCP forwarder receiver: successfully closed receiveDataChan for connID=%d (goroutine send-to-tunnel)", connID)
		}()
		defer f.connections.Delete(connID)

		log.Debugf("TCP forwarder receiver: starting send-to-tunnel goroutine for connID=%d", connID)

		totalSent := 0
		for {
			select {
			case data, ok := <-sendDataChan:
				if !ok {
					log.Debugf("TCP forwarder receiver: sendDataChan closed for connID=%d", connID)
					return
				}
				log.Debugf("TCP forwarder receiver: received %d bytes from sendDataChan for connID=%d", len(data), connID)
				log.Debugf("TCP forwarder receiver: sending %d bytes to tunnel for connID=%d", len(data), connID)
				// Отправляем данные в туннель
				if err := f.sendMessage(connID, ForwardMsgData, data); err != nil {
					log.Errorf("TCP forwarder receiver: failed to send data to tunnel for connID=%d: %v", connID, err)
					return
				}
				totalSent += len(data)
				log.Debugf("TCP forwarder receiver: sent %d bytes to tunnel (total=%d) for connID=%d", len(data), totalSent, connID)
			case <-f.stopChan:
				return
			case <-done:
				return
			}
		}
	}()

	// Запускаем горутину для получения данных из receiveDataChan и записи в localConn
	go func() {
		defer localConn.Close()
		defer f.connections.Delete(connID)

		log.Debugf("TCP forwarder receiver: starting write-to-local goroutine for connID=%d", connID)

		totalWritten := 0
		for {
			select {
			case data, ok := <-receiveDataChan:
				if !ok {
					log.Debugf("TCP forwarder receiver: receiveDataChan closed for connID=%d", connID)
					return
				}
				log.Debugf("TCP forwarder receiver: received %d bytes from receiveDataChan for connID=%d", len(data), connID)
				log.Debugf("TCP forwarder receiver: writing %d bytes to localConn for connID=%d", len(data), connID)
				if _, err := localConn.Write(data); err != nil {
					log.Debugf("TCP forwarder: write error for connection %d: %v", connID, err)
					return
				}
				totalWritten += len(data)
				log.Debugf("TCP forwarder receiver: wrote %d bytes to localConn (total=%d) for connID=%d", len(data), totalWritten, connID)
			case <-f.stopChan:
				return
			case <-done:
				return
			}
		}
	}()

	return nil
}

// senderLoop - отправитель принимает forwarded connections и пробрасывает к локальному серверу
func (f *TCPForwarder) senderLoop() {
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	go func() {
		for {
			data, err := f.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-f.stopChan:
				return
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-f.stopChan:
			log.Debug("TCP forwarder sender loop stopped")
			return
		case <-done:
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("TCP forwarder sender loop error: %v", result.err)
				return
			}
			f.handleSenderMessage(result.data)
		}
	}
}

// receiverLoop - получатель принимает сообщения из туннеля и направляет в соответствующие соединения
func (f *TCPForwarder) receiverLoop() {
	type readResult struct {
		data []byte
		err  error
	}
	readChan := make(chan readResult, 1)

	go func() {
		for {
			data, err := f.controlConn.Receive()
			select {
			case readChan <- readResult{data: data, err: err}:
			case <-f.stopChan:
				return
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-f.stopChan:
			log.Debug("TCP forwarder receiver loop stopped")
			return
		case <-done:
			return
		case result := <-readChan:
			if result.err != nil {
				log.Debugf("TCP forwarder receiver loop error: %v", result.err)
				return
			}
			f.handleReceiverMessage(result.data)
		}
	}
}

// handleSenderMessage обрабатывает сообщения на стороне отправителя
func (f *TCPForwarder) handleSenderMessage(data []byte) {
	connID, msgType, payload, err := f.decodeMessage(data)
	if err != nil {
		log.Errorf("TCP forwarder sender: failed to decode message: %v", err)
		return
	}

	switch msgType {
	case ForwardMsgOpen:
		log.Infof("TCP forwarder sender: ForwardMsgOpen received for connID=%d", connID)

		// Создаем соединение к локальному серверу
		if f.localServerAddr == "" {
			log.Errorf("TCP forwarder sender: localServerAddr is empty, cannot create connection for connID=%d", connID)
			f.sendMessage(connID, ForwardMsgError, []byte("local server address not configured"))
			return
		}

		localConn, err := net.Dial("tcp", f.localServerAddr)
		if err != nil {
			log.Errorf("TCP forwarder sender: failed to dial local server %s for connID=%d: %v", f.localServerAddr, connID, err)
			f.sendMessage(connID, ForwardMsgError, []byte(fmt.Sprintf("failed to connect to local server: %v", err)))
			return
		}

		log.Infof("TCP forwarder sender: connected to local server %s for connID=%d, remote=%s", f.localServerAddr, connID, localConn.RemoteAddr())

		// Сохраняем соединение в connections
		f.connections.Store(connID, localConn)

		// Запускаем горутину для чтения из локального соединения и отправки в туннель
		go f.senderReadLoop(connID, localConn)

	case ForwardMsgData:
		log.Debugf("TCP forwarder sender: received ForwardMsgData for connID=%d, payloadLen=%d", connID, len(payload))
		// Получаем соединение для записи данных
		val, ok := f.connections.Load(connID)
		if !ok {
			log.Warnf("TCP forwarder sender: no connection %d for data", connID)
			return
		}

		localConn, ok := val.(net.Conn)
		if !ok {
			log.Errorf("TCP forwarder sender: invalid connection %d, type=%T", connID, val)
			return
		}

		// Пишем данные в локальное соединение
		if len(payload) > 0 {
			if _, err := localConn.Write(payload); err != nil {
				log.Errorf("TCP forwarder sender: write error for connection %d: %v", connID, err)
				f.closeSenderConnection(connID, localConn)
				return
			}
			log.Debugf("TCP forwarder sender: wrote %d bytes to localConn for connID=%d", len(payload), connID)
		}

	case ForwardMsgClose:
		log.Debugf("TCP forwarder sender: received ForwardMsgClose for connID=%d", connID)
		val, ok := f.connections.Load(connID)
		if ok {
			if localConn, ok := val.(net.Conn); ok {
				log.Debugf("TCP forwarder sender: closing connection for connID=%d due to ForwardMsgClose", connID)
				f.closeSenderConnection(connID, localConn)
			}
		} else {
			log.Debugf("TCP forwarder sender: connection %d not found in map for ForwardMsgClose", connID)
		}
		log.Debugf("TCP forwarder sender: connection %d closed", connID)

	case ForwardMsgError:
		log.Errorf("TCP forwarder sender: error for connection %d: %s", connID, string(payload))
		val, ok := f.connections.Load(connID)
		if ok {
			if localConn, ok := val.(net.Conn); ok {
				f.closeSenderConnection(connID, localConn)
			}
		}
	}
}

// senderReadLoop читает данные из локального соединения и отправляет в туннель
func (f *TCPForwarder) senderReadLoop(connID ForwarderConnID, localConn net.Conn) {
	log.Debugf("TCP forwarder sender: senderReadLoop started for connID=%d", connID)
	defer func() {
		log.Debugf("TCP forwarder sender: senderReadLoop defer closing for connID=%d", connID)
		f.closeSenderConnection(connID, localConn)
	}()

	log.Debugf("TCP forwarder sender: starting read loop for connID=%d", connID)

	buffer := forwardBufferPool.Get().([]byte)
	defer forwardBufferPool.Put(buffer)

	totalRead := 0
	for {
		n, err := localConn.Read(buffer)
		if n > 0 {
			totalRead += n
			log.Debugf("TCP forwarder sender: read %d bytes from localConn (total=%d) for connID=%d", n, totalRead, connID)

			// Отправляем данные в туннель
			if err := f.sendMessage(connID, ForwardMsgData, buffer[:n]); err != nil {
				log.Errorf("TCP forwarder sender: failed to send data for connID=%d: %v", connID, err)
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Debugf("TCP forwarder sender: read error for connection %d: %v", connID, err)
			}
			break
		}
	}

	// Отправляем сообщение о закрытии соединения
	log.Debugf("TCP forwarder sender: sending ForwardMsgClose for connID=%d", connID)
	_ = f.sendMessage(connID, ForwardMsgClose, nil)
	log.Infof("TCP forwarder sender: closed connection %d (read %d total bytes)", connID, totalRead)
}

// closeSenderConnection закрывает соединение на стороне отправителя
func (f *TCPForwarder) closeSenderConnection(connID ForwarderConnID, localConn net.Conn) {
	log.Debugf("TCP forwarder sender: closeSenderConnection called for connID=%d, stack trace:", connID)

	// Проверяем, есть ли соединение в map
	_, exists := f.connections.Load(connID)
	log.Debugf("TCP forwarder sender: connID=%d exists in map: %v", connID, exists)

	f.connections.Delete(connID)

	// Пытаемся закрыть соединение с обработкой ошибки
	err := localConn.Close()
	if err != nil {
		log.Debugf("TCP forwarder sender: close error for connID=%d: %v", connID, err)
	}
	log.Debugf("TCP forwarder sender: closed local connection for connID=%d", connID)
}

// handleReceiverMessage обрабатывает сообщения на стороне получателя
func (f *TCPForwarder) handleReceiverMessage(data []byte) {
	connID, msgType, payload, err := f.decodeMessage(data)
	if err != nil {
		log.Errorf("TCP forwarder receiver: failed to decode message: %v", err)
		return
	}

	log.Debugf("TCP forwarder receiver: received msgType=0x%02x for connID=%d, payloadLen=%d", msgType, connID, len(payload))

	switch msgType {
	case ForwardMsgOpen:
		// Соединение открывается при первом ForwardConnection()
		log.Debugf("TCP forwarder receiver: connection %d open request", connID)

	case ForwardMsgData:
		log.Debugf("TCP forwarder receiver: ForwardMsgData for connID=%d, payloadLen=%d", connID, len(payload))
		val, ok := f.connections.Load(connID)
		if !ok {
			log.Warnf("TCP forwarder receiver: no connection %d for data", connID)
			return
		}
		// Логируем тип значения для диагностики
		log.Debugf("TCP forwarder receiver: connID=%d, value type=%T", connID, val)
		receiveDataChan, ok := val.(chan []byte)
		if !ok {
			log.Errorf("TCP forwarder receiver: invalid connection %d, type=%T", connID, val)
			return
		}

		// Отправляем данные в канал
		dataCopy := make([]byte, len(payload))
		copy(dataCopy, payload)
		log.Debugf("TCP forwarder receiver: sending %d bytes to receiveDataChan for connID=%d", len(dataCopy), connID)
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Debugf("TCP forwarder receiver: recovered from panic when sending to receiveDataChan for connID=%d: %v", connID, r)
				}
			}()
			select {
			case receiveDataChan <- dataCopy:
				log.Debugf("TCP forwarder receiver: successfully sent %d bytes to receiveDataChan for connID=%d", len(dataCopy), connID)
			case <-time.After(ForwardTimeout):
				log.Warnf("TCP forwarder receiver: timeout sending data for connection %d", connID)
			case <-f.stopChan:
			case <-done:
			}
		}()

	case ForwardMsgClose:
		val, ok := f.connections.Load(connID)
		if ok {
			if receiveDataChan, ok := val.(chan []byte); ok {
				close(receiveDataChan)
			}
			f.connections.Delete(connID)
		}
		log.Debugf("TCP forwarder receiver: connection %d closed", connID)

	case ForwardMsgError:
		log.Errorf("TCP forwarder receiver: error for connection %d: %s", connID, string(payload))
		val, ok := f.connections.Load(connID)
		if ok {
			if receiveDataChan, ok := val.(chan []byte); ok {
				close(receiveDataChan)
			}
			f.connections.Delete(connID)
		}
	}
}

// sendMessage отправляет зашифрованное сообщение через croc туннель
func (f *TCPForwarder) sendMessage(connID ForwarderConnID, msgType byte, payload []byte) error {
	msgTypeName := "Unknown"
	switch msgType {
	case ForwardMsgOpen:
		msgTypeName = "ForwardMsgOpen"
	case ForwardMsgData:
		msgTypeName = "ForwardMsgData"
	case ForwardMsgClose:
		msgTypeName = "ForwardMsgClose"
	case ForwardMsgError:
		msgTypeName = "ForwardMsgError"
	}
	log.Debugf("TCP forwarder: sendMessage called - connID=%d, msgType=0x%02x (%s), payloadLen=%d", connID, msgType, msgTypeName, len(payload))

	// Формируем пакет: [connID(8)][msgType(1)][payloadLen(4)][payload...]
	packet := make([]byte, 13+len(payload))

	// Записываем connID
	packet[0] = byte(connID)
	packet[1] = byte(connID >> 8)
	packet[2] = byte(connID >> 16)
	packet[3] = byte(connID >> 24)
	packet[4] = byte(connID >> 32)
	packet[5] = byte(connID >> 40)
	packet[6] = byte(connID >> 48)
	packet[7] = byte(connID >> 56)

	// Записываем тип сообщения
	packet[8] = msgType

	// Записываем длину payload
	packet[9] = byte(len(payload))
	packet[10] = byte(len(payload) >> 8)
	packet[11] = byte(len(payload) >> 16)
	packet[12] = byte(len(payload) >> 24)

	// Записываем payload
	copy(packet[13:], payload)

	// Шифруем пакет
	encrypted, err := crypt.Encrypt(packet, f.key)
	if err != nil {
		log.Errorf("TCP forwarder: encryption error for connID=%d: %v", connID, err)
		return err
	}

	// Отправляем через croc туннель
	err = f.controlConn.Send(encrypted)
	if err != nil {
		log.Errorf("TCP forwarder: send error for connID=%d: %v", connID, err)
	} else {
		log.Debugf("TCP forwarder: message sent successfully - connID=%d, msgType=0x%02x (%s)", connID, msgType, msgTypeName)
	}
	return err
}

// decodeMessage декодирует сообщение из croc туннеля
func (f *TCPForwarder) decodeMessage(data []byte) (connID ForwarderConnID, msgType byte, payload []byte, err error) {
	// Расшифровываем
	decrypted, err := crypt.Decrypt(data, f.key)
	if err != nil {
		return 0, 0, nil, err
	}

	// Проверяем минимальную длину
	if len(decrypted) < 13 {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}

	// Читаем connID
	connID = ForwarderConnID(decrypted[0]) |
		ForwarderConnID(decrypted[1])<<8 |
		ForwarderConnID(decrypted[2])<<16 |
		ForwarderConnID(decrypted[3])<<24 |
		ForwarderConnID(decrypted[4])<<32 |
		ForwarderConnID(decrypted[5])<<40 |
		ForwarderConnID(decrypted[6])<<48 |
		ForwarderConnID(decrypted[7])<<56

	// Читаем тип сообщения
	msgType = decrypted[8]

	// Читаем длину payload
	payloadLen := int(decrypted[9]) |
		int(decrypted[10])<<8 |
		int(decrypted[11])<<16 |
		int(decrypted[12])<<24

	// Читаем payload
	if payloadLen > 0 && len(decrypted) >= 13+payloadLen {
		payload = decrypted[13 : 13+payloadLen]
	} else if payloadLen > 0 {
		return 0, 0, nil, io.ErrUnexpectedEOF
	}

	return connID, msgType, payload, nil
}

// generateID генерирует уникальный ID соединения
func (f *TCPForwarder) generateID() ForwarderConnID {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		log.Errorf("Failed to generate connection ID: %v", err)
		return ForwarderConnID(time.Now().UnixNano())
	}
	return ForwarderConnID(b[0]) |
		ForwarderConnID(b[1])<<8 |
		ForwarderConnID(b[2])<<16 |
		ForwarderConnID(b[3])<<24 |
		ForwarderConnID(b[4])<<32 |
		ForwarderConnID(b[5])<<40 |
		ForwarderConnID(b[6])<<48 |
		ForwarderConnID(b[7])<<56
}
