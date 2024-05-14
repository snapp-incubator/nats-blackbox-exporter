package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/config"
	"github.com/snapp-incubator/nats-blackbox-exporter/internal/logger"
	"go.uber.org/zap"
)

var publishInterval = 2 * time.Second

func main() {
	cfg := config.New()

	logger := logger.New(cfg.Logger)

	nc, err := nats.Connect(cfg.NATS.URL)
	if err != nil {
		logger.Fatal("nats connection failed", zap.Error(err))
	}

	logger.Info("nats connection successful",
		zap.String("connected-addr", nc.ConnectedAddr()),
		zap.Strings("discovered-servers", nc.DiscoveredServers()))

	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		logger.Fatal("nats disconnected", zap.Error(err))
	})

	nc.SetReconnectHandler(func(_ *nats.Conn) {
		logger.Warn("nats reconnected")
	})

	go subscribe(nc, logger)

	go publish(nc, logger)

	waitForSignal(logger)
}

func publish(nc *nats.Conn, logger *zap.Logger) {
	for {
		err := nc.Publish("subject1", []byte("Hello, NATS!"))
		if err != nil {
			logger.Error("Error publishing message:", zap.Error(err))
		} else {
			logger.Info("Published message successfully")
		}

		time.Sleep(publishInterval)
	}
}

func subscribe(nc *nats.Conn, logger *zap.Logger) {
	_, err := nc.Subscribe("subject1", func(msg *nats.Msg) {
		logger.Info("Received message successfully: ", zap.ByteString("message", msg.Data))
	})
	if err != nil {
		logger.Error("Error subscribing to subject:", zap.Error(err))
	}

	select {}
}

func waitForSignal(logger *zap.Logger) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("Received termination signal. Exiting...")
	os.Exit(0)
}
