package config

import (
	"time"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/metric"
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
			Stream: natsclient.Stream{
				Name:    "stream",
				Subject: "test",
			},
			URL:                    "localhost:4222",
			PublishInterval:        2 * time.Second,
			RequestTimeout:         50 * time.Millisecond,
			MaxPubAcksInflight:     1000,
			QueueSubscriptionGroup: "group",
			FlushTimeout:           2 * time.Second,
		},
		Metric: metric.Config{
			Server:  metric.Server{Address: ":8080"},
			Enabled: true,
		},
	}
}
