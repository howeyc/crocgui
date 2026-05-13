// webdav.go
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	gomime "github.com/cubewise-code/go-mime"
	"github.com/schollz/croc/v10/src/comm"
	"github.com/schollz/croc/v10/src/croc"
	"github.com/schollz/croc/v10/src/models"
	"github.com/schollz/croc/v10/src/tcp"
	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
)

// WebDAVServer encapsulates the state and management of a WebDAV server.
type WebDAVServer struct {
	server *http.Server
	mu     sync.RWMutex
	active bool
	addr   string
	root   string
	useTLS bool

	// Для кеширования TLS конфигурации
	TLSConfig      *tls.Config
	tlsMu          sync.RWMutex
	tlsAddrs       []string
	tlsConfigError error

	localHandler http.Handler

	// Callback для уведомления о смене состояния прокси
	onProxyStateChanged func(enabled bool)
	remote              bool
	local               bool

	tcpForwarder     *TCPForwarder
	tcpListener      net.Listener
	tcpForwarding    bool
	tcpForwardingMu  sync.RWMutex
	listenerStopChan chan struct{}
}

// WebDAVWithDirectoryListing оборачивает стандартный WebDAV handler для поддержки
// отображения директорий при GET запросах
type WebDAVWithDirectoryListing struct {
	webdavHandler *webdav.Handler
	fileSystem    webdav.FileSystem
}

// DetectPathTraversal проверяет путь на попытки directory traversal
// Возвращает true, если обнаружена попытка выхода за пределы корневой директории
func DetectPathTraversal(p string) (hasTraversal bool, cleanedPath string) {
	// Нормализуем путь (убираем . и лишние слеши)
	cleaned := path.Clean(p)

	// Разбиваем на компоненты
	parts := strings.Split(cleaned, "/")

	// Отслеживаем глубину вложенности
	depth := 0
	for _, part := range parts {
		switch part {
		case "", ".":
			// Пустые компоненты и текущая директория - игнорируем
			continue
		case "..":
			// Попытка подняться выше
			depth--
			if depth < 0 {
				// Пытаемся выйти за пределы корня
				return true, cleaned
			}
		default:
			// Обычная директория
			depth++
		}
	}

	return false, cleaned
}

func (h *WebDAVWithDirectoryListing) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if hasTraversal, cleanedPath := DetectPathTraversal(r.URL.Path); hasTraversal {
		log.Warnf("Path traversal attempt detected: %s", r.URL.Path)
		http.Error(w, "Forbidden: Path traversal detected", http.StatusForbidden)
		return
	} else {
		// Используем cleanedPath для дальнейшей работы
		r.URL.Path = cleanedPath
	}

	// Отдаем встроенную иконку для favicon запросов
	if r.URL.Path == "/favicon.ico" {
		log.Debugf("Request %s %s", r.Method, r.URL.Path)
		h.serveFavicon(w, r)
		return
	}
	// Для GET запросов проверяем, является ли путь директорией
	if r.Method == http.MethodGet {
		log.Infof("Request %s %s", r.Method, r.URL.Path)
		// Используем ту же FileSystem, что и webdav.Handler
		if info, err := h.fileSystem.Stat(appCtx, r.URL.Path); err == nil && info.IsDir() {
			// Это директория - показываем список файлов
			h.serveDirectoryListing(w, r)
			return
		}

		// Если это файл, устанавливаем правильный Content-Type
		if info, err := h.fileSystem.Stat(appCtx, r.URL.Path); err == nil && !info.IsDir() {
			// Определяем MIME-тип по расширению через go-mime
			ext := path.Ext(r.URL.Path)
			mimeType := gomime.TypeByExtension(ext)

			if mimeType != "" {
				// Для FLAC используем правильный тип
				switch strings.ToLower(ext) {
				case ".flac":
					mimeType = "audio/flac"
				case ".mov":
					mimeType = "video/mp4"
				}
				w.Header().Set("Content-Type", mimeType)

				// Добавляем заголовки для поддержки потокового воспроизведения
				if strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/") {
					w.Header().Set("Accept-Ranges", "bytes")
				}
			} else {
				// Если тип не определен, используем DetectContentType как fallback
				file, err := h.fileSystem.OpenFile(appCtx, r.URL.Path, os.O_RDONLY, 0)
				if err == nil {
					defer file.Close()
					buffer := make([]byte, 512)
					_, err := file.Read(buffer)
					if err == nil {
						mimeType = http.DetectContentType(buffer)
						w.Header().Set("Content-Type", mimeType)
					}
				}
			}
		}
	}

	// Для MOVE и COPY запросов всегда исправляем Destination
	if r.Method == "MOVE" || r.Method == "COPY" {
		if dest := r.Header.Get("Destination"); dest != "" {
			log.Debugf("Original Destination: %s", dest)

			// Просто отрезаем схему и хост
			// Ищем "://" и следующий за ним "/"
			if idx := strings.Index(dest, "://"); idx != -1 {
				// Находим начало пути после хоста
				pathStart := strings.Index(dest[idx+3:], "/")
				if pathStart != -1 {
					fixedDest := dest[idx+3+pathStart:]
					// Убеждаемся, что путь начинается с /
					if !strings.HasPrefix(fixedDest, "/") {
						fixedDest = "/" + fixedDest
					}
					r.Header.Set("Destination", fixedDest)
					log.Debugf("Fixed Destination: %s", fixedDest)
				}
			}
		}
	}

	// Для всех остальных случаев используем стандартный WebDAV handler
	// log.Debugf("ServeHTTP %+v", r)
	h.webdavHandler.ServeHTTP(w, r)
}

func (h *WebDAVWithDirectoryListing) serveFavicon(w http.ResponseWriter, r *http.Request) {
	// Обработка OPTIONS для CORS
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		return
	}

	if len(iconData) == 0 {
		http.NotFound(w, r)
		return
	}

	// Важно! Добавляем заголовки для корректного кэширования в закладках
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24 часа, не слишком долго
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(iconData)))
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Для HEAD запросов только заголовки
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Для GET - отдаем данные
	w.WriteHeader(http.StatusOK)
	w.Write(iconData)
}

// NewWebDAVServer создает новый экземпляр WebDAV сервера
func NewWebDAVServer() *WebDAVServer {
	return &WebDAVServer{}
}

// Определяем какую FileSystem использовать
func createFileSystem(root string) webdav.FileSystem {
	if base := filepath.Base(root); base == SEND { //!CanCreateSymlinks() &&
		return &ResolvingFileSystem{root: root}
	}
	return webdav.Dir(root)
}

// createLocalHandler создаёт обычный WebDAV handler для локальных файлов
func (s *WebDAVServer) createLocalHandler(root string) http.Handler {
	fs := createFileSystem(root)

	webdavHandler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Errorf("WebDAV request %s %s: %v", r.Method, r.URL.Path, err)
			} else {
				log.Infof("WebDAV request %s %s", r.Method, r.URL.Path)
			}
			// Для MOVE и COPY запросов логируем заголовок Destination
			if r.Method == "MOVE" || r.Method == "COPY" {
				if dest := r.Header.Get("Destination"); dest != "" {
					log.Debugf("  Destination header: %s", dest)
				}
			}
		},
	}

	return &WebDAVWithDirectoryListing{
		webdavHandler: webdavHandler,
		fileSystem:    fs,
	}
}

// handlerRouter направляет запросы к текущему активному handler'у
func (s *WebDAVServer) handlerRouter(w http.ResponseWriter, r *http.Request) {
	// Обработка API чата
	if r.URL.Path == "/api/chat/ws" {
		handleChatWS(w, r)
		return
	}
	if r.URL.Path == "/api/messages" {
		if r.Method == http.MethodGet {
			handleGetMessages(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handleSendMessage(w, r)
			return
		}
	}

	// Обработка API видеозвонков
	if strings.HasPrefix(r.URL.Path, "/api/call/") {
		handleCallAPI(w, r)
		return
	}

	// Отдача страницы видеозвонка
	if r.URL.Path == "/videocall.html" {
		serveVideoCallHTML(w, r)
		return
	}

	s.mu.RLock()
	handler := s.localHandler
	s.mu.RUnlock()

	if handler == nil {
		http.Error(w, "WebDAV server not ready", http.StatusServiceUnavailable)
		return
	}

	handler.ServeHTTP(w, r)
}

// Start launches the WebDAV server on the specified address with the given root directory.
func (s *WebDAVServer) Start(addr, root string, useTLS bool, addrs ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop the previous server if it is active.
	if s.active {
		if s.addr == addr && s.root == root && s.useTLS == useTLS {
			return nil
		}
		s.stopLocked()
	}

	// Создаём корневой каталог если не существует
	if err := os.MkdirAll(root, 0700); err != nil {
		log.Warnf("WebDAV: failed to create root directory %s: %v", root, err)
	}

	// Устанавливаем корневой каталог для серверной записи видео
	callStore.SetWebDAVRoot(root)

	caffeinate(1)
	s.useTLS = useTLS

	// Создаем локальный handler
	s.localHandler = s.createLocalHandler(root)

	// Создаем сервер с handlerRouter, который выбирает текущий handler
	s.server = &http.Server{
		Addr:    addr,
		Handler: http.HandlerFunc(s.handlerRouter),
	}

	// Если нужен HTTPS, подготавливаем TLS конфиг
	if useTLS {
		if err := s.prepareTLSConfig(addrs...); err != nil {
			s.useTLS = false
			log.Errorf("failed to prepare TLS config: %v", err)
		} else {
			s.server.TLSConfig = s.TLSConfig
		}
	}

	// Start the server in a separate goroutine.
	go func() {
		var err error
		scheme := HTTP
		if useTLS && s.useTLS {
			scheme = HTTPS
		}
		log.Infof("WebDAV on %s://%s %s", scheme, addr, root)
		if useTLS && s.useTLS {
			err = s.server.ListenAndServeTLS("", "")
		} else {
			err = s.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Errorf("WebDAV listenAndServe: %v", err)
			s.mu.Lock()
			if s.active {
				s.active = false
				caffeinate(-1)
			}
			s.mu.Unlock()
		}
	}()

	s.active = true
	s.addr = addr
	s.root = root
	return nil
}

// prepareTLSConfig подготавливает TLS конфигурацию и сохраняет её в server.TLSConfig
func (s *WebDAVServer) prepareTLSConfig(addrs ...string) error {
	if len(addrs) == 0 {
		return fmt.Errorf("no addresses provided")
	}

	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()

	// Проверяем, изменились ли адреса
	if slices.Equal(addrs, s.tlsAddrs) && s.TLSConfig != nil {
		return nil // Используем существующий конфиг
	}

	// Генерируем новый конфиг
	config, err := generateTLSConfig(addrs...)
	if err != nil {
		s.TLSConfig = nil
		s.tlsConfigError = err
		s.tlsAddrs = nil
		return err
	}

	s.TLSConfig = config
	s.tlsAddrs = addrs
	s.tlsConfigError = nil

	return nil
}

// hardcodedTLSKey — захардкоженный приватный RSA-2048 ключ.
// Используется для детерминированной генерации самоподписанного сертификата:
// для одного и того же списка IP-адресов всегда получается один и тот же сертификат,
// поэтому пользователь подтверждает исключение безопасности только один раз.
var hardcodedTLSKey *rsa.PrivateKey

func init() {
	block, _ := pem.Decode([]byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIEoAIBAAKCAQEA/gTTr+gCtZYMyNvL6VfrEIW+G5eu1zDg0tBt2HoQa1bFMLlb
gEFhTbE1k7J/RKCVkHXUwUITy8zJTdS78LgIfIHsXGOVe0Qbi7MYXUz4hBtMAc7Y
sJzGkVRJzN1TLQf2CgZgsaPQgLFLRN/x9X5dGUEWQvzBpxvRr39zosg952S5rxVn
OgCjkyzvuyngzMqNHMGxgy3OeAe2VKrWoX1UCRoWhLGOfRl+rCLosAmC4q8436nW
pJSOqH0ZWG1YuOTMQZDr99IPGVWTaZgAvV+7P1OeFcZvk9oAxWR7c/zV/i2FGFRd
AAJyfE1533rkM7tptoqBmzDUkSC3P1WNNziKWQIDAQABAoH/c5i+vM5YbUpbhwx/
PzFDR8GVQflFF6imp0kys9DYqABUvFedzD/0h+ac+xm/0PtDFPqKV2g6mgQXl9O3
s1QMiJyXc3PeErprzqcx70OX1IaXkDsRYU33DyvMae5Oa6+zx9wfJLfnqqkEF9PR
yGY498Um3FUpy2Jdif/2H54Ajcvgm0D4nDA9Dlsycf3QiTnQO89RthOvlVk5epYW
vnVPpoSTL0MGzW2SVciX0mtOMCmbUyiKvYKyf2NH9tkLTZ8/rz4tIQJGz5EBBDT5
642IsRNdz9GUX8h4o3wFeMQHNZu43S+LXehOSqR0sb0ysf6SU+zPWJm8xaexXFo+
TrPDAoGBAP/Qu3YydciL3RGnFUuMCWLFm6qdoMf/ULsDtSTeDLF8Tf7BT6f0ZJ9A
GKQmqCQPYQYVAH5vPc6cKV05wcJE98/WyCk5cP9lh0QSy5FobJHrJpx7DG1aqRbd
VyjAT3ynURv+YzKYS2uW9pHCl9O1d7xI+noyCN8PSLKIBdo4rK4TAoGBAP4zwz9W
TI/HYOQCFjWawphM/UIuS1Qgvf7935JGFVIa6Cn/ElRDlmKwen6JWn+NDuyAdxi0
rJZHeHqFqnh0qMBptDDlhqAHpynM/v3inqlXOylnx26SfRCIj7fhMTHXHElMr3G0
uwrad1ML+l++ql62FcTkaJl+yLasOX8A3QNjAoGAS2DHDCH8QNatklkIVlVyIo+V
ueVujd/2etSx2KYxWU8GcG2nuhayW5Z4bE4Tt2Rss20W0yqWLL4pFhZBuKu31Z81
JaiOWkMhY3aiUztQ2oJOw0cit0pCjsEzwIdCJLnslXIU6sCjYJWAHB0ZvcE4AdwD
KmR55rhLNIgOKWoPv88CgYBI09ukUb0tlBmWOWLTiLsnlycXxtueBqNoYqOi7KE/
HKZXIdTGf3aeX6E4j3F2CZu09jkowtqPU3qY36KvT/zo41/Ugm3He2nQ+AI2Cq8a
JPu2KR1h+GYMTpOeQs4tUUuxVF8PXJAZ0+1Lxaq9s4psCA7EkgvFriUi8MSoNj8b
sQKBgHVACU7cvuH+mgNl1N6j3lyWwDyk9ToNoBNS9syeAcFII2heO/TFAaEBhis3
t9pqiQrPhdqnEDMN5/Sz2g+AYtXQmrBF+H07DlFA5rxL+2pv1q5lX2DoZVCSkKTR
I+8pZcDzhk499jsPRvHUzgiGkMBbxarpOTaDkTT1w+QGpPX6
-----END RSA PRIVATE KEY-----
`))
	if block == nil {
		panic("failed to decode hardcoded TLS private key PEM")
	}
	var err error
	hardcodedTLSKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(fmt.Sprintf("failed to parse hardcoded TLS private key: %v", err))
	}
}

// generateTLSConfig генерирует детерминированную TLS конфигурацию с самоподписанным сертификатом.
// Для одного и того же списка IP-адресов всегда выдаёт один и тот же сертификат,
// чтобы пользователь подтверждал исключение безопасности только один раз.
// Ожидает addr в формате "IP" (например, "192.168.0.107").
func generateTLSConfig(addrs ...string) (*tls.Config, error) {
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses provided")
	}

	var IPAddresses []net.IP
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, fmt.Errorf("invalid address format %v", addr)
		}
		IPAddresses = append(IPAddresses, ip)
	}

	// Сортируем адреса для детерминированности серийного номера
	sorted := make([]string, len(addrs))
	copy(sorted, addrs)
	slices.Sort(sorted)

	// Детерминированный серийный номер: SHA-256 от отсортированных адресов
	h := sha256.New()
	for _, addr := range sorted {
		h.Write([]byte(addr))
		h.Write([]byte{0}) // разделитель
	}
	serialNumber := new(big.Int).SetBytes(h.Sum(nil))
	// Убеждаемся, что serial number положительный
	if serialNumber.Sign() <= 0 {
		serialNumber.Add(serialNumber, new(big.Int).Lsh(big.NewInt(1), 255))
	}

	// Фиксированные даты, чтобы сертификат был детерминированным
	notBefore := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2124, 1, 1, 0, 0, 0, 0, time.UTC) // 100 лет

	// Создаем шаблон сертификата
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{CG},
			CommonName:   sorted[0], // Детерминированный CN — первый из отсортированных
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           IPAddresses,
	}

	// Создаем сам сертификат (самоподписанный, используем захардкоженный ключ)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &hardcodedTLSKey.PublicKey, hardcodedTLSKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	// Формируем структуру tls.Certificate для использования в сервере
	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  hardcodedTLSKey,
	}

	// Возвращаем итоговый конфиг (требуем минимум TLS 1.2)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Stop останавливает сервер и прокси если есть
func (s *WebDAVServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopLocked()
}

// stopLocked is an internal method for stopping the server (called while holding the lock).
func (s *WebDAVServer) stopLocked() error {
	if !s.active {
		return nil
	}
	s.active = false // Сразу гасим флаг
	caffeinate(-1)   // Уменьшаем счетчик только здесь

	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(appCtx, WebDAVTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Errorf("WebDAV shutdown: %v", err)
		return err
	}

	s.server = nil
	log.Info("WebDAV done")
	return nil
}

// IsActive returns the current state of the server.
func (s *WebDAVServer) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// IsLocal returns the current state of the server.
func (s *WebDAVServer) IsLocal() bool {
	if !s.IsActive() {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.local
}

// SetLocal
func (s *WebDAVServer) SetLocal(ok bool) {
	if !s.IsActive() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.local = ok
}

// SetProxyStateChangeCallback устанавливает callback для уведомления о смене состояния прокси
func (s *WebDAVServer) SetProxyStateChangeCallback(cb func(bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onProxyStateChanged = cb
}

// EnableTCPForwarding активирует TCP портфорвардинг через croc туннель
func (s *WebDAVServer) EnableTCPForwarding(client *croc.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, не активен ли уже форвардинг
	if s.tcpForwarding && s.tcpForwarder != nil {
		return fmt.Errorf("TCP forwarding already active")
	}

	// Получаем базовый адрес ретранслятора
	relayAddr := client.Options.RelayAddress
	if relayAddr == "" {
		return fmt.Errorf("no relay address configured")
	}

	// Парсим хост и порт
	host, portStr, _ := defAddress(relayAddr)
	basePort, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port number %s: %w", portStr, err)
	}

	// Параметры туннеля - создаем 2 соединения с roomSuffix от 1 до 2
	var conns []*comm.Comm
	var connErrors []error
	var firstRelayPort int
	var firstRoomName string

	for roomSuffix := 1; roomSuffix <= 2; roomSuffix++ {
		relayPort := basePort + roomSuffix + 1
		roomName := fmt.Sprintf("%s-%d", client.Options.RoomName, roomSuffix)
		relayAddrFull := net.JoinHostPort(host, strconv.Itoa(relayPort))

		log.Infof("TCP forwarding: establishing connection %d to %s (room: %s)", roomSuffix, relayAddrFull, roomName)

		// Устанавливаем соединение
		conn, banner, externalIP, err := tcp.ConnectToTCPServer(
			relayAddrFull,
			client.Options.RelayPassword,
			roomName,
			10*time.Second,
		)
		if err != nil {
			connErrors = append(connErrors, fmt.Errorf("failed to establish tunnel %d: %w", roomSuffix, err))
			// Продолжаем, чтобы закрыть все открытые соединения
			break
		}

		log.Debugf("TCP forwarding tunnel %d connected: banner=%s, externalIP=%s", roomSuffix, banner, externalIP)
		conns = append(conns, conn)

		// Сохраняем первый порт и комнату для логирования
		if roomSuffix == 1 {
			firstRelayPort = relayPort
			firstRoomName = roomName
		}
	}

	// Если были ошибки, закрываем все соединения и возвращаем ошибку
	if len(connErrors) > 0 || len(conns) != 2 {
		for _, conn := range conns {
			conn.Close()
		}
		if len(connErrors) > 0 {
			return fmt.Errorf("failed to establish all tunnels: %v", connErrors[0])
		}
		return fmt.Errorf("failed to establish all 2 tunnels")
	}

	// Создаем TCP форвардер
	// На стороне отправителя: localServerAddr - адрес локального WebDAV сервера
	// На стороне получателя: localServerAddr - не используется (может быть пустым)
	var localServerAddr string
	if client.Options.IsSender {
		localServerAddr = s.addr // Адрес локального WebDAV сервера
	}
	tcpForwarder := NewTCPForwarder(conns, client.Options.IsSender, client.Key, localServerAddr)

	if client.Options.IsSender {
		// Отправитель: запускаем форвардер
		if err := tcpForwarder.Start(); err != nil {
			// Закрываем все соединения при ошибке
			for _, conn := range conns {
				conn.Close()
			}
			return fmt.Errorf("failed to start TCP forwarder sender: %w", err)
		}
		log.Infof("TCP forwarder sender started (relay port %d, room %s, local server %s)", firstRelayPort, firstRoomName, localServerAddr)

	} else {
		s.remote = true
		// Получатель: останавливаем WebDAV сервер перед созданием TCP listener
		if s.active {
			log.Infof("Stopping WebDAV server to enable TCP forwarding on %s", s.addr)
			s.stopLocked()
		}

		// Запускаем форвардер
		if err := tcpForwarder.Start(); err != nil {
			// Закрываем все соединения при ошибке
			for _, conn := range conns {
				conn.Close()
			}
			return fmt.Errorf("failed to start TCP forwarder receiver: %w", err)
		}

		// Создаем TCP listener на локальном порту
		localAddr := s.addr // Используем тот же порт, что и WebDAV сервер
		listener, err := net.Listen("tcp", localAddr)
		if err != nil {
			tcpForwarder.Stop()
			// Закрываем все соединения при ошибке
			for _, conn := range conns {
				conn.Close()
			}
			return fmt.Errorf("failed to create TCP listener on %s: %w", localAddr, err)
		}

		s.tcpListener = listener
		s.listenerStopChan = make(chan struct{})
		log.Infof("TCP forwarder receiver listening on %s (relay port %d, room %s)",
			localAddr, firstRelayPort, firstRoomName)

		// Запускаем горутину для принятия локальных соединений
		go s.acceptLocalConnections(tcpForwarder)

		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(true)
		}
	}

	// Сохраняем форвардер
	s.tcpForwarder = tcpForwarder
	s.tcpForwarding = true
	log.Infof("TCP forwarding successfully enabled on relay port %d (room %s)", firstRelayPort, firstRoomName)

	return nil
}

// acceptLocalConnections принимает локальные соединения и пробрасывает их через форвардер
func (s *WebDAVServer) acceptLocalConnections(forwarder *TCPForwarder) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("acceptLocalConnections panic: %v", r)
		}
	}()

	for {
		select {
		case <-s.listenerStopChan:
			log.Debugf("TCP listener stopped via stop channel")
			return
		default:
			conn, err := s.tcpListener.Accept()
			if err != nil {
				s.tcpForwardingMu.RLock()
				active := s.tcpForwarding
				s.tcpForwardingMu.RUnlock()

				if active {
					if !errors.Is(err, net.ErrClosed) {
						log.Errorf("Failed to accept connection: %v", err)
					}
				} else {
					log.Debugf("TCP listener stopped (forwarding disabled)")
					return
				}
				continue
			}

			log.Debugf("TCP forwarding: accepted connection from %s", conn.RemoteAddr())

			// Пробрасываем соединение через форвардер
			go func(c net.Conn) {
				defer func() {
					if r := recover(); r != nil {
						log.Errorf("ForwardConnection panic: %v", r)
					}
				}()
				if err := forwarder.ForwardConnection(c); err != nil {
					if !errors.Is(err, net.ErrClosed) {
						log.Errorf("Failed to forward connection: %v", err)
					}
					c.Close()
				}
			}(conn)
		}
	}
}

// DisableTCPForwarding отключает TCP портфорвардинг
func (s *WebDAVServer) DisableTCPForwarding() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.tcpForwarding {
		return
	}

	log.Info("Disabling TCP forwarding...")

	// Останавливаем горутину acceptLocalConnections
	if s.listenerStopChan != nil {
		close(s.listenerStopChan)
		s.listenerStopChan = nil
	}

	// Останавливаем TCP listener если есть
	if s.tcpListener != nil {
		s.tcpListener.Close()
		s.tcpListener = nil
	}

	// Останавливаем форвардер
	if s.tcpForwarder != nil {
		s.tcpForwarder.Stop()
		s.tcpForwarder = nil
	}

	s.tcpForwarding = false
	s.remote = false

	// Восстанавливаем WebDAV сервер, если он был остановлен
	// Используем сохраненные параметры
	select {
	case <-appCtx.Done():
	default:
		if s.onProxyStateChanged != nil {
			s.onProxyStateChanged(false)
		}
		if s.root != "" && s.addr != "" {
			log.Infof("Restarting WebDAV server on %s for %s after disabling TCP forwarding", s.addr, s.root)
			s.startLocked()
		}
	}

	log.Info("TCP forwarding disabled")
}

// startLocked запускает WebDAV сервер (вызывается под lock)
func (s *WebDAVServer) startLocked() error {
	if s.active {
		return nil // Уже запущен
	}

	// Создаём корневой каталог если не существует
	if err := os.MkdirAll(s.root, 0700); err != nil {
		log.Warnf("WebDAV: failed to create root directory %s: %v", s.root, err)
	}

	// Создаем локальный handler
	s.localHandler = s.createLocalHandler(s.root)

	// Создаем сервер с handlerRouter
	s.server = &http.Server{
		Addr:    s.addr,
		Handler: http.HandlerFunc(s.handlerRouter),
	}

	// Если нужен HTTPS, подготавливаем TLS конфиг
	if s.useTLS {
		if s.tlsConfigError != nil {
			log.Errorf("failed to use cached TLS config: %v", s.tlsConfigError)
			s.useTLS = false
		} else if s.TLSConfig != nil {
			s.server.TLSConfig = s.TLSConfig
		}
	}

	// Запускаем сервер в отдельной горутине
	go func() {
		var err error
		scheme := HTTP
		if s.useTLS {
			scheme = HTTPS
		}
		log.Infof("WebDAV on %s://%s %s", scheme, s.addr, s.root)
		if s.useTLS {
			err = s.server.ListenAndServeTLS("", "")
		} else {
			err = s.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Errorf("WebDAV listenAndServe: %v", err)
			s.mu.Lock()
			if s.active {
				s.active = false
				caffeinate(-1)
			}
			s.mu.Unlock()
		}
	}()

	s.active = true
	caffeinate(1)
	return nil
}

// IsTCPForwardingActive возвращает true если TCP форвардинг активен
func (s *WebDAVServer) IsTCPForwardingActive() bool {
	s.tcpForwardingMu.RLock()
	defer s.tcpForwardingMu.RUnlock()
	return s.tcpForwarding && s.tcpForwarder != nil
}

func (s *WebDAVServer) IsRemote() bool {
	s.tcpForwardingMu.RLock()
	defer s.tcpForwardingMu.RUnlock()
	return s.remote
}

// UpdateForwardingAddr обновляет адрес локального сервера в активном TCP форвардере
func (s *WebDAVServer) UpdateForwardingAddr(addr string) {
	s.tcpForwardingMu.Lock()
	defer s.tcpForwardingMu.Unlock()

	if s.tcpForwarder == nil {
		return
	}

	// Используем tcpListener для идентификации: nil = отправитель, не-nil = получатель
	if s.remote {
		// Получатель: пересоздаем listener на новом порту
		s.updateListenerAddr(addr)
	} else {
		// Отправитель: обновляем адрес локального WebDAV сервера
		s.tcpForwarder.UpdateLocalServerAddr(addr)
		log.Infof("TCP forwarder: updated local server address to %s (sender)", addr)
	}
}

// updateListenerAddr пересоздает listener на новом адресе без прерывания туннеля
func (s *WebDAVServer) updateListenerAddr(addr string) {
	if s.tcpListener == nil {
		return
	}

	log.Infof("TCP forwarder: updating listener address from %s to %s", s.addr, addr)

	// 1. Останавливаем старую горутину acceptLocalConnections
	close(s.listenerStopChan)
	s.listenerStopChan = make(chan struct{})

	// 2. Закрываем текущий listener
	oldListener := s.tcpListener
	s.tcpListener = nil
	oldListener.Close()
	log.Debugf("TCP forwarder: old listener closed")

	// 2. Создаем новый listener на новом адресе
	newListener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Errorf("TCP forwarder: failed to create new listener on %s: %v", addr, err)
		// Восстанавливаем старый listener
		if restoreListener, restoreErr := net.Listen("tcp", s.addr); restoreErr == nil {
			s.tcpListener = restoreListener
			log.Infof("TCP forwarder: restored listener on old address %s", s.addr)
		}
		return
	}

	s.tcpListener = newListener
	s.addr = addr
	log.Infof("TCP forwarder: new listener created on %s", addr)

	// 3. Перезапускаем acceptLocalConnections с новым listener
	go s.acceptLocalConnections(s.tcpForwarder)
	log.Infof("TCP forwarder: restarted acceptLocalConnections with new listener")
}

func defAddress(hp string, ports ...string) (host, port, address string) {
	var err error
	host, port, err = net.SplitHostPort(hp)
	// Default port to :9009
	if err != nil {
		host = hp
		port = models.DEFAULT_PORT
		for _, p := range ports {
			port = p
			break
		}
	}
	log.Debugf("got host '%v' and port '%v'", host, port)
	address = net.JoinHostPort(host, port)
	return
}
