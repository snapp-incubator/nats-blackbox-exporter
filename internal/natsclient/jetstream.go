package natsclient

import (
	"context"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	successfulSubscribe = "successful subscribe"
	failedPublish       = "failed publish"
	successfulPublish   = "successful publish"
	subjectSuffix       = "_blackbox_exporter"
)

type Message struct {
	Subject string
	Data    []byte
}

type CreatedStream struct {
	name         string
	subject      string
	streamClient jetstream.Stream
}

// Jetstream represents the NATS core handler
type JetstreamClient struct {
	connection     *nats.Conn
	jetstream      jetstream.JetStream
	config         *Config
	logger         *zap.Logger
	metrics        Metrics
	ctx            context.Context
	createdStreams []CreatedStream
}

// NewJetstream initializes NATS JetStream connection
func NewJetstream(config Config, logger *zap.Logger, ctx *context.Context) *JetstreamClient {
	j := &JetstreamClient{
		config:  &config,
		logger:  logger,
		metrics: NewMetrics(),
		ctx:     *ctx,
	}

	j.connect()

	j.createJetstreamContext()

	j.UpdateOrCreateStream()

	return j
}

func (j *JetstreamClient) connect() {
	var err error
	j.connection, err = nats.Connect(j.config.URL)
	if err != nil {
		j.logger.Panic("could not connect to nats", zap.Error(err))
	}

	j.connection.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		j.metrics.Connection.WithLabelValues("disconnection").Add(1)
		j.logger.Error("nats disconnected", zap.Error(err))
	})

	j.connection.SetReconnectHandler(func(_ *nats.Conn) {
		j.logger.Warn("nats reconnected")
		j.metrics.Connection.WithLabelValues("reconnection").Add(1)
	})
}

func (j *JetstreamClient) createJetstreamContext() {
	var err error
	j.jetstream, err = jetstream.New(j.connection)
	if err != nil {
		j.logger.Panic("could not connect to jetstream", zap.Error(err))
	}
}

func (j *JetstreamClient) UpdateOrCreateStream() {
	for _, stream := range j.config.StreamsConfig {
		name := stream.Name
		subject := stream.Subject
		if subject == "" {
			subject = stream.Name + subjectSuffix
		}

		var str jetstream.Stream
		s, err := j.jetstream.Stream(j.ctx, name)
		if err == nil {
			info, _ := s.Info(j.ctx)
			str, err = j.updateStream(name, subject, info.Config)
		} else if err == jetstream.ErrStreamNotFound {
			str, err = j.createStream(name, subject)
		} else {
			j.logger.Error("could not add subject", zap.String("stream", stream.Name), zap.Error(err))
			continue
		}

		if err != nil {
			j.logger.Error("could not add subject", zap.String("stream", stream.Name), zap.Error(err))
			continue
		}

		j.createdStreams = append(j.createdStreams, CreatedStream{
			name:         name,
			subject:      subject,
			streamClient: str,
		})
		j.logger.Info("create or update stream", zap.String("stream", stream.Name))
	}
}

func (j *JetstreamClient) updateStream(name string, subject string, config jetstream.StreamConfig) (jetstream.Stream, error) {
	subjects := append(config.Subjects, subject)
	slices.Sort(subjects)
	config.Subjects = slices.Compact(subjects)
	str, err := j.jetstream.UpdateStream(j.ctx, config)
	if err != nil {
		j.logger.Error("could not add subject to existing stream", zap.String("stream", name), zap.Error(err))
		return nil, err
	}
	j.logger.Info("stream updated")
	return str, nil
}

func (j *JetstreamClient) createStream(name string, subject string) (jetstream.Stream, error) {
	str, err := j.jetstream.CreateStream(j.ctx, jetstream.StreamConfig{
		Name:     name,
		Subjects: []string{subject},
	})
	if err != nil {
		j.logger.Error("could not add stream", zap.String("stream", name), zap.Error(err))
		return nil, err
	}
	j.logger.Info("add new stream")
	return str, nil
}

func (j *JetstreamClient) StartBlackboxTest() {
	if j.createdStreams == nil {
		j.logger.Panic("at least one stream is required.")
	}
	for _, stream := range j.createdStreams {
		consumer := j.createConsumer(stream.streamClient, stream.subject)
		go j.jetstreamPublish(stream.subject, stream.name)
		go j.jetstreamSubscribe(consumer, stream.name)
	}
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages
func (j *JetstreamClient) createConsumer(str jetstream.Stream, subject string) jetstream.Consumer {

	c, err := str.CreateOrUpdateConsumer(j.ctx, jetstream.ConsumerConfig{
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: subject,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		j.logger.Panic("could not Subscribe", zap.Error(err))
	} else {
		j.logger.Info("Subscribed to %s successfully", zap.String("stream", str.CachedInfo().Config.Name))
	}

	return c
}

func (j *JetstreamClient) jetstreamSubscribe(c jetstream.Consumer, streamName string) {
	messageHandler, h := j.messageHandlerFactoryJetstream()

	cc, err := c.Consume(messageHandler)
	if err != nil {
		j.logger.Fatal("unable to consume messages", zap.Error(err))
	}
	defer cc.Stop()

	clusterName := j.connection.ConnectedClusterName()
	for msg := range h {
		var publishTime time.Time
		err = publishTime.UnmarshalBinary(msg.Data)
		if err != nil {
			j.logger.Error("unable to unmarshal binary data for publishTime.")
			j.logger.Info("received message but could not calculate latency due to unmarshalling error.", zap.String("subject", msg.Subject))
			return
		}
		latency := time.Since(publishTime).Seconds()
		j.metrics.Latency.With(prometheus.Labels{
			"stream":  streamName,
			"cluster": clusterName,
		}).Observe(latency)
		j.metrics.SuccessCounter.With(prometheus.Labels{
			"type":    successfulSubscribe,
			"stream":  streamName,
			"cluster": clusterName,
		}).Add(1)
		j.logger.Info("Received message: ", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
	}
}

func (j *JetstreamClient) jetstreamPublish(subject string, streamName string) {
	clusterName := j.connection.ConnectedClusterName()
	for {
		t, err := time.Now().MarshalBinary()
		if err != nil {
			j.logger.Error("could not marshal current time.", zap.Error(err))
		}
		if ack, err := j.jetstream.Publish(j.ctx, subject, t); err != nil {
			j.metrics.SuccessCounter.With(prometheus.Labels{
				"type":    failedPublish,
				"stream":  streamName,
				"cluster": clusterName,
			}).Add(1)
			if err == nats.ErrTimeout {
				j.logger.Error("Request timeout: No response received within the timeout period.")
			} else if err == nats.ErrNoStreamResponse {
				j.logger.Error("Request failed: No Stream available for the subject.")
			} else {
				j.logger.Error("Request failed: %v", zap.Error(err))
			}
		} else {
			j.metrics.SuccessCounter.With(prometheus.Labels{
				"type":    successfulPublish,
				"stream":  streamName,
				"cluster": clusterName,
			}).Add(1)
			j.logger.Info("receive ack", zap.String("stream", ack.Stream))
		}

		time.Sleep(j.config.PublishInterval)
	}
}

func (j *JetstreamClient) messageHandlerFactoryJetstream() (jetstream.MessageHandler, chan *Message) {
	ch := make(chan *Message)
	return func(msg jetstream.Msg) {
		ch <- &Message{
			Subject: msg.Subject(),
			Data:    msg.Data(),
		}
		err := msg.Ack()
		if err != nil {
			j.logger.Error("Failed to acknowledge the message", zap.Error(err))
		}
	}, ch
}

// Close closes NATS connection
func (j *JetstreamClient) Close() {
	if err := j.connection.FlushTimeout(j.config.FlushTimeout); err != nil {
		j.logger.Error("could not flush", zap.Error(err))
	}

	j.connection.Close()
	j.logger.Info("NATS is closed.")
}
