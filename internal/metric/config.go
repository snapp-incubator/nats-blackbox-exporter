package metric

type Config struct {
	Server  Server `json:"server,omitempty"  koanf:"server"`
	Enabled bool   `json:"enabled,omitempty" koanf:"enabled"`
}

type Server struct {
	Address string `json:"address,omitempty" koanf:"address"`
}
