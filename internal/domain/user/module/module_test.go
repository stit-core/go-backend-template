package module

import (
	"context"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"git.stit.tech/akadmin/go-backend-template/internal/domain/user"
	"git.stit.tech/akadmin/golibinfra/auditkit"
	"git.stit.tech/akadmin/golibinfra/authnkit"
	"git.stit.tech/akadmin/golibinfra/clockkit"
	"git.stit.tech/akadmin/golibinfra/logkit"
	"git.stit.tech/akadmin/golibinfra/outboxkit"
	"git.stit.tech/akadmin/golibinfra/pgkit"
	"git.stit.tech/akadmin/golibinfra/testkit/eventbus"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	noopt "go.opentelemetry.io/otel/trace/noop"
)

func TestModuleAssembles(t *testing.T) {
	app := fxtest.New(
		t,
		fx.NopLogger,
		Module,
		fx.Provide(
			func() *pgkit.DB { return nil },
			fx.Annotate(func() pgkit.Transactor { return stubTx{} }, fx.As(new(pgkit.Transactor))),
			func() outboxkit.Publisher { return &eventbus.Recorder{} },
			func() authnkit.Hasher { return stubHasher{} },
			func() auditkit.Logger { return stubAudit{} },
			func() clockkit.Clock { return clockkit.Real{} },
			func() user.EmailVerificationIssuer { return stubIssuer{} },
			logkit.Nop,
			func() metric.Meter { return noopmetric.NewMeterProvider().Meter("test") },
			func() trace.Tracer { return noopt.NewTracerProvider().Tracer("test") },
			func() huma.API {
				return humachi.New(chi.NewRouter(), huma.DefaultConfig("test", "0.0.1"))
			},
		),
	)
	defer app.RequireStart().RequireStop()
}

type stubTx struct{}

func (stubTx) WithTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type stubHasher struct{}

func (stubHasher) Hash(string) (string, error)         { return "", nil }
func (stubHasher) Verify(string, string) (bool, error) { return true, nil }

type stubAudit struct{}

func (stubAudit) Log(context.Context, auditkit.Entry) error { return nil }

type stubIssuer struct{}

func (stubIssuer) IssueEmailVerification(context.Context, string, string) error { return nil }
