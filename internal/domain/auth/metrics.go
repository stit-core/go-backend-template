package auth

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"git.stit.tech/stit-core/golibinfra/bizmetric"
)

// Metrics — security-relevant counters auth-домена. Имена строятся
// через bizmetric.* → `biz_auth_<event>_total`. Подходят под алерты
// уровня:
//
//	biz_auth_login_attempts_total{result="invalid_credentials"} > 100/min  → brute force
//	biz_auth_refresh_total{result="reused"} > 0                            → token reuse (security incident)
type Metrics struct {
	loginAttempts   metric.Int64Counter
	refreshAttempts metric.Int64Counter
	logouts         metric.Int64Counter
	passwordChange  metric.Int64Counter
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	return &Metrics{
		loginAttempts: bizmetric.Counter(
			m, "auth", "login_attempts",
			metric.WithDescription("Login attempts by outcome"),
		),
		refreshAttempts: bizmetric.Counter(
			m, "auth", "refresh",
			metric.WithDescription("Refresh-token attempts by outcome"),
		),
		logouts: bizmetric.Counter(
			m, "auth", "logouts",
			metric.WithDescription("Successful logouts"),
		),
		passwordChange: bizmetric.Counter(
			m, "auth", "password_change",
			metric.WithDescription("Password change attempts by outcome"),
		),
	}, nil
}

// Все Record* nil-safe: если Metrics не provide'ен в DI (unit-тесты,
// CLI-утилиты без Metrics-провайдера) — no-op. Observability не должна
// валить домен.

func (m *Metrics) RecordLogin(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.loginAttempts.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *Metrics) RecordRefresh(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.refreshAttempts.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *Metrics) RecordLogout(ctx context.Context) {
	if m == nil {
		return
	}
	m.logouts.Add(ctx, 1)
}

func (m *Metrics) RecordPasswordChange(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.passwordChange.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}
