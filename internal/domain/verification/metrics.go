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
	resultUserNotFound = outcome.New("user_not_found")
	resultInactive     = outcome.New("inactive")
	resultThrottled    = outcome.New("throttled")
)

// Metrics — сигналы для алертов по запросам сброса: перебор адресов, упёршиеся в лимит запросы и
// письма, которые не ушли (клиент об этом не узнаёт — ответ один).
type Metrics struct {
	resetRequests metric.Int64Counter
}

func NewMetrics(m metric.Meter) *Metrics {
	return &Metrics{
		resetRequests: bizmetric.Counter(m, "verification", "password_reset_requested",
			metric.WithDescription("Password reset requests by outcome")),
	}
}

// RecordPasswordResetRequested nil-safe: usecase без метрик (тест, утилита) работает так же.
func (m *Metrics) RecordPasswordResetRequested(ctx context.Context, result outcome.Result) {
	if m != nil {
		m.resetRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result.String())))
	}
}
