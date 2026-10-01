package verification

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/golibinfra/bizmetric"
)

// Исходы, особые для домена; общие — outcome.
var (
	resultDuplicate    = outcome.New("duplicate")
	resultAlreadyUsed  = outcome.New("already_used")
	resultUserNotFound = outcome.New("user_not_found")
	resultInactive     = outcome.New("inactive")
	resultThrottled    = outcome.New("throttled")
)

// Metrics — domain-уровень counters для verification (email confirm +
// password reset flows). Имена через bizmetric.* → `biz_verification_*`.
//
// Полезные алерты:
//
//	biz_verification_password_reset_requested_total{result="user_not_found"} > 50/min
//	    → enumeration probe от спамеров
//	biz_verification_email_issued_total{result="error"} > 10/min
//	    → проблемы с mailer'ом
type Metrics struct {
	emailIssued            metric.Int64Counter
	emailConfirmed         metric.Int64Counter
	passwordResetRequested metric.Int64Counter
	passwordResetConfirmed metric.Int64Counter
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	return &Metrics{
		emailIssued: bizmetric.Counter(
			m, "verification", "email_issued",
			metric.WithDescription("Email verification tokens issued by outcome"),
		),
		emailConfirmed: bizmetric.Counter(
			m, "verification", "email_confirmed",
			metric.WithDescription("Email verification confirmations by outcome"),
		),
		passwordResetRequested: bizmetric.Counter(
			m, "verification", "password_reset_requested",
			metric.WithDescription("Password reset requests by outcome"),
		),
		passwordResetConfirmed: bizmetric.Counter(
			m, "verification", "password_reset_confirmed",
			metric.WithDescription("Password reset confirmations by outcome"),
		),
	}, nil
}

// Все Record* nil-safe — observability не валит домен.

func (m *Metrics) RecordEmailIssued(ctx context.Context, result outcome.Result) {
	if m == nil {
		return
	}
	m.emailIssued.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
}

func (m *Metrics) RecordEmailConfirmed(ctx context.Context, result outcome.Result) {
	if m == nil {
		return
	}
	m.emailConfirmed.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
}

func (m *Metrics) RecordPasswordResetRequested(ctx context.Context, result outcome.Result) {
	if m == nil {
		return
	}
	m.passwordResetRequested.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
}

func (m *Metrics) RecordPasswordResetConfirmed(ctx context.Context, result outcome.Result) {
	if m == nil {
		return
	}
	m.passwordResetConfirmed.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
}
