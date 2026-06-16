#!/usr/bin/env bash
# new-domain.sh — скаффолд скелета нового домена в internal/domain/<name>/.
#
# Использование:
#   make new-domain NAME=order
#   bash scripts/new-domain.sh order
#
# Создаёт минимальную структуру по конвенции `.claude/rules/domain-architecture.md`.
# Дальше дописывает skill `scaffold` (вариант domain, references/domain.md): usecase,
# repository, handler, моки, тесты, миграцию, fx-wiring.

set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
    echo "ERROR: usage: $0 <domain_name>" >&2
    echo "       или: make new-domain NAME=<domain_name>" >&2
    exit 1
fi

# snake_case проверка
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
MODULE_PATH=$(awk '/^module / {print $2}' "$ROOT/go.mod")

echo "→ создаю $DOMAIN_DIR/"

mkdir -p "$DOMAIN_DIR/delivery/http"
mkdir -p "$DOMAIN_DIR/infra/postgres/queries"
mkdir -p "$DOMAIN_DIR/module"

cat >"$DOMAIN_DIR/entity.go" <<EOF
package $NAME

import "time"

// $TITLE — корневая сущность домена.
type $TITLE struct {
	ID        string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}
EOF

cat >"$DOMAIN_DIR/dto.go" <<EOF
package $NAME

// Create${TITLE}Input — вход usecase для создания.
type Create${TITLE}Input struct{}

// Create${TITLE}Output — выход usecase для создания.
type Create${TITLE}Output struct {
	${TITLE} *$TITLE
}
EOF

cat >"$DOMAIN_DIR/errors.go" <<EOF
package $NAME

import "errors"

var (
	ErrNotFound = errors.New("$NAME: not found")
	ErrConflict = errors.New("$NAME: version conflict")
)
EOF

cat >"$DOMAIN_DIR/events.go" <<EOF
package $NAME

import "time"

// Subject — литерал \`$NAME.events.created\` (см. .claude/rules/events-and-messaging.md).
// Публикатор передаёт его прямо в \`events.Publish(...)\`; consumer заводит свою
// локальную константу в \`<consumer>/delivery/nats/dto.go\`.
type ${TITLE}Created struct {
	${TITLE}ID string    \`json:"${NAME}_id"\`
	Timestamp time.Time \`json:"timestamp"\`
}
EOF

cat >"$DOMAIN_DIR/policy.go" <<EOF
package $NAME

import "context"

// Policy — authorization decisions для домена.
type Policy struct{}

func NewPolicy() *Policy { return &Policy{} }

func (p *Policy) CanCreate(_ context.Context) error {
	return nil
}
EOF

cat >"$DOMAIN_DIR/policy_test.go" <<EOF
package ${NAME}_test

import (
	"context"
	"testing"

	"$MODULE_PATH/internal/domain/$NAME"
)

func TestPolicy_CanCreate(t *testing.T) {
	p := $NAME.NewPolicy()
	if err := p.CanCreate(context.Background()); err != nil {
		t.Fatalf("CanCreate: %v", err)
	}
}
EOF

cat >"$DOMAIN_DIR/usecase.go" <<EOF
package $NAME

import "context"

// Repository — порт persistence для $NAME.
type Repository interface {
	Insert(ctx context.Context, e *$TITLE) error
	GetByID(ctx context.Context, id string) (*$TITLE, error)
}

// Authorizer — порт policy decisions.
type Authorizer interface {
	CanCreate(ctx context.Context) error
}

// Usecase — бизнес-логика домена.
type Usecase struct {
	repo Repository
	auth Authorizer
}

func NewUsecase(repo Repository, auth Authorizer) *Usecase {
	return &Usecase{repo: repo, auth: auth}
}
EOF

cat >"$DOMAIN_DIR/usecase_test.go" <<EOF
package ${NAME}_test

import (
	"testing"

	"$MODULE_PATH/internal/domain/$NAME"
)

func TestUsecase_New(t *testing.T) {
	uc := $NAME.NewUsecase(nil, nil)
	if uc == nil {
		t.Fatal("usecase is nil")
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/handler.go" <<EOF
package http

import (
	"github.com/danielgtaylor/huma/v2"
)

// Usecase — порт для handler-а.
type Usecase interface{}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

// Register — регистрирует роуты в huma API.
func Register(api huma.API, h *Handler) {
	_ = api
	_ = h
	// huma.Register(api, huma.Operation{...}, h.Create${TITLE})
}
EOF

cat >"$DOMAIN_DIR/delivery/http/dto.go" <<EOF
package http
EOF

cat >"$DOMAIN_DIR/delivery/http/view.go" <<EOF
package http
EOF

cat >"$DOMAIN_DIR/delivery/http/handler_test.go" <<EOF
package http_test

import (
	"testing"

	httpdelivery "$MODULE_PATH/internal/domain/$NAME/delivery/http"
)

func TestNewHandler(t *testing.T) {
	if h := httpdelivery.NewHandler(nil); h == nil {
		t.Fatal("handler is nil")
	}
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/repository.go" <<EOF
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"$MODULE_PATH/internal/domain/$NAME"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

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

# infra/postgres/queries/$NAME.sql — пустой файл с placeholder'ом
cat >"$DOMAIN_DIR/infra/postgres/queries/$NAME.sql" <<EOF
-- Запросы для $NAME домена.
-- Следуй sqlc-конвенции: '-- name: <Method> :<one|many|exec|execrows>'
-- См. .claude/rules/sqlc-and-migrations.md

-- name: GetByID :one
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
	"$NAME",
	fx.Provide(
		${NAME}pg.NewRepository,
		fx.Annotate(${NAME}.NewPolicy, fx.As(new(${NAME}.Authorizer))),
		func(r *${NAME}pg.Repository) ${NAME}.Repository { return r },
		${NAME}.NewUsecase,
		${NAME}http.NewHandler,
		func(u *${NAME}.Usecase) ${NAME}http.Usecase { return u },
	),
	fx.Invoke(${NAME}http.Register),
)
EOF

cat >"$DOMAIN_DIR/module/adapters.go" <<EOF
package module

// Cross-domain адаптеры: если $NAME-домену нужны данные из другого домена,
// определи порт в $NAME/usecase.go и реализуй адаптер здесь.
EOF

echo
echo "✓ создан $DOMAIN_DIR/"
echo
echo "Дальше (см. .claude/skills/scaffold/references/domain.md):"
echo "  1. Добавь миграцию: migrations/NNNN_${NAME}.{up,down}.sql"
echo "  2. Добавь sqlc-блок в sqlc.yaml для пути queries/$NAME"
echo "  3. Заполни запросы в infra/postgres/queries/$NAME.sql"
echo "  4. make gen — регенерация sqlc+mocks"
echo "  5. Заполни usecase, handler, тесты"
echo "  6. Подключи модуль в cmd/api/main.go (domainModules)"
echo
echo "─── snippet для .mockery.yml (вставить под секцию packages:) ───"
cat <<MOCKERY
  ${MODULE_PATH}/internal/domain/${NAME}:
    interfaces:
      Repository:
      Authorizer:
  ${MODULE_PATH}/internal/domain/${NAME}/delivery/http:
    interfaces:
      Usecase:
MOCKERY
echo
echo "─── snippet для .golangci.yml (depguard rules, изоляция домена) ───"
cat <<DEPGUARD
        ${NAME}-domain-isolation:
          list-mode: lax
          files:
            - '**/internal/domain/$NAME/**'
            - '!**/*_test.go'
            - '!**/internal/domain/$NAME/module/**'
          allow:
            - '${MODULE_PATH}/internal/domain/$NAME'
          deny:
            - pkg: '${MODULE_PATH}/internal/domain'
              desc: 'cross-domain imports are forbidden; use a port + adapter in module/'
DEPGUARD
echo
echo "После правок: make gen && make check"
