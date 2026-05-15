package client

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "nats_blackbox_exporter"
	Subsystem = "client"
)

// Label values for the messages_total counter.
const (
	OpPublish   = "publish"
	OpSubscribe = "subscribe"

	ResultSuccess = "success"
	ResultFailure = "failure"
)

// Label values for the connection_events_total counter.
const (
	EventDisconnect       = "disconnect"
	EventReconnect        = "reconnect"
	EventReconnectFailure = "reconnect_failure"
)

// Metrics holds all client-side Prometheus collectors.
type Metrics struct {
	ConnectionEvents prometheus.CounterVec
	ConnectionUp     prometheus.Gauge
	Latency          prometheus.HistogramVec
	Messages         prometheus.CounterVec
	MessagesDropped  prometheus.CounterVec
	StreamRestarts   prometheus.CounterVec
}

// nolint: ireturn
func newHistogramVec(histogramOpts prometheus.HistogramOpts, labelNames []string) prometheus.HistogramVec {
	ev := prometheus.NewHistogramVec(histogramOpts, labelNames)

	if err := prometheus.Register(ev); err != nil {
		var are prometheus.AlreadyRegisteredError
		if ok := errors.As(err, &are); ok {
			ev, ok = are.ExistingCollector.(*prometheus.HistogramVec)
			if !ok {
				panic("different metric type registration")
			}
		} else {
			panic(err)
		}
	}

	return *ev
}

// nolint: ireturn
func newCounterVec(counterOpts prometheus.CounterOpts, labelNames []string) prometheus.CounterVec {
	ev := prometheus.NewCounterVec(counterOpts, labelNames)

	if err := prometheus.Register(ev); err != nil {
		var are prometheus.AlreadyRegisteredError
		if ok := errors.As(err, &are); ok {
			ev, ok = are.ExistingCollector.(*prometheus.CounterVec)
			if !ok {
				panic("different metric type registration")
			}
		} else {
			panic(err)
		}
	}

	return *ev
}

// nolint: ireturn
func newGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	g := prometheus.NewGauge(opts)

	if err := prometheus.Register(g); err != nil {
		var are prometheus.AlreadyRegisteredError
		if ok := errors.As(err, &are); ok {
			g, ok = are.ExistingCollector.(prometheus.Gauge)
			if !ok {
				panic("different metric type registration")
			}
		} else {
			panic(err)
		}
	}

	return g
}

// NewMetrics constructs and registers the client metric set.
// `conn` is "jetstream" or "core" and `region` is the deployment region;
// both are emitted as ConstLabels (process-constant) so they don't bloat
// per-series cardinality on the variable label set.
func NewMetrics(conn, region string) Metrics { //nolint: funlen
	constLabels := prometheus.Labels{
		"conn":   conn,
		"region": region,
	}

	latencyBuckets := []float64{
		0.001, 0.0015, 0.002, 0.0025, 0.003, 0.004, 0.005, 0.006, 0.008,
		0.01, 0.015, 0.02, 0.025, 0.03, 0.05, 0.08, 0.1, 0.5, 1,
	}

	return Metrics{
		ConnectionEvents: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "connection_events_total",
			Help:        "Total NATS connection lifecycle events (disconnect, reconnect, reconnect_failure).",
			ConstLabels: constLabels,
		}, []string{"event", "cluster"}),
		ConnectionUp: newGauge(prometheus.GaugeOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "connection_up",
			Help:        "Whether the NATS connection is currently established (1) or not (0).",
			ConstLabels: constLabels,
		}),
		// nolint: exhaustruct
		Latency: newHistogramVec(prometheus.HistogramOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "latency_seconds",
			Help:        "End-to-end latency from publish to consume, in seconds.",
			ConstLabels: constLabels,
			Buckets:     latencyBuckets,
		}, []string{"stream", "subject", "cluster"}),
		Messages: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "messages_total",
			Help:        "Total publish/subscribe attempts, partitioned by operation and result.",
			ConstLabels: constLabels,
		}, []string{"operation", "result", "stream", "subject", "cluster"}),
		MessagesDropped: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "messages_dropped_total",
			Help:        "Messages dropped because the in-process handler channel was full.",
			ConstLabels: constLabels,
		}, []string{"stream", "subject"}),
		StreamRestarts: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "stream_restarts_total",
			Help:        "Times a stream's publish/subscribe loop has been restarted due to a stall.",
			ConstLabels: constLabels,
		}, []string{"stream", "cluster"}),
	}
}
