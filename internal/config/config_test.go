package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.yaml")

	configContent := `
server:
  port: 8080
  metrics_port: 9090
  gossip_port: 7946
  hot_reload: true
tls:
  cert_file: cert.pem
  key_file: key.pem
health_check:
  interval: 30s
  timeout: 10s
  path: /health
timeouts:
  read: 60s
  write: 60s
  idle: 300s
logging:
  level: debug
  format: json
cluster:
  join_address: localhost:7946
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.MetricsPort != 9090 {
		t.Errorf("Expected metrics port 9090, got %d", cfg.Server.MetricsPort)
	}
	if cfg.Server.GossipPort != 7946 {
		t.Errorf("Expected gossip port 7946, got %d", cfg.Server.GossipPort)
	}
	if !cfg.Server.HotReload {
		t.Error("Expected hot reload to be enabled")
	}

	if cfg.TLS == nil {
		t.Error("Expected TLS config, got nil")
	} else {
		if cfg.TLS.CertFile != "cert.pem" {
			t.Errorf("Expected cert file cert.pem, got %s", cfg.TLS.CertFile)
		}
		if cfg.TLS.KeyFile != "key.pem" {
			t.Errorf("Expected key file key.pem, got %s", cfg.TLS.KeyFile)
		}
	}

	if cfg.HealthCheck.Interval != 30*time.Second {
		t.Errorf("Expected health check interval 30s, got %v", cfg.HealthCheck.Interval)
	}
	if cfg.HealthCheck.Timeout != 10*time.Second {
		t.Errorf("Expected health check timeout 10s, got %v", cfg.HealthCheck.Timeout)
	}
	if cfg.HealthCheck.Path != "/health" {
		t.Errorf("Expected health check path /health, got %s", cfg.HealthCheck.Path)
	}

	if cfg.Timeouts.Read != 60*time.Second {
		t.Errorf("Expected read timeout 60s, got %v", cfg.Timeouts.Read)
	}
	if cfg.Timeouts.Write != 60*time.Second {
		t.Errorf("Expected write timeout 60s, got %v", cfg.Timeouts.Write)
	}
	if cfg.Timeouts.Idle != 300*time.Second {
		t.Errorf("Expected idle timeout 300s, got %v", cfg.Timeouts.Idle)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format json, got %s", cfg.Logging.Format)
	}

	if cfg.Cluster.JoinAddress != "localhost:7946" {
		t.Errorf("Expected join address localhost:7946, got %s", cfg.Cluster.JoinAddress)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := Load("non-existent-file.yaml")
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.MetricsPort != 9090 {
		t.Errorf("Expected default metrics port 9090, got %d", cfg.Server.MetricsPort)
	}
	if cfg.Server.GossipPort != 7946 {
		t.Errorf("Expected default gossip port 7946, got %d", cfg.Server.GossipPort)
	}
	if !cfg.Server.HotReload {
		t.Error("Expected default hot reload to be enabled")
	}

	if cfg.HealthCheck.Interval != 10*time.Second {
		t.Errorf("Expected default health check interval 10s, got %v", cfg.HealthCheck.Interval)
	}
	if cfg.HealthCheck.Timeout != 5*time.Second {
		t.Errorf("Expected default health check timeout 5s, got %v", cfg.HealthCheck.Timeout)
	}
	if cfg.HealthCheck.Path != "/health" {
		t.Errorf("Expected default health check path /health, got %s", cfg.HealthCheck.Path)
	}

	if cfg.Timeouts.Read != 30*time.Second {
		t.Errorf("Expected default read timeout 30s, got %v", cfg.Timeouts.Read)
	}
	if cfg.Timeouts.Write != 30*time.Second {
		t.Errorf("Expected default write timeout 30s, got %v", cfg.Timeouts.Write)
	}
	if cfg.Timeouts.Idle != 120*time.Second {
		t.Errorf("Expected default idle timeout 120s, got %v", cfg.Timeouts.Idle)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Expected default log format text, got %s", cfg.Logging.Format)
	}

	if cfg.TLS != nil {
		t.Error("Expected TLS to be nil by default")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid default config",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				HealthCheck: HealthConfig{
					Interval: 10 * time.Second,
					Timeout:  5 * time.Second,
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
			},
			wantErr: false,
		},
		{
			name: "invalid server port - too low",
			config: Config{
				Server: ServerConfig{
					Port:        -1,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid server port - too high",
			config: Config{
				Server: ServerConfig{
					Port:        70000,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
			},
			wantErr: true,
		},
		{
			name: "port conflict - server and metrics",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 8080,
					GossipPort:  7946,
				},
			},
			wantErr: true,
		},
		{
			name: "port conflict - server and gossip",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  8080,
				},
			},
			wantErr: true,
		},
		{
			name: "port conflict - metrics and gossip",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  9090,
				},
			},
			wantErr: true,
		},
		{
			name: "health check interval too short",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				HealthCheck: HealthConfig{
					Interval: 500 * time.Millisecond,
					Timeout:  5 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "health check timeout too short",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				HealthCheck: HealthConfig{
					Interval: 10 * time.Second,
					Timeout:  500 * time.Millisecond,
				},
			},
			wantErr: true,
		},
		{
			name: "health check timeout >= interval",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				HealthCheck: HealthConfig{
					Interval: 5 * time.Second,
					Timeout:  5 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				Logging: LoggingConfig{
					Level: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				Logging: LoggingConfig{
					Format: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "TLS missing cert file",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				TLS: &TLS{
					KeyFile: "key.pem",
				},
			},
			wantErr: true,
		},
		{
			name: "TLS missing key file",
			config: Config{
				Server: ServerConfig{
					Port:        8080,
					MetricsPort: 9090,
					GossipPort:  7946,
				},
				TLS: &TLS{
					CertFile: "cert.pem",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.setDefaults()

			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigManager(t *testing.T) {
	manager := NewManager()

	cfg := manager.Get()
	if cfg.Server.Port != 0 {
		t.Errorf("Expected initial config to have zero port, got %d", cfg.Server.Port)
	}

	callbackChan := make(chan bool, 1)
	manager.Subscribe(func(cfg *Config) {
		callbackChan <- true
	})

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.yaml")

	configContent := `
server:
  port: 8080
  metrics_port: 9090
  gossip_port: 7946
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	err = manager.Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	select {
	case <-callbackChan:
	case <-time.After(100 * time.Millisecond):
		t.Error("Subscriber was not called within timeout")
	}

	cfg = manager.Get()
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestConvenienceMethods(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port:        8080,
			MetricsPort: 9090,
			GossipPort:  7946,
			HotReload:   true,
		},
		HealthCheck: HealthConfig{
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
	}

	if cfg.GetPort() != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.GetPort())
	}
	if cfg.GetMetricsPort() != 9090 {
		t.Errorf("Expected metrics port 9090, got %d", cfg.GetMetricsPort())
	}
	if cfg.GetGossipPort() != 7946 {
		t.Errorf("Expected gossip port 7946, got %d", cfg.GetGossipPort())
	}
	if !cfg.IsHotReloadEnabled() {
		t.Error("Expected hot reload to be enabled")
	}
	if cfg.GetHealthCheckInterval() != 30*time.Second {
		t.Errorf("Expected health check interval 30s, got %v", cfg.GetHealthCheckInterval())
	}
	if cfg.GetHealthCheckTimeout() != 10*time.Second {
		t.Errorf("Expected health check timeout 10s, got %v", cfg.GetHealthCheckTimeout())
	}
}
