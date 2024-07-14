package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/natsclient"
	"go.uber.org/zap"
)

func main(cfg config.Config, logger *zap.Logger) {
	natsConfig := cfg.NATS

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jetstreamClient := natsclient.NewJetstream(natsConfig, logger, &ctx)
	jetstreamClient.StartBlackboxTest()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	jetstreamClient.Close()
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
