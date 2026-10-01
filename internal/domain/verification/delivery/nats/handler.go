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
	// durableNameV1 — прежний консьюмер: события до .v2 он дочитывает с места, где остановился.
	durableNameV1 = "verification-user-registered"
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

// Register: старый subject читает прежний durable тем же обработчиком — событие, записанное в outbox
// до перехода на .v2 или не доставленное прежнему консьюмеру, письма не теряет (бэклог events-v1-reader).
func Register(lc fx.Lifecycle, bus natskit.StreamBus, h *Handler) {
	subscribe := func(subject, durable string) {
		natskit.SubscribeOnStart(lc, func(ctx context.Context) (natskit.Subscription, error) {
			return natskit.SubscribeStreamJSON(bus, ctx, streamUser, subject, durable, h.onUserRegistered)
		})
	}
	subscribe(SubjectUserRegistered, durableName)
	subscribe(subjectUserRegisteredV1, durableNameV1)
}
