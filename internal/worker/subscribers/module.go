package subscribers

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"git.stit.tech/akadmin/golibinfra/logkit"
	"git.stit.tech/akadmin/golibinfra/natskit"

	"git.stit.tech/akadmin/golibinfra/auditkit"
)

var Module = fx.Module(
	"worker.subscribers",
	fx.Invoke(register),
)

func register(lc fx.Lifecycle, bus natskit.Bus, log logkit.Logger, auditLog auditkit.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			if err := natskit.SubscribeJSON(bus, ctx, SubjectUserRegistered, OnUserRegistered(log, auditLog)); err != nil {
				return fmt.Errorf("subscribe %s: %w", SubjectUserRegistered, err)
			}
			if err := natskit.SubscribeJSON(bus, ctx, SubjectEmailVerificationRequested, OnEmailVerificationRequested(log, auditLog)); err != nil {
				return fmt.Errorf("subscribe %s: %w", SubjectEmailVerificationRequested, err)
			}
			if err := natskit.SubscribeJSON(bus, ctx, SubjectPasswordResetRequested, OnPasswordResetRequested(log, auditLog)); err != nil {
				return fmt.Errorf("subscribe %s: %w", SubjectPasswordResetRequested, err)
			}
			log.Info(ctx, "worker subscribers registered")
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})
}
