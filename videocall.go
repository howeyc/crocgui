// videocall.go
package main

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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

	// Время последнего GET-запроса чанков от каждого пира
	LastPollTime map[string]time.Time

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

const (
	maxChunksPerPeer = 300             // ~30 секунд при 10 чанков/сек
	maxChunkSize     = 1 << 20         // 1 MB максимальный размер чанка
	peerTimeout      = 5 * time.Second // Таймаут отсутствия GET от пира
)

// createRoom создаёт новую комнату видеозвонка или возвращает ошибку если комната уже существует
func (vs *VideoCallStorage) createRoom(roomID string) (*VideoCallRoom, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if _, exists := vs.rooms[roomID]; exists {
		return nil, fmt.Errorf("room already exists")
	}

	room := &VideoCallRoom{
		ID:           roomID,
		CreatedAt:    time.Now(),
		Chunks:       make(map[string][]VideoChunk),
		ChunkIdx:     make(map[string]int64),
		ChunkLock:    make(map[string]*sync.Mutex),
		LastPollTime: make(map[string]time.Time),
		WaitingChan:  make(chan struct{}),
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

// handleCallCreate обрабатывает POST /api/call/create — создаёт комнату
func handleCallCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Room   string   `json:"room"`
		Codecs []string `json:"codecs"`
		Width  int      `json:"width"`
		Height int      `json:"height"`
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

	// Ждём до 30 секунд пока подключится второй участник
	select {
	case <-room.WaitingChan:
		room.mu.Lock()
		codec := room.NegotiatedCodec
		gw := room.GuestWidth
		gh := room.GuestHeight
		room.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "joined",
			"codec":  codec,
			"width":  gw,
			"height": gh,
		})
	case <-time.After(30 * time.Second):
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "timeout",
		})
	case <-r.Context().Done():
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
		Room   string   `json:"room"`
		Codecs []string `json:"codecs"`
		Width  int      `json:"width"`
		Height int      `json:"height"`
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
	cw := room.CreatorWidth
	ch := room.CreatorHeight
	room.mu.Unlock()

	log.Debugf("Resolution: creator=%dx%d guest=%dx%d", cw, ch, req.Width, req.Height)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "joined",
		"room":   room.ID,
		"codec":  negotiatedCodec,
		"width":  cw,
		"height": ch,
	})
}

// handleCallChunkPost обрабатывает POST /api/call/chunk — отправка WebM-чанка
func handleCallChunkPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	// Ограничиваем размер чанка
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkSize)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read chunk", http.StatusBadRequest)
		return
	}

	idx := room.addChunk(peerID, data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"index": idx,
	})
}

// handleCallChunkGet обрабатывает GET /api/call/chunk — получение WebM-чанков (long poll)
func handleCallChunkGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID := r.URL.Query().Get("room")
	peerID := r.URL.Query().Get("peer")    // Чьи чанки хотим получить
	afterStr := r.URL.Query().Get("after") // После какого индекса

	if roomID == "" || peerID == "" {
		http.Error(w, "room and peer are required", http.StatusBadRequest)
		return
	}

	selfPeerID := r.URL.Query().Get("self") // Кто запрашивает (для alive check)

	var afterIndex int64 = -1
	if afterStr != "" {
		fmt.Sscanf(afterStr, "%d", &afterIndex)
	}

	room := callStore.getRoom(roomID)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Записываем время опроса от инициатора запроса (для проверки активности пира)
	if selfPeerID != "" {
		room.mu.Lock()
		room.LastPollTime[selfPeerID] = time.Now()
		room.mu.Unlock()
	}

	// Пытаемся получить чанки сразу
	chunks := room.getChunksAfter(peerID, afterIndex)
	if len(chunks) > 0 {
		writeChunksBinary(w, chunks)
		return
	}

	// Long poll: ждём до 10 секунд
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			chunks = room.getChunksAfter(peerID, afterIndex)
			if len(chunks) > 0 {
				writeChunksBinary(w, chunks)
				return
			}
		case <-deadline:
			// Пустой ответ — 0 чанков
			writeChunksBinary(w, nil)
			return
		case <-r.Context().Done():
			return
		}
	}
}

// handleCallAlive обрабатывает GET /api/call/alive — проверка активности пира
func handleCallAlive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID := r.URL.Query().Get("room")
	peerID := r.URL.Query().Get("peer") // Чью активность проверяем

	if roomID == "" || peerID == "" {
		http.Error(w, "room and peer are required", http.StatusBadRequest)
		return
	}

	room := callStore.getRoom(roomID)
	if room == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"alive": false})
		return
	}

	room.mu.Lock()
	lastPoll, exists := room.LastPollTime[peerID]
	room.mu.Unlock()

	alive := exists && time.Since(lastPoll) < peerTimeout

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"alive": alive})
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

	callStore.deleteRoom(roomID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ended",
	})
}

// handleCallAPI маршрутизирует запросы API видеозвонков
func handleCallAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/create"):
		handleCallCreate(w, r)
	case strings.HasSuffix(path, "/wait"):
		handleCallWait(w, r)
	case strings.HasSuffix(path, "/join"):
		handleCallJoin(w, r)
	case strings.HasSuffix(path, "/chunk") && r.Method == http.MethodPost:
		handleCallChunkPost(w, r)
	case strings.HasSuffix(path, "/chunk") && r.Method == http.MethodGet:
		handleCallChunkGet(w, r)
	case strings.HasSuffix(path, "/alive"):
		handleCallAlive(w, r)
	case strings.HasSuffix(path, "/room"):
		handleCallEnd(w, r)
	default:
		http.NotFound(w, r)
	}
}

// writeChunksBinary записывает чанки в бинарном формате:
// [4 байта: count (uint32 BE)]
// для каждого чанка:
//
//	[4 байта: index (uint32 BE)]
//	[4 байта: size (uint32 BE)]
//	[N байт: data (raw binary)]
func writeChunksBinary(w http.ResponseWriter, chunks []VideoChunk) {
	w.Header().Set("Content-Type", "application/octet-stream")
	count := uint32(len(chunks))
	binary.Write(w, binary.BigEndian, count)
	for _, chunk := range chunks {
		binary.Write(w, binary.BigEndian, uint32(chunk.Index))
		binary.Write(w, binary.BigEndian, uint32(len(chunk.Data)))
		w.Write(chunk.Data)
	}
}

// serveVideoCallHTML отдаёт страницу видеоконференции
func serveVideoCallHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(videocallHTML)
}
