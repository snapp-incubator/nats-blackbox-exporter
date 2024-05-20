package natsclient

import "time"

type Config struct {
	URL                    string        `json:"url,omitempty"            koanf:"url"`
	PublishInterval        time.Duration `json:"publish_interval" koanf:"publish_interval"`
	RequestTimeout         time.Duration `json:"request_timeout" koanf:"request_timeout"`
	DefaultSubject         string        `json:"default_subject" koanf:"default_subject"`
	MaxPubAcksInflight     int           `json:"max_pubAcks_inflight" koanf:"max_pubAcks_inflight"`
	QueueSubscriptionGroup string        `json:"queue_subscription_group" koanf:"queue_subscription_group"`
	FlushTimeout           time.Duration `json:"flush_timeout" koanf:"flush_timeout"`
}
