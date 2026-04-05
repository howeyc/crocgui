// videocall.go
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

	// Согласование кодеков
	CreatorCodecs   []string
	NegotiatedCodec string

	// Разрешение для адаптивного качества
	CreatorWidth  int
	CreatorHeight int
	GuestWidth    int
	GuestHeight   int

	// Желаемое разрешение от пира (какой размер видео пир хочет получать)
	CreatorDesiredW int
	CreatorDesiredH int
	GuestDesiredW   int
	GuestDesiredH   int

	// WebSocket соединения для мгновенной доставки чанков
	wsConns map[string]*wsConn // peerID -> ws connection (с мьютексом на запись)
	wsMu    sync.Mutex

	mu sync.Mutex
}

// VideoCallStorage хранит активные комнаты видеозвонков
type VideoCallStorage struct {
	rooms map[string]*VideoCallRoom
	mu    sync.RWMutex
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
	maxChunksPerPeer = 100     // ~10 секунд при 10 чанков/сек
	maxChunkSize     = 1 << 20 // 1 MB максимальный размер чанка
	wsHistoryLimit   = 20      // Максимум чанков истории при WS подключении
)

// createRoom создаёт новую комнату видеозвонка или возвращает ошибку если комната уже существует
func (vs *VideoCallStorage) createRoom(roomID string) (*VideoCallRoom, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if _, exists := vs.rooms[roomID]; exists {
		return nil, fmt.Errorf("room already exists")
	}

	room := &VideoCallRoom{
		ID:          roomID,
		CreatedAt:   time.Now(),
		Chunks:      make(map[string][]VideoChunk),
		ChunkIdx:    make(map[string]int64),
		ChunkLock:   make(map[string]*sync.Mutex),
		WaitingChan: make(chan struct{}),
		wsConns:     make(map[string]*wsConn),
	}
	vs.rooms[roomID] = room
	return room, nil
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

	// Ограничиваем количество хранимых чанков
	if len(r.Chunks[peerID]) > maxChunksPerPeer {
		r.Chunks[peerID] = r.Chunks[peerID][len(r.Chunks[peerID])-maxChunksPerPeer:]
	}

	return idx
}

// getChunksAfter возвращает чанки от указанного пира с индексом > afterIndex
func (r *VideoCallRoom) getChunksAfter(peerID string, afterIndex int64) []VideoChunk {
	r.mu.Lock()
	defer r.mu.Unlock()

	chunks, ok := r.Chunks[peerID]
	if !ok {
		return nil
	}

	// Ищем чанки с индексом > afterIndex
	var result []VideoChunk
	for _, chunk := range chunks {
		if chunk.Index > afterIndex {
			result = append(result, chunk)
		}
	}
	return result
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
		Room     string   `json:"room"`
		Codecs   []string `json:"codecs"`
		Width    int      `json:"width"`
		Height   int      `json:"height"`
		DesiredW int      `json:"desiredW"`
		DesiredH int      `json:"desiredH"`
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
		// Комната уже существует — значит мы не создатель, а присоединяющийся
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "already_exists",
			"room":   req.Room,
		})
		return
	}

	// Сохраняем кодеки и разрешение создателя
	room.mu.Lock()
	room.CreatorCodecs = req.Codecs
	room.CreatorWidth = req.Width
	room.CreatorHeight = req.Height
	room.CreatorDesiredW = req.DesiredW
	room.CreatorDesiredH = req.DesiredH
	room.mu.Unlock()

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

	// Ждём пока подключится второй участник (бесконечно, пока клиент не уйдёт или приложение не завершится)
	select {
	case <-room.WaitingChan:
		room.mu.Lock()
		codec := room.NegotiatedCodec
		gw := room.GuestWidth
		gh := room.GuestHeight
		gdw := room.GuestDesiredW
		gdh := room.GuestDesiredH
		room.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "joined",
			"codec":    codec,
			"width":    gw,
			"height":   gh,
			"desiredW": gdw,
			"desiredH": gdh,
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
		Room     string   `json:"room"`
		Codecs   []string `json:"codecs"`
		Width    int      `json:"width"`
		Height   int      `json:"height"`
		DesiredW int      `json:"desiredW"`
		DesiredH int      `json:"desiredH"`
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

	// Согласовываем кодек: находим первый кодек создателя, который поддерживает гость
	room.mu.Lock()
	negotiatedCodec := ""
	if len(room.CreatorCodecs) > 0 && len(req.Codecs) > 0 {
		for _, cc := range room.CreatorCodecs {
			for _, gc := range req.Codecs {
				if cc == gc {
					negotiatedCodec = cc
					break
				}
			}
			if negotiatedCodec != "" {
				break
			}
		}
	}
	if negotiatedCodec != "" {
		room.NegotiatedCodec = negotiatedCodec
	}
	room.mu.Unlock()

	log.Debugf("Codec negotiation: creator=%v guest=%v negotiated=%s", room.CreatorCodecs, req.Codecs, negotiatedCodec)

	// Уведомляем инициатора
	room.notifyPeerJoined()

	// Сохраняем разрешение гостя и возвращаем разрешение создателя
	room.mu.Lock()
	room.GuestWidth = req.Width
	room.GuestHeight = req.Height
	room.GuestDesiredW = req.DesiredW
	room.GuestDesiredH = req.DesiredH
	cw := room.CreatorWidth
	ch := room.CreatorHeight
	cdw := room.CreatorDesiredW
	cdh := room.CreatorDesiredH
	room.mu.Unlock()

	log.Debugf("Resolution: creator=%dx%d guest=%dx%d", cw, ch, req.Width, req.Height)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "joined",
		"room":     room.ID,
		"codec":    negotiatedCodec,
		"width":    cw,
		"height":   ch,
		"desiredW": cdw,
		"desiredH": cdh,
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

			// Мгновенно перенаправляем remote peer через WS
			if remotePeer != "" {
				room.forwardChunkToWS(remotePeer, data)
			}
		}
	}

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

	// Закрываем все WS соединения перед удалением комнаты
	room := callStore.getRoom(roomID)
	if room != nil {
		room.wsMu.Lock()
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
	// Для отладки: если videocall.html есть рядом с бинарником — читаем из файла
	if exe, err := os.Executable(); err == nil {
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "videocall.html")); err == nil {
			w.Write(data)
			return
		}
	}
	w.Write(videocallHTML)
}
