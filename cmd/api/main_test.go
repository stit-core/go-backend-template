package main

import (
	"context"
	"testing"

	"go.uber.org/fx"

	"git.stit.tech/stit-core/golibinfra/logkit"
)

// Граф fx должен быть разрешим (все зависимости предоставлены), включая per-domain
// outbox (fx.Private в доменных модулях) и durable JetStream subscriber поверх
// natskit.StreamBus. ValidateApp проверяет wiring без запуска lifecycle и без
// обращения к pg/nats.
func TestFxGraphValidates(t *testing.T) {
	err := fx.ValidateApp(
		infraModules,
		platformModules,
		domainModules,
		fx.Invoke(func(context.Context, logkit.Logger) {}),
	)
	if err != nil {
		t.Fatalf("fx graph invalid: %v", err)
	}
}
