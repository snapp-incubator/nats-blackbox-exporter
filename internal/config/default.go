package config

import (
	"time"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/natsclient"
)

// Default return default configuration.
// nolint: mnd
func Default() Config {
	return Config{
		Logger: logger.Config{
			Level: "debug",
		},
		NATS: natsclient.Config{
			URL:             "localhost:4222",
			PublishInterval: 2 * time.Second,
			RequestTimeout:  50 * time.Millisecond,
			DefaultSubject:  "test",
		},
	}
}
