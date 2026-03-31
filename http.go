// http.go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

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

// handleGetMessages обрабатывает GET запрос для получения всех сообщений
func handleGetMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messages := chatStore.getMessages()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// handleSendMessage обрабатывает POST запрос для отправки сообщения
func handleSendMessage(w http.ResponseWriter, r *http.Request) {
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

	if req.Sender == "" {
		req.Sender = "Anonymous"
	}

	msg := chatStore.addMessage(req.Text, req.Sender)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
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

		// Используем strings.Builder для построения HTML
		var html strings.Builder

		html.WriteString(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>` + title + `</title>
	<link rel="icon" type="image/png" href="/favicon.ico">
    <link rel="shortcut icon" href="/favicon.ico" type="image/x-icon">
    <link rel="apple-touch-icon" href="/favicon.ico">
	<style>
		body { 
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
			margin: 20px;
			background-color: #f5f5f5;
			-webkit-tap-highlight-color: transparent;
		}
		.container {
			max-width: 1200px;
			margin: 0 auto;
			background-color: white;
			border-radius: 8px;
			box-shadow: 0 2px 4px rgba(0,0,0,0.1);
			padding: 20px;
		}
		.breadcrumbs {
			margin: 0 0 20px 0;
			padding: 10px;
			background-color: #f8f9fa;
			border-radius: 4px;
			font-size: 1.1em;
		}
		.breadcrumbs a {
			color: #0066cc;
			text-decoration: none;
		}
		.breadcrumbs a:hover {
			text-decoration: underline;
		}
		.breadcrumbs .separator {
			color: #666;
			margin: 0 5px;
		}
		.breadcrumbs .current {
			color: #333;
			font-weight: 500;
		}
		.directory-listing {
			width: 100%;
			border-collapse: collapse;
			box-sizing: border-box;
		}

		.directory-listing tr {
			transition: background-color 0.15s ease;
		}

		.directory-listing tr:hover {
			background-color: #f5f5f5;
		}

		.directory-listing td {
			padding: 8px 12px;
			border-bottom: 1px solid #eee;
		}

		.directory-listing .name {
			width: auto;
			white-space: nowrap;
		}

		.directory-listing .size {
			width: 1%;
			white-space: nowrap;
			text-align: right;
			color: #666;
			font-family: monospace;
		}

		.directory-listing .date {
			width: 1%;
			white-space: nowrap;
			color: #666;
			font-family: monospace;
		}

		.directory-listing a {
			color: #0066cc;
			text-decoration: none;
		}

		.directory-listing a:hover {
			text-decoration: underline;
		}

		/* Стили чата */
		.chat-container {
			margin-top: 15px;
			padding-top: 15px;
		}
		.chat-messages {
			max-height: 200px;
			overflow-y: auto;
			background-color: #f8f9fa;
			border: 1px solid #ddd;
			border-radius: 4px;
			padding: 10px;
			margin-bottom: 10px;
		}
		.chat-message {
			margin-bottom: 8px;
			display: flex;
			flex-direction: column;
		}
		.chat-message.own {
			align-items: flex-end;
		}
		.chat-message-content {
			max-width: 80%;
			padding: 6px 10px;
			border-radius: 12px;
			word-wrap: break-word;
			word-break: break-word;
			overflow-wrap: break-word;
			white-space: pre-wrap;
			font-size: 0.85em;
		}
		.chat-message.own .chat-message-content {
			background-color: #007bff;
			color: white;
			border-bottom-right-radius: 2px;
		}
		.chat-message.other .chat-message-content {
			background-color: #e9ecef;
			color: #333;
			border-bottom-left-radius: 2px;
		}
		.chat-message-sender {
			font-size: 0.75em;
			color: #999;
			margin-bottom: 2px;
			margin-left: 3px;
		}
		.chat-message-time {
			font-size: 0.7em;
			color: #999;
			margin-top: 2px;
			text-align: right;
		}
		.chat-input-container {
			display: flex;
			gap: 8px;
		}
		.chat-input {
			flex: 1;
			padding: 6px 10px;
			border: 1px solid #ccc;
			border-radius: 4px;
			font-family: inherit;
			font-size: 0.85em;
			height: 30px;
		}
		.chat-input:focus {
			outline: none;
			border-color: #007bff;
		}
		.chat-send-btn {
			padding: 6px 15px;
			background-color: #007bff;
			color: white;
			border: none;
			border-radius: 4px;
			cursor: pointer;
			font-size: 0.85em;
			font-weight: 500;
			transition: background-color 0.2s;
		}
		.chat-send-btn:hover {
			background-color: #0056b3;
		}
		.chat-send-btn:active {
			background-color: #004494;
		}
		.chat-message-content a.call-link {
			display: inline-block;
			padding: 4px 12px;
			border-radius: 16px;
			text-decoration: none;
			font-weight: 600;
			font-size: 0.95em;
			transition: all 0.2s;
		}
		.chat-message.own .chat-message-content a.call-link {
			background: rgba(255,255,255,0.25);
			color: #fff;
		}
		.chat-message.own .chat-message-content a.call-link:hover {
			background: rgba(255,255,255,0.4);
		}
		.chat-message.other .chat-message-content a.call-link {
			background: #28a745;
			color: #fff;
		}
		.chat-message.other .chat-message-content a.call-link:hover {
			background: #218838;
		}

		/* Медиа-запрос для мобильных устройств */
		@media (max-width: 768px) {
			body {
				margin: 8px;
				font-size: 16px;
			}
			.container {
				border-radius: 6px;
			}
			.breadcrumbs {
				border-radius: 8px;
				margin-bottom: 16px;
			}
			.directory-listing .name {
				white-space: normal;
				width: auto;
			}
			.directory-listing .name a {
				display: inline-block;
				word-wrap: break-word;
				word-break: break-word;
				overflow-wrap: break-word;
			}
			.directory-listing .size {
				width: 25%;
				font-size: 0.5em;
				white-space: normal;
			}
			.directory-listing .date {
				display: none;
			}
		}
		
		/* Для очень маленьких экранов */
		@media (max-width: 480px) {
			body {
				margin: 4px;
			}
			.container {
				padding: 10px;
			}
			.breadcrumbs {
				font-size: 1em;
			}
			.breadcrumbs .separator {
				margin: 0 8px;
			}
			.directory-listing td {
				padding: 10px 5px;
			}
			.directory-listing .name a {
				font-size: 1em;
			}
			.directory-listing .size {
				width: 25%;
				font-size: 0.5em;
			}
			.directory-listing .date {
				display: none;
			}
		}
	</style>
</head>
<body>
	<div class="container">
	` + breadcrumbs + `
	<table class="directory-listing">
			<tbody>
`)

		// Проверяем каждый элемент через Stat FileSystem для правильного определения типа
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

			html.WriteString(`<tr>
	<td class="name"><a href="` + filePath + `">` + name + `</a></td>
	<td class="size">` + size + `</td>
	<td class="date">` + modTime + `</td>
</tr>
`)
		}

		html.WriteString(`			</tbody>
		</table>
	</div>

	<!-- Панель чата -->
	<div class="chat-container">
		<div class="chat-messages" id="chatMessages"></div>
		<div class="chat-input-container">
			<input type="text" class="chat-input" id="chatInput" placeholder="Type a message...">
			<button class="chat-send-btn" id="chatSendBtn">💬Send</button>
			<button class="chat-call-btn chat-send-btn" id="chatCallBtn" style="padding:6px 10px;" title="Video Call">📞Call</button>
		</div>
	</div>

	<script>
		// Получаем уникальный идентификатор для текущего пользователя
		let currentUserId = localStorage.getItem('chatUserId') || 'user_' + Math.random().toString(36).substr(2, 9);
		localStorage.setItem('chatUserId', currentUserId);

		const chatMessages = document.getElementById('chatMessages');
		const chatInput = document.getElementById('chatInput');
		const chatSendBtn = document.getElementById('chatSendBtn');

		let lastMessageCount = 0;

		// Форматирование времени
		function formatTime(timestamp) {
			const date = new Date(timestamp);
			return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		}

		// Определение является ли сообщение своим
		function isOwnMessage(sender) {
			return sender === currentUserId;
		}

		// Отображение сообщения
		function displayMessage(msg) {
			const msgDiv = document.createElement('div');
			msgDiv.className = 'chat-message ' + (isOwnMessage(msg.sender) ? 'own' : 'other');

			const senderDiv = document.createElement('div');
			senderDiv.className = 'chat-message-sender';

			const contentDiv = document.createElement('div');
			contentDiv.className = 'chat-message-content';

			// Проверяем, является ли сообщение приглашением на звонок
			var callMatch = msg.text.match(/^📞\s*(\/videocall\S+)/);
			if (callMatch) {
				var a = document.createElement('a');
				a.href = callMatch[1];
				a.target = '_blank';
				a.className = 'call-link';
				a.textContent = '📞Call';
				contentDiv.appendChild(a);
			} else {
				contentDiv.textContent = msg.text;
			}

			const timeDiv = document.createElement('div');
			timeDiv.className = 'chat-message-time';
			timeDiv.textContent = formatTime(msg.timestamp);

			if (!isOwnMessage(msg.sender)) {
				msgDiv.appendChild(senderDiv);
			}
			msgDiv.appendChild(contentDiv);
			msgDiv.appendChild(timeDiv);

			chatMessages.appendChild(msgDiv);
			chatMessages.scrollTop = chatMessages.scrollHeight;
		}

		// Загрузка сообщений
		async function loadMessages() {
			try {
				const response = await fetch('/api/messages');
				if (!response.ok) return;

				const messages = await response.json();

				// Отображаем только новые сообщения
				if (messages.length > lastMessageCount) {
					const newMessages = messages.slice(lastMessageCount);
					newMessages.forEach(displayMessage);
					lastMessageCount = messages.length;
				}
			} catch (error) {
				console.error('Error loading messages:', error);
			}
		}

		// Отправка сообщения
		async function sendMessage() {
			const text = chatInput.value.trim();
			if (!text) return;

			try {
				const response = await fetch('/api/messages', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json',
					},
					body: JSON.stringify({
						text: text,
						sender: currentUserId
					})
				});

				if (!response.ok) {
					console.error('Failed to send message');
					return;
				}

				chatInput.value = '';
				await loadMessages();
			} catch (error) {
				console.error('Error sending message:', error);
			}
		}

		// Обработчики событий
		chatSendBtn.addEventListener('click', sendMessage);

		chatInput.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') {
				e.preventDefault();
				sendMessage();
			}
		});

		// Периодическая загрузка сообщений (каждые 2 секунды)
		setInterval(loadMessages, 2000);

		// Начальная загрузка
		loadMessages();

		// Видеозвонок
		const chatCallBtn = document.getElementById('chatCallBtn');
		chatCallBtn.addEventListener('click', function() {
			const roomId = 'call_' + Math.random().toString(36).substr(2, 8);
			// Отправляем относительную ссылку в чат
			const callUrl = '/videocall.html?room=' + roomId;
			fetch('/api/messages', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
						text: '📞 ' + callUrl,
						sender: currentUserId
					})
			}).then(function() {
				// Открываем видеозвонок в новой вкладке
				window.open(callUrl, '_blank');
				loadMessages();
			});
		});
	</script>
</body>
</html>`)

		// Отправляем HTML клиенту
		w.Write([]byte(html.String()))
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
