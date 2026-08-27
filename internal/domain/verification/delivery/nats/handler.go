package nats

import (
	"context"
	"errors"
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
	// Background is the legitimate root here: the subscription outlives OnStart's ctx.
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			err := natskit.SubscribeStreamJSON(bus, ctx, streamUser, SubjectUserRegistered, durableName, h.onUserRegistered)
			// JetStream выключен (local-dev без -js) → лог + skip: сервис стартует,
			// событие просто не потребляется (для приёма нужен NATS_JETSTREAM_ENABLED).
			if errors.Is(err, natskit.ErrJetStreamDisabled) {
				h.log.Warn(ctx, "jetstream disabled: user-registered intake off", "subject", SubjectUserRegistered)
				return nil
			}
			if err != nil {
				return fmt.Errorf("subscribe %s: %w", SubjectUserRegistered, err)
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})
}
