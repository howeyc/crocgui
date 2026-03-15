// tcp_forwarder.go
package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/crypt"
	log "github.com/schollz/logger"
)

const (
	ForwardBufferSize = 32*1024 - 8
	MaxPendingChunks  = 1000
	PendingTimeout    = 30 * time.Second
)

type ForwarderConnID uint64

const (
	ForwardMsgOpen  = 0x01
	ForwardMsgData  = 0x02
	ForwardMsgClose = 0x03
)

type TCPForwarder struct {
	conns           []*comm.Comm
	key             []byte
	isSender        bool
	localServerAddr string

	activeConns  sync.Map
	pendingConns sync.Map   // connID -> *pendingData
	closedConns  sync.Map   // connID -> time.Time (время закрытия)
	pendingMu    sync.Mutex // защита для создания pending данных
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

type pendingData struct {
	chunks [][]byte
	timer  *time.Timer
	connID ForwarderConnID
	f      *TCPForwarder
}

type forwardedConn struct {
	local    net.Conn
	tunnelCh chan []byte
	closeCh  chan struct{}
	once     sync.Once
	connID   ForwarderConnID
	f        *TCPForwarder
}

func NewTCPForwarder(conns []*comm.Comm, isSender bool, sharedKey []byte, localServerAddr string) *TCPForwarder {
	return &TCPForwarder{
		conns:           conns,
		key:             sharedKey,
		isSender:        isSender,
		localServerAddr: localServerAddr,
		stopChan:        make(chan struct{}),
	}
}

func (f *TCPForwarder) Start() error {
	log.Infof("Starting TCP forwarder (sender=%v)", f.isSender)

	// Запускаем горутины для очистки просроченных соединений
	if !f.isSender {
		f.wg.Add(1)
		go f.cleanupPendingConns()
		f.wg.Add(1)
		go f.cleanupClosedConns()
	}

	f.wg.Add(1)
	go f.reader()

	return nil
}

func (f *TCPForwarder) cleanupPendingConns() {
	defer f.wg.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopChan:
			return
		case <-ticker.C:
			f.pendingConns.Range(func(key, value interface{}) bool {
				if pd, ok := value.(*pendingData); ok {
					// Если соединение уже закрыто, сразу удаляем буфер
					if _, closed := f.closedConns.Load(pd.connID); closed {
						pd.timer.Stop()
						f.pendingConns.Delete(key)
						return true
					}

					select {
					case <-pd.timer.C:
						if len(pd.chunks) > 0 {
							log.Errorf("Pending conn %d: timeout, cleaning up (%d chunks lost)", pd.connID, len(pd.chunks))
						}
						f.pendingConns.Delete(key)
					default:
					}
				}
				return true
			})
		}
	}
}

func (f *TCPForwarder) cleanupClosedConns() {
	defer f.wg.Done()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopChan:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute) // удаляем записи старше 10 минут
			deleted := 0
			f.closedConns.Range(func(key, value interface{}) bool {
				if closedTime, ok := value.(time.Time); ok {
					if closedTime.Before(cutoff) {
						f.closedConns.Delete(key)
						deleted++
					}
				}
				return true
			})
			if deleted > 0 {
				// log.Debugf("Cleaned up %d old closed connection entries", deleted)
			}
		}
	}
}

func (f *TCPForwarder) Stop() error {
	log.Info("Stopping TCP forwarder")
	close(f.stopChan)

	f.activeConns.Range(func(_, v interface{}) bool {
		if fc, ok := v.(*forwardedConn); ok {
			fc.close()
		}
		return true
	})

	// Очищаем буфер ожидающих соединений
	f.pendingConns.Range(func(key, value interface{}) bool {
		if pd, ok := value.(*pendingData); ok {
			pd.timer.Stop()
		}
		f.pendingConns.Delete(key)
		return true
	})

	// Очищаем список закрытых соединений
	f.closedConns.Range(func(key, value interface{}) bool {
		f.closedConns.Delete(key)
		return true
	})

	for _, conn := range f.conns {
		if conn != nil {
			conn.Close()
		}
	}

	f.wg.Wait()
	return nil
}

// UpdateLocalServerAddr обновляет адрес локального сервера
// Закрывает все активные TCP соединения и очищает карты
func (f *TCPForwarder) UpdateLocalServerAddr(addr string) {
	log.Infof("TCP forwarder: updating local server address to %s", addr)

	// 1. Закрываем все активные TCP соединения
	f.activeConns.Range(func(_, v interface{}) bool {
		if fc, ok := v.(*forwardedConn); ok {
			fc.close()
		}
		return true
	})

	// 2. Очищаем карту активных соединений
	f.activeConns.Range(func(key, _ interface{}) bool {
		f.activeConns.Delete(key)
		return true
	})

	// 3. Очищаем pending (останавливаем таймеры)
	f.pendingConns.Range(func(key, value interface{}) bool {
		if pd, ok := value.(*pendingData); ok {
			pd.timer.Stop()
		}
		f.pendingConns.Delete(key)
		return true
	})

	// 4. Очищаем closedConns
	f.closedConns.Range(func(key, _ interface{}) bool {
		f.closedConns.Delete(key)
		return true
	})

	// 5. Обновляем адрес
	f.localServerAddr = addr

	log.Infof("TCP forwarder: all connections closed, address updated to %s", addr)
}

// reader читает из обоих каналов в одном select
func (f *TCPForwarder) reader() {
	defer f.wg.Done()
	// log.Debug("Reader started")

	// Создаем каналы для получения сообщений от comm.Receive
	type msg struct {
		data []byte
		err  error
		idx  int
	}

	ch0 := make(chan msg)
	ch1 := make(chan msg)

	// Запускаем горутины для чтения из каждого канала
	go func() {
		for {
			data, err := f.conns[0].Receive()
			select {
			case <-f.stopChan:
				return
			case ch0 <- msg{data: data, err: err, idx: 0}:
			}
		}
	}()

	go func() {
		for {
			data, err := f.conns[1].Receive()
			select {
			case <-f.stopChan:
				return
			case ch1 <- msg{data: data, err: err, idx: 1}:
			}
		}
	}()

	// Единый цикл обработки сообщений
	for {
		select {
		case <-f.stopChan:
			// log.Debug("Reader stopped")
			return

		case m := <-ch0:
			if m.err != nil {
				if !errors.Is(m.err, net.ErrClosed) && !errors.Is(m.err, io.EOF) {
					log.Errorf("Reader ch0 error: %v", m.err)
				}
				return
			}
			f.processMessage(m.data, 0)

		case m := <-ch1:
			if m.err != nil {
				if !errors.Is(m.err, net.ErrClosed) && !errors.Is(m.err, io.EOF) {
					log.Errorf("Reader ch1 error: %v", m.err)
				}
				return
			}
			f.processMessage(m.data, 1)
		}
	}
}

func (f *TCPForwarder) processMessage(encrypted []byte, readerIdx int) {
	decrypted, err := crypt.Decrypt(encrypted, f.key)
	if err != nil {
		log.Errorf("Reader %d: decrypt error (ignored): %v", readerIdx, err)
		return
	}

	if len(decrypted) < 16 {
		log.Errorf("Reader %d: message too short", readerIdx)
		return
	}

	connID := ForwarderConnID(binary.LittleEndian.Uint64(decrypted[0:8]))
	msgType := decrypted[8]
	payloadLen := binary.LittleEndian.Uint32(decrypted[12:16])

	if len(decrypted) < 16+int(payloadLen) {
		// log.Debugf("Reader %d: incomplete message", readerIdx)
		return
	}
	payload := decrypted[16 : 16+int(payloadLen)]

	// log.Debugf("Reader %d: conn=%d type=%d len=%d", readerIdx, connID, msgType, payloadLen)

	if f.isSender {
		f.handleSenderMessage(connID, msgType, payload, readerIdx)
	} else {
		f.handleReceiverMessage(connID, msgType, payload, readerIdx)
	}
}

func (f *TCPForwarder) handleSenderMessage(connID ForwarderConnID, msgType byte, payload []byte, readerIdx int) {
	switch msgType {
	case ForwardMsgOpen:
		log.Infof("OPEN conn=%d from reader=%d", connID, readerIdx)

		local, err := net.Dial("tcp", f.localServerAddr)
		if err != nil {
			log.Errorf("OPEN conn=%d dial error: %v", connID, err)
			f.pendingConns.Delete(connID)
			return
		}
		// log.Debugf("OPEN conn=%d connected to %s", connID, f.localServerAddr)

		fc := &forwardedConn{
			local:    local,
			tunnelCh: make(chan []byte, 100),
			closeCh:  make(chan struct{}),
			connID:   connID,
			f:        f,
		}

		// Проверяем буфер ожидания
		if pendingVal, ok := f.pendingConns.LoadAndDelete(connID); ok {
			if pd, ok := pendingVal.(*pendingData); ok {
				pd.timer.Stop()
				if len(pd.chunks) > 0 {
					// log.Debugf("OPEN conn=%d: found %d pending data chunks", connID, len(pd.chunks))
					for _, data := range pd.chunks {
						select {
						case fc.tunnelCh <- data:
							// log.Debugf("OPEN conn=%d: delivered pending %d bytes", connID, len(data))
						default:
							go func(d []byte) {
								select {
								case fc.tunnelCh <- d:
								case <-fc.closeCh:
								case <-f.stopChan:
								}
							}(data)
						}
					}
				}
			}
		}

		f.activeConns.Store(connID, fc)

		f.wg.Add(2)
		go fc.readLoop()
		go fc.writeLoop()

		// log.Debugf("OPEN conn=%d both loops started", connID)

	case ForwardMsgData:
		// Проверяем активные соединения
		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				select {
				case fc.tunnelCh <- payload:
					// log.Debugf("DATA conn=%d: queued %d bytes", connID, len(payload))
				case <-fc.closeCh:
					// log.Debugf("DATA conn=%d: dropped (closing)", connID)
				case <-f.stopChan:
					// log.Debugf("DATA conn=%d: dropped (stopping)", connID)
				}
				return
			}
		}

		// Проверяем, не было ли соединение уже закрыто
		if _, ok := f.closedConns.Load(connID); ok {
			// log.Debugf("DATA conn=%d: already closed, dropping %d bytes", connID, len(payload))
			return
		}

		// Если активного соединения нет и оно не закрыто - буферизируем
		// log.Debugf("DATA conn=%d: not found, buffering %d bytes (waiting for OPEN)", connID, len(payload))

		pd := f.getOrCreatePending(connID)

		// Добавляем данные в буфер с ограничением
		if len(pd.chunks) < MaxPendingChunks {
			dataCopy := make([]byte, len(payload))
			copy(dataCopy, payload)
			pd.chunks = append(pd.chunks, dataCopy)
			f.pendingConns.Store(connID, pd)

			// Сброс таймера при получении данных
			pd.timer.Reset(PendingTimeout)
		} else {
			log.Errorf("DATA conn=%d: buffer full, dropping packet", connID)
		}

	case ForwardMsgClose:
		// log.Debugf("CLOSE conn=%d from reader=%d", connID, readerIdx)
		f.pendingConns.Delete(connID)

		// Помечаем соединение как закрытое (даже если активного соединения нет)
		f.closedConns.Store(connID, time.Now())

		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				fc.close()
			}
		}
	}
}

func (f *TCPForwarder) handleReceiverMessage(connID ForwarderConnID, msgType byte, payload []byte, readerIdx int) {
	switch msgType {
	case ForwardMsgData:
		// Проверяем активные соединения
		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				select {
				case fc.tunnelCh <- payload:
					// log.Debugf("DATA conn=%d: queued %d bytes", connID, len(payload))
				case <-fc.closeCh:
					// log.Debugf("DATA conn=%d: dropped (closing)", connID)
				case <-f.stopChan:
					// log.Debugf("DATA conn=%d: dropped (stopping)", connID)
				}
				return
			}
		}

		// Проверяем, не было ли соединение уже закрыто
		if _, ok := f.closedConns.Load(connID); ok {
			// log.Debugf("DATA conn=%d: already closed, dropping %d bytes", connID, len(payload))
			return
		}

		// Если активного соединения нет и оно не закрыто - буферизируем
		// log.Debugf("DATA conn=%d: not found, buffering %d bytes", connID, len(payload))

		pd := f.getOrCreatePending(connID)

		// Добавляем данные в буфер с ограничением
		if len(pd.chunks) < MaxPendingChunks {
			dataCopy := make([]byte, len(payload))
			copy(dataCopy, payload)
			pd.chunks = append(pd.chunks, dataCopy)
			// log.Debugf("DATA conn=%d: buffered, total %d chunks", connID, len(pd.chunks))

			// Сброс таймера при получении данных
			pd.timer.Reset(PendingTimeout)
		} else {
			log.Errorf("DATA conn=%d: buffer full, dropping packet", connID)
		}

	case ForwardMsgClose:
		log.Infof("CLOSE conn=%d from reader=%d", connID, readerIdx)

		// Помечаем соединение как закрытое (даже если активного соединения нет)
		f.closedConns.Store(connID, time.Now())

		// Очищаем буфер ожидания
		if pendingVal, ok := f.pendingConns.Load(connID); ok {
			if pd, ok := pendingVal.(*pendingData); ok {
				pd.timer.Stop()
			}
			f.pendingConns.Delete(connID)
		}

		// Закрываем активное соединение
		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				fc.close()
			}
		}
	}
}

func (f *TCPForwarder) ForwardConnection(local net.Conn) error {
	if f.isSender {
		return errors.New("ForwardConnection only for receiver")
	}

	connID := f.generateID()
	log.Infof("Forwarding new connection %d (%s -> %s)", connID, local.LocalAddr(), local.RemoteAddr())

	fc := &forwardedConn{
		local:    local,
		tunnelCh: make(chan []byte, 100),
		closeCh:  make(chan struct{}),
		connID:   connID,
		f:        f,
	}

	// Проверяем, не было ли уже данных в буфере ожидания
	if pendingVal, ok := f.pendingConns.LoadAndDelete(connID); ok {
		if pd, ok := pendingVal.(*pendingData); ok {
			pd.timer.Stop()
			if len(pd.chunks) > 0 {
				// log.Debugf("Conn %d: found %d pending data chunks", connID, len(pd.chunks))
				// Отправляем все накопленные данные в канал
				for _, data := range pd.chunks {
					select {
					case fc.tunnelCh <- data:
						log.Tracef("Conn %d: delivered pending %d bytes", connID, len(data))
					default:
						// Асинхронная отправка если канал заполнен
						go func(d []byte) {
							select {
							case fc.tunnelCh <- d:
							case <-fc.closeCh:
								// log.Debugf("Conn %d: dropped pending data (closing)", connID)
							case <-f.stopChan:
								// log.Debugf("Conn %d: dropped pending data (stopping)", connID)
							}
						}(data)
					}
				}
			}
		}
	}

	f.activeConns.Store(connID, fc)

	// log.Debugf("Conn %d: sending OPEN", connID)
	if err := f.sendMessage(connID, ForwardMsgOpen, nil); err != nil {
		log.Errorf("Conn %d: OPEN failed: %v", connID, err)
		f.activeConns.Delete(connID)
		local.Close()
		return err
	}

	f.wg.Add(2)
	go fc.readLoop()
	go fc.writeLoop()

	// log.Debugf("Conn %d: read/write loops started", connID)
	return nil
}

func (fc *forwardedConn) readLoop() {
	defer fc.f.wg.Done()
	defer fc.close()

	buf := make([]byte, ForwardBufferSize)
	// log.Debugf("Conn %d readLoop started", fc.connID)

	for {
		select {
		case <-fc.closeCh:
			// log.Debugf("Conn %d readLoop: closed", fc.connID)
			return
		case <-fc.f.stopChan:
			// log.Debugf("Conn %d readLoop: stopped", fc.connID)
			return
		default:
			n, err := fc.local.Read(buf)
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					log.Errorf("Conn %d read error: %v", fc.connID, err)
					// } else {
					// 	log.Debugf("Conn %d readLoop: local closed (%v)", fc.connID, err)
				}
				return
			}

			if n > 0 {
				if err := fc.f.sendMessage(fc.connID, ForwardMsgData, buf[:n]); err != nil {
					log.Errorf("Conn %d: send error: %v", fc.connID, err)
					return
				}
				// log.Debugf("Conn %d: read %d bytes from local", fc.connID, n)
			}
		}
	}
}

func (fc *forwardedConn) writeLoop() {
	defer fc.f.wg.Done()
	defer fc.close()

	// log.Debugf("Conn %d writeLoop started", fc.connID)

	for {
		select {
		case <-fc.closeCh:
			// log.Debugf("Conn %d writeLoop: closed", fc.connID)
			return
		case <-fc.f.stopChan:
			// log.Debugf("Conn %d writeLoop: stopped", fc.connID)
			return
		case data, ok := <-fc.tunnelCh:
			if !ok {
				// log.Debugf("Conn %d writeLoop: channel closed", fc.connID)
				return
			}

			// log.Debugf("Conn %d: writing %d bytes to local", fc.connID, len(data))
			if _, err := fc.local.Write(data); err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Errorf("Conn %d write error: %v", fc.connID, err)
				} else {
					// log.Debugf("Conn %d writeLoop: local closed", fc.connID)
				}
				return
			}
		}
	}
}

func (fc *forwardedConn) close() {
	fc.once.Do(func() {
		// log.Debugf("Closing conn %d", fc.connID)
		close(fc.closeCh)
		fc.local.Close()
		fc.f.activeConns.Delete(fc.connID)

		// Помечаем соединение как закрытое с временной меткой
		fc.f.closedConns.Store(fc.connID, time.Now())

		// Очищаем pending данные если есть
		if pendingVal, ok := fc.f.pendingConns.Load(fc.connID); ok {
			if pd, ok := pendingVal.(*pendingData); ok {
				pd.timer.Stop()
			}
			fc.f.pendingConns.Delete(fc.connID)
		}

		go fc.f.sendMessage(fc.connID, ForwardMsgClose, nil)
	})
}

func (f *TCPForwarder) sendMessage(connID ForwarderConnID, msgType byte, payload []byte) error {
	packet := make([]byte, 16+len(payload))

	binary.LittleEndian.PutUint64(packet[0:8], uint64(connID))
	packet[8] = msgType
	binary.LittleEndian.PutUint32(packet[12:16], uint32(len(payload)))
	copy(packet[16:], payload)

	// log.Debugf("Send: conn=%d type=%d len=%d", connID, msgType, len(payload))

	encrypted, err := crypt.Encrypt(packet, f.key)
	if err != nil {
		return err
	}

	idx := 0
	if msgType == ForwardMsgData {
		idx = 1
	}

	return f.conns[idx].Send(encrypted)
}

func (f *TCPForwarder) generateID0() ForwarderConnID {
	var b [8]byte
	rand.Read(b[:])
	return ForwarderConnID(binary.LittleEndian.Uint64(b[:]))
}

func (f *TCPForwarder) generateID() ForwarderConnID {
	// Используем nanosecond timestamp как основу
	timestamp := uint64(time.Now().UnixNano())

	// Добавляем немного случайности для уникальности в пределах одной наносекунды
	var b [4]byte
	rand.Read(b[:])
	random := binary.LittleEndian.Uint32(b[:])

	// Комбинируем: старшие 32 бита - время, младшие 32 бита - случайность
	id := (timestamp << 32) | uint64(random)

	return ForwarderConnID(id)
}

func (f *TCPForwarder) getOrCreatePending(connID ForwarderConnID) *pendingData {
	// Первая проверка без блокировки
	if pendingVal, ok := f.pendingConns.Load(connID); ok {
		return pendingVal.(*pendingData)
	}

	// Блокировка для избежания двойного создания
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()

	// Проверяем еще раз после блокировки (double-checked locking)
	if pendingVal, ok := f.pendingConns.Load(connID); ok {
		return pendingVal.(*pendingData)
	}

	pd := &pendingData{
		chunks: make([][]byte, 0, 10),
		connID: connID,
		f:      f,
	}
	pd.timer = time.AfterFunc(PendingTimeout, func() {
		if len(pd.chunks) > 0 {
			log.Errorf("Pending conn %d: timeout, cleaning up (%d chunks lost)", connID, len(pd.chunks))
		}
		f.pendingConns.Delete(connID)
	})
	f.pendingConns.Store(connID, pd)
	return pd
}
