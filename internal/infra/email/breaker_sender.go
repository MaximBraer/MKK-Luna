package email

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sony/gobreaker"

	"MKK-Luna/internal/config"
	metricsinfra "MKK-Luna/internal/infra/metrics"
)

type BreakerSender struct {
	next    Sender
	cb      *gobreaker.CircuitBreaker
	logger  *slog.Logger
	metrics *metricsinfra.Metrics
}

type Sender interface {
	SendInvite(ctx context.Context, toEmail, teamName string) error
}

func NewBreakerSender(next Sender, cfg config.CircuitBreakerConfig, logger *slog.Logger, metrics *metricsinfra.Metrics) *BreakerSender {
	settings := gobreaker.Settings{
		Name:        "email",
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.FailureThreshold
		},
	}
	cb := gobreaker.NewCircuitBreaker(settings)
	return &BreakerSender{next: next, cb: cb, logger: logger, metrics: metrics}
}

func (s *BreakerSender) SendInvite(ctx context.Context, toEmail, teamName string) error {
	if s.next == nil {
		return errors.New("email sender is nil")
	}

	_, err := s.cb.Execute(func() (any, error) {
		return nil, s.next.SendInvite(ctx, toEmail, teamName)
	})
	s.observeState()
	if err != nil {
		if s.metrics != nil {
			s.metrics.EmailSendErrors.Inc()
			if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
				s.metrics.EmailCircuitOpen.Inc()
			}
		}
		if s.logger != nil {
			s.logger.Warn("email send failed", "err", err)
		}
		return err
	}
	return nil
}

func (s *BreakerSender) observeState() {
	if s.metrics == nil || s.cb == nil {
		return
	}
	switch s.cb.State() {
	case gobreaker.StateClosed:
		s.metrics.EmailCircuitState.Set(0)
	case gobreaker.StateHalfOpen:
		s.metrics.EmailCircuitState.Set(1)
	case gobreaker.StateOpen:
		s.metrics.EmailCircuitState.Set(2)
	}
}
