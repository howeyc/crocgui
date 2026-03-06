// tcp_forwarder.go
package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/crypt"
	log "github.com/schollz/logger"
)

const (
	ForwardBufferSize = 32*1024 - 8
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

	activeConns sync.Map
	stopChan    chan struct{}
	wg          sync.WaitGroup
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

	// Единый ридер для обоих каналов
	f.wg.Add(1)
	go f.reader()

	return nil
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

	for _, conn := range f.conns {
		if conn != nil {
			conn.Close()
		}
	}

	f.wg.Wait()
	return nil
}

// reader читает из обоих каналов в одном select
func (f *TCPForwarder) reader() {
	defer f.wg.Done()
	log.Debug("Reader started")

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
			log.Debug("Reader stopped")
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
		log.Debugf("Reader %d: decrypt error (ignored): %v", readerIdx, err)
		return
	}

	if len(decrypted) < 16 {
		log.Debugf("Reader %d: message too short", readerIdx)
		return
	}

	connID := ForwarderConnID(binary.LittleEndian.Uint64(decrypted[0:8]))
	msgType := decrypted[8]
	payloadLen := binary.LittleEndian.Uint32(decrypted[12:16])

	if len(decrypted) < 16+int(payloadLen) {
		log.Debugf("Reader %d: incomplete message", readerIdx)
		return
	}
	payload := decrypted[16 : 16+int(payloadLen)]

	log.Debugf("Reader %d: conn=%d type=%d len=%d", readerIdx, connID, msgType, payloadLen)

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
			return
		}
		log.Debugf("OPEN conn=%d connected to %s", connID, f.localServerAddr)

		fc := &forwardedConn{
			local:    local,
			tunnelCh: make(chan []byte, 10),
			closeCh:  make(chan struct{}),
			connID:   connID,
			f:        f,
		}
		f.activeConns.Store(connID, fc)

		f.wg.Add(2)
		go fc.readLoop()
		go fc.writeLoop()

		log.Debugf("OPEN conn=%d both loops started", connID)

	case ForwardMsgData:
		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				select {
				case fc.tunnelCh <- payload:
					log.Debugf("DATA conn=%d: queued %d bytes", connID, len(payload))
				case <-fc.closeCh:
					log.Debugf("DATA conn=%d: dropped (closing)", connID)
				case <-f.stopChan:
					log.Debugf("DATA conn=%d: dropped (stopping)", connID)
				}
			}
		} else {
			log.Debugf("DATA conn=%d: not found (waiting for OPEN?)", connID)
		}

	case ForwardMsgClose:
		log.Infof("CLOSE conn=%d from reader=%d", connID, readerIdx)
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
		if val, ok := f.activeConns.Load(connID); ok {
			if fc, ok := val.(*forwardedConn); ok {
				select {
				case fc.tunnelCh <- payload:
					log.Debugf("DATA conn=%d: queued %d bytes", connID, len(payload))
				case <-fc.closeCh:
					log.Debugf("DATA conn=%d: dropped (closing)", connID)
				case <-f.stopChan:
					log.Debugf("DATA conn=%d: dropped (stopping)", connID)
				}
			}
		} else {
			log.Debugf("DATA conn=%d: not found", connID)
		}

	case ForwardMsgClose:
		log.Infof("CLOSE conn=%d from reader=%d", connID, readerIdx)
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
		tunnelCh: make(chan []byte, 10),
		closeCh:  make(chan struct{}),
		connID:   connID,
		f:        f,
	}
	f.activeConns.Store(connID, fc)

	log.Debugf("Conn %d: sending OPEN", connID)
	if err := f.sendMessage(connID, ForwardMsgOpen, nil); err != nil {
		log.Errorf("Conn %d: OPEN failed: %v", connID, err)
		f.activeConns.Delete(connID)
		local.Close()
		return err
	}

	f.wg.Add(2)
	go fc.readLoop()
	go fc.writeLoop()

	log.Debugf("Conn %d: read/write loops started", connID)
	return nil
}

func (fc *forwardedConn) readLoop() {
	defer fc.f.wg.Done()
	defer fc.close()

	buf := make([]byte, ForwardBufferSize)
	log.Debugf("Conn %d readLoop started", fc.connID)

	for {
		select {
		case <-fc.closeCh:
			log.Debugf("Conn %d readLoop: closed", fc.connID)
			return
		case <-fc.f.stopChan:
			log.Debugf("Conn %d readLoop: stopped", fc.connID)
			return
		default:
		}

		n, err := fc.local.Read(buf)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				log.Errorf("Conn %d read error: %v", fc.connID, err)
			} else {
				log.Debugf("Conn %d readLoop: local closed (%v)", fc.connID, err)
			}
			return
		}

		if n > 0 {
			log.Debugf("Conn %d: read %d bytes from local", fc.connID, n)
			if err := fc.f.sendMessage(fc.connID, ForwardMsgData, buf[:n]); err != nil {
				log.Errorf("Conn %d: send error: %v", fc.connID, err)
				return
			}
		}
	}
}

func (fc *forwardedConn) writeLoop() {
	defer fc.f.wg.Done()
	defer fc.close()

	log.Debugf("Conn %d writeLoop started", fc.connID)

	for {
		select {
		case <-fc.closeCh:
			log.Debugf("Conn %d writeLoop: closed", fc.connID)
			return
		case <-fc.f.stopChan:
			log.Debugf("Conn %d writeLoop: stopped", fc.connID)
			return
		case data, ok := <-fc.tunnelCh:
			if !ok {
				log.Debugf("Conn %d writeLoop: channel closed", fc.connID)
				return
			}

			log.Debugf("Conn %d: writing %d bytes to local", fc.connID, len(data))
			if _, err := fc.local.Write(data); err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Errorf("Conn %d write error: %v", fc.connID, err)
				} else {
					log.Debugf("Conn %d writeLoop: local closed", fc.connID)
				}
				return
			}
		}
	}
}

func (fc *forwardedConn) close() {
	fc.once.Do(func() {
		log.Infof("Closing conn %d", fc.connID)
		close(fc.closeCh)
		fc.local.Close()
		fc.f.activeConns.Delete(fc.connID)
		go fc.f.sendMessage(fc.connID, ForwardMsgClose, nil)
	})
}

func (f *TCPForwarder) sendMessage(connID ForwarderConnID, msgType byte, payload []byte) error {
	packet := make([]byte, 16+len(payload))

	binary.LittleEndian.PutUint64(packet[0:8], uint64(connID))
	packet[8] = msgType
	binary.LittleEndian.PutUint32(packet[12:16], uint32(len(payload)))
	copy(packet[16:], payload)

	log.Debugf("Send: conn=%d type=%d len=%d", connID, msgType, len(payload))

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

func (f *TCPForwarder) generateID() ForwarderConnID {
	var b [8]byte
	rand.Read(b[:])
	return ForwarderConnID(binary.LittleEndian.Uint64(b[:]))
}
