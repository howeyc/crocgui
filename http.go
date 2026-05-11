// http.go
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/schollz/logger"

	"github.com/gorilla/websocket"
)

//go:embed directory.html
var directoryHTML string

// getDirectoryHTML возвращает HTML-шаблон директории.
// Для отладки: если directory.html есть рядом с бинарником и он новее бинарника — читаем из файла
func getDirectoryHTML() string {
	if exe, err := os.Executable(); err == nil {
		htmlPath := filepath.Join(filepath.Dir(exe), "directory.html")
		if htmlInfo, err := os.Stat(htmlPath); err == nil {
			if exeInfo, err := os.Stat(exe); err == nil {
				if htmlInfo.ModTime().After(exeInfo.ModTime()) {
					if data, err := os.ReadFile(htmlPath); err == nil {
						return string(data)
					}
				}
			}
		}
	}
	return directoryHTML
}

// chatWSUpgrader — WebSocket upgrader для чата
var chatWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return isLocalRequest(r)
	},
}

// chatWSClient представляет подключённого клиента чата
type chatWSClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// chatWSClients хранит всех подключённых клиентов чата
var chatWSClients struct {
	clients map[*chatWSClient]struct{}
	mu      sync.RWMutex
}

func init() {
	chatWSClients.clients = make(map[*chatWSClient]struct{})
}

// broadcastChatMessage отправляет JSON-сообщение всем подключённым клиентам чата
func broadcastChatMessage(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Errorf("broadcastChatMessage marshal error: %v", err)
		return
	}
	chatWSClients.mu.RLock()
	defer chatWSClients.mu.RUnlock()
	for client := range chatWSClients.clients {
		client.mu.Lock()
		if err := client.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Debugf("chat WS broadcast error: %v", err)
		}
		client.mu.Unlock()
	}
}

// handleChatWS обрабатывает WebSocket соединение для чата
func handleChatWS(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	conn, err := chatWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("chat WS upgrade error: %v", err)
		return
	}

	client := &chatWSClient{conn: conn}

	// Регистрируем клиента
	chatWSClients.mu.Lock()
	chatWSClients.clients[client] = struct{}{}
	chatWSClients.mu.Unlock()

	defer func() {
		chatWSClients.mu.Lock()
		delete(chatWSClients.clients, client)
		chatWSClients.mu.Unlock()
		conn.Close()
	}()

	// Отправляем историю сообщений при подключении
	messages := chatStore.getMessages()
	if len(messages) > 0 {
		data, err := json.Marshal(messages)
		if err == nil {
			client.mu.Lock()
			conn.WriteMessage(websocket.TextMessage, data)
			client.mu.Unlock()
		}
	}

	// Ping goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		defer conn.Close()
		for {
			select {
			case <-ticker.C:
				client.mu.Lock()
				err := conn.WriteMessage(websocket.PingMessage, nil)
				client.mu.Unlock()
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
		client.mu.Lock()
		defer client.mu.Unlock()
		return conn.WriteMessage(websocket.PongMessage, nil)
	})

	// Graceful shutdown: отправляем CloseMessage при appCtx.Done()
	readDone := make(chan struct{})
	go func() {
		select {
		case <-appCtx.Done():
			client.mu.Lock()
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutdown"))
			client.mu.Unlock()
		case <-readDone:
		}
	}()

	// Читаем входящие сообщения
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debugf("chat WS read error: %v", err)
			}
			break
		}

		if msgType == websocket.TextMessage && len(data) > 0 {
			var req struct {
				Text   string `json:"text"`
				Sender string `json:"sender"`
			}
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			if req.Text == "" {
				continue
			}
			if req.Sender == "" {
				req.Sender = "Anonymous"
			}

			msg := chatStore.addMessage(req.Text, req.Sender)
			// Рассылаем всем подключённым клиентам
			broadcastChatMessage(msg)
		}
	}
	close(readDone)
}

// Message представляет сообщение в чате
type Message struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Sender    string    `json:"sender"`
	Timestamp time.Time `json:"timestamp"`
}

// ChatStorage хранит сообщения в памяти
type ChatStorage struct {
	messages []Message
	mu       sync.RWMutex
}

var chatStore = &ChatStorage{
	messages: make([]Message, 0),
}

// chatOpened — флаг: браузер чата уже открыт (auto или вручную)
var chatOpened atomic.Bool

// chatURL — URL для открытия чата (устанавливается в switchToWebDAVTree)
var chatURL string

// addMessage добавляет новое сообщение в хранилище
func (cs *ChatStorage) addMessage(text, sender string) Message {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	msg := Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Text:      text,
		Sender:    sender,
		Timestamp: time.Now(),
	}

	cs.messages = append(cs.messages, msg)
	return msg
}

// getMessages возвращает все сообщения
func (cs *ChatStorage) getMessages() []Message {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	result := make([]Message, len(cs.messages))
	copy(result, cs.messages)
	return result
}

// isLocalRequest проверяет, что запрос пришёл с локального IP адреса
func isLocalRequest(r *http.Request) bool {
	return true
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	localIPs := hostSelectOptions(LOCAL)
	return slices.Contains(localIPs, host) || host == "::1"
}

// handleGetMessages обрабатывает GET запрос для получения сообщений
// GET /api/messages          — все сообщения
// GET /api/messages?since=N  — сообщения начиная с индекса N
func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messages := chatStore.getMessages()

	// Проверяем параметр ?since=N
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := strconv.Atoi(sinceStr); err == nil && since >= 0 {
			if since > len(messages) {
				messages = []Message{}
			} else {
				messages = messages[since:]
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// handleSendMessage обрабатывает POST запрос для отправки сообщения
func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Text   string `json:"text"`
		Sender string `json:"sender"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)

	if req.Sender == "" {
		req.Sender = "Anonymous"
	}

	log.Debugf("%s>%s", req.Sender, req.Text)
}

func (h *WebDAVWithDirectoryListing) serveDirectoryListing(w http.ResponseWriter, r *http.Request) {
	// Открываем директорию через FileSystem
	f, err := h.fileSystem.OpenFile(appCtx, r.URL.Path, os.O_RDONLY, 0)
	if err != nil {
		http.Error(w, "Error opening directory", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Проверяем, что это действительно директория через Stat
	stat, err := f.Stat()
	if err != nil || !stat.IsDir() {
		http.Error(w, "Not a directory", http.StatusBadRequest)
		return
	}

	// Для webdav.File нужно использовать Readdir если он поддерживается
	type readdirFile interface {
		Readdir(count int) ([]os.FileInfo, error)
	}

	if readdir, ok := f.(readdirFile); !ok {
		// Если не можем получить интерфейс с Readdir, используем стандартный WebDAV handler
		h.webdavHandler.ServeHTTP(w, r)
	} else {
		// Читаем все файлы в директории
		fileInfos, err := readdir.Readdir(-1)
		if err != nil {
			http.Error(w, "Error reading directory", http.StatusInternalServerError)
			return
		}

		// Сортируем: сначала каталоги, потом файлы, внутри групп по алфавиту
		sort.Slice(fileInfos, func(i, j int) bool {
			if fileInfos[i].IsDir() != fileInfos[j].IsDir() {
				return fileInfos[i].IsDir()
			}
			return fileInfos[i].Name() < fileInfos[j].Name()
		})

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Нормализуем путь для отображения
		displayPath := r.URL.Path
		if displayPath == "" {
			displayPath = "/"
		}
		displayPath = "/" + strings.TrimLeft(path.Clean(displayPath), "/")
		if displayPath == "/." {
			displayPath = "/"
		}

		// Формируем кликабельную цепочку родительских каталогов
		breadcrumbs := h.generateBreadcrumbs(displayPath)

		// Формируем заголовок страницы
		title := "WebDAV"
		rootDir := join()
		if rootDir != "" {
			title = filepath.Base(rootDir)
		}
		if displayPath != "/" {
			title += displayPath
		}

		// Формируем HTML строк файлов
		var filesHTML strings.Builder
		for _, info := range fileInfos {
			name := info.Name()

			// Получаем полный путь в FileSystem
			fullPath := path.Join(r.URL.Path, name)

			// Используем Stat от FileSystem для определения типа
			// Это гарантирует правильную обработку симлинков через ResolvingFileSystem
			stat, err := h.fileSystem.Stat(appCtx, fullPath)
			if err != nil {
				// Если не можем получить stat, пропускаем элемент
				continue
			}

			// Формируем путь для ссылки - без суффикса /
			filePath := path.Join(r.URL.Path, name)
			if stat.IsDir() {
				name += "/"
			}

			// Форматируем размер (только для файлов)
			var size string
			if !stat.IsDir() {
				size = formatSize(stat.Size())
			}

			// Форматируем дату
			modTime := stat.ModTime().Format("2006-01-02 15:04:05")

			filesHTML.WriteString(`<tr>
	<td class="name"><a href="` + filePath + `">` + name + `</a></td>
	<td class="size">` + size + `</td>
	<td class="date">` + modTime + `</td>
</tr>
`)
		}

		// Получаем шаблон и подставляем данные
		tmpl := getDirectoryHTML()
		result := strings.ReplaceAll(tmpl, "<!--PH-->TITLE<!--PH-->", title)
		result = strings.ReplaceAll(result, "<!--PH-->BREADCRUMBS<!--PH-->", breadcrumbs)
		result = strings.ReplaceAll(result, "<!--PH-->FILES<!--PH-->", filesHTML.String())

		// Обрабатываем секцию чата: показываем только для локальных IP
		const chatStartMarker = "<!--PH:CHAT_START-->"
		const chatEndMarker = "<!--PH:CHAT_END-->"
		if isLocalRequest(r) {
			// Оставляем чат, убираем только маркеры
			result = strings.ReplaceAll(result, chatStartMarker, "")
			result = strings.ReplaceAll(result, chatEndMarker, "")
		} else {
			// Удаляем всю секцию чата
			startIdx := strings.Index(result, chatStartMarker)
			endIdx := strings.Index(result, chatEndMarker)
			if startIdx >= 0 && endIdx > startIdx {
				result = result[:startIdx] + result[endIdx+len(chatEndMarker):]
			}
		}

		w.Write([]byte(result))
	}
}

// formatSize форматирует размер файла в человеко-читаемый вид
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// generateBreadcrumbs создает кликабельную цепочку родительских каталогов
func (h *WebDAVWithDirectoryListing) generateBreadcrumbs(currentPath string) string {
	separator := `<span class="separator">›</span>`

	// Получаем имя корневой директории
	root := "Root"
	rootDir := join()
	if rootDir != "" {
		base := filepath.Base(rootDir)
		if base != "" && base != "." {
			root = base
		}
	}

	if currentPath == "/" {
		return fmt.Sprintf(`<div class="breadcrumbs"><a href="/">%s</a>%s</div>`, root, separator)
	}

	parts := strings.Split(strings.Trim(currentPath, "/"), "/")
	var breadcrumbs strings.Builder
	breadcrumbs.WriteString(fmt.Sprintf(`<div class="breadcrumbs"><a href="/">%s</a>`, root))

	pathSoFar := ""
	for i, part := range parts {
		pathSoFar += "/" + part
		if i == len(parts)-1 {
			// Текущий каталог (не ссылка)
			breadcrumbs.WriteString(fmt.Sprintf(`%s <span class="current">%s</span>`, separator, part))
		} else {
			// Родительский каталог (ссылка)
			breadcrumbs.WriteString(fmt.Sprintf(`%s <a href="%s">%s</a>`, separator, pathSoFar, part))
		}
	}
	breadcrumbs.WriteString("</div>")
	return breadcrumbs.String()
}
