package user

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"git.stit.tech/stit-infra/golibinfra/bizmetric"
)

// Metrics — domain-уровень счётчики user (register/update/delete).
// Имена через bizmetric.* → `biz_users_*_total`.
type Metrics struct {
	registered metric.Int64Counter
	updated    metric.Int64Counter
	deleted    metric.Int64Counter
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	return &Metrics{
		registered: bizmetric.Counter(
			m, "users", "registered",
			metric.WithDescription("User registrations by outcome"),
		),
		updated: bizmetric.Counter(
			m, "users", "updated",
			metric.WithDescription("User profile/status updates by outcome"),
		),
		deleted: bizmetric.Counter(
			m, "users", "deleted",
			metric.WithDescription("User soft-deletes by outcome"),
		),
	}, nil
}

// Все Record* nil-safe — observability не валит домен.

func (m *Metrics) RecordRegistered(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.registered.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *Metrics) RecordUpdated(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.updated.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *Metrics) RecordDeleted(ctx context.Context, result string) {
	if m == nil {
		return
	}
	m.deleted.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}
