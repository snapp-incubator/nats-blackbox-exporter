package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
)

func main() {
	cfg := config.New()
	natsConfig := cfg.NATS

	logger := logger.New(cfg.Logger)

	nc := client.Connect(logger, natsConfig)

	natsClient := client.New(nc, logger, natsConfig)

	go natsClient.Subscribe("subject1")

	go natsClient.Publish("subject1")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
