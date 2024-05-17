package client

import "time"

type Config struct {
	URL             string        `json:"url,omitempty"            koanf:"url"`
	PublishInterval time.Duration `json:"publish_interval" koanf:"publish_interval"`
}
