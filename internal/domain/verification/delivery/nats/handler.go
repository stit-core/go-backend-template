package nats

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/logkit"
	"git.stit.tech/stit-core/golibinfra/natskit"
)

const (
	// streamUser — JetStream-стрим, покрывающий user.> (источник user.events.registered.v2).
	streamUser            = "user"
	durableUserRegistered = "verification-user-registered-v2"
	// streamVerification — свой стрим домена: письмо сброса шлёт подписчик своего события после коммита запроса.
	streamVerification            = "verification"
	durablePasswordResetRequested = "verification-password-reset-requested"
)

type Usecase interface {
	IssueEmailVerification(ctx context.Context, userID string) error
	IssuePasswordReset(ctx context.Context, requestID, userID string, requestedAt time.Time) error
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

// onPasswordResetRequested: ошибку логирует natskit вместе с повтором доставки — здесь её только возвращают.
func (h *Handler) onPasswordResetRequested(ctx context.Context, e PasswordResetRequestedEvent) error {
	if err := h.uc.IssuePasswordReset(ctx, e.RequestID, e.UserID, e.At); err != nil {
		return fmt.Errorf("issue password reset %s: %w", e.RequestID, err)
	}
	h.log.Info(ctx, "password reset handled", "request_id", e.RequestID)
	return nil
}

func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
	natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
		return natskit.SubscribeStreamJSON(bus, ctx, streamUser, SubjectUserRegistered, durableUserRegistered, h.onUserRegistered)
	})
	natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
		return natskit.SubscribeStreamJSON(bus, ctx, streamVerification, SubjectPasswordResetRequested, durablePasswordResetRequested,
			h.onPasswordResetRequested)
	})
}
