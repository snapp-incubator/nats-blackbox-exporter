package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/natsClient"
	"go.uber.org/zap"
)

func main(cfg config.Config, logger *zap.Logger) {
	natsConfig := cfg.NATS

	client := natsClient.New(natsClient.Connect(logger, natsConfig), logger, natsConfig)

	client.StartMessaging()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
