package observability

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SchedulerCollector exposes scheduler-specific Prometheus metrics.
type SchedulerCollector struct {
	gatherer prometheus.Gatherer

	PathComputationDuration prometheus.Histogram
	ServiceRequestsQueued   prometheus.Gauge
	PreemptionsTotal        prometheus.Counter
	ContactWindowCacheRatio prometheus.Gauge
}

// NewSchedulerCollector registers scheduler metrics against the provided registerer.
func NewSchedulerCollector(reg prometheus.Registerer) (*SchedulerCollector, error) {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	gatherer := prometheus.DefaultGatherer
	if g, ok := reg.(prometheus.Gatherer); ok {
		gatherer = g
	}

	pathHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "scheduler_path_computation_duration_seconds",
		Help:    "Duration of multi-hop path computations performed by the scheduler.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	})
	pathHistogram, err := registerHistogram(reg, pathHistogram, "scheduler_path_computation_duration_seconds")
	if err != nil {
		return nil, err
	}

	queueGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scheduler_service_requests_queued",
		Help: "Number of service requests currently queued for scheduling.",
	})
	queueGauge, err = registerGauge(reg, queueGauge, "scheduler_service_requests_queued")
	if err != nil {
		return nil, err
	}

	preemptions := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "scheduler_preemptions_total",
		Help: "Cumulative number of service request preemptions performed by the scheduler.",
	})
	preemptions, err = registerCounter(reg, preemptions, "scheduler_preemptions_total")
	if err != nil {
		return nil, err
	}

	cacheRatio := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "scheduler_contact_window_cache_hit_ratio",
		Help: "Hit ratio for the scheduler's contact window cache.",
	})
	cacheRatio, err = registerGauge(reg, cacheRatio, "scheduler_contact_window_cache_hit_ratio")
	if err != nil {
		return nil, err
	}

	return &SchedulerCollector{
		gatherer:                gatherer,
		PathComputationDuration: pathHistogram,
		ServiceRequestsQueued:   queueGauge,
		PreemptionsTotal:        preemptions,
		ContactWindowCacheRatio: cacheRatio,
	}, nil
}

// Gatherer returns the Prometheus gatherer associated with the collector.
func (c *SchedulerCollector) Gatherer() prometheus.Gatherer {
	if c == nil {
		return nil
	}
	return c.gatherer
}

// ObservePathComputation records a path computation duration measurement.
func (c *SchedulerCollector) ObservePathComputation(d time.Duration) {
	if c == nil || c.PathComputationDuration == nil {
		return
	}
	c.PathComputationDuration.Observe(d.Seconds())
}

// SetQueuedRequests updates the queue depth gauge.
func (c *SchedulerCollector) SetQueuedRequests(count int) {
	if c == nil || c.ServiceRequestsQueued == nil {
		return
	}
	c.ServiceRequestsQueued.Set(float64(count))
}

// IncPreemptions increments the preemption counter.
func (c *SchedulerCollector) IncPreemptions() {
	if c == nil || c.PreemptionsTotal == nil {
		return
	}
	c.PreemptionsTotal.Inc()
}

// SetContactWindowHitRatio sets the contact window cache hit ratio.
func (c *SchedulerCollector) SetContactWindowHitRatio(ratio float64) {
	if c == nil || c.ContactWindowCacheRatio == nil {
		return
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	c.ContactWindowCacheRatio.Set(ratio)
}

func registerHistogram(reg prometheus.Registerer, hist prometheus.Histogram, name string) (prometheus.Histogram, error) {
	if err := reg.Register(hist); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(prometheus.Histogram); ok {
				return existing, nil
			}
			return nil, fmt.Errorf("collector %s already registered with incompatible type", name)
		}
		return nil, err
	}
	return hist, nil
}

func registerCounter(reg prometheus.Registerer, counter prometheus.Counter, name string) (prometheus.Counter, error) {
	if err := reg.Register(counter); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := are.ExistingCollector.(prometheus.Counter); ok {
				return existing, nil
			}
			return nil, fmt.Errorf("collector %s already registered with incompatible type", name)
		}
		return nil, err
	}
	return counter, nil
}
