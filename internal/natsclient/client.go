package natsclient

import (
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type NatsClient struct {
	Conn   *nats.Conn
	Logger *zap.Logger
	Config Config
}

func Connect(logger *zap.Logger, cfg Config) *nats.Conn {
	nc, err := nats.Connect(cfg.URL)
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

	return nc
}

func New(nc *nats.Conn, logger *zap.Logger, cfg Config) *NatsClient {
	return &NatsClient{
		Conn:   nc,
		Logger: logger,
		Config: cfg,
	}
}

func (c *NatsClient) StartMessaging() {
	go c.Subscribe("")
	go c.Publish("")
}

func (c *NatsClient) Publish(subject string) {
	if subject == "" {
		subject = c.Config.DefaultSubject
	}
	for {
		msg, err := c.Conn.Request(subject, []byte("Hello, NATS!"), c.Config.RequestTimeout)
		if err != nil {
			if err == nats.ErrTimeout {
				c.Logger.Error("Request timeout: No response received within the timeout period.")
			} else if err == nats.ErrNoResponders {
				c.Logger.Error("Request failed: No responders available for the subject.")
			} else {
				c.Logger.Error("Request failed: %v", zap.Error(err))
			}
		} else {
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
		c.Logger.Info("Received message successfully: ", zap.ByteString("message", msg.Data))
		err := c.Conn.Publish(msg.Reply, []byte("Hi!"))
		if err != nil {
			c.Logger.Error("Failed to publish response: %v", zap.Error(err))
		}
	})
	if err != nil {
		c.Logger.Error("Failed to subscribe to subject 'subject1': %v", zap.Error(err))
	}

}
