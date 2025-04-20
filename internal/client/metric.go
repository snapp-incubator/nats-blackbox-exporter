package client

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "nats_blackbox_exporter"
	Subsystem = "client"
)

// Metrics has all the client metrics.
type Metrics struct {
	Connection     prometheus.CounterVec
	Latency        prometheus.HistogramVec
	AckDuration    prometheus.HistogramVec
	SuccessCounter prometheus.CounterVec
	StreamRestart  prometheus.CounterVec
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

// nolint: funlen
func NewMetrics(conn string) Metrics {
	latencyBuckets := []float64{
		0.001,
		0.0015,
		0.002,
		0.0025,
		0.003,
		0.004,
		0.005,
		0.006,
		0.008,
		0.01,
		0.015,
		0.02,
		0.025,
		0.03,
		0.05,
		0.08,
		0.1,
		0.5,
		1,
	}

	return Metrics{
		Connection: newCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "connection_errors_total",
			Help:      "total number of disconnections and reconnections",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
		}, []string{"type", "cluster"}),
		// nolint: exhaustruct
		Latency: newHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "latency",
			Help:      "from publish to consume duration in seconds",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
			Buckets: latencyBuckets,
		}, []string{"stream", "cluster", "subject", "region"}),
		AckDuration: newHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "ack_duration",
			Help:      "from publish to publish ack in seconds",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
			Buckets: latencyBuckets,
		}, []string{"stream", "cluster", "subject", "region"}),
		SuccessCounter: newCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "success_counter",
			Help:      "publish and consume success rate",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
		}, []string{"type", "stream", "cluster", "subject", "region"}),
		StreamRestart: newCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "stream_restart",
			Help:      "stream restart",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
		}, []string{"stream", "cluster", "region"}),
	}
}
