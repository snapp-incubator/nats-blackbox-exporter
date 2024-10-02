package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/snapp-incubator/nats-blackbox-exporter/internal/client"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"go.uber.org/zap"
)

func main(cfg config.Config, logger *zap.Logger) {
	ctx := context.Background()
	jetstreamClient := client.New(cfg.NATS, logger)
	jetstreamClient.StartBlackboxTest(ctx)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	jetstreamClient.Close()
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
