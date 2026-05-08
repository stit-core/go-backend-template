package user

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	Registered metric.Int64Counter
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	c, err := m.Int64Counter(
		"users_registered_total",
		metric.WithDescription("Number of successfully registered users"),
	)
	if err != nil {
		return nil, fmt.Errorf("user metrics: counter: %w", err)
	}
	return &Metrics{Registered: c}, nil
}
