package metric

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server contains information about metrics server.
type Server struct {
	srv     *http.ServeMux
	address string
}

// NewServer creates a new monitoring server.
func NewServer(cfg Config) Server {
	var srv *http.ServeMux

	if cfg.Enabled {
		srv = http.NewServeMux()
		srv.Handle("/metrics", promhttp.Handler())
	}

	return Server{
		address: cfg.Address,
		srv:     srv,
	}
}

// Start creates and run a metric server for prometheus in new go routine.
// nolint: mnd
func (s Server) Start(logger *zap.Logger) {
	go func() {
		// nolint: exhaustruct
		srv := http.Server{
			Addr:         s.address,
			Handler:      s.srv,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
			TLSConfig:    nil,
		}

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metric server initiation failed", zap.Error(err))
		}
	}()
}
