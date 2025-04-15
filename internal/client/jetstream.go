package client

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

const (
	successfulSubscribe           = "successful subscribe"
	failedPublish                 = "failed publish"
	successfulPublish             = "successful publish"
	subjectSuffix                 = "_blackbox_exporter"
	subscribeHealthMultiplication = 10
)

type Payload struct {
	PublishTime time.Time `json:"publish_time"`
	Region      string    `json:"region"`
}

type Message struct {
	Subject string
	Data    []byte
}

// Client represents a connection to NATS (core/jetstream).
type Client struct {
	connection *nats.Conn
	jetstream  jetstream.JetStream

	config Config

	logger  *zap.Logger
	metrics Metrics
}

// New initializes NATS connection.
func New(config Config, logger *zap.Logger) *Client {
	conn := "jetstream"
	if !config.IsJetstream {
		conn = "core"
	}

	client := &Client{
		jetstream:  nil,
		connection: nil,
		config:     config,
		logger:     logger,
		metrics:    NewMetrics(conn),
	}

	client.connect()

	if config.IsJetstream {
		client.connectJetstream()

		client.UpdateOrCreateStream(context.Background())
	}

	return client
}

// Close closes NATS connection.
func (client *Client) Close() {
	if err := client.connection.FlushTimeout(client.config.FlushTimeout); err != nil {
		client.logger.Error("could not flush", zap.Error(err))
	}

	client.connection.Close()
	client.logger.Info("NATS is closed.")
}

// connect connects create a nats core connection and fills its field.
func (client *Client) connect() {
	var err error

	if client.connection, err = nats.Connect(client.config.URL); err != nil {
		client.logger.Panic("could not connect to nats", zap.Error(err))
	}

	client.connection.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		client.metrics.Connection.WithLabelValues("disconnection").Add(1)
		client.logger.Error("nats disconnected", zap.Error(err))
	})

	client.connection.SetReconnectHandler(func(_ *nats.Conn) {
		client.logger.Warn("nats reconnected")
		client.metrics.Connection.WithLabelValues("reconnection").Add(1)
	})
}

// connectJetstream create a jetstream connection using already connected nats conntion and fills its field.
func (client *Client) connectJetstream() {
	var err error

	if client.jetstream, err = jetstream.New(client.connection); err != nil {
		client.logger.Panic("could not connect to jetstream", zap.Error(err))
	}
}

func (client *Client) UpdateOrCreateStream(ctx context.Context) {
	if client.config.AllExistingStreams {
		streamNames := client.jetstream.StreamNames(ctx)
		for stream := range streamNames.Name() {
			client.config.Streams = append(client.config.Streams, Stream{Name: stream, Subject: ""})
		}
	}

	for i, str := range client.config.Streams {
		if str.Subject == "" {
			client.config.Streams[i].Subject = str.Name + subjectSuffix
		}

		stream, err := client.jetstream.Stream(ctx, str.Name)
		if err == nil {
			info, err := stream.Info(ctx)
			if err == nil {
				client.updateStream(ctx, client.config.Streams[i], info)

				continue
			}
		} else if errors.Is(err, jetstream.ErrStreamNotFound) && client.config.NewStreamAllow {
			client.createStream(ctx, client.config.Streams[i])

			continue
		}

		client.logger.Error("could not add subject", zap.String("stream", str.Name), zap.Error(err))
	}
}

func (client *Client) updateStream(ctx context.Context, stream Stream, info *jetstream.StreamInfo) {
	subjects := make([]string, len(info.Config.Subjects)+1)
	copy(subjects, info.Config.Subjects)
	subjects[len(info.Config.Subjects)] = stream.Subject

	// remove duplicate subjects in the list.
	slices.Sort(subjects)
	info.Config.Subjects = slices.Compact(subjects)

	_, err := client.jetstream.UpdateStream(ctx, info.Config)
	if err != nil {
		client.logger.Error("could not add subject to existing stream", zap.String("stream", stream.Name), zap.Error(err))
	}

	client.logger.Info("stream updated")
}

func (client *Client) createStream(ctx context.Context, stream Stream) {
	// nolint: exhaustruct
	if _, err := client.jetstream.CreateStream(ctx, jetstream.StreamConfig{
		Name:     stream.Name,
		Subjects: []string{stream.Subject},
	}); err != nil {
		client.logger.Error("could not add stream", zap.String("stream", stream.Name), zap.Error(err))
	}

	client.logger.Info("add new stream")
}

func (client *Client) setupPublishAndSubscribe(parentCtx context.Context, stream *Stream) {
	var payload Payload

	clusterName := client.connection.ConnectedClusterName()
	ctx, baseCancel := context.WithCancel(context.Background())

	customCancel := func() {
		client.logger.Info("Cancel function called")
		client.metrics.StreamRestart.With(prometheus.Labels{
			"stream":  stream.Name,
			"cluster": clusterName,
			"region":  payload.Region,
		}).Add(1)
		baseCancel()
		client.setupPublishAndSubscribe(parentCtx, stream)
	}

	messageChannel := client.createSubscribe(ctx, stream)
	messageReceived := make(chan struct{})
	go client.jetstreamPublish(ctx, stream)                                    //nolint:contextcheck
	go client.jetstreamSubscribe(ctx, messageReceived, messageChannel, stream) //nolint:contextcheck
	go client.subscribeHealth(ctx, customCancel, stream, messageReceived)      //nolint:contextcheck
}

func (client *Client) StartBlackboxTest(ctx context.Context) {
	if len(client.config.Streams) == 0 {
		client.logger.Panic("at least one stream is required.")
	}

	if client.config.IsJetstream {
		for _, stream := range client.config.Streams {
			client.setupPublishAndSubscribe(ctx, &stream)
		}
	} else {
		for _, stream := range client.config.Streams {
			go client.coreSubscribe(stream.Subject)
			go client.corePublish(stream.Subject)
		}
	}
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages.
func (client *Client) createSubscribe(ctx context.Context, stream *Stream) <-chan *Message {
	messageHandler, h := client.messageHandlerJetstreamFactory()

	con, err := client.jetstream.CreateOrUpdateConsumer(
		ctx,
		stream.Name,
		jetstream.ConsumerConfig{ // nolint: exhaustruct
			DeliverPolicy: jetstream.DeliverNewPolicy,
			ReplayPolicy:  jetstream.ReplayInstantPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
			MaxAckPending: client.config.MaxPubAcksInflight,
			FilterSubject: stream.Subject,
		},
	)
	if err != nil {
		client.logger.Panic("Create consumer failed", zap.Error(err))
	}

	if _, err := con.Consume(messageHandler); err != nil {
		client.logger.Panic("Consuming failed", zap.Error(err))
	}

	client.logger.Info("Subscribed to %s successfully", zap.String("subject", stream.Subject))

	return h
}

func (client *Client) subscribeHealth(ctx context.Context, cancelFunc context.CancelFunc, stream *Stream, messageReceived <-chan struct{}) {
	waitTime := subscribeHealthMultiplication * client.config.PublishInterval
	timer := time.NewTimer(waitTime)

	defer timer.Stop()

	for {
		select {
		case <-messageReceived:
			if !timer.Stop() {
				<-timer.C
			}

			timer.Reset(waitTime)

		case <-timer.C:
			client.logger.Warn("No message received in", zap.Duration("seconds", waitTime), zap.String("stream", stream.Name))
			cancelFunc()
			client.setupPublishAndSubscribe(ctx, stream)

			return
		}
	}
}

func (client *Client) jetstreamSubscribe(ctx context.Context, messageReceived chan struct{}, h <-chan *Message, stream *Stream) {
	var payload Payload

	clusterName := client.connection.ConnectedClusterName()

	defer func() {
		if r := recover(); r != nil {
			client.logger.Error("Subscription goroutine panicked", zap.Any("error", r))
		} else {
			client.logger.Info("Subscription goroutine exited normally")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping subscription", zap.String("stream", stream.Name))

			return
		case msg := <-h:
			messageReceived <- struct{}{}

			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				client.logger.Error("received message but could not calculate latency due to unmarshalling error.",
					zap.String("subject", msg.Subject),
					zap.Error(err),
				)

				continue
			}

			latency := time.Since(payload.PublishTime).Seconds()

			client.metrics.Latency.With(prometheus.Labels{
				"subject": msg.Subject,
				"stream":  stream.Name,
				"cluster": clusterName,
				"region":  payload.Region,
			}).Observe(latency)

			client.metrics.SuccessCounter.With(prometheus.Labels{
				"subject": msg.Subject,
				"type":    successfulSubscribe,
				"stream":  stream.Name,
				"cluster": clusterName,
				"region":  payload.Region,
			}).Add(1)

			client.logger.Info("Received message: ", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
		}
	}
}

func (client *Client) coreSubscribe(subject string) {
	clusterName := client.connection.ConnectedClusterName()

	messageHandler, h := client.messageHandlerCoreFactory()

	if _, err := client.connection.Subscribe(subject, messageHandler); err != nil {
		client.logger.Panic("Consuming failed", zap.Error(err))
	}

	for msg := range h {
		var payload Payload

		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			client.logger.Error("received message but could not calculate latency due to unmarshalling error.",
				zap.String("subject", msg.Subject),
				zap.Error(err),
			)

			continue
		}

		latency := time.Since(payload.PublishTime).Seconds()

		client.metrics.Latency.With(prometheus.Labels{
			"subject": subject,
			"stream":  "-",
			"cluster": clusterName,
			"region":  payload.Region,
		}).Observe(latency)

		client.metrics.SuccessCounter.With(prometheus.Labels{
			"subject": subject,
			"stream":  "-",
			"type":    successfulSubscribe,
			"cluster": clusterName,
			"region":  payload.Region,
		}).Add(1)

		client.logger.Info("Received message: ", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
	}
}

func (client *Client) corePublish(subject string) {
	clusterName := client.connection.ConnectedClusterName()

	for {
		t, err := json.Marshal(Payload{
			PublishTime: time.Now(),
			Region:      client.config.Region,
		})
		if err != nil {
			client.logger.Error("could not marshal current time.", zap.Error(err))

			continue
		}

		if err := client.connection.Publish(subject, t); err != nil {
			client.metrics.SuccessCounter.With(prometheus.Labels{
				"stream":  "-",
				"subject": subject,
				"type":    failedPublish,
				"cluster": clusterName,
			}).Add(1)

			switch {
			case errors.Is(err, nats.ErrTimeout):
				client.logger.Error("Request timeout: No response received within the timeout period.")
			case errors.Is(err, nats.ErrNoStreamResponse):
				client.logger.Error("Request failed: No Stream available for the subject.")
			default:
				client.logger.Error("Request failed: %v", zap.Error(err))
			}
		} else {
			client.metrics.SuccessCounter.With(prometheus.Labels{
				"stream":  "-",
				"type":    successfulPublish,
				"cluster": clusterName,
				"subject": subject,
				"region":  client.config.Region,
			}).Add(1)
		}

		time.Sleep(client.config.PublishInterval)
	}
}

func (client *Client) jetstreamPublish(ctx context.Context, stream *Stream) {
	clusterName := client.connection.ConnectedClusterName()

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping publish", zap.String("stream", stream.Name))

			return
		case <-time.After(client.config.PublishInterval):
			t, err := json.Marshal(Payload{
				PublishTime: time.Now(),
				Region:      client.config.Region,
			})
			if err != nil {
				client.logger.Error("could not marshal current time.", zap.Error(err))

				continue
			}

			if ack, err := client.jetstream.Publish(ctx, stream.Subject, t); err != nil {
				client.metrics.SuccessCounter.With(prometheus.Labels{
					"region":  client.config.Region,
					"subject": stream.Subject,
					"type":    failedPublish,
					"stream":  stream.Name,
					"cluster": clusterName,
				}).Add(1)

				switch {
				case errors.Is(err, nats.ErrTimeout):
					client.logger.Error("Request timeout: No response received within the timeout period.")
				case errors.Is(err, nats.ErrNoStreamResponse):
					client.logger.Error("Request failed: No Stream available for the subject.")
				default:
					client.logger.Error("Request failed: %v", zap.Error(err))
				}
			} else {
				client.metrics.SuccessCounter.With(prometheus.Labels{
					"subject": stream.Subject,
					"type":    successfulPublish,
					"stream":  stream.Name,
					"cluster": clusterName,
					"region":  client.config.Region,
				}).Add(1)
				client.logger.Info("receive ack", zap.String("stream", ack.Stream))
			}
		}
	}
}

func (client *Client) messageHandlerJetstreamFactory() (jetstream.MessageHandler, <-chan *Message) {
	ch := make(chan *Message)

	return func(msg jetstream.Msg) {
		ch <- &Message{
			Subject: msg.Subject(),
			Data:    msg.Data(),
		}

		if err := msg.Ack(); err != nil {
			client.logger.Error("Failed to acknowledge the message", zap.Error(err))
		}
	}, ch
}

func (client *Client) messageHandlerCoreFactory() (nats.MsgHandler, <-chan *Message) {
	ch := make(chan *Message)

	return func(msg *nats.Msg) {
		ch <- &Message{
			Subject: msg.Subject,
			Data:    msg.Data,
		}
	}, ch
}
