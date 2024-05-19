package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"go.uber.org/zap"
)

func main(cfg config.Config, logger *zap.Logger) {
	natsConfig := cfg.NATS

	natsClient := client.New(client.Connect(logger, natsConfig), logger, natsConfig)

	natsClient.StartMessaging()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
