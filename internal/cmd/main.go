package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/natsclient"
	"go.uber.org/zap"
)

func main(cfg config.Config, logger *zap.Logger) {
	natsConfig := cfg.NATS

	// client := natsclient.New(natsConfig, logger)
	// client.StartMessaging()

	jetstreamClient := natsclient.NewJetstream(natsConfig, logger)
	jetstreamClient.StartBlackboxTest()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	jetstreamClient.Close()
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
