package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Registry          *prometheus.Registry
	HTTPRequests      *prometheus.CounterVec
	HTTPDuration      *prometheus.HistogramVec
	HTTPInFlight      prometheus.Gauge
	HTTPErrors        *prometheus.CounterVec
	RedisDegraded     *prometheus.CounterVec
	AuthEvents        *prometheus.CounterVec
	EmailSendErrors   prometheus.Counter
	EmailCircuitOpen  prometheus.Counter
	EmailCircuitState prometheus.Gauge
}

func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests.",
			},
			[]string{"method", "path", "code"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),
		HTTPInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_in_flight_requests",
				Help: "Number of in-flight HTTP requests.",
			},
		),
		HTTPErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_errors_total",
				Help: "Total number of HTTP 5xx errors.",
			},
			[]string{"method", "path", "code"},
		),
		RedisDegraded: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "redis_degraded_total",
				Help: "Total number of Redis degradation events.",
			},
			[]string{"component"},
		),
		AuthEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "auth_events_total",
				Help: "Total number of auth events.",
			},
			[]string{"event"},
		),
		EmailSendErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "email_send_errors_total",
				Help: "Total number of email send errors.",
			},
		),
		EmailCircuitOpen: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "email_circuit_open_total",
				Help: "Total number of email circuit breaker open events.",
			},
		),
		EmailCircuitState: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "email_circuit_state",
				Help: "Email circuit breaker state: 0=closed,1=half_open,2=open.",
			},
		),
	}

	reg.MustRegister(
		m.HTTPRequests,
		m.HTTPDuration,
		m.HTTPInFlight,
		m.HTTPErrors,
		m.RedisDegraded,
		m.AuthEvents,
		m.EmailSendErrors,
		m.EmailCircuitOpen,
		m.EmailCircuitState,
	)

	return m
}

func (m *Metrics) IncAuthEvent(event string) {
	if m == nil {
		return
	}
	m.AuthEvents.WithLabelValues(event).Inc()
}
