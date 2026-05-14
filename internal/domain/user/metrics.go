package user

import (
	"go.opentelemetry.io/otel/metric"

	"git.stit.tech/stit/golibinfra/bizmetric"
)

// Metrics — domain-уровень счётчики. Имена строятся через bizmetric.*
// (конвенция `biz_<domain>_<event>_total|_seconds`). Это позволяет
// дашборду Apps/business автодискаверить метрики через
// {__name__=~"biz_.*", service_name="$service"} без ручного списка.
type Metrics struct {
	Registered metric.Int64Counter
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	return &Metrics{
		Registered: bizmetric.Counter(
			m, "users", "registered",
			metric.WithDescription("Successfully registered users"),
		),
	}, nil
}
