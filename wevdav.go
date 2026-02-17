// webdav.go
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gomime "github.com/cubewise-code/go-mime"
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
}

// WebDAVWithDirectoryListing оборачивает стандартный WebDAV handler для поддержки
// красивого отображения директорий при GET запросах
type WebDAVWithDirectoryListing struct {
	webdavHandler *webdav.Handler
	fileSystem    webdav.FileSystem
}

func (h *WebDAVWithDirectoryListing) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Отдаем встроенную иконку для favicon запросов
	if r.URL.Path == "/favicon.ico" {
		log.Debugf("Request %s %s", r.Method, r.URL.Path)
		h.serveFavicon(w, r)
		return
	}
	// Для GET запросов проверяем, является ли путь директорией
	if r.Method == http.MethodGet {
		log.Debugf("Request %s %s", r.Method, r.URL.Path)
		// Используем ту же FileSystem, что и webdav.Handler
		if info, err := h.fileSystem.Stat(context.Background(), r.URL.Path); err == nil && info.IsDir() {
			// Это директория - показываем список файлов
			h.serveDirectoryListing(w, r)
			return
		}

		// Если это файл, устанавливаем правильный Content-Type
		if info, err := h.fileSystem.Stat(context.Background(), r.URL.Path); err == nil && !info.IsDir() {
			// Определяем MIME-тип по расширению через go-mime
			ext := filepath.Ext(r.URL.Path)
			mimeType := gomime.TypeByExtension(ext)

			if mimeType != "" {
				// Для FLAC используем правильный тип
				if ext == ".flac" {
					mimeType = "audio/flac"
				}
				w.Header().Set("Content-Type", mimeType)

				// Добавляем заголовки для поддержки потокового воспроизведения
				if strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "video/") {
					w.Header().Set("Accept-Ranges", "bytes")
				}
			} else {
				// Если тип не определен, используем DetectContentType как fallback
				file, err := h.fileSystem.OpenFile(context.Background(), r.URL.Path, os.O_RDONLY, 0)
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

	// Для всех остальных случаев используем стандартный WebDAV handler
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

// NewWebDAVServer creates a new instance of a WebDAV server.
func NewWebDAVServer() *WebDAVServer {
	return &WebDAVServer{}
}

// Start launches the WebDAV server on the specified address with the given root directory.
func (s *WebDAVServer) Start(addr, root string, useTLS bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop the previous server if it is active.
	if s.active {
		if s.addr == addr && s.root == root && s.useTLS == useTLS {
			return nil
		}
		s.stopLocked()
	}
	caffeinate(1)
	s.useTLS = useTLS

	// Определяем какую FileSystem использовать
	var fs webdav.FileSystem
	if base := filepath.Base(root); !CanCreateSymlinks() && base == SEND {
		fs = &ResolvingFileSystem{root: root}
	} else {
		fs = webdav.Dir(root)
	}

	// Создаем стандартный WebDAV handler
	webdavHandler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			// if r.Method == "PROPFIND" {
			// 	for k, v := range r.Header {
			// 		log.Debugf("Request Header %s: %s", k, v)
			// 	}
			// }

			if err != nil {
				log.Errorf("Request %s %s: %v", r.Method, r.URL.Path, err)
			} else {
				log.Debugf("Request %s %s", r.Method, r.URL.Path)
			}
		},
	}

	// Оборачиваем handler для поддержки листинга директорий
	handler := &WebDAVWithDirectoryListing{
		webdavHandler: webdavHandler,
		fileSystem:    fs,
	}

	s.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Если нужен HTTPS, генерируем TLS конфиг в памяти для этого адреса
	if useTLS {
		tlsConfig, err := GenerateTLSConfig(addr)
		if err != nil {
			return fmt.Errorf("failed to generate TLS config: %v", err)
		}
		s.server.TLSConfig = tlsConfig
	}

	// Start the server in a separate goroutine.
	go func() {
		var err error
		scheme := "http"
		if useTLS {
			scheme = "https"
			log.Infof("WebDAV on %s://%s %s", scheme, addr, root)
			err = s.server.ListenAndServeTLS("", "")
		} else {
			log.Infof("WebDAV on %s://%s %s", scheme, addr, root)
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

// GenerateTLSConfig генерирует TLS конфигурацию с самоподписанным сертификатом в памяти.
// Ожидает addr в формате "IP:Port" (например, "192.168.0.107:8443").
func GenerateTLSConfig(addr string) (*tls.Config, error) {
	// 1. Извлекаем IP из строки адреса
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address format, expected IP:Port: %v", err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address in addr: %s", host)
	}

	// 2. Генерируем приватный ключ (RSA 2048 бит)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	// 3. Настраиваем сроки действия (на 1 год)
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	// 4. Генерируем уникальный серийный номер
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %v", err)
	}

	// 5. Создаем шаблон сертификата
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"crocgui"},
			CommonName:   host, // Для совместимости со старым софтом
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		// КРИТИЧЕСКИ ВАЖНО для Windows: заполняем Subject Alternative Name (SAN)
		IPAddresses: []net.IP{ip},
	}

	// 6. Создаем сам сертификат (самоподписанный)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %v", err)
	}

	// 7. Формируем структуру tls.Certificate для использования в сервере
	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	// 8. Возвращаем итоговый конфиг (требуем минимум TLS 1.2)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Stop gracefully shuts down the server with a timeout.
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

// GetAddr returns the address on which the server is running (if active).
func (s *WebDAVServer) GetAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.server != nil {
		return s.server.Addr
	}
	return ""
}
