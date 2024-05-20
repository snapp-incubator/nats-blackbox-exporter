package natsclient

import (
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

type Message struct {
	Subject string
	Data    []byte
}

// Jetstream represents the NATS core handler
type Jetstream struct {
	connection *nats.Conn
	jetstream  nats.JetStreamContext
	config     *Config
	logger     *zap.Logger
}

// NewJetstream initializes NATS JetStream connection
func NewJetstream(config Config, logger *zap.Logger) *Jetstream {
	j := Jetstream{
		config: &config,
		logger: logger,
	}

	var err error
	j.connection, err = nats.Connect(j.config.URL)
	if err != nil {
		logger.Panic("could not connect to nats", zap.Error(err))
	}

	// JetStream connection
	j.jetstream, err = j.connection.JetStream()
	if err != nil {
		logger.Panic("could not connect to jetstream", zap.Error(err))
	}

	// Create a stream
	_, err = j.jetstream.AddStream(&nats.StreamConfig{
		Name:     "test",
		Subjects: []string{"test.*"},
	})
	if err != nil {
		logger.Panic("could not add stream", zap.Error(err))
	}

	return &j
}

func (j *Jetstream) StartJetstreamMessaging() {
	messageChannel := j.CreateSubscribe("test.1")
	go j.JetstreamPublish("test.1")
	go j.JetstreamSubscribe(messageChannel)
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages
func (j *Jetstream) CreateSubscribe(subject string) chan *Message {

	messageHandler, h := messageHandlerFactoryJetstream()
	_, err := j.jetstream.Subscribe(
		subject,
		messageHandler,
		nats.DeliverNew(),
		nats.ReplayInstant(),
		nats.AckExplicit(),
		nats.MaxAckPending(j.config.MaxPubAcksInflight),
	)
	if err != nil {
		j.logger.Error("could not Subscribe", zap.Error(err))
	} else {
		j.logger.Info("Subscribed to %s successfully", zap.String("subject", subject))
	}

	return h

}

func (j *Jetstream) JetstreamSubscribe(h chan *Message) {
	for msg := range h {
		var publishTime time.Time
		publishTime.UnmarshalBinary(msg.Data)
		j.logger.Info("Received message: ", zap.String("subject", msg.Subject), zap.Float64("latency", time.Since(publishTime).Seconds()))
	}
}

func (j *Jetstream) JetstreamPublish(subject string) {
	for {
		t := time.Now()
		tt, _ := t.MarshalBinary()

		if ack, err := j.jetstream.Publish(subject, tt); err != nil {
			j.logger.Error("jetstream publish message failed", zap.Error(err))
		} else {
			j.logger.Info("receive ack", zap.String("ack", ack.Stream))
		}
		time.Sleep(j.config.PublishInterval)
	}
}

// Close closes NATS connection
func (j *Jetstream) Close() {
	if err := j.connection.FlushTimeout(j.config.FlushTimeout); err != nil {
		j.logger.Error("could not flush", zap.Error(err))
	}

	j.connection.Close()
	j.logger.Info("NATS is closed.")
}

func messageHandlerFactoryJetstream() (nats.MsgHandler, chan *Message) {
	ch := make(chan *Message)
	return func(msg *nats.Msg) {
		ch <- &Message{
			Subject: msg.Subject,
			Data:    msg.Data,
		}
		msg.Ack()
	}, ch
}
