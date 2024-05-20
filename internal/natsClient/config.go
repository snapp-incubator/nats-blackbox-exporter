package natsclient

import "time"

type Config struct {
	URL             string        `json:"url,omitempty"            koanf:"url"`
	PublishInterval time.Duration `json:"publish_interval" koanf:"publish_interval"`
	RequestTimeout  time.Duration `json:"request_timeout" koanf:"request_timeout"`
	DefaultSubject  string        `json:"default_subject" koanf:"default_subject"`
}
