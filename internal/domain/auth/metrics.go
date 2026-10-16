package auth

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"git.stit.tech/stit-core/go-backend-template/pkg/outcome"
	"git.stit.tech/stit-core/golibinfra/bizmetric"
)

// Исходы, особые для домена; общие — outcome.
var (
	resultInvalidLogin = outcome.New("invalid_credentials")
	resultLocked       = outcome.New("locked")
	resultBlocked      = outcome.New("blocked")
	resultReused       = outcome.New("reused")
	resultBusy         = outcome.New("busy")
)

// Metrics — сигналы для алертов: перебор паролей (исходы входа) и кража сессии (повтор refresh).
type Metrics struct {
	logins    metric.Int64Counter
	refreshes metric.Int64Counter
}

func NewMetrics(m metric.Meter) *Metrics {
	return &Metrics{
		logins:    bizmetric.Counter(m, "auth", "login_attempts", metric.WithDescription("Login attempts by outcome")),
		refreshes: bizmetric.Counter(m, "auth", "refresh", metric.WithDescription("Session refreshes by outcome")),
	}
}

// Record* nil-safe: usecase без метрик (тест, утилита) работает так же.

func (m *Metrics) RecordLogin(ctx context.Context, result outcome.Result) {
	if m != nil {
		m.logins.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
	}
}

func (m *Metrics) RecordRefresh(ctx context.Context, result outcome.Result) {
	if m != nil {
		m.refreshes.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
	}
}
