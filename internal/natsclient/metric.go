package natsclient

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	Namespace = "nats_blackbox_exporter"
	Subsystem = "client"
)

var latencyBuckets = []float64{
	0.001,
	0.0013,
	0.0015,
	0.0017,
	0.002,
	0.0023,
	0.0025,
	0.0027,
	0.003,
	0.004,
}

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

func NewMetrics() Metrics {
	return Metrics{
		Connection: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "connection_errors_total",
			Help:        "total number of disconnections and reconnections",
			ConstLabels: nil,
		}, []string{"type", "cluster"}),
		// nolint: exhaustruct
		Latency: newHistogramVec(prometheus.HistogramOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "latency",
			Help:        "from publish to consume duration in seconds",
			ConstLabels: nil,
			Buckets:     latencyBuckets,
		}, []string{"stream", "cluster"}),
		SuccessCounter: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "success_counter",
			Help:        "publish and consume success rate",
			ConstLabels: nil,
		}, []string{"type", "stream", "cluster"}),
	}
}
