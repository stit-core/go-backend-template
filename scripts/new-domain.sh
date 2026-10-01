#!/usr/bin/env bash
# new-domain.sh — скаффолд скелета нового домена в internal/domain/<name>/.
#
# Использование:
#   make new-domain NAME=order
#   bash scripts/new-domain.sh order
#
# Создаёт минимальную структуру по канону `.claude/rules/domain-architecture.md`
# (обязательность файлов, naming, policy.Decision, subject-const) и сам вписывает
# интерфейсы в .mockery.yml; границы нового домена держит internal/archtest без правки
# конфигов. Дальше дописывает skill `scaffold` (вариант domain): полноценный usecase, sqlc,
# handler, миграция.

set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
    echo "ERROR: usage: $0 <domain_name>" >&2
    echo "       или: make new-domain NAME=<domain_name>" >&2
    exit 1
fi

if ! [[ "$NAME" =~ ^[a-z][a-z0-9_]*$ ]]; then
    echo "ERROR: имя домена — snake_case (a-z, 0-9, _), начинается с буквы" >&2
    echo "       получено: '$NAME'" >&2
    exit 1
fi

ROOT=$(git rev-parse --show-toplevel)
DOMAIN_DIR="$ROOT/internal/domain/$NAME"

if [ -d "$DOMAIN_DIR" ]; then
    echo "ERROR: $DOMAIN_DIR уже существует" >&2
    exit 1
fi

# Title-case для типов: order → Order, multi_word → MultiWord
TITLE=$(echo "$NAME" | sed -E 's/(^|_)([a-z])/\U\2/g')
# lowerCamel для переменных — имя таблицы ошибок, которое ждёт archtest: multi_word → multiWordErrors
CAMEL=$(echo "$TITLE" | sed -E 's/^./\L&/')
MODULE_PATH=$(awk '/^module / {print $2}' "$ROOT/go.mod")

echo "→ создаю $DOMAIN_DIR/"

mkdir -p "$DOMAIN_DIR/delivery/http"
mkdir -p "$DOMAIN_DIR/infra/postgres/queries"
mkdir -p "$DOMAIN_DIR/module"

cat >"$DOMAIN_DIR/entity.go" <<EOF
package $NAME

import "time"

type $TITLE struct {
	ID        string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}
EOF

cat >"$DOMAIN_DIR/dto.go" <<EOF
package $NAME

// Канон naming — <Op>Input/<Op>Output (rule domain-architecture).
// Output-DTO заводится, только когда результат usecase ≠ entity и ≠ скаляр.
type CreateInput struct{}
EOF

cat >"$DOMAIN_DIR/errors.go" <<EOF
package $NAME

import "errors"

var (
	ErrNotFound         = errors.New("$NAME: not found")
	ErrConcurrentUpdate = errors.New("$NAME: concurrent update")
)
EOF

cat >"$DOMAIN_DIR/policy.go" <<EOF
package $NAME

import (
	"context"

	"$MODULE_PATH/internal/platform/identity"
	"$MODULE_PATH/pkg/policy"
)

type Policy struct{}

func NewPolicy() Authorizer { return &Policy{} }

func (p *Policy) CanCreate(_ context.Context, caller identity.Identity) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	return policy.Deny("admin only")
}
EOF

cat >"$DOMAIN_DIR/policy_test.go" <<EOF
package ${NAME}_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	"$MODULE_PATH/internal/platform/identity"
)

func TestPolicy_CanCreate(t *testing.T) {
	t.Parallel()
	p := $NAME.NewPolicy()
	cases := []struct {
		name   string
		caller identity.Identity
		want   bool
	}{
		{"admin", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, true},
		{"client", identity.Identity{UserID: "u1", Role: identity.RoleClient}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanCreate(context.Background(), tc.caller).IsAllowed)
		})
	}
}
EOF

cat >"$DOMAIN_DIR/usecase.go" <<EOF
package $NAME

import (
	"context"

	"go.uber.org/fx"

	"$MODULE_PATH/internal/platform/identity"
	"$MODULE_PATH/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

type Repository interface {
	Insert(ctx context.Context, e *$TITLE) error
	GetByID(ctx context.Context, id string) (*$TITLE, error)
}

type Authorizer interface {
	CanCreate(ctx context.Context, caller identity.Identity) policy.Decision
}

type UsecaseDeps struct {
	fx.In

	Repo   Repository
	Policy Authorizer
	Log    logkit.Logger
	Clock  clockkit.Clock
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "$NAME")
	return &Usecase{d: d}
}
EOF

cat >"$DOMAIN_DIR/usecase_test.go" <<EOF
package ${NAME}_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	"git.stit.tech/stit-core/golibinfra/clockkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestNewUsecase(t *testing.T) {
	t.Parallel()
	uc := $NAME.NewUsecase($NAME.UsecaseDeps{
		Log:   logkit.Nop(),
		Clock: clockkit.NewFixed(fixedNow),
	})
	require.NotNil(t, uc)
}
EOF

cat >"$DOMAIN_DIR/delivery/http/handler.go" <<EOF
package http

import (
	"github.com/danielgtaylor/huma/v2"
)

type Usecase interface{}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

func Register(api huma.API, h *Handler) {
	_ = api
	_ = h
	// huma.Register(api, huma.Operation{OperationID: "create${TITLE}", ...}, h.create)
	// HTTP-DTO — в dto.go, имена ${TITLE}<Op>Input/Output (huma-registry глобальна);
	// ошибки — таблица домена ${CAMEL}Errors (правило на каждую ошибку errors.go, Map в хендлере) и
	// таблица операции из тех же правил для Declare (см. user/delivery/http).
}
EOF

cat >"$DOMAIN_DIR/delivery/http/handler_test.go" <<EOF
package http_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	httpdelivery "$MODULE_PATH/internal/domain/$NAME/delivery/http"
)

func TestNewHandler(t *testing.T) {
	t.Parallel()
	require.NotNil(t, httpdelivery.NewHandler(nil))
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/repository.go" <<EOF
package postgres

import (
	"context"

	"$MODULE_PATH/internal/domain/$NAME"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type Repository struct {
	db *pgkit.DB
}

func NewRepository(database *pgkit.DB) ${NAME}.Repository {
	return &Repository{db: database}
}

// После sqlc-генерации замени db на db.Queries (см. auth/infra/postgres/repository.go).

func (r *Repository) Insert(ctx context.Context, e *${NAME}.${TITLE}) error {
	_ = ctx
	_ = e
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*${NAME}.${TITLE}, error) {
	_ = ctx
	_ = id
	return nil, ${NAME}.ErrNotFound
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/queries/$NAME.sql" <<EOF
-- Запросы для $NAME домена.
-- sqlc-конвенция: '-- name: <Method> :<one|many|exec|execrows>'; создание — Insert<Entity>.
-- См. .claude/rules/sqlc-and-migrations.md

-- name: Get${TITLE} :one
-- SELECT * FROM ${NAME}s WHERE id = \$1;
EOF

cat >"$DOMAIN_DIR/module/module.go" <<EOF
package module

import (
	"go.uber.org/fx"

	"$MODULE_PATH/internal/domain/$NAME"
	${NAME}http "$MODULE_PATH/internal/domain/$NAME/delivery/http"
	${NAME}pg "$MODULE_PATH/internal/domain/$NAME/infra/postgres"
)

var Module = fx.Module(
	"internal/domain/$NAME/module",
	fx.Provide(
		${NAME}pg.NewRepository,
		${NAME}.NewPolicy,
		${NAME}.NewUsecase,
		${NAME}http.NewHandler,
		func(u *${NAME}.Usecase) ${NAME}http.Usecase { return u },
	),
)

// Cross-domain порт нужен → тип-адаптер и его конструктор в module/adapters.go
// (identity-closure — только внутри домена). См. rule domain-architecture.
EOF

# ── авто-вписывание конфигов ──────────────────────────────────────────────

echo "→ вписываю интерфейсы в .mockery.yml"
cat >>"$ROOT/.mockery.yml" <<EOF
  ${MODULE_PATH}/internal/domain/${NAME}:
    interfaces:
      Repository:
      Authorizer:
  ${MODULE_PATH}/internal/domain/${NAME}/delivery/http:
    interfaces:
      Usecase:
EOF

echo
echo "✓ создан $DOMAIN_DIR/ + интерфейсы вписаны в .mockery.yml"
echo
echo "Дальше (см. .claude/skills/scaffold/references/domain.md):"
echo "  1. Миграция: migrations/NNNN_${NAME}.{up,down}.sql (нумерация непрерывна — migrations-check)"
echo "  2. sqlc-блок в sqlc.yaml для infra/postgres/queries/$NAME + запросы"
echo "  3. make gen — регенерация sqlc+mocks"
echo "  4. Usecase/handler/тесты; HTTP-DTO в delivery/http/dto.go (${TITLE}<Op>Input/Output),"
echo "     view.go — когда появится тело ответа; events.go (payload + unexported subject-const)"
echo "     + infra/outbox/outbox.go — при первой публикации (rule events-and-messaging)"
echo "  5. Подключи модуль в cmd/api/main.go (domainModules), HTTP-вход — в cmd/api/http.go (registerHTTP)"
echo "  6. make check (нужен Docker)"
