// videocall.go
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/schollz/logger"
)

//go:embed videocall.html
var videocallHTML []byte

// VideoChunk представляет один WebM-чанк видео+аудио
type VideoChunk struct {
	Index int64  `json:"index"`
	Data  []byte `json:"data"`
}

// recEntry — одна активная серверная запись
type recEntry struct {
	file       *os.File
	recordPeer string // кого записываем (recordPeerID от клиента)
	sourcePeer string // кто реально шлёт чанки (для writeChunk/clearRecPending)
	pending    bool   // ожидаем initCodec от sourcePeer
}

// VideoCallRoom представляет комнату видеозвонка между двумя участниками
type VideoCallRoom struct {
	ID        string
	CreatedAt time.Time

	// Чанки от каждого участника (peer ID → ring buffer)
	Chunks    map[string][]VideoChunk
	ChunkIdx  map[string]int64
	ChunkLock map[string]*sync.Mutex

	// Для уведомления о входящем звонке
	WaitingChan chan struct{} // закрывается когда второй участник подключился

	// WebSocket соединения для мгновенной доставки чанков
	wsConns map[string]*wsConn // peerID -> ws connection (с мьютексом на запись)
	wsMu    sync.Mutex

	// Peer settings (peerID → JSON-decoded settings map)
	peerSettings   map[string]map[string]interface{}
	peerSettingsCh map[string]chan struct{} // peerID → сигнал "settings получены"
	peerSettingsMu sync.Mutex

	// Серверная запись: fileName → запись
	recEntries map[string]*recEntry // fileName → активная запись
	recMu      sync.Mutex           // защита состояния записи

	mu sync.Mutex
}

// VideoCallStorage хранит активные комнаты видеозвонков
type VideoCallStorage struct {
	rooms      map[string]*VideoCallRoom
	mu         sync.RWMutex
	webdavRoot string // Корневой каталог WebDAV для записи файлов
}

var callStore = &VideoCallStorage{
	rooms: make(map[string]*VideoCallRoom),
}

// WebSocket upgrader
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1 << 20, // 1 MB
	WriteBufferSize: 1 << 20, // 1 MB
	CheckOrigin: func(r *http.Request) bool {
		return isLocalRequest(r)
	},
}

// wsConn оборачивает WebSocket соединение с мьютексом для безопасной записи
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

const (
	maxChunksPerPeer = 300     // ~15 секунд при 20 чанков/сек (CHUNK_INTERVAL_MS=50)
	maxChunkSize     = 1 << 20 // 1 MB максимальный размер чанка
	wsHistoryLimit   = 40      // Максимум чанков истории при WS подключении (~2 сек при 20 чанков/сек)
)

// createRoom создаёт новую комнату видеозвонка или возвращает ошибку если комната уже существует
func (vs *VideoCallStorage) createRoom(roomID string) (*VideoCallRoom, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if _, exists := vs.rooms[roomID]; exists {
		return nil, fmt.Errorf("room already exists")
	}

	room := &VideoCallRoom{
		ID:             roomID,
		CreatedAt:      time.Now(),
		Chunks:         make(map[string][]VideoChunk),
		ChunkIdx:       make(map[string]int64),
		ChunkLock:      make(map[string]*sync.Mutex),
		WaitingChan:    make(chan struct{}),
		wsConns:        make(map[string]*wsConn),
		peerSettings:   make(map[string]map[string]interface{}),
		peerSettingsCh: make(map[string]chan struct{}),
		recEntries:     make(map[string]*recEntry),
	}
	vs.rooms[roomID] = room
	return room, nil
}

// SetWebDAVRoot устанавливает корневой каталог WebDAV для записи файлов
func (vs *VideoCallStorage) SetWebDAVRoot(root string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.webdavRoot = root
}

// getRoom возвращает комнату по ID
func (vs *VideoCallStorage) getRoom(roomID string) *VideoCallRoom {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.rooms[roomID]
}

// deleteRoom удаляет комнату
func (vs *VideoCallStorage) deleteRoom(roomID string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	delete(vs.rooms, roomID)
}

// addChunk добавляет чанк от участника
func (r *VideoCallRoom) addChunk(peerID string, data []byte) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Инициализируем структуры для нового пира
	if _, ok := r.ChunkLock[peerID]; !ok {
		r.ChunkLock[peerID] = &sync.Mutex{}
		r.Chunks[peerID] = make([]VideoChunk, 0, maxChunksPerPeer)
		r.ChunkIdx[peerID] = 0
	}

	idx := r.ChunkIdx[peerID]
	r.ChunkIdx[peerID]++

	chunk := VideoChunk{
		Index: idx,
		Data:  data,
	}

	r.Chunks[peerID] = append(r.Chunks[peerID], chunk)

	// Ограничиваем количество хранимых чанков — копируем в новый slice
	// чтобы освободить старые Data для GC
	if len(r.Chunks[peerID]) > maxChunksPerPeer {
		excess := len(r.Chunks[peerID]) - maxChunksPerPeer
		trimmed := make([]VideoChunk, maxChunksPerPeer)
		copy(trimmed, r.Chunks[peerID][excess:])
		r.Chunks[peerID] = trimmed
	}

	return idx
}

// getChunksAfter возвращает чанки от указанного пира с индексом > afterIndex
// Использует бинарный поиск — чанки упорядочены по Index
func (r *VideoCallRoom) getChunksAfter(peerID string, afterIndex int64) []VideoChunk {
	r.mu.Lock()
	defer r.mu.Unlock()

	chunks, ok := r.Chunks[peerID]
	if !ok || len(chunks) == 0 {
		return nil
	}

	// Бинарный поиск первого чанка с Index > afterIndex
	n := len(chunks)
	i := sort.Search(n, func(j int) bool {
		return chunks[j].Index > afterIndex
	})
	if i >= n {
		return nil
	}

	// Возвращаем слайс без копирования
	return chunks[i:]
}

// getLastIndex возвращает последний индекс чанка от пира
func (r *VideoCallRoom) getLastIndex(peerID string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if idx, ok := r.ChunkIdx[peerID]; ok {
		return idx - 1
	}
	return -1
}

// clearAllChunks очищает все чанки и индексы в комнате (вызывается при WS reconnect после reload)
func (r *VideoCallRoom) clearAllChunks() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for peerID := range r.Chunks {
		r.Chunks[peerID] = nil
		delete(r.ChunkIdx, peerID)
	}
}

// startRecording создаёт файл для записи чанков указанного пира
func (r *VideoCallRoom) startRecording(recordPeerID, sourcePeerID, codec string) error {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	// Если уже есть запись этого пира — закрываем предыдущую
	for fn, e := range r.recEntries {
		if e.recordPeer == recordPeerID {
			e.file.Close()
			delete(r.recEntries, fn)
		}
	}

	// Определяем расширение файла по кодеку
	ext := ".webm"
	if codec != "" && strings.Contains(strings.ToLower(codec), "mp4") {
		ext = ".mp4"
	}

	// Генерируем уникальное имя файла: YYYYMMDD_HHMMSS_mmm.<ext>
	now := time.Now()
	fileName := now.Format("20060102_150405") + fmt.Sprintf("_%03d", now.Nanosecond()/1e6) + ext

	// Получаем корневой каталог WebDAV
	callStore.mu.RLock()
	root := callStore.webdavRoot
	callStore.mu.RUnlock()

	if root == "" {
		return fmt.Errorf("webdav root not set")
	}

	// Создаём файл
	filePath := filepath.Join(root, fileName)
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create recording file %s: %w", filePath, err)
	}

	r.recEntries[fileName] = &recEntry{
		file:       f,
		recordPeer: recordPeerID,
		sourcePeer: sourcePeerID,
		pending:    true, // ожидаем initCodec от sourcePeer перед записью чанков
	}

	log.Debugf("Recording started: room=%s recordPeer=%s sourcePeer=%s file=%s", r.ID, recordPeerID, sourcePeerID, fileName)
	return nil
}

// stopRecording закрывает файл записи для указанного пира, возвращает имя файла
func (r *VideoCallRoom) stopRecording(recordPeerID string) (string, error) {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	for fn, e := range r.recEntries {
		if e.recordPeer == recordPeerID {
			err := e.file.Close()
			delete(r.recEntries, fn)
			if err != nil {
				log.Debugf("Recording close error: room=%s peer=%s file=%s err=%v", r.ID, recordPeerID, fn, err)
				return "", err
			}
			log.Debugf("Recording stopped: room=%s peer=%s file=%s", r.ID, recordPeerID, fn)
			return fn, nil
		}
	}
	return "", fmt.Errorf("no recording for peer %s", recordPeerID)
}

// writeChunk записывает чанк в файлы где sourcePeer совпадает с отправителем
func (r *VideoCallRoom) writeChunk(sourcePeerID string, data []byte) {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	for _, e := range r.recEntries {
		if e.sourcePeer == sourcePeerID && !e.pending && e.file != nil {
			if _, err := e.file.Write(data); err != nil {
				log.Debugf("Recording write error: room=%s file err=%v", r.ID, err)
			}
		}
	}
}

// clearRecPending снимает флаг ожидания initCodec — сервер начнёт писать чанки
func (r *VideoCallRoom) clearRecPending(sourcePeerID string) {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	for fn, e := range r.recEntries {
		if e.sourcePeer == sourcePeerID && e.pending {
			e.pending = false
			log.Debugf("Recording pending cleared: room=%s sourcePeer=%s file=%s", r.ID, sourcePeerID, fn)
		}
	}
}

// closeAllRecordings закрывает все открытые файлы записи в комнате
func (r *VideoCallRoom) closeAllRecordings() {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	for fn, e := range r.recEntries {
		if err := e.file.Close(); err != nil {
			log.Debugf("Recording close error on cleanup: room=%s file=%s err=%v", r.ID, fn, err)
		} else {
			log.Debugf("Recording closed on cleanup: room=%s file=%s", r.ID, fn)
		}
	}
	r.recEntries = make(map[string]*recEntry)
}

// closePeerRecordings закрывает файлы записи связанные с указанным пиром
func (r *VideoCallRoom) closePeerRecordings(peerID string) {
	r.recMu.Lock()
	defer r.recMu.Unlock()

	for fn, e := range r.recEntries {
		if e.sourcePeer == peerID || e.recordPeer == peerID {
			if err := e.file.Close(); err != nil {
				log.Debugf("Recording close error on peer disconnect: room=%s peer=%s file=%s err=%v", r.ID, peerID, fn, err)
			} else {
				log.Debugf("Recording closed on peer disconnect: room=%s peer=%s file=%s", r.ID, peerID, fn)
			}
			delete(r.recEntries, fn)
		}
	}
}

// notifyPeerJoined уведомляет инициатора что второй участник подключился
func (r *VideoCallRoom) notifyPeerJoined() {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.WaitingChan:
		// Уже закрыт
	default:
		close(r.WaitingChan)
	}
}

// storePeerSettings сохраняет настройки пира и сигналит ожидающим
func (r *VideoCallRoom) storePeerSettings(peerID string, settings map[string]interface{}) {
	r.peerSettingsMu.Lock()
	defer r.peerSettingsMu.Unlock()
	r.peerSettings[peerID] = settings
	ch, ok := r.peerSettingsCh[peerID]
	if !ok {
		ch = make(chan struct{}, 1)
		r.peerSettingsCh[peerID] = ch
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

// waitForPeerSettings ждёт настройки от пира с таймаутом
func (r *VideoCallRoom) waitForPeerSettings(peerID string, timeout time.Duration) map[string]interface{} {
	r.peerSettingsMu.Lock()
	if s, ok := r.peerSettings[peerID]; ok {
		r.peerSettingsMu.Unlock()
		return s
	}
	ch, ok := r.peerSettingsCh[peerID]
	if !ok {
		ch = make(chan struct{}, 1)
		r.peerSettingsCh[peerID] = ch
	}
	r.peerSettingsMu.Unlock()

	select {
	case <-ch:
		r.peerSettingsMu.Lock()
		defer r.peerSettingsMu.Unlock()
		return r.peerSettings[peerID]
	case <-time.After(timeout):
		log.Debugf("waitForPeerSettings timeout for %s", peerID)
		return nil
	}
}

// getPeerSettings возвращает сохранённые настройки пира (без ожидания)
func (r *VideoCallRoom) getPeerSettings(peerID string) map[string]interface{} {
	r.peerSettingsMu.Lock()
	defer r.peerSettingsMu.Unlock()
	return r.peerSettings[peerID]
}

// setWS сохраняет WebSocket соединение пира
func (r *VideoCallRoom) setWS(peerID string, conn *websocket.Conn) {
	r.wsMu.Lock()
	defer r.wsMu.Unlock()
	// Закрываем предыдущее соединение если есть
	if old, ok := r.wsConns[peerID]; ok {
		old.conn.Close()
	}
	r.wsConns[peerID] = &wsConn{conn: conn}
}

// getWS возвращает WebSocket соединение пира
func (r *VideoCallRoom) getWS(peerID string) *wsConn {
	r.wsMu.Lock()
	defer r.wsMu.Unlock()
	return r.wsConns[peerID]
}

// removeWS удаляет WebSocket соединение пира
func (r *VideoCallRoom) removeWS(peerID string) {
	r.wsMu.Lock()
	defer r.wsMu.Unlock()
	delete(r.wsConns, peerID)
}

// forwardChunkToWS отправляет чанк пиру через WebSocket (thread-safe)
func (r *VideoCallRoom) forwardChunkToWS(peerID string, data []byte) {
	r.wsMu.Lock()
	wsc := r.wsConns[peerID]
	r.wsMu.Unlock()

	if wsc == nil {
		return
	}

	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	if err := wsc.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		log.Debugf("WS forward error to %s: %v", peerID, err)
	}
}

// forwardTextToWS отправляет текстовое сообщение пиру через WebSocket (thread-safe)
func (r *VideoCallRoom) forwardTextToWS(peerID string, msg string) {
	r.wsMu.Lock()
	wsc := r.wsConns[peerID]
	r.wsMu.Unlock()

	if wsc == nil {
		return
	}

	wsc.mu.Lock()
	defer wsc.mu.Unlock()
	if err := wsc.conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		log.Debugf("WS text forward error to %s: %v", peerID, err)
	}
}

// handleCallCreate обрабатывает POST /api/call/create — создаёт комнату
func handleCallCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Room == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room, err := callStore.createRoom(req.Room)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "already_exists",
			"room":   req.Room,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "created",
		"room":      room.ID,
		"createdAt": room.CreatedAt,
	})
}

// handleCallWait обрабатывает GET /api/call/wait — ожидание второго участника
func handleCallWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room := callStore.getRoom(roomID)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	select {
	case <-room.WaitingChan:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "joined",
		})
	case <-r.Context().Done():
		return
	case <-appCtx.Done():
		return
	}
}

// handleCallJoin обрабатывает POST /api/call/join — второй участник подключается
func handleCallJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room string `json:"room"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Room == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room := callStore.getRoom(req.Room)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Уведомляем инициатора
	room.notifyPeerJoined()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "joined",
		"room":   room.ID,
	})
}

// getRemotePeer возвращает ID другого пира в комнате
func getRemotePeer(room *VideoCallRoom, myPeerID string) string {
	room.mu.Lock()
	defer room.mu.Unlock()
	for peerID := range room.ChunkIdx {
		if peerID != myPeerID {
			return peerID
		}
	}
	// Если только один пир, пробуем по имени
	if myPeerID == "host" {
		return "guest"
	}
	return "host"
}

// handleCallWS обрабатывает WebSocket соединение для мгновенной доставки чанков
func handleCallWS(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	peerID := r.URL.Query().Get("peer")

	if roomID == "" || peerID == "" {
		http.Error(w, "room and peer are required", http.StatusBadRequest)
		return
	}

	room := callStore.getRoom(roomID)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Очищаем историю чанков — при reconnect после reload старые чанки не нужны
	room.clearAllChunks()

	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Создаём wsConn с мьютексом для безопасной записи
	wsc := &wsConn{conn: conn}

	// Сохраняем соединение
	room.wsMu.Lock()
	if old, ok := room.wsConns[peerID]; ok {
		old.conn.Close()
	}
	room.wsConns[peerID] = wsc
	room.wsMu.Unlock()
	defer room.removeWS(peerID)

	log.Debugf("WS connected: room=%s peer=%s", roomID, peerID)

	// Определяем remote peer
	remotePeer := getRemotePeer(room, peerID)

	// Отправляем последние чанки (ограничено wsHistoryLimit) при подключении
	lastIdx := room.getLastIndex(remotePeer)
	if lastIdx >= 0 {
		chunks := room.getChunksAfter(remotePeer, -1)
		// Ограничиваем историю до последних wsHistoryLimit чанков
		start := 0
		if len(chunks) > wsHistoryLimit {
			start = len(chunks) - wsHistoryLimit
		}
		history := chunks[start:]
		wsc.mu.Lock()
		for _, chunk := range history {
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk.Data); err != nil {
				wsc.mu.Unlock()
				log.Debugf("WS send history error: %v", err)
				return
			}
		}
		wsc.mu.Unlock()
		log.Debugf("WS sent %d/%d history chunks to %s", len(history), len(chunks), peerID)
	}

	// Ping goroutine для поддержания WS alive
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		defer conn.Close()
		for {
			select {
			case <-ticker.C:
				wsc.mu.Lock()
				err := conn.WriteMessage(websocket.PingMessage, nil)
				wsc.mu.Unlock()
				if err != nil {
					return
				}
			case <-done:
				return
			case <-appCtx.Done():
				return
			}
		}
	}()
	defer close(done)

	conn.SetPingHandler(func(appData string) error {
		wsc.mu.Lock()
		defer wsc.mu.Unlock()
		return conn.WriteMessage(websocket.PongMessage, nil)
	})

	// Читаем входящие чанки и перенаправляем их remote peer
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debugf("WS read error room=%s peer=%s: %v", roomID, peerID, err)
			}
			break
		}

		if msgType == websocket.BinaryMessage && len(data) > 0 {
			// Сохраняем чанк в памяти (для reconnect/history)
			room.addChunk(peerID, data)

			// Записываем чанк в файл если активна серверная запись
			room.writeChunk(peerID, data)

			// Перенаправляем remote peer через WS, или echo обратно (loopback preview)
			if remotePeer != "" {
				if room.getWS(remotePeer) != nil {
					room.forwardChunkToWS(remotePeer, data)
				} else {
					room.forwardChunkToWS(peerID, data)
				}
			}
		} else if msgType == websocket.TextMessage && len(data) > 0 {
			dataStr := string(data)
			// Перехватываем JSON команды
			var msg map[string]interface{}
			json.Unmarshal(data, &msg)
			if msg != nil {
				cmd, _ := msg["cmd"].(string)
				switch cmd {
				case "settings":
					room.storePeerSettings(peerID, msg)
				case "initCodec":
					// Снимаем pending — сервер начнёт писать чанки от этого пира
					room.clearRecPending(peerID)
				case "startRecording":
					// Серверная команда: начать запись чанков указанного пира в файл
					recordPeerID, _ := msg["recordPeerID"].(string)
					codec, _ := msg["codec"].(string)
					if recordPeerID != "" {
						// Определяем sourcePeerID: кто реально будет слать чанки
						// Если у recordPeerID есть активный WS — это реальный пир
						// Если нет (loopback) — чанки придут от запрашивающего пира
						sourcePeerID := recordPeerID
						if room.getWS(recordPeerID) == nil {
							sourcePeerID = peerID
						}
						if err := room.startRecording(recordPeerID, sourcePeerID, codec); err != nil {
							log.Debugf("startRecording error: %v", err)
						}
					}
					// НЕ пересылаем remote peer — это серверная команда
					continue
				case "stopRecording":
					// Серверная команда: остановить запись и закрыть файл
					recordPeerID, _ := msg["recordPeerID"].(string)
					if recordPeerID != "" {
						if fileName, err := room.stopRecording(recordPeerID); err != nil {
							log.Debugf("stopRecording error: %v", err)
						} else if fileName != "" {
							chatUserId, _ := msg["chatUserId"].(string)
							if chatUserId == "" {
								chatUserId = peerID
							}
							chatMsg := chatStore.addMessage("📹 /"+fileName, chatUserId)
							broadcastChatMessage(chatMsg)
							// Ремукс файла в фоне: дописать индекс (WebM) или moov в начало (MP4)
							callStore.mu.RLock()
							fixRoot := callStore.webdavRoot
							callStore.mu.RUnlock()
							go fixRecordingFile(fixRoot, fileName)
						}
					}
					// НЕ пересылаем remote peer — это серверная команда
					continue
				case "chatMessage":
					// Клиент просит отправить сообщение в чат (скриншот и т.д.)
					chatText, _ := msg["text"].(string)
					if chatText != "" {
						sender, _ := msg["sender"].(string)
						if sender == "" {
							sender = peerID
						}
						chatMsg := chatStore.addMessage(chatText, sender)
						broadcastChatMessage(chatMsg)
					}
					continue
				}
			}
			// Текстовые сообщения (settings, restart_recorder, initCodec) — перенаправляем remote peer или echo
			if remotePeer != "" {
				if room.getWS(remotePeer) != nil {
					room.forwardTextToWS(remotePeer, dataStr)
				} else {
					room.forwardTextToWS(peerID, dataStr)
				}
			}
		}
	}

	// Закрываем файлы записи для этого пира при отключении
	room.closePeerRecordings(peerID)

	// Отправляем remote peer уведомление об отключении
	if remotePeer != "" {
		room.forwardTextToWS(remotePeer, "peer_left")
	}

	log.Debugf("WS disconnected: room=%s peer=%s", roomID, peerID)
}

// handleCallEnd обрабатывает DELETE /api/call/room — завершение звонка
func handleCallEnd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	// Уведомляем все пиры об отключении и закрываем WS соединения перед удалением комнаты
	room := callStore.getRoom(roomID)
	if room != nil {
		room.wsMu.Lock()
		// 1. СНАЧАЛА отправляем peer_left всем пирам
		for _, wsc := range room.wsConns {
			if wsc != nil {
				wsc.mu.Lock()
				wsc.conn.WriteMessage(websocket.TextMessage, []byte("peer_left"))
				wsc.mu.Unlock()
			}
		}
		// 2. ПОТОМ закрываем соединения
		for _, wsc := range room.wsConns {
			if wsc != nil {
				wsc.mu.Lock()
				wsc.conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "call ended"))
				wsc.conn.Close()
				wsc.mu.Unlock()
			}
		}
		room.wsConns = make(map[string]*wsConn)
		room.wsMu.Unlock()

		// Закрываем все файлы записи в комнате
		room.closeAllRecordings()
	}

	callStore.deleteRoom(roomID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ended",
	})
}

// handleCallAPI маршрутизирует запросы API видеозвонков
func handleCallAPI(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/create"):
		handleCallCreate(w, r)
	case strings.HasSuffix(path, "/wait"):
		handleCallWait(w, r)
	case strings.HasSuffix(path, "/join"):
		handleCallJoin(w, r)
	case strings.HasSuffix(path, "/ws"):
		handleCallWS(w, r)
	case strings.HasSuffix(path, "/room"):
		handleCallEnd(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveVideoCallHTML отдаёт страницу видеоконференции
func serveVideoCallHTML(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Для отладки: если videocall.html есть рядом с бинарником и он новее бинарника — читаем из файла
	if exe, err := os.Executable(); err == nil {
		htmlPath := filepath.Join(filepath.Dir(exe), "videocall.html")
		if htmlInfo, err := os.Stat(htmlPath); err == nil {
			if exeInfo, err := os.Stat(exe); err == nil {
				if htmlInfo.ModTime().After(exeInfo.ModTime()) {
					if data, err := os.ReadFile(htmlPath); err == nil {
						w.Write(data)
						return
					}
				}
			}
		}
	}
	w.Write(videocallHTML)
}
