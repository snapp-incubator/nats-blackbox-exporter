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
	SuccessCounter prometheus.CounterVec
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
		0.0035,
		0.004,
		0.0045,
		0.005,
		0.0055,
		0.006,
		0.0065,
		0.007,
		0.0075,
		0.008,
		0.0085,
		0.009,
		0.0095,
		0.01,
		0.015,
		0.02,
		0.025,
		0.03,
		0.045,
		0.05,
		0.065,
		0.07,
		0.08,
		0.09,
		0.1,
		0.2,
		0.3,
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
		}, []string{"stream", "cluster", "subject"}),
		SuccessCounter: newCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: Subsystem,
			Name:      "success_counter",
			Help:      "publish and consume success rate",
			ConstLabels: prometheus.Labels{
				"conn": conn,
			},
		}, []string{"type", "stream", "cluster", "subject"}),
	}
}
