package client

import (
	"time"
)

type Config struct {
	IsJetstream            bool          `json:"is_jetstream"             koanf:"is_jetstream"`
	AllExistingStreams     bool          `json:"all_existing_streams"     koanf:"all_existing_streams"`
	NewStreamAllow         bool          `json:"new_stream_allow"         koanf:"new_stream_allow"`
	Streams                []Stream      `json:"streams,omitempty"        koanf:"streams"`
	URL                    string        `json:"url,omitempty"            koanf:"url"`
	PublishInterval        time.Duration `json:"publish_interval"         koanf:"publish_interval"`
	RequestTimeout         time.Duration `json:"request_timeout"          koanf:"request_timeout"`
	MaxPubAcksInflight int           `json:"max_pub_acks_inflight" koanf:"max_pub_acks_inflight"`
	FlushTimeout       time.Duration `json:"flush_timeout"         koanf:"flush_timeout"`
	Region             string        `json:"region"                koanf:"region"`
	MaxReconnection        int           `json:"max_reconnection"         koanf:"max_reconnection"`
	MaxRetries             int           `json:"max_retries"              koanf:"max_retries"`
	RetryDelay             time.Duration `json:"retry_delay"              koanf:"retry_delay"`
	RetryBackoffMultiplier float64       `json:"retry_backoff_multiplier" koanf:"retry_backoff_multiplier"`
}

type Stream struct {
	Name    string `json:"name,omitempty"    koanf:"name"`
	Subject string `json:"subject,omitempty" koanf:"subject"`
}
