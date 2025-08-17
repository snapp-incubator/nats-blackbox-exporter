package metric

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ServerInfo contains information about metrics server.
type ServerInfo struct {
	srv     *http.ServeMux
	address string
}

// Provide creates a new monitoring server.
func Provide(lc fx.Lifecycle, cfg Config, logger *zap.Logger) ServerInfo {
	var srv *http.ServeMux

	if cfg.Enabled {
		srv = http.NewServeMux()
		srv.Handle("/metrics", promhttp.Handler())
	}

	s := ServerInfo{
		address: cfg.Server.Address,
		srv:     srv,
	}

	lc.Append(
		fx.StartHook(func() {
			s.Start(logger)
		}),
	)

	return s
}

// Start creates and run a metric server for prometheus in new go routine.
// nolint: mnd
func (s ServerInfo) Start(logger *zap.Logger) {
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
