// videocall.go
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
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
	maxChunksPerPeer = 300     // ~30 секунд при 10 чанков/сек
	maxChunkSize     = 1 << 20 // 1 MB максимальный размер чанка
)

// createRoom создаёт новую комнату видеозвонка
func (vs *VideoCallStorage) createRoom(roomID string) *VideoCallRoom {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	room := &VideoCallRoom{
		ID:          roomID,
		CreatedAt:   time.Now(),
		Chunks:      make(map[string][]VideoChunk),
		ChunkIdx:    make(map[string]int64),
		ChunkLock:   make(map[string]*sync.Mutex),
		WaitingChan: make(chan struct{}),
	}
	vs.rooms[roomID] = room
	return room
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

	room := callStore.createRoom(req.Room)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "joined",
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

	var afterIndex int64 = -1
	if afterStr != "" {
		fmt.Sscanf(afterStr, "%d", &afterIndex)
	}

	room := callStore.getRoom(roomID)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Пытаемся получить чанки сразу
	chunks := room.getChunksAfter(peerID, afterIndex)
	if len(chunks) > 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"chunks": chunks,
		})
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
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"chunks": chunks,
				})
				return
			}
		case <-deadline:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chunks": []VideoChunk{},
			})
			return
		case <-r.Context().Done():
			return
		}
	}
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
	case strings.HasSuffix(path, "/room"):
		handleCallEnd(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveVideoCallHTML отдаёт страницу видеоконференции
func serveVideoCallHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(videocallHTML)
}
