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
	"path/filepath"
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
	useTLS bool
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

	handler := &webdav.Handler{
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			// if r.Method == "PROPFIND" {
			// 	for k, v := range r.Header {
			// 		log.Debugf("http.Request Header %s: %s", k, v)
			// 	}
			// }

			if err != nil {
				log.Errorf("http.Request %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}

	if base := filepath.Base(root); !CanCreateSymlinks() && base == SEND {
		handler.FileSystem = &ResolvingFileSystem{root: root}
	} else {
		handler.FileSystem = webdav.Dir(root)
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
