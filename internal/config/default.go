package config

import (
	"time"

	"go.uber.org/fx"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/metric"
)

// Default return default configuration.
// nolint: mnd
func Default() Config {
	return Config{
		Out: fx.Out{},
		Logger: logger.Config{
			Level: "debug",
		},
		NATS: client.Config{
			IsJetstream:            true,
			NewStreamAllow:         true,
			AllExistingStreams:     false,
			Streams:                []client.Stream{},
			URL:                    "localhost:4222",
			PublishInterval:        2 * time.Second,
			RequestTimeout:         50 * time.Millisecond,
			MaxPubAcksInflight:     1000,
			FlushTimeout:           2 * time.Second,
			Region:                 "-",
			MaxReconnection:        100,
			MaxRetries:             10,
			RetryDelay:             1 * time.Second,
			RetryBackoffMultiplier: 2.0,
		},
		Metric: metric.Config{
			Server:  metric.Server{Address: ":8080"},
			Enabled: true,
		},
	}
}
