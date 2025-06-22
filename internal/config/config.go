package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig  `yaml:"server"`
	TLS         *TLS          `yaml:"tls,omitempty"`
	HealthCheck HealthConfig  `yaml:"health_check,omitempty"`
	Timeouts    TimeoutConfig `yaml:"timeouts,omitempty"`
	Logging     LoggingConfig `yaml:"logging,omitempty"`
	Cluster     ClusterConfig `yaml:"cluster,omitempty"`
}

type ServerConfig struct {
	Port        int  `yaml:"port,omitempty"`
	MetricsPort int  `yaml:"metrics_port,omitempty"`
	GossipPort  int  `yaml:"gossip_port,omitempty"`
	HotReload   bool `yaml:"hot_reload,omitempty"`
}

type HealthConfig struct {
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
	Path     string        `yaml:"path,omitempty"`
}

type TimeoutConfig struct {
	Read  time.Duration `yaml:"read,omitempty"`
	Write time.Duration `yaml:"write,omitempty"`
	Idle  time.Duration `yaml:"idle,omitempty"`
}

type LoggingConfig struct {
	Level  string `yaml:"level,omitempty"`
	Format string `yaml:"format,omitempty"`
}

type ClusterConfig struct {
	JoinAddress string `yaml:"join_address,omitempty"`
}

type TLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type Manager struct {
	config    *Config
	mu        sync.RWMutex
	listeners []func(*Config)
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
					HotReload:   true,
				},
				HealthCheck: HealthConfig{
					Interval: 10 * time.Second,
					Timeout:  5 * time.Second,
					Path:     "/health",
				},
				Timeouts: TimeoutConfig{
					Read:  30 * time.Second,
					Write: 30 * time.Second,
					Idle:  120 * time.Second,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
			}, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.setDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.MetricsPort == 0 {
		c.Server.MetricsPort = 9090
	}
	if c.Server.GossipPort == 0 {
		c.Server.GossipPort = 7946
	}

	if c.HealthCheck.Interval == 0 {
		c.HealthCheck.Interval = 10 * time.Second
	}
	if c.HealthCheck.Timeout == 0 {
		c.HealthCheck.Timeout = 5 * time.Second
	}
	if c.HealthCheck.Path == "" {
		c.HealthCheck.Path = "/health"
	}

	if c.Timeouts.Read == 0 {
		c.Timeouts.Read = 30 * time.Second
	}
	if c.Timeouts.Write == 0 {
		c.Timeouts.Write = 30 * time.Second
	}
	if c.Timeouts.Idle == 0 {
		c.Timeouts.Idle = 120 * time.Second
	}

	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.MetricsPort < 1 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535, got %d", c.Server.MetricsPort)
	}
	if c.Server.GossipPort < 1 || c.Server.GossipPort > 65535 {
		return fmt.Errorf("gossip port must be between 1 and 65535, got %d", c.Server.GossipPort)
	}

	if c.Server.Port == c.Server.MetricsPort {
		return fmt.Errorf("server port and metrics port cannot be the same: %d", c.Server.Port)
	}
	if c.Server.Port == c.Server.GossipPort {
		return fmt.Errorf("server port and gossip port cannot be the same: %d", c.Server.Port)
	}
	if c.Server.MetricsPort == c.Server.GossipPort {
		return fmt.Errorf("metrics port and gossip port cannot be the same: %d", c.Server.MetricsPort)
	}

	if c.HealthCheck.Interval < time.Second {
		return fmt.Errorf("health check interval must be at least 1s, got %v", c.HealthCheck.Interval)
	}
	if c.HealthCheck.Timeout < time.Second {
		return fmt.Errorf("health check timeout must be at least 1s, got %v", c.HealthCheck.Timeout)
	}
	if c.HealthCheck.Timeout >= c.HealthCheck.Interval {
		return fmt.Errorf("health check timeout (%v) must be less than interval (%v)", c.HealthCheck.Timeout, c.HealthCheck.Interval)
	}

	if c.Timeouts.Read < time.Second {
		return fmt.Errorf("read timeout must be at least 1s, got %v", c.Timeouts.Read)
	}
	if c.Timeouts.Write < time.Second {
		return fmt.Errorf("write timeout must be at least 1s, got %v", c.Timeouts.Write)
	}
	if c.Timeouts.Idle < time.Second {
		return fmt.Errorf("idle timeout must be at least 1s, got %v", c.Timeouts.Idle)
	}

	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level '%s', must be one of: debug, info, warn, error", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"text": true, "json": true,
	}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		return fmt.Errorf("invalid log format '%s', must be one of: text, json", c.Logging.Format)
	}

	if c.TLS != nil {
		if c.TLS.CertFile == "" {
			return fmt.Errorf("tls cert_file is required when TLS is enabled")
		}
		if c.TLS.KeyFile == "" {
			return fmt.Errorf("tls key_file is required when TLS is enabled")
		}
	}

	return nil
}

func (c *Config) GetPort() int {
	return c.Server.Port
}

func (c *Config) GetMetricsPort() int {
	return c.Server.MetricsPort
}

func (c *Config) GetGossipPort() int {
	return c.Server.GossipPort
}

func (c *Config) IsHotReloadEnabled() bool {
	return c.Server.HotReload
}

func (c *Config) GetHealthCheckInterval() time.Duration {
	return c.HealthCheck.Interval
}

func (c *Config) GetHealthCheckTimeout() time.Duration {
	return c.HealthCheck.Timeout
}

func NewManager() *Manager {
	return &Manager{
		config:    &Config{},
		listeners: make([]func(*Config), 0),
	}
}

func (m *Manager) Load(filename string) error {
	cfg, err := Load(filename)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.config = cfg
	listeners := m.listeners
	m.mu.Unlock()

	for _, listener := range listeners {
		go listener(cfg)
	}

	return nil
}

func (m *Manager) Subscribe(listener func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}
