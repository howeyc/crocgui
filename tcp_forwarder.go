// tcp_forwarder.go
package main

import (
	"crypto/rand"
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

	active   bool
	stopChan chan struct{}
	mu       sync.RWMutex
}

// NewTCPForwarder создает новый TCP форвардер
func NewTCPForwarder(conn *comm.Comm, isSender bool, key []byte) *TCPForwarder {
	return &TCPForwarder{
		controlConn: conn,
		isSender:    isSender,
		key:         key,
		stopChan:    make(chan struct{}),
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
	f.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(net.Conn); ok {
			conn.Close()
		} else if ch, ok := value.(chan []byte); ok {
			close(ch)
		}
		return true
	})

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

	// Создаем канал для отправки данных в croc туннель
	dataChan := make(chan []byte, 10)
	f.connections.Store(connID, dataChan)

	// Отправляем сообщение об открытии соединения
	if err := f.sendMessage(connID, ForwardMsgOpen, nil); err != nil {
		f.connections.Delete(connID)
		return err
	}

	log.Debugf("TCP forwarder: opened connection %d", connID)

	// Запускаем горутину для чтения из локального соединения и отправки в туннель
	go func() {
		defer localConn.Close()
		defer close(dataChan)
		defer f.connections.Delete(connID)

		buffer := forwardBufferPool.Get().([]byte)
		defer forwardBufferPool.Put(buffer)

		for {
			n, err := localConn.Read(buffer)
			if n > 0 {
				// Отправляем данные в туннель через канал
				data := make([]byte, n)
				copy(data, buffer[:n])
				select {
				case dataChan <- data:
				case <-time.After(ForwardTimeout):
					log.Warnf("TCP forwarder: timeout sending data for connection %d", connID)
					return
				case <-f.stopChan:
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
		_ = f.sendMessage(connID, ForwardMsgClose, nil)
		log.Debugf("TCP forwarder: closed connection %d", connID)
	}()

	// Запускаем горутину для чтения из туннеля и записи в локальное соединение
	go func() {
		defer localConn.Close()
		defer f.connections.Delete(connID)

		buffer := forwardBufferPool.Get().([]byte)
		defer forwardBufferPool.Put(buffer)

		for {
			select {
			case data, ok := <-dataChan:
				if !ok {
					return
				}
				if _, err := localConn.Write(data); err != nil {
					log.Debugf("TCP forwarder: write error for connection %d: %v", connID, err)
					return
				}
			case <-f.stopChan:
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
			}
		}
	}()

	for {
		select {
		case <-f.stopChan:
			log.Debug("TCP forwarder sender loop stopped")
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
			}
		}
	}()

	for {
		select {
		case <-f.stopChan:
			log.Debug("TCP forwarder receiver loop stopped")
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
		log.Debugf("TCP forwarder sender: connection %d opened", connID)
		// Соединение будет открыто когда получатель отправит первые данные
		// Или мы можем предсоздать соединение к локальному серверу

	case ForwardMsgData:
		// Получаем канал для отправки данных
		val, ok := f.connections.Load(connID)
		if !ok {
			log.Warnf("TCP forwarder sender: no connection %d for data", connID)
			return
		}
		dataChan, ok := val.(chan []byte)
		if !ok {
			log.Errorf("TCP forwarder sender: invalid connection %d", connID)
			return
		}

		// Отправляем данные
		dataCopy := make([]byte, len(payload))
		copy(dataCopy, payload)
		select {
		case dataChan <- dataCopy:
		case <-time.After(ForwardTimeout):
			log.Warnf("TCP forwarder sender: timeout sending data for connection %d", connID)
		case <-f.stopChan:
		}

	case ForwardMsgClose:
		val, ok := f.connections.Load(connID)
		if ok {
			if dataChan, ok := val.(chan []byte); ok {
				close(dataChan)
			}
			f.connections.Delete(connID)
		}
		log.Debugf("TCP forwarder sender: connection %d closed", connID)

	case ForwardMsgError:
		log.Errorf("TCP forwarder sender: error for connection %d: %s", connID, string(payload))
		val, ok := f.connections.Load(connID)
		if ok {
			if dataChan, ok := val.(chan []byte); ok {
				close(dataChan)
			}
			f.connections.Delete(connID)
		}
	}
}

// handleReceiverMessage обрабатывает сообщения на стороне получателя
func (f *TCPForwarder) handleReceiverMessage(data []byte) {
	connID, msgType, payload, err := f.decodeMessage(data)
	if err != nil {
		log.Errorf("TCP forwarder receiver: failed to decode message: %v", err)
		return
	}

	switch msgType {
	case ForwardMsgOpen:
		// Соединение открывается при первом ForwardConnection()
		log.Debugf("TCP forwarder receiver: connection %d open request", connID)

	case ForwardMsgData:
		val, ok := f.connections.Load(connID)
		if !ok {
			log.Warnf("TCP forwarder receiver: no connection %d for data", connID)
			return
		}
		dataChan, ok := val.(chan []byte)
		if !ok {
			log.Errorf("TCP forwarder receiver: invalid connection %d", connID)
			return
		}

		// Отправляем данные в канал
		dataCopy := make([]byte, len(payload))
		copy(dataCopy, payload)
		select {
		case dataChan <- dataCopy:
		case <-time.After(ForwardTimeout):
			log.Warnf("TCP forwarder receiver: timeout sending data for connection %d", connID)
		case <-f.stopChan:
		}

	case ForwardMsgClose:
		val, ok := f.connections.Load(connID)
		if ok {
			if dataChan, ok := val.(chan []byte); ok {
				close(dataChan)
			}
			f.connections.Delete(connID)
		}
		log.Debugf("TCP forwarder receiver: connection %d closed", connID)

	case ForwardMsgError:
		log.Errorf("TCP forwarder receiver: error for connection %d: %s", connID, string(payload))
		val, ok := f.connections.Load(connID)
		if ok {
			if dataChan, ok := val.(chan []byte); ok {
				close(dataChan)
			}
			f.connections.Delete(connID)
		}
	}
}

// sendMessage отправляет зашифрованное сообщение через croc туннель
func (f *TCPForwarder) sendMessage(connID ForwarderConnID, msgType byte, payload []byte) error {
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
		return err
	}

	// Отправляем через croc туннель
	return f.controlConn.Send(encrypted)
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
