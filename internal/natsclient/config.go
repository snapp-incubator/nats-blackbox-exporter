package natsclient

import "time"

type Config struct {
	Stream                 Stream        `json:"stream,omitempty"            koanf:"stream"`
	URL                    string        `json:"url,omitempty"            koanf:"url"`
	PublishInterval        time.Duration `json:"publish_interval" koanf:"publish_interval"`
	RequestTimeout         time.Duration `json:"request_timeout" koanf:"request_timeout"`
	DefaultSubject         string        `json:"default_subject" koanf:"default_subject"`
	MaxPubAcksInflight     int           `json:"max_pub_acks_inflight" koanf:"max_pub_acks_inflight"`
	QueueSubscriptionGroup string        `json:"queue_subscription_group" koanf:"queue_subscription_group"`
	FlushTimeout           time.Duration `json:"flush_timeout" koanf:"flush_timeout"`
}

type Stream struct {
	Name    string `json:"name,omitempty"            koanf:"name"`
	Subject string `json:"subject,omitempty"            koanf:"subject"`
}
