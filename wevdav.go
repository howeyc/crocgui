// webdav.go
package main

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	log "github.com/schollz/logger"
	"golang.org/x/net/webdav"
)

// WebDAVServer encapsulates the state and management of a WebDAV server.
type WebDAVServer struct {
	server      *http.Server
	mu          sync.RWMutex
	active      bool
	currentAddr string
	currentDir  string
	refCount    int // счетчик активных пользователей
}

// GetWebDAVServer returns the singleton WebDAV server instance.
func GetWebDAVServer() *WebDAVServer {
	onceWebDAV.Do(func() {
		davServer = &WebDAVServer{}
	})
	return davServer
}

// normalizeAddr добавляет порт по умолчанию если не указан
func normalizeAddr(addr string) string {
	// Если адрес уже содержит порт
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}

	// Добавляем порт по умолчанию через net.JoinHostPort
	// Эта функция сама правильно обработает IPv6
	return net.JoinHostPort(addr, "8080")
}

// StartOrUpdate запускает или обновляет сервер при необходимости
func (s *WebDAVServer) StartOrUpdate(addr, dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizedAddr := normalizeAddr(addr)

	// Если сервер уже работает с теми же параметрами
	if s.active && s.currentAddr == normalizedAddr && s.currentDir == dir {
		s.refCount++
		log.Debugf("WebDAV server already running on %s, refCount: %d", normalizedAddr, s.refCount)
		return nil
	}

	// Если параметры изменились или сервер не активен
	if s.active {
		log.Debugf("Restarting WebDAV server: %s -> %s", s.currentAddr, normalizedAddr)
		if err := s.stopLocked(); err != nil {
			log.Errorf("Failed to stop old server: %v", err)
		}
	}

	// Создаем новый сервер
	return s.startLocked(normalizedAddr, dir)
}

// startLocked запускает сервер (вызывается под блокировкой)
func (s *WebDAVServer) startLocked(addr, dir string) error {
	// Create the WebDAV handler.
	fs := &webdav.Handler{
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Debugf("WebDAV request %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	// Create and configure the HTTP server.
	s.server = &http.Server{
		Addr:    addr,
		Handler: fs,
	}

	// Start the server in a separate goroutine.
	go func() {
		log.Infof("WebDAV started on %s serving %s", addr, dir)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("WebDAV listenAndServe: %v", err)
			s.mu.Lock()
			s.active = false
			s.currentAddr = ""
			s.currentDir = ""
			s.refCount = 0
			s.mu.Unlock()
		}
	}()

	s.active = true
	s.currentAddr = addr
	s.currentDir = dir
	s.refCount = 1

	return nil
}

// Stop уменьшает счетчик и останавливает сервер если нужно
func (s *WebDAVServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stopWithRefCount()
}

// stopWithRefCount уменьшает счетчик и останавливает при необходимости
func (s *WebDAVServer) stopWithRefCount() error {
	if s.refCount > 0 {
		s.refCount--
	}

	log.Debugf("Stop called, refCount: %d", s.refCount)

	// Останавливаем сервер только если нет активных пользователей
	if s.refCount == 0 && s.active {
		log.Info("Stopping WebDAV server (no active users)")
		return s.stopLocked()
	}

	return nil
}

// StopNow принудительно останавливает сервер независимо от счетчика
func (s *WebDAVServer) StopNow() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refCount = 0
	return s.stopLocked()
}

// stopLocked is an internal method for stopping the server (called while holding the lock).
func (s *WebDAVServer) stopLocked() error {
	if !s.active || s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Errorf("WebDAV shutdown: %v", err)
		return err
	}

	s.active = false
	s.server = nil
	s.currentAddr = ""
	s.currentDir = ""
	log.Info("WebDAV stopped")
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
	return s.currentAddr
}

// GetDir returns the directory being served.
func (s *WebDAVServer) GetDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentDir
}

// GetURL returns the WebDAV URL (dav://...)
func (s *WebDAVServer) GetURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.active && s.currentAddr != "" {
		return "dav://" + s.currentAddr
	}
	return ""
}

// GetStatus returns full server status
func (s *WebDAVServer) GetStatus() (addr, dir string, active bool, refCount int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentAddr, s.currentDir, s.active, s.refCount
}
