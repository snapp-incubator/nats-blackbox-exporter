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
	0.0001,
	0.0005,
	0.0007,
	0.001,
	0.002,
	0.003,
	0.004,
	0.005,
	0.006,
}

// Metrics has all the client metrics.
type Metrics struct {
	Connection     prometheus.CounterVec
	Latency        prometheus.Histogram
	SuccessCounter prometheus.CounterVec
}

// nolint: ireturn
func newHistogram(histogramOpts prometheus.HistogramOpts) prometheus.Histogram {
	ev := prometheus.NewHistogram(histogramOpts)

	if err := prometheus.Register(ev); err != nil {
		var are prometheus.AlreadyRegisteredError
		if ok := errors.As(err, &are); ok {
			ev, ok = are.ExistingCollector.(prometheus.Histogram)
			if !ok {
				panic("different metric type registration")
			}
		} else {
			panic(err)
		}
	}

	return ev
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
		}, []string{"type"}),
		// nolint: exhaustruct
		Latency: newHistogram(prometheus.HistogramOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "latency",
			Help:        "from publish to consume duration in seconds",
			ConstLabels: nil,
			Buckets:     latencyBuckets,
		}),
		SuccessCounter: newCounterVec(prometheus.CounterOpts{
			Namespace:   Namespace,
			Subsystem:   Subsystem,
			Name:        "success_counter",
			Help:        "publish and consume success rate",
			ConstLabels: nil,
		}, []string{"type"}),
	}
}
