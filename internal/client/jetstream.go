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
	subjectSuffix                 = "_blackbox_exporter"
	subscribeHealthMultiplication = 10
)

// ErrNoStreams is returned when no streams are configured for blackbox testing.
var ErrNoStreams = errors.New("at least one stream is required")

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
	ctx    context.Context //nolint:containedctx
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
		metrics:     NewMetrics(conn, config.Region),
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

			return client.StartBlackboxTest(ctx)
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

	if client.connection == nil {
		return
	}

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
	subjects := slices.Concat(info.Config.Subjects, []string{stream.Subject})

	// remove duplicate subjects in the list.
	slices.Sort(subjects)
	info.Config.Subjects = slices.Compact(subjects)

	if _, err := client.jetstream.UpdateStream(ctx, info.Config); err != nil {
		client.logger.Error("could not add subject to existing stream", zap.String("stream", stream.Name), zap.Error(err))

		return
	}

	client.logger.Info("stream updated", zap.String("stream", stream.Name))
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

// runStream owns the publish/subscribe lifecycle for a single jetstream stream.
// It restarts the inner attempt on stall (signalled by subscribeHealth) with
// exponential backoff, until MaxRetries is exceeded or parentCtx is cancelled.
func (client *Client) runStream(parentCtx context.Context, stream *Stream) {
	clusterName := client.connection.ConnectedClusterName()

	for {
		if parentCtx.Err() != nil {
			return
		}

		restart := client.runStreamAttempt(parentCtx, stream, clusterName)

		select {
		case <-parentCtx.Done():
			return
		case <-restart:
		}

		client.metrics.StreamRestarts.With(prometheus.Labels{
			"stream":  stream.Name,
			"cluster": clusterName,
		}).Add(1)

		client.retryMu.Lock()
		currentRetry := client.retryCounts[stream.Name]
		client.retryCounts[stream.Name] = currentRetry + 1
		client.retryMu.Unlock()

		if client.config.MaxRetries > 0 && currentRetry+1 > client.config.MaxRetries {
			client.logger.Warn("Max retry reached, will not restart stream",
				zap.String("stream", stream.Name),
				zap.Int("retries", currentRetry+1))

			return
		}

		multiplier := client.config.RetryBackoffMultiplier
		if multiplier <= 0 {
			multiplier = 1
		}

		wait := time.Duration(math.Pow(multiplier, float64(currentRetry)) * float64(client.config.RetryDelay))

		client.logger.Info("Waiting before restart",
			zap.String("stream", stream.Name),
			zap.Duration("wait", wait),
			zap.Int("retry", currentRetry+1))

		timer := time.NewTimer(wait)
		select {
		case <-parentCtx.Done():
			timer.Stop()

			return
		case <-timer.C:
		}
	}
}

// runStreamAttempt wires up one publish/subscribe attempt and returns a channel
// that closes when the attempt should be restarted (subscribe failure or stall).
func (client *Client) runStreamAttempt(parentCtx context.Context, stream *Stream, clusterName string) <-chan struct{} {
	restart := make(chan struct{})
	ctx, cancel := context.WithCancel(parentCtx) //nolint: gosec

	triggerRestart := func() {
		cancel()

		select {
		case <-restart:
		default:
			close(restart)
		}
	}

	messageChannel, consumeCtx, err := client.createSubscribe(ctx, stream)
	if err != nil {
		triggerRestart()

		return restart
	}

	messageReceived := make(chan struct{}, 1)

	go client.jetstreamPublish(ctx, stream)
	go client.jetstreamSubscribe(ctx, messageReceived, messageChannel, stream, clusterName)
	go client.subscribeHealth(ctx, triggerRestart, stream, messageReceived, consumeCtx)

	return restart
}

func (client *Client) StartBlackboxTest(ctx context.Context) error {
	if len(client.config.Streams) == 0 {
		return ErrNoStreams
	}

	if client.config.IsJetstream {
		for i := range client.config.Streams {
			go client.runStream(ctx, &client.config.Streams[i])
		}
	} else {
		for _, stream := range client.config.Streams {
			go client.coreSubscribe(ctx, stream.Subject)
			go client.corePublish(ctx, stream.Subject)
		}
	}

	return nil
}

// Subscribe subscribes to a list of subjects and returns a channel with incoming messages.
func (client *Client) createSubscribe(ctx context.Context, stream *Stream) (<-chan *Message, jetstream.ConsumeContext, error) {
	messageHandler, h := client.messageHandlerJetstreamFactory(stream)

	con, err := client.jetstream.CreateOrUpdateConsumer(
		ctx,
		stream.Name,
		jetstream.ConsumerConfig{ // nolint: exhaustruct
			DeliverPolicy: jetstream.DeliverNewPolicy,
			ReplayPolicy:  jetstream.ReplayInstantPolicy,
			AckPolicy:     jetstream.AckExplicitPolicy,
			MaxAckPending: client.config.MaxAckPending,
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

func (client *Client) subscribeHealth(ctx context.Context, triggerRestart func(), stream *Stream, messageReceived <-chan struct{}, consumeCtx jetstream.ConsumeContext) {
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
			timer.Reset(waitTime)

		case <-ctx.Done():
			client.logger.Info("Health checker context canceled", zap.String("stream", stream.Name))

			return
		case <-timer.C:
			client.logger.Warn("No message received in", zap.Duration("seconds", waitTime), zap.String("stream", stream.Name))
			triggerRestart()

			return
		}
	}
}

func (client *Client) jetstreamSubscribe(ctx context.Context, messageReceived chan<- struct{}, h <-chan *Message, stream *Stream, clusterName string) {
	var payload Payload

	defer func() {
		if r := recover(); r != nil {
			client.logger.Error("Subscription goroutine panicked", zap.Any("error", r))
		} else {
			client.logger.Debug("Subscription goroutine exited normally", zap.String("stream", stream.Name))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			client.logger.Info("Context canceled, stopping subscription", zap.String("stream", stream.Name))

			return
		case msg := <-h:
			// non-blocking signal: if the health checker hasn't drained the
			// previous tick yet, coalesce — it only needs to know "alive".
			select {
			case messageReceived <- struct{}{}:
			default:
			}

			client.resetRetry(stream.Name)

			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				client.logger.Error("received message but could not calculate latency due to unmarshalling error.",
					zap.String("subject", msg.Subject),
					zap.Error(err),
				)
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpSubscribe,
					"result":    ResultFailure,
					"stream":    stream.Name,
					"subject":   msg.Subject,
					"cluster":   clusterName,
				}).Add(1)

				continue
			}

			latency := time.Since(payload.PublishTime).Seconds()

			client.metrics.Latency.With(prometheus.Labels{
				"subject": msg.Subject,
				"stream":  stream.Name,
				"cluster": clusterName,
			}).Observe(latency)

			client.metrics.Messages.With(prometheus.Labels{
				"operation": OpSubscribe,
				"result":    ResultSuccess,
				"stream":    stream.Name,
				"subject":   msg.Subject,
				"cluster":   clusterName,
			}).Add(1)

			client.logger.Debug("Received message", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
		}
	}
}

// resetRetry clears the per-stream retry counter — called whenever a message
// arrives so a healthy stream isn't permanently disabled by accumulated
// transient failures from earlier in the process lifetime.
func (client *Client) resetRetry(stream string) {
	client.retryMu.Lock()
	defer client.retryMu.Unlock()

	if client.retryCounts[stream] != 0 {
		client.retryCounts[stream] = 0
	}
}

func (client *Client) coreSubscribe(ctx context.Context, subject string) {
	clusterName := client.connection.ConnectedClusterName()

	messageHandler, h := client.messageHandlerCoreFactory(subject)

	sub, err := client.connection.Subscribe(subject, messageHandler)
	if err != nil {
		client.logger.Error("Consuming failed, stopping core subscription",
			zap.String("subject", subject), zap.Error(err))

		return
	}

	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			client.logger.Error("Unsubscribe failed",
				zap.String("subject", subject), zap.Error(err))
		}
	}()

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
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpSubscribe,
					"result":    ResultFailure,
					"stream":    "-",
					"subject":   subject,
					"cluster":   clusterName,
				}).Add(1)

				continue
			}

			latency := time.Since(payload.PublishTime).Seconds()

			client.metrics.Latency.With(prometheus.Labels{
				"subject": subject,
				"stream":  "-",
				"cluster": clusterName,
			}).Observe(latency)

			client.metrics.Messages.With(prometheus.Labels{
				"operation": OpSubscribe,
				"result":    ResultSuccess,
				"stream":    "-",
				"subject":   subject,
				"cluster":   clusterName,
			}).Add(1)

			client.logger.Debug("Received message", zap.String("subject", msg.Subject), zap.Float64("latency", latency))
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
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpPublish,
					"result":    ResultFailure,
					"stream":    "-",
					"subject":   subject,
					"cluster":   clusterName,
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
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpPublish,
					"result":    ResultSuccess,
					"stream":    "-",
					"subject":   subject,
					"cluster":   clusterName,
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
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpPublish,
					"result":    ResultFailure,
					"stream":    stream.Name,
					"subject":   stream.Subject,
					"cluster":   clusterName,
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
				client.metrics.Messages.With(prometheus.Labels{
					"operation": OpPublish,
					"result":    ResultSuccess,
					"stream":    stream.Name,
					"subject":   stream.Subject,
					"cluster":   clusterName,
				}).Add(1)
				client.logger.Debug("receive ack", zap.String("stream", ack.Stream))
			}
		}
	}
}

const messageChannelBuffer = 128

func (client *Client) messageHandlerJetstreamFactory(stream *Stream) (jetstream.MessageHandler, <-chan *Message) {
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
			client.metrics.MessagesDropped.WithLabelValues(stream.Name, msg.Subject()).Add(1)
			client.logger.Warn("Message channel full, dropping message",
				zap.String("stream", stream.Name),
				zap.String("subject", msg.Subject()))
		}
	}, ch
}

func (client *Client) messageHandlerCoreFactory(subject string) (nats.MsgHandler, <-chan *Message) {
	ch := make(chan *Message, messageChannelBuffer)

	return func(msg *nats.Msg) {
		select {
		case ch <- &Message{
			Subject: msg.Subject,
			Data:    msg.Data,
		}:
		default:
			client.metrics.MessagesDropped.WithLabelValues("-", subject).Add(1)
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
			client.metrics.ConnectionEvents.WithLabelValues(EventDisconnect, conn.ConnectedClusterName()).Add(1)
			client.metrics.ConnectionUp.Set(0)
			client.logger.Error("nats disconnected", zap.Error(err))
		}),
		nats.ReconnectErrHandler(func(conn *nats.Conn, err error) {
			client.metrics.ConnectionEvents.WithLabelValues(EventReconnectFailure, conn.ConnectedClusterName()).Add(1)
			client.logger.Info("nats reconnection failed", zap.Error(err))
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			client.metrics.ConnectionEvents.WithLabelValues(EventReconnect, conn.ConnectedClusterName()).Add(1)
			client.metrics.ConnectionUp.Set(1)
			client.logger.Info("nats reconnected")
		}),
		nats.MaxReconnects(client.config.MaxReconnection),
		nats.Name(hostname),
		nats.RetryOnFailedConnect(true),
	); err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}

	client.metrics.ConnectionUp.Set(1)

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
