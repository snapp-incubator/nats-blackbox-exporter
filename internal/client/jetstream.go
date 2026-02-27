package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/fx"
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

	// retryCounts keeps track of restart attempts per stream name
	retryCounts map[string]int
	retryMu     sync.Mutex

	// ctx and cancel control the lifetime of background goroutines (e.g. core publish/subscribe).
	ctx    context.Context    //nolint:containedctx
	cancel context.CancelFunc
}

// Provide initializes NATS connection.
func Provide(lc fx.Lifecycle, config Config, logger *zap.Logger) (*Client, error) {
	conn := "jetstream"
	if !config.IsJetstream {
		conn = "core"
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		jetstream:   nil,
		connection:  nil,
		config:      config,
		logger:      logger,
		metrics:     NewMetrics(conn),
		retryCounts: make(map[string]int),
		retryMu:     sync.Mutex{},
		ctx:         ctx,
		cancel:      cancel,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := client.connect(); err != nil {
				return fmt.Errorf("could not connect to nats: %w", err)
			}

			if config.IsJetstream {
				if err := client.connectJetstream(); err != nil {
					return fmt.Errorf("could not connect to jetstream: %w", err)
				}

				client.updateOrCreateStream(ctx)
			}

			return client.StartBlackboxTest()
		},
		OnStop: func(_ context.Context) error {
			client.Close()

			return nil
		},
	})

	return client, nil
}

// Close cancels background goroutines and closes the NATS connection.
func (client *Client) Close() {
	client.cancel()

	if err := client.connection.FlushTimeout(client.config.FlushTimeout); err != nil {
		client.logger.Error("could not flush", zap.Error(err))
	}

	client.connection.Close()
	client.logger.Info("NATS is closed.")
}

func (client *Client) updateOrCreateStream(ctx context.Context) {
	if client.config.AllExistingStreams {
		streamNames := client.jetstream.StreamNames(ctx)
		for stream := range streamNames.Name() {
			client.config.Streams = append(client.config.Streams, Stream{Name: stream, Subject: ""})
		}

		if err := streamNames.Err(); err != nil {
			client.logger.Error("could not list stream names", zap.Error(err))
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

func (client *Client) setupPublishAndSubscribe(stream *Stream) {
	clusterName := client.connection.ConnectedClusterName()
	ctx, baseCancel := context.WithCancel(context.Background())

	customCancel := func() {
		client.logger.Info("Cancel function called")
		client.metrics.StreamRestart.With(prometheus.Labels{
			"stream":  stream.Name,
			"cluster": clusterName,
			"region":  client.config.Region,
		}).Add(1)

		// increment retry count and decide whether to retry based on limits
		client.retryMu.Lock()
		currentRetry := client.retryCounts[stream.Name]
		client.retryCounts[stream.Name] = currentRetry + 1
		client.retryMu.Unlock()

		baseCancel()

		if client.config.MaxRetries > 0 && currentRetry+1 > client.config.MaxRetries {
			client.logger.Warn("Max retry reached, will not restart stream",
				zap.String("stream", stream.Name),
				zap.Int("retries", currentRetry+1))

			return
		}

		// calculate wait with exponential backoff and optional multiplier cap
		multiplier := client.config.RetryBackoffMultiplier
		if multiplier <= 0 {
			multiplier = 1
		}

		backoffFactor := math.Pow(multiplier, float64(currentRetry))
		wait := time.Duration(backoffFactor * float64(client.config.RetryDelay))

		client.logger.Info("Waiting before restart",
			zap.String("stream", stream.Name),
			zap.Duration("wait", wait),
			zap.Int("retry", currentRetry+1))

		// sleep and restart in a separate goroutine to avoid blocking the health checker
		go func() {
			time.Sleep(wait)
			client.setupPublishAndSubscribe(stream)
		}()
	}

	messageChannel, consumeCtx, err := client.createSubscribe(ctx, stream) //nolint:contextcheck
	if err != nil {
		customCancel()

		return
	}

	messageReceived := make(chan struct{})

	go client.jetstreamPublish(ctx, stream) //nolint:contextcheck

	go client.jetstreamSubscribe(ctx, messageReceived, messageChannel, stream) //nolint:contextcheck

	go client.subscribeHealth(ctx, customCancel, stream, messageReceived, consumeCtx) //nolint:contextcheck
}

func (client *Client) StartBlackboxTest() error {
	if len(client.config.Streams) == 0 {
		return errors.New("at least one stream is required")
	}

	if client.config.IsJetstream {
		for _, stream := range client.config.Streams {
			client.setupPublishAndSubscribe(&stream) //nolint:contextcheck
		}
	} else {
		for _, stream := range client.config.Streams {
			go client.coreSubscribe(client.ctx, stream.Subject)
			go client.corePublish(client.ctx, stream.Subject)
		}
	}

	return nil
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages.
func (client *Client) createSubscribe(ctx context.Context, stream *Stream) (<-chan *Message, jetstream.ConsumeContext, error) {
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
		client.logger.Error("Create consumer failed", zap.Error(err))
		// return handler channel so health check can detect stall and retry
		return h, nil, fmt.Errorf("create consumer failed: %w", err)
	}

	consumeCtx, err := con.Consume(messageHandler)
	if err != nil {
		client.logger.Error("Consuming failed", zap.Error(err))
		// return handler channel so health check can detect stall and retry
		return h, nil, fmt.Errorf("consume failed: %w", err)
	}

	client.logger.Info("Subscribed successfully", zap.String("subject", stream.Subject))

	return h, consumeCtx, nil
}

func (client *Client) subscribeHealth(ctx context.Context, cancelFunc context.CancelFunc, stream *Stream, messageReceived <-chan struct{}, consumeCtx jetstream.ConsumeContext) {
	waitTime := subscribeHealthMultiplication * client.config.PublishInterval
	timer := time.NewTimer(waitTime)

	defer func() {
		timer.Stop()

		if consumeCtx != nil {
			consumeCtx.Stop()
		}
	}()

	for {
		select {
		case <-messageReceived:
			if !timer.Stop() {
				<-timer.C
			}

			timer.Reset(waitTime)

		case <-ctx.Done():
			client.logger.Info("Health checker context canceled", zap.String("stream", stream.Name))

			return
		case <-timer.C:
			client.logger.Warn("No message received in", zap.Duration("seconds", waitTime), zap.String("stream", stream.Name))
			cancelFunc()

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

func (client *Client) coreSubscribe(ctx context.Context, subject string) {
	clusterName := client.connection.ConnectedClusterName()

	messageHandler, h := client.messageHandlerCoreFactory()

	if _, err := client.connection.Subscribe(subject, messageHandler); err != nil {
		client.logger.Error("Consuming failed, stopping core subscription",
			zap.String("subject", subject), zap.Error(err))

		return
	}

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping core subscription", zap.String("subject", subject))

			return
		case msg := <-h:
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

			client.logger.Info("Received message", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
		}
	}
}

func (client *Client) corePublish(ctx context.Context, subject string) {
	clusterName := client.connection.ConnectedClusterName()
	ticker := time.NewTicker(client.config.PublishInterval)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping core publish", zap.String("subject", subject))

			return
		case <-ticker.C:
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
					"region":  client.config.Region,
				}).Add(1)

				switch {
				case errors.Is(err, nats.ErrTimeout):
					client.logger.Error("Request timeout: No response received within the timeout period.")
				case errors.Is(err, nats.ErrNoStreamResponse):
					client.logger.Error("Request failed: No Stream available for the subject.")
				default:
					client.logger.Error("Request failed", zap.Error(err))
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
		}
	}
}

func (client *Client) jetstreamPublish(ctx context.Context, stream *Stream) {
	clusterName := client.connection.ConnectedClusterName()
	ticker := time.NewTicker(client.config.PublishInterval)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping publish", zap.String("stream", stream.Name))

			return
		case <-ticker.C:
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
					client.logger.Error("Request failed", zap.Error(err))
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

const messageChannelBuffer = 128

func (client *Client) messageHandlerJetstreamFactory() (jetstream.MessageHandler, <-chan *Message) {
	ch := make(chan *Message, messageChannelBuffer)

	return func(msg jetstream.Msg) {
		if err := msg.Ack(); err != nil {
			client.logger.Error("Failed to acknowledge the message", zap.Error(err))
		}

		select {
		case ch <- &Message{
			Subject: msg.Subject(),
			Data:    msg.Data(),
		}:
		default:
			client.logger.Warn("Message channel full, dropping message",
				zap.String("subject", msg.Subject()))
		}
	}, ch
}

func (client *Client) messageHandlerCoreFactory() (nats.MsgHandler, <-chan *Message) {
	ch := make(chan *Message, messageChannelBuffer)

	return func(msg *nats.Msg) {
		select {
		case ch <- &Message{
			Subject: msg.Subject,
			Data:    msg.Data,
		}:
		default:
			client.logger.Warn("Message channel full, dropping message",
				zap.String("subject", msg.Subject))
		}
	}, ch
}

// connect creates a nats core connection and fills its field.
func (client *Client) connect() error {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "nats-blackbox-exporter"
	}

	if client.connection, err = nats.Connect(
		client.config.URL,
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			client.metrics.Connection.WithLabelValues("disconnection", conn.ConnectedClusterName()).Add(1)
			client.logger.Error("nats disconnected", zap.Error(err))
		}),
		nats.ReconnectErrHandler(func(conn *nats.Conn, err error) {
			client.logger.Info("nats reconnection failed", zap.Error(err))
			client.metrics.Connection.WithLabelValues("reconnection-failure", conn.ConnectedClusterName()).Add(1)
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			client.logger.Info("nats reconnected")
			client.metrics.Connection.WithLabelValues("reconnection", conn.ConnectedClusterName()).Add(1)
		}),
		nats.MaxReconnects(client.config.MaxReconnection),
		nats.Name(hostname),
		nats.RetryOnFailedConnect(true),
	); err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}

	return nil
}

// connectJetstream creates a jetstream connection using an already connected nats connection.
func (client *Client) connectJetstream() error {
	var err error
	if client.jetstream, err = jetstream.New(client.connection); err != nil {
		return fmt.Errorf("jetstream connect: %w", err)
	}

	return nil
}
