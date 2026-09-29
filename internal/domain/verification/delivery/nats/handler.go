package nats

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
)

const (
	// streamUser — JetStream-стрим, покрывающий user.> (источник user.events.registered).
	streamUser  = "user"
	durableName = "verification-user-registered"
)

type Usecase interface {
	IssueEmailVerification(ctx context.Context, userID, email string) error
}

type Handler struct {
	uc  Usecase
	log logkit.Logger
}

func NewHandler(uc Usecase, log logkit.Logger) *Handler {
	return &Handler{uc: uc, log: log.With("delivery", "verification.nats")}
}

func (h *Handler) onUserRegistered(ctx context.Context, e UserRegisteredEvent) error {
	if err := h.uc.IssueEmailVerification(ctx, e.ID, e.Email); err != nil {
		h.log.Error(ctx, "issue email verification",
			"user_id", e.ID, "email", e.Email, "err", err)
		return fmt.Errorf("issue email verification: %w", err)
	}
	h.log.Info(ctx, "email verification issued",
		"user_id", e.ID, "email", e.Email)
	return nil
}

func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
	natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
		return natskit.SubscribeStreamJSON(bus, ctx, streamUser, SubjectUserRegistered, durableName, h.onUserRegistered)
	})
}
