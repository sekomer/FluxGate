package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"sync"

	"github.com/fluxgate/fluxgate/internal/config"
)

type TLSManager struct {
	config    *config.TLS
	cert      *tls.Certificate
	mu        sync.RWMutex
	onChange  []func(*tls.Config)
}

func NewTLSManager(tlsConfig *config.TLS) (*TLSManager, error) {
	m := &TLSManager{
		config:   tlsConfig,
		onChange: make([]func(*tls.Config), 0),
	}

	if tlsConfig != nil && tlsConfig.CertFile != "" && tlsConfig.KeyFile != "" {
		if err := m.loadCertificate(); err != nil {
			return nil, err
		}
	}

	return m, nil
}

func (m *TLSManager) loadCertificate() error {
	cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
	if err != nil {
		return fmt.Errorf("loading TLS certificate: %w", err)
	}

	m.mu.Lock()
	m.cert = &cert
	m.mu.Unlock()

	log.Printf("Loaded TLS certificate from %s", m.config.CertFile)
	return nil
}

func (m *TLSManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.cert == nil {
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*m.cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		NextProtos:               []string{"h2", "http/1.1"},
	}
}

func (m *TLSManager) UpdateConfig(tlsConfig *config.TLS) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if tlsConfig == nil || tlsConfig.CertFile == "" || tlsConfig.KeyFile == "" {
		m.config = nil
		m.cert = nil
		log.Printf("TLS disabled")
		m.notifyListeners()
		return nil
	}

	m.config = tlsConfig
	
	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("loading new TLS certificate: %w", err)
	}

	m.cert = &cert
	log.Printf("Updated TLS certificate from %s", tlsConfig.CertFile)
	m.notifyListeners()
	
	return nil
}

func (m *TLSManager) Subscribe(fn func(*tls.Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, fn)
}

func (m *TLSManager) notifyListeners() {
	tlsConfig := m.GetTLSConfig()
	for _, fn := range m.onChange {
		go fn(tlsConfig)
	}
}

func (m *TLSManager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cert != nil
}