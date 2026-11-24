#!/usr/bin/env bash
# new-domain.sh — новый домен internal/domain/<name>/: вертикальный срез Create и Get его агрегата по канону.
#
# Использование:
#   make new-domain NAME=order                 агрегат — контекст: Order, таблица orders, путь /api/orders
#   make new-domain NAME=objects               агрегат — контекст в единственном числе: Object, таблица objects
#   make new-domain NAME=ordering AGG=order    агрегат Order контекста ordering: таблица orders, путь /api/orders
#   bash scripts/new-domain.sh <контекст> [<агрегат>]
#
# Агрегат — единственное число (order, order_item), таблица и ресурс пути — его множественное (имя на -s, кроме -ss,
# -us и -is, уже множественное или неизменяемое: series). Имена агрегата — тип, порт <Агрегат>Repository, методы
# usecase и policy, HTTP-типы <Домен><Агрегат>, хендлеры и таблицы ошибок операций: следующий агрегат контекста
# встаёт рядом без переименований (skill scaffold, вариант domain).
#
# Создаёт рабочий срез — миграцию таблицы, sqlc-запросы, репозиторий с integration-тестом,
# сущность, policy и usecase с тестами, HTTP-операции под cookie-сессией с тестами, fx-модуль —
# подключает домен в cmd/api, вписывает sqlc.yaml, .mockery.yml и блоки мутаций среза в
# scripts/tools/mutants.txt, сверяет якоря его мутаций (make mutate A=--anchors, нужен python3), гонит make gen,
# проверяет, что срез собирается (go vet с тегом integration), и ставит созданное в индекс: следующий
# шаг — make check.
# Негодное имя и занятая таблица — отказ до первой правки; сбой после неё — откат всех правок скрипта.
# Дальше домен растёт по skill `scaffold`.

set -euo pipefail
# Имя, регистр и множественное число — по ASCII при любой локали: в турецкой [a-z] не берёт «i».
export LC_ALL=C

NAME="${1:-}" AGG="${2:-}"
if [ -z "$NAME" ]; then
    echo "ERROR: usage: $0 <domain_name> [<aggregate_name>]" >&2
    echo "       или: make new-domain NAME=<domain_name> [AGG=<aggregate_name>]" >&2
    exit 1
fi

# Имя проверяется до первой правки: срез с негодным именем не собрался бы, когда cmd/api и make gen
# уже тронуты.
if ! [[ "$NAME" =~ ^[a-z][a-z0-9]*(_[a-z0-9]+)*$ ]]; then
    echo "ERROR: имя домена — snake_case: a-z и 0-9, «_» только между словами, начинается с буквы" >&2
    echo "       получено: '$NAME'" >&2
    exit 1
fi
# Агрегат по умолчанию — контекст в единственном числе: objects → object, categories → category; status, access и
# analysis — уже единственное.
if [ -z "$AGG" ]; then
    case "$NAME" in
        *ss | *us | *is) AGG=$NAME ;;
        *ies) AGG=${NAME%ies}y ;;
        *s) AGG=${NAME%s} ;;
        *) AGG=$NAME ;;
    esac
fi
if ! [[ "$AGG" =~ ^[a-z][a-z0-9]*(_[a-z0-9]+)*$ ]]; then
    echo "ERROR: имя агрегата — snake_case в единственном числе: a-z и 0-9, «_» только между словами, начинается с буквы" >&2
    echo "       получено: '$AGG' — задай его явно: make new-domain NAME=$NAME AGG=<агрегат>" >&2
    exit 1
fi
# Пакет домена и агрегат не могут называться ключевым словом или встроенным именем Go и именем, с которым тип агрегата
# совпал бы с объявлением пакета домена (Usecase, Policy…) или пакет — с пакетом, который он берёт (policy). Имя,
# которое код среза объявляет или зовёт рядом с пакетом домена (repo, id, tests…), ловит сборка среза (go vet
# ниже) с откатом правок: список таких слов отвергал бы и имена, на которых срез собирается (status, body, api…).
reserved=(
    break case chan const continue default defer else fallthrough for func go goto if import interface
    map package range return select struct switch type var
    any append bool byte cap clear close comparable complex complex64 complex128 copy delete error false
    float32 float64 imag int int8 int16 int32 int64 iota len make max min new nil panic print println
    real recover rune string true uint uint8 uint16 uint32 uint64 uintptr
    authorizer create_input policy repository usecase usecase_deps
)
for word in "${reserved[@]}"; do
    for name in "$NAME" "$AGG"; do
        if [ "$name" = "$word" ]; then
            echo "ERROR: '$name' — ключевое слово или встроенное имя Go либо объявление пакета домена: возьми другое" >&2
            exit 1
        fi
    done
done

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
DOMAIN_DIR="internal/domain/$NAME"

if [ -d "$DOMAIN_DIR" ]; then
    echo "ERROR: $DOMAIN_DIR уже существует" >&2
    exit 1
fi
for tool in sqlc mockery gofumpt; do
    command -v "$tool" >/dev/null || { echo "ERROR: $tool не найден — make setup" >&2; exit 1; }
done
# python3 make setup не ставит, а без него срез откатился бы в конце, на сверке якорей своих мутаций.
command -v python3 >/dev/null || { echo "ERROR: python3 не найден — им make new-domain, как make mutate, сверяет мутации: поставь его" >&2; exit 1; }

# Домен order_items, агрегат order_item → OrderItems (префикс HTTP-типов домена), orderItems (таблица ошибок
# домена), orderitems (алиасы импортов), ORDER_ITEMS (коды ошибок); OrderItem (тип агрегата), OrderItems (поле порта в
# зависимостях usecase), order_items (таблица), order-items (путь API). Инициализм — по правилу revive var-naming
# (api_key → APIKey и apiKey): иначе lint требует переименовать типы скелета.
initialisms=" ACL API ASCII CPU CSS DNS EOF GUID HTML HTTP HTTPS ID IDS IP JSON LHS QPS RAM RHS RPC SLA SMTP SQL SSH TCP TLS TTL UDP UI UID UUID URI URL UTF8 VM XML XMPP XSRF XSS "
# title NAME — CamelCase snake_case-имени.
title() {
    local out="" part word
    for part in ${1//_/ }; do
        word=$(tr '[:lower:]' '[:upper:]' <<<"$part")
        case "$initialisms" in
            *" $word "*) [ "$word" != IDS ] || word=IDs ;;
            *) word=$(tr '[:lower:]' '[:upper:]' <<<"${part:0:1}")${part:1} ;;
        esac
        out+=$word
    done
    printf '%s' "$out"
}
TITLE=$(title "$NAME")
first=${NAME%%_*}
CAMEL=$first$(title "${NAME#"$first"}")
FLAT=${NAME//_/}
UPPER=$(echo "$NAME" | tr '[:lower:]' '[:upper:]')
# Множественное число агрегата — по правилам английского (status → statuses, category → categories); имя на -s
# (кроме -ss, -us, -is) — уже множественное или неизменяемое (series). Исключения (person → people) правят руками.
case "$AGG" in
    *ss | *us | *is | *x | *z | *ch | *sh) TABLE="${AGG}es" ;;
    *s) TABLE=$AGG ;;
    *[!aeiou]y) TABLE="${AGG%y}ies" ;;
    *) TABLE="${AGG}s" ;;
esac
ATITLE=$(title "$AGG")
PTITLE=$(title "$TABLE")
# HTTP-типы — с префиксом домена (archtest TestHTTPTypes) и именем агрегата: OrderCreateInput, ObjectsObjectView.
HTTPT=$TITLE
[ "$ATITLE" = "$TITLE" ] || HTTPT+=$ATITLE
ROUTE=${TABLE//_/-}
MODULE_PATH=$(awk '/^module / {print $2}' go.mod)

# Алиас импорта — имя без «_»: us_er дал бы usermod, который уже занят доменом user.
for f in cmd/api/main.go cmd/api/http.go; do
    if grep -qwE "${FLAT}(mod|http)" "$f"; then
        echo "ERROR: алиас ${FLAT}mod или ${FLAT}http уже занят в $f — имя совпадает с другим доменом без «_»" >&2
        exit 1
    fi
done

# Таблица, которую миграции создают и не удаляют, занята: sqlc не соберёт схему с двумя такими.
last_ddl=$(cat migrations/[0-9][0-9][0-9][0-9]_*.up.sql 2>/dev/null | tr '[:upper:]' '[:lower:]' |
    grep -oE "(create|drop)[[:space:]]+table[[:space:]]+(if[[:space:]]+(not[[:space:]]+)?exists[[:space:]]+)?${TABLE}([^a-z0-9_]|$)" |
    tail -1 || true)
if [[ $last_ddl == create* ]]; then
    echo "ERROR: таблицу ${TABLE} уже создаёт миграция — возьми другое имя домена" >&2
    exit 1
fi

last=$(ls migrations | grep -E '^[0-9]{4}_.*\.up\.sql$' | sort | tail -1 | cut -c1-4)
MIGRATION="migrations/$(printf '%04d' $((10#${last:-0} + 1)))_${TABLE}"

# Дальше — правки. Сбой любой из них откатывает все: общие файлы — из копий, созданное — удаляется.
shared=(sqlc.yaml .mockery.yml scripts/tools/mutants.txt cmd/api/main.go cmd/api/http.go openapi.json)
backup=$(mktemp -d)
for f in "${shared[@]}"; do cp "$f" "$backup/${f//\//_}"; done
completed=""
rollback() {
    if [ -z "$completed" ]; then
        for f in "${shared[@]}"; do cp "$backup/${f//\//_}" "$f"; done
        rm -rf "$DOMAIN_DIR" "${MIGRATION}.up.sql" "${MIGRATION}.down.sql"
        echo "ERROR: срез $NAME не собран (причина — выше): правки откатаны, дерево — как до запуска" >&2
    fi
    rm -rf "$backup"
}
trap rollback EXIT

echo "→ создаю $DOMAIN_DIR/ и ${MIGRATION}.{up,down}.sql"

mkdir -p "$DOMAIN_DIR/delivery/http" "$DOMAIN_DIR/infra/postgres/queries" "$DOMAIN_DIR/module"

cat >"${MIGRATION}.up.sql" <<EOF
CREATE TABLE ${TABLE} (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
EOF

cat >"${MIGRATION}.down.sql" <<EOF
DROP TABLE ${TABLE};
EOF

cat >"$DOMAIN_DIR/entity.go" <<EOF
package $NAME

import (
	"strings"
	"time"
	"unicode/utf8"
)

type $ATITLE struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// New$ATITLE — запись до сохранения: ID и время создания проставляет репозиторий. NUL в имени
// Postgres в TEXT не примет.
func New$ATITLE(name string) (*$ATITLE, error) {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > maxNameLen || strings.ContainsRune(name, 0) {
		return nil, ErrInvalidName
	}
	return &$ATITLE{Name: name}, nil
}
EOF

cat >"$DOMAIN_DIR/dto.go" <<EOF
package $NAME

type Create${ATITLE}Input struct {
	Name string
}
EOF

cat >"$DOMAIN_DIR/errors.go" <<EOF
package $NAME

import "errors"

var (
	ErrNotFound    = errors.New("$NAME: not found")
	ErrInvalidName = errors.New("$NAME: invalid name")
)
EOF

cat >"$DOMAIN_DIR/policy.go" <<EOF
package $NAME

import (
	"context"

	"$MODULE_PATH/internal/platform/identity"
	"$MODULE_PATH/pkg/policy"
)

// maxNameLen — предел имени в символах; его же держит maxLength поля name в delivery/http.
const maxNameLen = 200

type Policy struct{}

func NewPolicy() Authorizer { return &Policy{} }

// Права скелета — только админ: шире их открывает правило продукта, явно.
func (p *Policy) CanCreate${ATITLE}(_ context.Context, caller identity.Identity) policy.Decision {
	return adminOnly(caller)
}

// Ресурс с владельцем решает видимость (свои или все), а не id: образец — architecture.md § Usecase-канон.
func (p *Policy) CanGet${ATITLE}(_ context.Context, caller identity.Identity) policy.Decision {
	return adminOnly(caller)
}

func adminOnly(caller identity.Identity) policy.Decision {
	if caller.Role == identity.RoleAdmin {
		return policy.Allow()
	}
	return policy.Deny("admin only")
}
EOF

cat >"$DOMAIN_DIR/usecase.go" <<EOF
package $NAME

import (
	"context"

	"go.uber.org/fx"

	"$MODULE_PATH/internal/platform/identity"
	"$MODULE_PATH/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

type ${ATITLE}Repository interface {
	// Insert сохраняет запись и проставляет ей ID и время создания.
	Insert(ctx context.Context, e *$ATITLE) error
	GetByID(ctx context.Context, id string) (*$ATITLE, error)
}

type Authorizer interface {
	CanCreate${ATITLE}(ctx context.Context, caller identity.Identity) policy.Decision
	CanGet${ATITLE}(ctx context.Context, caller identity.Identity) policy.Decision
}

type UsecaseDeps struct {
	fx.In

	${PTITLE} ${ATITLE}Repository
	Policy Authorizer
	Log    logkit.Logger
}

type Usecase struct {
	d UsecaseDeps
}

func NewUsecase(d UsecaseDeps) *Usecase {
	d.Log = d.Log.With("domain", "$NAME")
	return &Usecase{d: d}
}

func (uc *Usecase) Create${ATITLE}(ctx context.Context, in Create${ATITLE}Input) (*$ATITLE, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanCreate${ATITLE}(ctx, caller); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	e, err := New$ATITLE(in.Name)
	if err != nil {
		return nil, err
	}
	if err := uc.d.${PTITLE}.Insert(ctx, e); err != nil {
		return nil, err
	}
	uc.d.Log.Info(ctx, "$AGG created", "${AGG}_id", e.ID)
	return e, nil
}

func (uc *Usecase) Get${ATITLE}(ctx context.Context, id string) (*$ATITLE, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanGet${ATITLE}(ctx, caller); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	return uc.d.${PTITLE}.GetByID(ctx, id)
}
EOF

cat >"$DOMAIN_DIR/entity_test.go" <<EOF
package ${NAME}_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
)

func TestNew$ATITLE(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"plain name", "first", nil},
		{"name at the limit", strings.Repeat("я", 200), nil},
		{"empty", "", $NAME.ErrInvalidName},
		{"blank", "  ", $NAME.ErrInvalidName},
		{"over the limit", strings.Repeat("я", 201), $NAME.ErrInvalidName},
		{"NUL inside", "a\x00b", $NAME.ErrInvalidName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := $NAME.New$ATITLE(tc.in)
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantErr == nil {
				require.Equal(t, tc.in, got.Name)
			}
		})
	}
}
EOF

cat >"$DOMAIN_DIR/entity_fuzz_test.go" <<EOF
package ${NAME}_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
)

func FuzzNew$ATITLE(f *testing.F) {
	f.Add("first")
	f.Add("")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, name string) {
		got, err := $NAME.New$ATITLE(name)
		valid := strings.TrimSpace(name) != "" && utf8.RuneCountInString(name) <= 200 && !strings.ContainsRune(name, 0)
		if !valid {
			require.ErrorIs(t, err, $NAME.ErrInvalidName, "name %q", name)
			return
		}
		require.NoError(t, err)
		require.Equal(t, name, got.Name)
	})
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

func TestPolicy(t *testing.T) {
	t.Parallel()
	p := $NAME.NewPolicy()
	tests := []struct {
		name   string
		caller identity.Identity
		want   bool
	}{
		{"admin", identity.Identity{UserID: "a1", Role: identity.RoleAdmin}, true},
		{"client", identity.Identity{UserID: "u1", Role: identity.RoleClient}, false},
		{"no role", identity.Identity{UserID: "u1"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, p.CanCreate${ATITLE}(context.Background(), tc.caller).IsAllowed)
			require.Equal(t, tc.want, p.CanGet${ATITLE}(context.Background(), tc.caller).IsAllowed)
		})
	}
}
EOF

cat >"$DOMAIN_DIR/usecase_test.go" <<EOF
package ${NAME}_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	"$MODULE_PATH/internal/domain/$NAME/mocks"
	"$MODULE_PATH/internal/testkit/identitytest"
	"$MODULE_PATH/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

var createdAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// newUC — usecase на строгом моке репозитория и настоящей policy: вызов, которого тест не ждёт, —
// провал.
func newUC(t *testing.T, repo *mocks.Mock${ATITLE}Repository) *$NAME.Usecase {
	t.Helper()
	return $NAME.NewUsecase($NAME.UsecaseDeps{${PTITLE}: repo, Policy: $NAME.NewPolicy(), Log: logkit.Nop()})
}

func TestCreate${ATITLE}(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMock${ATITLE}Repository(t)
	repo.EXPECT().Insert(mock.Anything, &$NAME.$ATITLE{Name: "first"}).
		RunAndReturn(func(_ context.Context, e *$NAME.$ATITLE) error {
			e.ID, e.CreatedAt = "id-1", createdAt
			return nil
		}).Once()

	got, err := newUC(t, repo).Create${ATITLE}(identitytest.Admin("a1"), $NAME.Create${ATITLE}Input{Name: "first"})

	require.NoError(t, err)
	require.Equal(t, &$NAME.$ATITLE{ID: "id-1", Name: "first", CreatedAt: createdAt}, got)
}

// Отказ policy до репозитория не доходит: строгий мок без ожиданий.
func TestCreate${ATITLE}_Forbidden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"client", identitytest.Client("u1")},
		{"no identity", identitytest.Anon()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newUC(t, mocks.NewMock${ATITLE}Repository(t)).Create${ATITLE}(tc.ctx, $NAME.Create${ATITLE}Input{Name: "first"})

			var forbidden *policy.ForbiddenError
			require.ErrorAs(t, err, &forbidden)
		})
	}
}

func TestCreate${ATITLE}_InvalidName(t *testing.T) {
	t.Parallel()
	_, err := newUC(t, mocks.NewMock${ATITLE}Repository(t)).Create${ATITLE}(identitytest.Admin("a1"), $NAME.Create${ATITLE}Input{Name: ""})

	require.ErrorIs(t, err, $NAME.ErrInvalidName)
}

func TestCreate${ATITLE}_RepositoryError(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMock${ATITLE}Repository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(errors.New("db down")).Once()

	_, err := newUC(t, repo).Create${ATITLE}(identitytest.Admin("a1"), $NAME.Create${ATITLE}Input{Name: "first"})

	require.EqualError(t, err, "db down")
}

func TestGet${ATITLE}(t *testing.T) {
	t.Parallel()
	want := &$NAME.$ATITLE{ID: "id-1", Name: "first", CreatedAt: createdAt}
	repo := mocks.NewMock${ATITLE}Repository(t)
	repo.EXPECT().GetByID(mock.Anything, "id-1").Return(want, nil).Once()
	repo.EXPECT().GetByID(mock.Anything, "id-2").Return(nil, $NAME.ErrNotFound).Once()
	uc := newUC(t, repo)

	got, err := uc.Get${ATITLE}(identitytest.Admin("a1"), "id-1")
	require.NoError(t, err)
	require.Equal(t, want, got)
	_, err = uc.Get${ATITLE}(identitytest.Admin("a1"), "id-2")
	require.ErrorIs(t, err, $NAME.ErrNotFound)
}

func TestGet${ATITLE}_Forbidden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"client", identitytest.Client("u1")},
		{"no identity", identitytest.Anon()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newUC(t, mocks.NewMock${ATITLE}Repository(t)).Get${ATITLE}(tc.ctx, "id-1")

			var forbidden *policy.ForbiddenError
			require.ErrorAs(t, err, &forbidden)
		})
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/dto.go" <<EOF
package http

type ${HTTPT}CreateInput struct {
	Body struct {
		Name string \`json:"name" maxLength:"200" minLength:"1" required:"true"\`
	}
}

type ${HTTPT}CreateOutput struct {
	Body struct {
		Data ${HTTPT}View \`json:"data"\`
	}
}

type ${HTTPT}GetInput struct {
	ID string \`path:"id"\`
}

type ${HTTPT}GetOutput struct {
	Body struct {
		Data ${HTTPT}View \`json:"data"\`
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/view.go" <<EOF
package http

import (
	"time"

	"$MODULE_PATH/internal/domain/$NAME"
)

type ${HTTPT}View struct {
	ID        string    \`json:"id"\`
	Name      string    \`json:"name"\`
	CreatedAt time.Time \`json:"created_at"\`
}

func (v *${HTTPT}View) FromEntity(e *$NAME.$ATITLE) {
	v.ID = e.ID
	v.Name = e.Name
	v.CreatedAt = e.CreatedAt
}
EOF

cat >"$DOMAIN_DIR/delivery/http/handler.go" <<EOF
package http

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"$MODULE_PATH/internal/domain/$NAME"
	"$MODULE_PATH/internal/platform/authgate"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

type Usecase interface {
	Create${ATITLE}(ctx context.Context, in $NAME.Create${ATITLE}Input) (*$NAME.$ATITLE, error)
	Get${ATITLE}(ctx context.Context, id string) (*$NAME.$ATITLE, error)
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, create${ATITLE}Errors.Declare(authgate.Secured(huma.Operation{
		OperationID:   "create${ATITLE}",
		Method:        http.MethodPost,
		Path:          "/api/${ROUTE}",
		Tags:          []string{"${ROUTE}"},
		DefaultStatus: http.StatusCreated,
		Summary:       "Создать $AGG",
	})), h.create${ATITLE})

	huma.Register(api, get${ATITLE}Errors.Declare(authgate.Secured(huma.Operation{
		OperationID: "get${ATITLE}",
		Method:      http.MethodGet,
		Path:        "/api/${ROUTE}/{id}",
		Tags:        []string{"${ROUTE}"},
		Summary:     "Получить $AGG по ID",
	})), h.get${ATITLE})
}

func (h *Handler) create${ATITLE}(ctx context.Context, in *${HTTPT}CreateInput) (*${HTTPT}CreateOutput, error) {
	e, err := h.uc.Create${ATITLE}(ctx, $NAME.Create${ATITLE}Input{Name: in.Body.Name})
	if err != nil {
		return nil, ${CAMEL}Errors.Map(err)
	}
	out := &${HTTPT}CreateOutput{}
	out.Body.Data.FromEntity(e)
	return out, nil
}

func (h *Handler) get${ATITLE}(ctx context.Context, in *${HTTPT}GetInput) (*${HTTPT}GetOutput, error) {
	e, err := h.uc.Get${ATITLE}(ctx, in.ID)
	if err != nil {
		return nil, ${CAMEL}Errors.Map(err)
	}
	out := &${HTTPT}GetOutput{}
	out.Body.Data.FromEntity(e)
	return out, nil
}

// Правило каждой ошибки домена — одно. ${CAMEL}Errors переводит в хендлерах их все (ни одна не уходит
// 500 — archtest), таблица операции объявляет в OpenAPI те, что отдаёт её usecase.
var (
	notFoundRule    = httpkit.ErrorRule{Err: $NAME.ErrNotFound, Status: http.StatusNotFound, Code: "${UPPER}_NOT_FOUND", Detail: "Запись не найдена. Проверьте, что ее не удалили."}
	invalidNameRule = httpkit.ErrorRule{Err: $NAME.ErrInvalidName, Status: http.StatusUnprocessableEntity, Code: "${UPPER}_INVALID_NAME", Detail: "Название пустое, длиннее 200 знаков или содержит недопустимый символ. Укажите название от 1 до 200 знаков.", Field: "body.name"}

	${CAMEL}Errors = httpkit.NewErrorMap(notFoundRule, invalidNameRule, authgate.ForbiddenRule)

	create${ATITLE}Errors = httpkit.NewErrorMap(invalidNameRule, authgate.ForbiddenRule)
	get${ATITLE}Errors    = httpkit.NewErrorMap(notFoundRule, authgate.ForbiddenRule)
)
EOF

cat >"$DOMAIN_DIR/delivery/http/errors_test.go" <<EOF
// White-box (package http): таблицы ошибок неэкспортируемые.
package http

import (
	"testing"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	"$MODULE_PATH/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// OpenAPI операции объявляет ошибки, которые отдаёт её usecase (список — по коду usecase). Что ни
// одна ошибка домена не уходит 500, держит таблица ${CAMEL}Errors и archtest TestDomainErrorTables.
func TestErrorMaps(t *testing.T) {
	t.Parallel()
	forbidden := &policy.ForbiddenError{Reason: "test"}
	tests := []struct {
		op   string
		m    httpkit.ErrorMap
		errs []error
	}{
		{"create${ATITLE}", create${ATITLE}Errors, []error{$NAME.ErrInvalidName, forbidden}},
		{"get${ATITLE}", get${ATITLE}Errors, []error{$NAME.ErrNotFound, forbidden}},
	}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, tc.m.Unmapped(tc.errs...))
		})
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/handler_test.go" <<EOF
package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	${FLAT}http "$MODULE_PATH/internal/domain/$NAME/delivery/http"
	"$MODULE_PATH/internal/domain/$NAME/delivery/http/mocks"
	"$MODULE_PATH/internal/platform/authgate"
	"$MODULE_PATH/internal/testkit/identitytest"
	"$MODULE_PATH/pkg/policy"
	"git.stit.tech/stit-core/golibinfra/testkit/handlertest"
)

func setup(t *testing.T, uc *mocks.MockUsecase) handlertest.API {
	t.Helper()
	return handlertest.NewAPI(t, func(api huma.API) {
		${FLAT}http.Register(api, ${FLAT}http.NewHandler(uc))
	}, handlertest.WithSecuritySchemes(authgate.CookieScheme(identitytest.Extract)))
}

var created = &$NAME.$ATITLE{ID: "id-1", Name: "first", CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

// assertView — тело ответа: envelope data с записью created.
func assertView(t *testing.T, raw []byte) {
	t.Helper()
	var body struct {
		Data struct {
			ID        string    \`json:"id"\`
			Name      string    \`json:"name"\`
			CreatedAt time.Time \`json:"created_at"\`
		} \`json:"data"\`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	require.Equal(t, created.ID, body.Data.ID)
	require.Equal(t, created.Name, body.Data.Name)
	require.Equal(t, created.CreatedAt, body.Data.CreatedAt)
}

func TestCreate${ATITLE}_Created(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Create${ATITLE}(mock.Anything, $NAME.Create${ATITLE}Input{Name: "first"}).Return(created, nil).Once()

	resp := setup(t, uc).PostCtx(identitytest.Admin("a1"), "/api/${ROUTE}", strings.NewReader(\`{"name":"first"}\`))

	handlertest.AssertStatus(t, resp, http.StatusCreated)
	assertView(t, resp.Body.Bytes())
}

func TestCreate${ATITLE}_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		ctx    context.Context
		err    error
		status int
		code   string
		field  string // "" — ошибка не поля
	}{
		{"invalid name", identitytest.Admin("a1"), $NAME.ErrInvalidName, http.StatusUnprocessableEntity, "${UPPER}_INVALID_NAME", "body.name"},
		{"forbidden", identitytest.Client("u1"), &policy.ForbiddenError{Reason: "admin only"}, http.StatusForbidden, "FORBIDDEN", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uc := mocks.NewMockUsecase(t)
			uc.EXPECT().Create${ATITLE}(mock.Anything, $NAME.Create${ATITLE}Input{Name: "first"}).Return(nil, tc.err).Once()

			resp := setup(t, uc).PostCtx(tc.ctx, "/api/${ROUTE}", strings.NewReader(\`{"name":"first"}\`))

			handlertest.AssertAPIError(t, resp, tc.status, tc.code)
			if tc.field != "" {
				errs := handlertest.DecodeAPIError(t, resp).Errors
				require.Len(t, errs, 1)
				require.Equal(t, tc.field, errs[0].Location)
			}
		})
	}
}

func TestGet${ATITLE}_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Get${ATITLE}(mock.Anything, "id-1").Return(created, nil).Once()

	resp := setup(t, uc).GetCtx(identitytest.Admin("a1"), "/api/${ROUTE}/id-1")

	handlertest.AssertStatus(t, resp, http.StatusOK)
	assertView(t, resp.Body.Bytes())
}

func TestGet${ATITLE}_NotFound(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Get${ATITLE}(mock.Anything, "id-2").Return(nil, $NAME.ErrNotFound).Once()

	resp := setup(t, uc).GetCtx(identitytest.Admin("a1"), "/api/${ROUTE}/id-2")

	handlertest.AssertAPIError(t, resp, http.StatusNotFound, "${UPPER}_NOT_FOUND")
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/queries/${AGG}.sql" <<EOF
-- name: Insert${ATITLE} :one
INSERT INTO ${TABLE} (name)
VALUES (\$1)
RETURNING id, created_at;

-- name: Get${ATITLE}ByID :one
SELECT id, name, created_at
FROM ${TABLE}
WHERE id = \$1;
EOF

cat >"$DOMAIN_DIR/infra/postgres/repository.go" <<EOF
package postgres

import (
	"context"
	"fmt"

	"$MODULE_PATH/internal/domain/$NAME"
	"$MODULE_PATH/internal/domain/$NAME/infra/postgres/db"
	"git.stit.tech/stit-core/golibinfra/pgkit"
)

type Repository struct {
	q *db.Queries
}

func NewRepository(database *pgkit.DB) $NAME.${ATITLE}Repository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) Insert(ctx context.Context, e *$NAME.$ATITLE) error {
	row, err := r.q.Insert${ATITLE}(ctx, e.Name)
	if err != nil {
		return fmt.Errorf("$NAME repo: insert: %w", err)
	}
	e.ID = pgkit.UUIDString(row.ID)
	e.CreatedAt = row.CreatedAt.Time
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*$NAME.$ATITLE, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, $NAME.ErrNotFound
	}
	row, err := r.q.Get${ATITLE}ByID(ctx, uid)
	if pgkit.IsNoRows(err) {
		return nil, $NAME.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("$NAME repo: select: %w", err)
	}
	return &$NAME.$ATITLE{ID: pgkit.UUIDString(row.ID), Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/repository_test.go" <<EOF
//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"$MODULE_PATH/internal/domain/$NAME"
	${FLAT}pg "$MODULE_PATH/internal/domain/$NAME/infra/postgres"
	"$MODULE_PATH/internal/testkit/repopath"
	"git.stit.tech/stit-core/golibinfra/testkit"
)

func TestInsertAndGetByID(t *testing.T) {
	t.Parallel()
	repo := ${FLAT}pg.NewRepository(testkit.StartPostgres(t, testkit.WithMigrationsDir(repopath.Migrations())).DB(t))
	ctx := context.Background()

	e := &$NAME.$ATITLE{Name: "first"}
	require.NoError(t, repo.Insert(ctx, e))
	require.NotEmpty(t, e.ID)
	require.WithinDuration(t, time.Now(), e.CreatedAt, time.Minute, "the database sets the creation time")

	got, err := repo.GetByID(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, e, got)

	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		_, err := repo.GetByID(ctx, id)
		require.ErrorIs(t, err, $NAME.ErrNotFound, "id %q", id)
	}

	// Сбой БД — ошибка с причиной, а не ErrNotFound: недоступная база не выглядит как «нет такого».
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, repo.Insert(canceled, &$NAME.$ATITLE{Name: "second"}), context.Canceled)
	_, err = repo.GetByID(canceled, e.ID)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, $NAME.ErrNotFound)
}
EOF

cat >"$DOMAIN_DIR/module/module.go" <<EOF
package module

import (
	"go.uber.org/fx"

	"$MODULE_PATH/internal/domain/$NAME"
	${FLAT}http "$MODULE_PATH/internal/domain/$NAME/delivery/http"
	${FLAT}pg "$MODULE_PATH/internal/domain/$NAME/infra/postgres"
)

var Module = fx.Module(
	"internal/domain/$NAME/module",
	fx.Provide(
		${FLAT}pg.NewRepository,
		$NAME.NewPolicy,
		$NAME.NewUsecase,
		${FLAT}http.NewHandler,
		func(u *$NAME.Usecase) ${FLAT}http.Usecase { return u },
	),
)
EOF

# ── конфиги и подключение ─────────────────────────────────────────────────

echo "→ вписываю sqlc.yaml и .mockery.yml"
cat >>sqlc.yaml <<EOF
  - engine: postgresql
    schema: migrations/
    queries: internal/domain/${NAME}/infra/postgres/queries/
    gen:
      go:
        package: db
        out: internal/domain/${NAME}/infra/postgres/db
        sql_package: pgx/v5
        emit_interface: false
        emit_exact_table_names: false
        emit_pointers_for_null_types: true
        emit_json_tags: false
        emit_prepared_queries: false
        emit_methods_with_db_argument: false
        omit_unused_structs: true
EOF
cat >>.mockery.yml <<EOF
  ${MODULE_PATH}/internal/domain/${NAME}:
    interfaces:
      ${ATITLE}Repository:
  ${MODULE_PATH}/internal/domain/${NAME}/delivery/http:
    interfaces:
      Usecase:
EOF

# Сила тестов среза — его мутации (rule tests-pipeline § Мутации): спецификация растёт с кодом с первого коммита.
echo "→ вписываю блоки мутаций среза в scripts/tools/mutants.txt"
UC=$DOMAIN_DIR/usecase.go ENT=$DOMAIN_DIR/entity.go POL=$DOMAIN_DIR/policy.go
REPO=$DOMAIN_DIR/infra/postgres/repository.go SQL=$DOMAIN_DIR/infra/postgres/queries/$AGG.sql
HANDLER=$DOMAIN_DIR/delivery/http/handler.go VIEW=$DOMAIN_DIR/delivery/http/view.go
cat >>scripts/tools/mutants.txt <<EOF

# Домен $NAME: срез make new-domain.

## ${NAME}-01 usecase — Create: отказ policy не останавливает запись
$UC
- if d := uc.d.Policy.CanCreate${ATITLE}(ctx, caller); !d.IsAllowed {
+ if d := uc.d.Policy.CanCreate${ATITLE}(ctx, caller); !d.IsAllowed && caller.UserID == "" {

## ${NAME}-02 usecase — Get: отказ policy не останавливает чтение
$UC
- if d := uc.d.Policy.CanGet${ATITLE}(ctx, caller); !d.IsAllowed {
+ if d := uc.d.Policy.CanGet${ATITLE}(ctx, caller); !d.IsAllowed && caller.UserID == "" {

## ${NAME}-03 usecase — Create: сбой записи глотается
$UC
- if err := uc.d.${PTITLE}.Insert(ctx, e); err != nil {
+ if err := uc.d.${PTITLE}.Insert(ctx, e); err != nil && ctx.Err() != nil {

## ${NAME}-04 usecase — Create: в запись уходит пустое имя
$UC
- if err := uc.d.${PTITLE}.Insert(ctx, e); err != nil {
+ e.Name = ""
+ if err := uc.d.${PTITLE}.Insert(ctx, e); err != nil {

## ${NAME}-05 entity — New$ATITLE: имя ровно в предел отвергнуто (граница)
$ENT
- utf8.RuneCountInString(name) > maxNameLen
+ utf8.RuneCountInString(name) >= maxNameLen

## ${NAME}-06 entity — New$ATITLE: NUL в имени пропускается
$ENT
- strings.ContainsRune(name, 0)
+ strings.ContainsRune(name, 1)

## ${NAME}-07 entity — New$ATITLE: имя из пробелов принято
$ENT
- strings.TrimSpace(name) == "" ||
+ name == "" ||

## ${NAME}-08 policy — adminOnly: права админа у любой роли
$POL
- if caller.Role == identity.RoleAdmin {
+ if caller.Role != identity.RoleUnknown {

## ${NAME}-09 policy — CanGet${ATITLE}: читать может любой
$POL
- func (p *Policy) CanGet${ATITLE}(_ context.Context, caller identity.Identity) policy.Decision {
- 	return adminOnly(caller)
+ func (p *Policy) CanGet${ATITLE}(_ context.Context, _ identity.Identity) policy.Decision {
+ 	return policy.Allow()

## ${NAME}-10 repository — GetByID: имя не читается
$REPO
- Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
+ Name: "", CreatedAt: row.CreatedAt.Time}, nil

## ${NAME}-11 repository — Insert: время создания не проставляется
$REPO
- e.CreatedAt = row.CreatedAt.Time

## ${NAME}-12 repository — GetByID: битый id — сбой (500), а не «нет такого»
$REPO
- uid, err := pgkit.ParseUUID(id)
- if err != nil {
- 	return nil, $NAME.ErrNotFound
- }
+ uid, err := pgkit.ParseUUID(id)
+ if err != nil {
+ 	return nil, err
+ }

## ${NAME}-13 repository — GetByID: нет строки — сбой (500), а не «нет такого»
$REPO
- if pgkit.IsNoRows(err) {
- 	return nil, $NAME.ErrNotFound
- }

## ${NAME}-14 repository — Insert: сбой БД — «нет такого» домена
$REPO
- return fmt.Errorf("$NAME repo: insert: %w", err)
+ return $NAME.ErrNotFound

## ${NAME}-15 sql — Get${ATITLE}ByID: условие перевёрнуто
$SQL
- WHERE id = \$1;
+ WHERE id != \$1;

## ${NAME}-16 handler — create: имя не доходит до usecase
$HANDLER
- h.uc.Create${ATITLE}(ctx, $NAME.Create${ATITLE}Input{Name: in.Body.Name})
+ h.uc.Create${ATITLE}(ctx, $NAME.Create${ATITLE}Input{Name: ""})

## ${NAME}-17 handler — create: 200 вместо 201
$HANDLER
- DefaultStatus: http.StatusCreated,
+ DefaultStatus: http.StatusOK,

## ${NAME}-18 handler — get${ATITLE}}: операция открыта без сессии
$HANDLER
- huma.Register(api, get${ATITLE}Errors.Declare(authgate.Secured(huma.Operation{
+ huma.Register(api, get${ATITLE}Errors.Declare(httpkit.Public(huma.Operation{

## ${NAME}-19 handler — ${UPPER}_NOT_FOUND: 400 вместо 404
$HANDLER
- Err: $NAME.ErrNotFound, Status: http.StatusNotFound,
+ Err: $NAME.ErrNotFound, Status: http.StatusBadRequest,

## ${NAME}-20 handler — create: 422 имени не объявлен в OpenAPI
$HANDLER
- create${ATITLE}Errors = httpkit.NewErrorMap(invalidNameRule, authgate.ForbiddenRule)
+ create${ATITLE}Errors = httpkit.NewErrorMap(authgate.ForbiddenRule)

## ${NAME}-21 view — ответ без created_at
$VIEW
- v.CreatedAt = e.CreatedAt

## ${NAME}-22 view — в name — ID
$VIEW
- v.Name = e.Name
+ v.Name = e.ID

## ${NAME}-23 handler — get: в ответе только ID
$HANDLER
- e, err := h.uc.Get${ATITLE}(ctx, in.ID)
+ e, err := h.uc.Get${ATITLE}(ctx, in.ID)
+ if err == nil && e != nil {
+ 	e = &$NAME.$ATITLE{ID: e.ID}
+ }

## ${NAME}-24 migration — created_at по умолчанию — константа, а не время записи
${MIGRATION}.up.sql
- created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
+ created_at TIMESTAMPTZ NOT NULL DEFAULT '2000-01-01'
EOF

# insert_before FILE START CLOSE LINE: LINE — перед первой строкой CLOSE после строки START.
insert_before() {
    local file=$1 tmp
    tmp=$(mktemp)
    if ! START=$2 CLOSE=$3 LINE=$4 awk '
        !done && $0 ~ ENVIRON["START"] { inside = 1 }
        inside && !done && $0 ~ ENVIRON["CLOSE"] { print ENVIRON["LINE"]; done = 1 }
        { print }
        END { exit done ? 0 : 1 }
    ' "$file" >"$tmp"; then
        rm -f "$tmp"
        echo "ERROR: $file: нет блока «$2» … «$3» — подключение домена в cmd/api по skill scaffold, вариант domain" >&2
        exit 1
    fi
    mv "$tmp" "$file"
}

echo "→ подключаю домен в cmd/api"
insert_before cmd/api/main.go '^import \($' '^\)$' "	${FLAT}mod \"${MODULE_PATH}/internal/domain/${NAME}/module\""
insert_before cmd/api/main.go '^	domainModules = fx\.Options\($' '^	\)$' "		${FLAT}mod.Module,"
insert_before cmd/api/http.go '^import \($' '^\)$' "	${FLAT}http \"${MODULE_PATH}/internal/domain/${NAME}/delivery/http\""
insert_before cmd/api/http.go '^type httpHandlers struct \{$' '^\}$' "	${TITLE} *${FLAT}http.Handler"
insert_before cmd/api/http.go '^func registerHTTP\(' '^\}$' "	${FLAT}http.Register(api, h.${TITLE})"
# Импорты среза и cmd/api — по порядку module path форка: он сортируется и после golibinfra.
gofumpt -w cmd/api/main.go cmd/api/http.go "$DOMAIN_DIR"

# Блоки мутаций среза правят его рукописные файлы: якоря сверяются до make gen — так их видит и тест скрипта.
echo "→ make mutate A=--anchors: якоря мутаций среза"
mutants=$(grep -oE "^## ${NAME}-[0-9]+ " scripts/tools/mutants.txt | awk '{print $2}')
# shellcheck disable=SC2086 # id мутаций — отдельными словами
python3 -I scripts/tools/mutate.py --anchors $mutants || {
    echo "ERROR: блоки мутаций среза разошлись с его кодом — их пишет scripts/new-domain.sh: правка скрипта, а не имени" >&2
    exit 1
}

echo "→ make gen"
make --no-print-directory gen

echo "→ go vet: срез и cmd/api собираются"
go vet -tags=integration "./$DOMAIN_DIR/..." ./cmd/api/ || {
    echo "ERROR: срез не собирается (вывод выше): ошибка об имени $NAME — оно занято кодом среза, возьми другое" >&2
    exit 1
}

completed=1
git add "$DOMAIN_DIR" "${MIGRATION}.up.sql" "${MIGRATION}.down.sql" sqlc.yaml .mockery.yml scripts/tools/mutants.txt \
    cmd/api/main.go cmd/api/http.go openapi.json

echo
echo "✓ домен $NAME: срез Create и Get (POST /api/${ROUTE}, GET /api/${ROUTE}/{id}, только admin),"
echo "  таблица ${TABLE}, подключение в cmd/api, мутации ${NAME}-01…$(wc -w <<<"$mutants" | xargs printf '%02d')"
echo "  в scripts/tools/mutants.txt; созданное — в индексе git"
echo
echo "Дальше:"
echo "  1. make check (нужен Docker); make mutate A=\"$(xargs <<<"$mutants")\" — мутации среза убиты"
echo "     (новая логика домена — свои блоки в mutants.txt тем же коммитом, rule tests-pipeline § Мутации)"
echo "  2. Домен — по артефакту strategy: .claude/skills/scaffold/references/domain.md § 4"
echo "     (поля — в миграцию среза до первого коммита; права, операции, события)"
