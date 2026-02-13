package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	Registry                *prometheus.Registry
	HTTPRequests            *prometheus.CounterVec
	HTTPDuration            *prometheus.HistogramVec
	HTTPInFlight            prometheus.Gauge
	HTTPErrors              *prometheus.CounterVec
	RedisDegraded           *prometheus.CounterVec
	AuthEvents              *prometheus.CounterVec
	AuthEventReasons        *prometheus.CounterVec
	EmailSendErrors         prometheus.Counter
	EmailCircuitOpen        prometheus.Counter
	EmailCircuitState       prometheus.Gauge
	IdempotencyBypass       *prometheus.CounterVec
	IdempotencyHits         prometheus.Counter
	IdempotencyConflict     prometheus.Counter
	JWTBlacklistRedisErrors prometheus.Counter
	LoginLockouts           prometheus.Counter
	LockReleaseErrors       prometheus.Counter
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
		AuthEventReasons: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "auth_event_reasons_total",
				Help: "Total number of auth event reasons.",
			},
			[]string{"event", "reason"},
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
		IdempotencyBypass: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "idempotency_bypass_total",
				Help: "Total idempotency bypass events.",
			},
			[]string{"reason"},
		),
		IdempotencyHits: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "idempotency_hits_total",
				Help: "Total idempotency cache hits.",
			},
		),
		IdempotencyConflict: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "idempotency_conflicts_total",
				Help: "Total idempotency key/hash conflicts.",
			},
		),
		JWTBlacklistRedisErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "jwt_blacklist_redis_errors_total",
				Help: "Total Redis errors while checking JWT blacklist.",
			},
		),
		LoginLockouts: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "login_lockouts_total",
				Help: "Total login lockout activations.",
			},
		),
		LockReleaseErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "lock_release_errors_total",
				Help: "Total distributed lock release errors.",
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
		m.AuthEventReasons,
		m.EmailSendErrors,
		m.EmailCircuitOpen,
		m.EmailCircuitState,
		m.IdempotencyBypass,
		m.IdempotencyHits,
		m.IdempotencyConflict,
		m.JWTBlacklistRedisErrors,
		m.LoginLockouts,
		m.LockReleaseErrors,
	)

	return m
}

func (m *Metrics) IncAuthEvent(event string) {
	if m == nil {
		return
	}
	m.AuthEvents.WithLabelValues(event).Inc()
}

func (m *Metrics) IncAuthEventReason(event, reason string) {
	if m == nil {
		return
	}
	m.AuthEventReasons.WithLabelValues(event, reason).Inc()
}

func (m *Metrics) IncJWTBlacklistRedisError() {
	if m == nil {
		return
	}
	m.JWTBlacklistRedisErrors.Inc()
}

func (m *Metrics) IncLockReleaseError() {
	if m == nil {
		return
	}
	m.LockReleaseErrors.Inc()
}
