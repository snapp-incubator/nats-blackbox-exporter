package natsclient

import "time"

type Config struct {
	AllExistingStreams     bool           `json:"all_existing_streams" koanf:"all_existing_streams"`
	NewStreamAllow         bool           `json:"new_stream_allow" koanf:"new_stream_allow"`
	StreamsConfig          []StreamConfig `json:"streams,omitempty"            koanf:"streams"`
	URL                    string         `json:"url,omitempty"            koanf:"url"`
	PublishInterval        time.Duration  `json:"publish_interval" koanf:"publish_interval"`
	RequestTimeout         time.Duration  `json:"request_timeout" koanf:"request_timeout"`
	MaxPubAcksInflight     int            `json:"max_pub_acks_inflight" koanf:"max_pub_acks_inflight"`
	QueueSubscriptionGroup string         `json:"queue_subscription_group" koanf:"queue_subscription_group"`
	FlushTimeout           time.Duration  `json:"flush_timeout" koanf:"flush_timeout"`
	ClientName             string         `json:"client_name" koanf:"client_name"`
}

type StreamConfig struct {
	Name    string `json:"name,omitempty"            koanf:"name"`
	Subject string `json:"subject,omitempty"            koanf:"subject"`
}
