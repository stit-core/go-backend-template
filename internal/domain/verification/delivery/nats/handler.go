package nats

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
)

const (
	// streamUser — JetStream-стрим, покрывающий user.> (источник user.events.registered.v2).
	streamUser  = "user"
	durableName = "verification-user-registered-v2"
)

type Usecase interface {
	IssueEmailVerification(ctx context.Context, userID string) error
}

type Handler struct {
	uc  Usecase
	log logkit.Logger
}

func NewHandler(uc Usecase, log logkit.Logger) *Handler {
	return &Handler{uc: uc, log: log.With("delivery", "verification.nats")}
}

// onUserRegistered: ошибку логирует natskit вместе с повтором доставки — здесь её только возвращают.
func (h *Handler) onUserRegistered(ctx context.Context, e UserRegisteredEvent) error {
	if err := h.uc.IssueEmailVerification(ctx, e.ID); err != nil {
		return fmt.Errorf("issue email verification for user %s: %w", e.ID, err)
	}
	h.log.Info(ctx, "email verification handled", "user_id", e.ID)
	return nil
}

func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
	natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
		return natskit.SubscribeStreamJSON(bus, ctx, streamUser, SubjectUserRegistered, durableName, h.onUserRegistered)
	})
}
