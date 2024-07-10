package natsclient

import (
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var (
	successfulSubscribe = "successful subscribe"
	failedPublish       = "failed publish"
	successfulPublish   = "successful publish"
	subject_suffix      = "_blackbox_exporter"
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
	metrics    Metrics
}

// NewJetstream initializes NATS JetStream connection
func NewJetstream(config Config, logger *zap.Logger) *Jetstream {
	j := &Jetstream{
		config:  &config,
		logger:  logger,
		metrics: NewMetrics(),
	}

	j.connect()

	j.createJetstreamContext()

	j.UpdateOrCreateStream()

	return j
}

func (j *Jetstream) connect() {
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

func (j *Jetstream) createJetstreamContext() {
	var err error
	j.jetstream, err = j.connection.JetStream()
	if err != nil {
		j.logger.Panic("could not connect to jetstream", zap.Error(err))
	}
}

func (j *Jetstream) UpdateOrCreateStream() {
	if j.config.AllExistingStreams {
		streamNames := j.jetstream.StreamNames()
		for stream := range streamNames {
			j.config.Streams = append(j.config.Streams, Stream{Name: stream})
		}
	}
	for i, stream := range j.config.Streams {
		if stream.Subject == "" {
			j.config.Streams[i].Subject = stream.Name + subject_suffix
		}

		info, err := j.jetstream.StreamInfo(stream.Name)
		if err == nil {
			j.updateStream(j.config.Streams[i], info)
		} else if err == nats.ErrStreamNotFound && j.config.NewStreamAllow {
			j.createStream(j.config.Streams[i])
		} else {
			j.logger.Panic("could not add subject", zap.Error(err))
		}
	}
}
func (j *Jetstream) updateStream(stream Stream, info *nats.StreamInfo) {
	subjects := append(info.Config.Subjects, stream.Subject)
	slices.Sort(subjects)
	subjects = slices.Compact(subjects)
	_, err := j.jetstream.UpdateStream(&nats.StreamConfig{
		Name:     stream.Name,
		Subjects: subjects,
	})
	if err != nil {
		j.logger.Panic("could not add subject to existing stream", zap.Error(err))
	}
	j.logger.Info("stream updated")
}

func (j *Jetstream) createStream(stream Stream) {
	_, err := j.jetstream.AddStream(&nats.StreamConfig{
		Name:     stream.Name,
		Subjects: []string{stream.Subject},
	})
	if err != nil {
		j.logger.Panic("could not add stream", zap.Error(err))
	}
	j.logger.Info("add new stream")
}

func (j *Jetstream) StartBlackboxTest() {
	for _, stream := range j.config.Streams {
		messageChannel := j.createSubscribe(stream.Subject)
		go j.jetstreamPublish(stream.Subject, stream.Name)
		go j.jetstreamSubscribe(messageChannel, stream.Name)
	}
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages
func (j *Jetstream) createSubscribe(subject string) chan *Message {

	messageHandler, h := j.messageHandlerFactoryJetstream()
	_, err := j.jetstream.Subscribe(
		subject,
		messageHandler,
		nats.DeliverNew(),
		nats.ReplayInstant(),
		nats.AckExplicit(),
		nats.MaxAckPending(j.config.MaxPubAcksInflight),
	)
	if err != nil {
		j.logger.Panic("could not Subscribe", zap.Error(err))
	} else {
		j.logger.Info("Subscribed to %s successfully", zap.String("subject", subject))
	}

	return h

}

func (j *Jetstream) jetstreamSubscribe(h chan *Message, streamName string) {
	clusterName := j.connection.ConnectedClusterName()
	for msg := range h {
		var publishTime time.Time
		err := publishTime.UnmarshalBinary(msg.Data)
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

func (j *Jetstream) jetstreamPublish(subject string, streamName string) {
	clusterName := j.connection.ConnectedClusterName()
	for {
		t, err := time.Now().MarshalBinary()
		if err != nil {
			j.logger.Error("could not marshal current time.", zap.Error(err))
		}
		if ack, err := j.jetstream.Publish(subject, t); err != nil {
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

func (j *Jetstream) messageHandlerFactoryJetstream() (nats.MsgHandler, chan *Message) {
	ch := make(chan *Message)
	return func(msg *nats.Msg) {
		ch <- &Message{
			Subject: msg.Subject,
			Data:    msg.Data,
		}
		err := msg.Ack()
		if err != nil {
			j.logger.Error("Failed to acknowledge the message", zap.Error(err))
		}
	}, ch
}

// Close closes NATS connection
func (j *Jetstream) Close() {
	if err := j.connection.FlushTimeout(j.config.FlushTimeout); err != nil {
		j.logger.Error("could not flush", zap.Error(err))
	}

	j.connection.Close()
	j.logger.Info("NATS is closed.")
}
