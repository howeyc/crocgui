// webdav.go
package main

import (
	"context"
	"net/http"
	"sync"
	"time"

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
}

// NewWebDAVServer creates a new instance of a WebDAV server.
func NewWebDAVServer() *WebDAVServer {
	return &WebDAVServer{}
}

// Start launches the WebDAV server on the specified address with the given root directory.
func (s *WebDAVServer) Start(addr, root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop the previous server if it is active.
	if s.active {
		if s.addr == addr && s.root == root {
			return nil
		}
		s.stopLocked()
	}

	// Create the WebDAV handler with resolving filesystem
	fs := &ResolvingFileSystem{root: root}

	handler := &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			// Подробное логирование всех запросов
			log.Debugf("[WEBDAV] %s %s - User-Agent: %s, Depth: %s",
				r.Method, r.URL.Path, r.UserAgent(), r.Header.Get("Depth"))

			// Логируем все заголовки для PROPFIND (обычно используется для листинга)
			if r.Method == "PROPFIND" {
				for k, v := range r.Header {
					log.Debugf("[WEBDAV] Header %s: %s", k, v)
				}
			}

			if err != nil {
				log.Debugf("[WEBDAV] Error: %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	// Create and configure the HTTP server.
	s.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Start the server in a separate goroutine.
	go func() {
		log.Infof("WebDAV on %s %s", addr, root)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("WebDAV listenAndServe: %v", err)
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
		}
	}()

	s.active = true
	s.addr = addr
	s.root = root
	return nil
}

// Stop gracefully shuts down the server with a timeout.
func (s *WebDAVServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
