package cmd

import (
	"os"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/metric"
	"github.com/spf13/cobra"
)

// ExitFailure status code.
const ExitFailure = 1

func Execute() {
	cfg := config.New()

	logger := logger.New(cfg.Logger)

	metric.NewServer(cfg.Metric).Start(logger.Named("metric"))

	// nolint: exhaustruct
	root := &cobra.Command{
		Use:   "nats-blackbox-exporter",
		Short: "ping pong with nats broker",
		Run: func(_ *cobra.Command, _ []string) {
			main(cfg, logger)
		},
	}

	if err := root.Execute(); err != nil {
		os.Exit(ExitFailure)
	}
}
