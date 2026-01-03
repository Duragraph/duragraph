// Package config provides configuration for the Go mock worker.
package config

import (
	"time"

	"github.com/caarlos0/env/v10"
)

// Config holds the worker configuration.
type Config struct {
	// Control plane connection
	ControlPlaneURL string `env:"CONTROL_PLANE_URL" envDefault:"http://localhost:8081"`

	// Worker identity
	WorkerID   string `env:"WORKER_ID"`
	WorkerName string `env:"WORKER_NAME" envDefault:"go-mock-worker"`

	// Graph configuration
	MockGraph         string `env:"MOCK_WORKER_GRAPH" envDefault:"simple_echo"`
	MockDelayMS       int    `env:"MOCK_WORKER_DELAY_MS" envDefault:"100"`
	MockFailAtNode    string `env:"MOCK_WORKER_FAIL_AT_NODE"`
	MockInterruptNode string `env:"MOCK_WORKER_INTERRUPT_AT_NODE"`
	MockTokenCount    int    `env:"MOCK_WORKER_TOKEN_COUNT" envDefault:"100"`

	// Worker behavior
	HeartbeatInterval time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"10s"`
	MaxConcurrentRuns int           `env:"MAX_CONCURRENT_RUNS" envDefault:"5"`
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"1s"`

	// Logging
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// MockDelay returns the mock delay as a duration.
func (c *Config) MockDelay() time.Duration {
	return time.Duration(c.MockDelayMS) * time.Millisecond
}
