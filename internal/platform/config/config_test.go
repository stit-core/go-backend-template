package config_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"git.stit.tech/stit-core/go-backend-template/internal/platform/authgate"
	appconfig "git.stit.tech/stit-core/go-backend-template/internal/platform/config"
	libconfig "git.stit.tech/stit-core/golibinfra/config"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type sourceFunc func(ctx context.Context, dst any) error

func (f sourceFunc) Apply(ctx context.Context, dst any) error { return f(ctx, dst) }

func TestLoad(t *testing.T) {
	t.Parallel()

	errSource := errors.New("source failed")

	set := func(mutate func(app *appconfig.App)) libconfig.Source {
		return sourceFunc(func(_ context.Context, dst any) error {
			app, ok := dst.(*appconfig.App)
			if !ok {
				return errors.New("unexpected destination type")
			}
			mutate(app)
			return nil
		})
	}

	cases := []struct {
		name    string
		srcs    []libconfig.Source
		wantErr error
		want    appconfig.App
	}{
		{
			name: "no sources yields zero config",
			want: appconfig.App{},
		},
		{
			name: "later source overrides earlier, untouched fields survive",
			srcs: []libconfig.Source{
				set(func(app *appconfig.App) {
					app.Postgres.DSN = "postgres://first"
					app.Cookie.Secure = true
				}),
				set(func(app *appconfig.App) {
					app.Postgres.DSN = "postgres://second"
				}),
			},
			want: appconfig.App{
				Postgres: pgkit.PoolConfig{DSN: "postgres://second"},
				Cookie:   authgate.CookieConfig{Secure: true},
			},
		},
		{
			name: "source error aborts loading",
			srcs: []libconfig.Source{
				sourceFunc(func(_ context.Context, _ any) error { return errSource }),
			},
			wantErr: errSource,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := appconfig.Load(context.Background(), tc.srcs)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tc.want, *got)
		})
	}
}
