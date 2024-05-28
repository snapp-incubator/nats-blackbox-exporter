package natsclient

import (
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type NatsClient struct {
	Conn    *nats.Conn
	Logger  *zap.Logger
	Config  Config
	Metrics Metrics
}

func New(cfg Config, logger *zap.Logger) *NatsClient {
	nc := connect(logger, cfg)

	metrics := NewMetrics()

	client := &NatsClient{
		Conn:    nc,
		Logger:  logger,
		Config:  cfg,
		Metrics: NewMetrics(),
	}

	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		metrics.Connection.WithLabelValues("disconnection").Add(1)
		logger.Error("nats disconnected", zap.Error(err))
	})

	nc.SetReconnectHandler(func(_ *nats.Conn) {
		metrics.Connection.WithLabelValues("reconnection").Add(1)
		logger.Warn("nats reconnected")
	})

	return client
}

func connect(logger *zap.Logger, cfg Config) *nats.Conn {
	nc, err := nats.Connect(cfg.URL)
	if err != nil {
		logger.Fatal("nats connection failed", zap.Error(err))
	}

	logger.Info("nats connection successful",
		zap.String("connected-addr", nc.ConnectedAddr()),
		zap.Strings("discovered-servers", nc.DiscoveredServers()))

	return nc
}

func (c *NatsClient) StartMessaging() {
	go c.Subscribe("")
	go c.Publish("")
}

// func New(nc *nats.Conn, logger *zap.Logger, cfg Config) *NatsClient {
// 	return &NatsClient{
// 		Conn:   nc,
// 		Logger: logger,
// 		Config: cfg,
// 	}
// }

func (c *NatsClient) Publish(subject string) {
	if subject == "" {
		subject = c.Config.DefaultSubject
	}
	for {
		t := time.Now()
		tt, _ := t.MarshalBinary()
		msg, err := c.Conn.Request(subject, tt, c.Config.RequestTimeout)
		if err != nil {
			c.Metrics.SuccessCounter.WithLabelValues("failed publish").Add(1)
			if err == nats.ErrTimeout {
				c.Logger.Error("Request timeout: No response received within the timeout period.")
			} else if err == nats.ErrNoResponders {
				c.Logger.Error("Request failed: No responders available for the subject.")
			} else {
				c.Logger.Error("Request failed: %v", zap.Error(err))
			}
		} else {
			c.Metrics.SuccessCounter.WithLabelValues("successful publish").Add(1)
			c.Logger.Info("Received response successfully:", zap.ByteString("response", msg.Data))
		}

		time.Sleep(c.Config.PublishInterval)
	}
}

func (c *NatsClient) Subscribe(subject string) {
	if subject == "" {
		subject = c.Config.DefaultSubject
	}
	_, err := c.Conn.Subscribe(subject, func(msg *nats.Msg) {
		var publishTime time.Time
		err := publishTime.UnmarshalBinary(msg.Data)
		if err != nil {
			c.Logger.Error("Received message successfully but message is not valid time value")
		} else {
			latency := time.Since(publishTime).Seconds()
			c.Metrics.Latency.Observe(latency)
			c.Logger.Info("Received message successfully: ", zap.Float64("latency", latency))
		}
		c.Metrics.SuccessCounter.WithLabelValues("successful subscribe").Add(1)

		err = c.Conn.Publish(msg.Reply, []byte("ack!"))
		if err != nil {
			c.Logger.Error("Failed to publish response: %v", zap.Error(err))
		}
	})
	if err != nil {
		c.Logger.Error("Failed to subscribe to subject '%v': %v", zap.String(subject, subject), zap.Error(err))
	}

}
