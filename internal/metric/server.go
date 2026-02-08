package metric

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// ServerInfo contains information about metrics server.
type ServerInfo struct {
	server *http.Server
}

// Provide creates a new monitoring server.
// nolint: mnd
func Provide(lc fx.Lifecycle, cfg Config, logger *zap.Logger) ServerInfo {
	s := ServerInfo{} //nolint:exhaustruct

	if cfg.Enabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		// nolint: exhaustruct
		s.server = &http.Server{
			Addr:         cfg.Server.Address,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			if s.server != nil {
				go func() {
					if err := s.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
						logger.Error("metric server initiation failed", zap.Error(err))
					}
				}()
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			if s.server != nil {
				return s.server.Shutdown(ctx)
			}

			return nil
		},
	})

	return s
}
