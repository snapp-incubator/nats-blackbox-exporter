package metric

type Config struct {
	Address string `json:"address,omitempty" koanf:"address"`
	Enabled bool   `json:"enabled,omitempty" koanf:"enabled"`
}
