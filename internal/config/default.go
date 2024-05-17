package config

import (
	"time"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
)

// Default return default configuration.
// nolint: mnd
func Default() Config {
	return Config{
		Logger: logger.Config{
			Level: "debug",
		},
		NATS: client.Config{
			URL:             "localhost:4222",
			PublishInterval: 2 * time.Second,
			RequestTimeout:  50 * time.Millisecond,
			DefaultSubject:  "test",
		},
	}
}
