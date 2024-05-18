package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/metric"
)

func main() {
	cfg := config.New()
	natsConfig := cfg.NATS

	logger := logger.New(cfg.Logger)

	metric.NewServer(cfg.Metric).Start(logger.Named("metric"))

	natsClient := client.New(logger, natsConfig)

	go natsClient.Subscribe("")

	go natsClient.Publish("")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
