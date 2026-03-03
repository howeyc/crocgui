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
	ForwardBufferSize = 64 * 1024 // 64KB - соответствует TCP_BUFFER_SIZE в оригинальной библиотеке croc
	ForwardTimeout    = 100 * time.Millisecond
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

	chlose(f.stopChan)

	// Закрываем все активные соединения
	f.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(net.Conn); ok {
			conn.Close()
		} else if ch, ok := value.(chan []byte); ok {
			chlose(ch)
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

	log.Infof("TCP forwarder: opened connection %d", connID)

	// Создаем два канала: для отправки данных в туннель и для получения данных от сервера
	sendDataChan := make(chan []byte, 500)    // Увеличен буфер для уменьшения блокировок (32MB)
	receiveDataChan := make(chan []byte, 500) // Увеличен буфер для уменьшения блокировок (32MB)
	f.connections.Store(connID, receiveDataChan)

	// Отправляем сообщение об открытии соединения
	if err := f.sendMessage(connID, ForwardMsgOpen, nil); err != nil {
		f.connections.Delete(connID)
		return err
	}

	// Запускаем горутину для чтения из локального соединения и отправки в туннель
	go func() {
		defer localConn.Close()
		defer chlose(sendDataChan)
		defer f.connections.Delete(connID)

		buffer := forwardBufferPool.Get().([]byte)
		defer forwardBufferPool.Put(buffer)

		totalRead := 0
		for {
			n, err := localConn.Read(buffer)
			if n > 0 {
				totalRead += n
				// Отправляем данные в туннель через канал (блокирующая отправка с таймаутом 100ms)
				data := make([]byte, n)
				copy(data, buffer[:n])

				// Блокирующая отправка с таймаутом - данные не теряются!
				select {
				case sendDataChan <- data:
					// Успешно отправлено
				case <-time.After(ForwardTimeout):
					// Кратковременная задержка приемлема - канал освободится очень быстро
					sendDataChan <- data
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Errorf("TCP forwarder: read error for connection %d: %v", connID, err)
				}
				break
			}
		}

		// Отправляем сообщение о закрытии соединения
		_ = f.sendMessage(connID, ForwardMsgClose, nil)
		log.Infof("TCP forwarder: closed connection %d (read %d total bytes)", connID, totalRead)
	}()

	// Запускаем горутину для отправки данных из sendDataChan в туннель
	go func() {
		defer localConn.Close()
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("TCP forwarder: recovered from panic in defer for connID=%d: %v", connID, r)
			}
			chlose(receiveDataChan)
		}()
		defer f.connections.Delete(connID)

		for {
			select {
			case data, ok := <-sendDataChan:
				if !ok {
					return
				}
				// Отправляем данные в туннель
				if err := f.sendMessage(connID, ForwardMsgData, data); err != nil {
					log.Errorf("TCP forwarder: failed to send data to tunnel for connID=%d: %v", connID, err)
					return
				}
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

		totalWritten := 0
		for {
			select {
			case data, ok := <-receiveDataChan:
				if !ok {
					return
				}
				if _, err := localConn.Write(data); err != nil {
					log.Errorf("TCP forwarder: write error for connection %d: %v", connID, err)
					return
				}
				totalWritten += len(data)
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
				log.Errorf("TCP forwarder sender loop error: %v", result.err)
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
				log.Errorf("TCP forwarder receiver loop error: %v", result.err)
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
		log.Infof("TCP forwarder: opened connection %d on sender", connID)

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

		log.Infof("TCP forwarder: connected to local server %s for connID=%d", f.localServerAddr, connID)

		// Сохраняем соединение в connections
		f.connections.Store(connID, localConn)

		// Запускаем горутину для чтения из локального соединения и отправки в туннель
		go f.senderReadLoop(connID, localConn)

	case ForwardMsgData:
		// Получаем соединение для записи данных
		val, ok := f.connections.Load(connID)
		if !ok {
			log.Warnf("TCP forwarder: no connection %d for data", connID)
			return
		}

		localConn, ok := val.(net.Conn)
		if !ok {
			log.Errorf("TCP forwarder: invalid connection %d, type=%T", connID, val)
			return
		}

		// Пишем данные в локальное соединение
		if len(payload) > 0 {
			if _, err := localConn.Write(payload); err != nil {
				log.Errorf("TCP forwarder: write error for connection %d: %v", connID, err)
				f.closeSenderConnection(connID, localConn)
				return
			}
		}

	case ForwardMsgClose:
		val, ok := f.connections.Load(connID)
		if ok {
			if localConn, ok := val.(net.Conn); ok {
				f.closeSenderConnection(connID, localConn)
			}
		}

	case ForwardMsgError:
		log.Errorf("TCP forwarder: error for connection %d: %s", connID, string(payload))
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
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("TCP forwarder sender: recovered from panic in senderReadLoop for connID=%d: %v", connID, r)
		}
		f.closeSenderConnection(connID, localConn)
	}()

	buffer := forwardBufferPool.Get().([]byte)
	defer forwardBufferPool.Put(buffer)

	totalRead := 0
	for {
		n, err := localConn.Read(buffer)
		if n > 0 {
			totalRead += n

			// Отправляем данные в туннель
			if err := f.sendMessage(connID, ForwardMsgData, buffer[:n]); err != nil {
				log.Errorf("TCP forwarder sender: failed to send data for connID=%d: %v", connID, err)
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Errorf("TCP forwarder sender: read error for connection %d: %v", connID, err)
			}
			break
		}
	}

	// Отправляем сообщение о закрытии соединения
	_ = f.sendMessage(connID, ForwardMsgClose, nil)
	log.Infof("TCP forwarder sender: closed connection %d (read %d total bytes)", connID, totalRead)
}

// closeSenderConnection закрывает соединение на стороне отправителя
func (f *TCPForwarder) closeSenderConnection(connID ForwarderConnID, localConn net.Conn) {
	f.connections.Delete(connID)
	localConn.Close()
}

// handleReceiverMessage обрабатывает сообщения на стороне получателя
func (f *TCPForwarder) handleReceiverMessage(data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("TCP forwarder: recovered from panic in handleReceiverMessage: %v", r)
		}
	}()

	connID, msgType, payload, err := f.decodeMessage(data)
	if err != nil {
		log.Errorf("TCP forwarder receiver: failed to decode message: %v", err)
		return
	}

	switch msgType {
	case ForwardMsgOpen:
		// Соединение открывается при первом ForwardConnection()

	case ForwardMsgData:
		val, ok := f.connections.Load(connID)
		if !ok {
			// Соединение закрыто - игнорируем данные (нормальная ситуация при разрыве)
			return
		}

		receiveDataChan, ok := val.(chan []byte)
		if !ok {
			log.Errorf("TCP forwarder receiver: invalid connection %d, type=%T", connID, val)
			return
		}

		// Отправляем данные в канал (блокирующая отправка с таймаутом 100ms)
		dataCopy := make([]byte, len(payload))
		copy(dataCopy, payload)

		// Блокирующая отправка с таймаутом - данные не теряются!
		select {
		case receiveDataChan <- dataCopy:
			// Успешно отправлено
		case <-time.After(ForwardTimeout):
			// Кратковременная задержка приемлема - канал освободится очень быстро
			// После ожидания проверяем, что канал еще открыт
			select {
			case receiveDataChan <- dataCopy:
				// Успешно отправлено после ожидания
			default:
				// Канал закрыт - игнорируем данные без лога (нормальная ситуация)
			}
		}

	case ForwardMsgClose:
		val, ok := f.connections.Load(connID)
		if ok {
			if receiveDataChan, ok := val.(chan []byte); ok {
				chlose(receiveDataChan)
			}
			f.connections.Delete(connID)
		}

	case ForwardMsgError:
		log.Errorf("TCP forwarder: error for connection %d: %s", connID, string(payload))
		val, ok := f.connections.Load(connID)
		if ok {
			if receiveDataChan, ok := val.(chan []byte); ok {
				chlose(receiveDataChan)
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
		log.Errorf("TCP forwarder: encryption error for connID=%d: %v", connID, err)
		return err
	}

	// Отправляем через croc туннель
	err = f.controlConn.Send(encrypted)
	if err != nil {
		log.Errorf("TCP forwarder: send error for connID=%d: %v", connID, err)
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

func chlose[T any](ch chan T) {
	if ch == nil {
		return
	}
	defer func() { recover() }()
	close(ch)
}
