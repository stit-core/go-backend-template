#!/usr/bin/env bash
# new-domain.sh — новый домен internal/domain/<name>/: вертикальный срез Create и Get по канону.
#
# Использование:
#   make new-domain NAME=order
#   bash scripts/new-domain.sh order
#
# Создаёт рабочий срез — миграцию таблицы, sqlc-запросы, репозиторий с integration-тестом,
# сущность, policy и usecase с тестами, HTTP-операции под cookie-сессией с тестами, fx-модуль —
# подключает домен в cmd/api, вписывает sqlc.yaml и .mockery.yml, гонит make gen, проверяет, что срез
# собирается (go vet с тегом integration), и ставит созданное в индекс: следующий шаг — make check.
# Негодное имя и занятая таблица — отказ до первой правки; сбой после неё — откат всех правок скрипта.
# Дальше домен растёт по skill `scaffold`.

set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
    echo "ERROR: usage: $0 <domain_name>" >&2
    echo "       или: make new-domain NAME=<domain_name>" >&2
    exit 1
fi

# Имя проверяется до первой правки: срез с негодным именем не собрался бы, когда cmd/api и make gen
# уже тронуты.
if ! [[ "$NAME" =~ ^[a-z][a-z0-9]*(_[a-z0-9]+)*$ ]]; then
    echo "ERROR: имя домена — snake_case: a-z и 0-9, «_» только между словами, начинается с буквы" >&2
    echo "       получено: '$NAME'" >&2
    exit 1
fi
# Пакет домена не может называться ключевым словом или встроенным именем Go, именем, с которым тип
# домена совпал бы с объявлением пакета домена (Repository, Usecase…), и именем, которое код среза
# объявляет или зовёт рядом с пакетом домена (repo, id, got…): срез не собрался бы. Последние берутся из
# Go-шаблонов этого скрипта — список не отстаёт от шаблонов.
reserved=(
    break case chan const continue default defer else fallthrough for func go goto if import interface
    map package range return select struct switch type var
    any append bool byte cap clear close comparable complex complex64 complex128 copy delete error false
    float32 float64 imag int int8 int16 int32 int64 iota len make max min new nil panic print println
    real recover rune string true uint uint8 uint16 uint32 uint64 uintptr
    authorizer create_input policy repository usecase usecase_deps
)
template_words=$(awk '/<<EOF$/ && /\.go"/ { go = 1; next } /^EOF$/ { go = 0 } go' "$0" |
    sed -E 's://.*$::; s/"([^"\\]|\\.)*"//g; s/`[^`]*`//g' | grep -oE '[A-Za-z_][A-Za-z0-9_]*' | grep -E '^[a-z]' | sort -u)
for word in "${reserved[@]}" $template_words; do
    if [ "$NAME" = "$word" ]; then
        echo "ERROR: '$NAME' — ключевое слово или встроенное имя Go либо имя из кода среза: возьми другое" >&2
        exit 1
    fi
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

# order_item → OrderItem (типы), orderItem (переменные), orderitem (алиасы импортов),
# ORDER_ITEM (коды ошибок), order_items (таблица), order-items (путь API). Инициализм — по правилу
# revive var-naming (api_key → APIKey и apiKey): иначе lint требует переименовать типы скелета.
initialisms=" ACL API ASCII CPU CSS DNS EOF GUID HTML HTTP HTTPS ID IDS IP JSON LHS QPS RAM RHS RPC SLA SMTP SQL SSH TCP TLS TTL UDP UI UID UUID URI URL UTF8 VM XML XMPP XSRF XSS "
TITLE="" CAMEL=""
for part in ${NAME//_/ }; do
    word=$(tr '[:lower:]' '[:upper:]' <<<"$part")
    case "$initialisms" in
        *" $word "*) [ "$word" != IDS ] || word=IDs ;;
        *) word=$(tr '[:lower:]' '[:upper:]' <<<"${part:0:1}")${part:1} ;;
    esac
    TITLE+=$word
    if [ -z "$CAMEL" ]; then CAMEL=$part; else CAMEL+=$word; fi
done
FLAT=${NAME//_/}
UPPER=$(echo "$NAME" | tr '[:lower:]' '[:upper:]')
# Множественное число — по правилам английского (status → statuses, category → categories);
# исключения (person → people) правят руками.
case "$NAME" in
    *s | *x | *z | *ch | *sh) TABLE="${NAME}es" ;;
    *[!aeiou]y) TABLE="${NAME%y}ies" ;;
    *) TABLE="${NAME}s" ;;
esac
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
shared=(sqlc.yaml .mockery.yml cmd/api/main.go cmd/api/http.go openapi.json)
backup=$(mktemp -d)
for f in "${shared[@]}"; do cp "$f" "$backup/${f//\//_}"; done
completed=""
rollback() {
    if [ -z "$completed" ]; then
        for f in "${shared[@]}"; do cp "$backup/${f//\//_}" "$f"; done
        rm -rf "$DOMAIN_DIR" "${MIGRATION}.up.sql" "${MIGRATION}.down.sql"
        echo "ERROR: срез $NAME не собрался (вывод выше) — правки откатаны, дерево — как до запуска" >&2
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

type $TITLE struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// New$TITLE — запись до сохранения: ID и время создания проставляет репозиторий. NUL в имени
// Postgres в TEXT не примет.
func New$TITLE(name string) (*$TITLE, error) {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > maxNameLen || strings.ContainsRune(name, 0) {
		return nil, ErrInvalidName
	}
	return &$TITLE{Name: name}, nil
}
EOF

cat >"$DOMAIN_DIR/dto.go" <<EOF
package $NAME

type CreateInput struct {
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

// maxNameLen — предел имени в символах.
const maxNameLen = 200

type Policy struct{}

func NewPolicy() Authorizer { return &Policy{} }

// Права скелета — только админ: шире их открывает правило продукта, явно.
func (p *Policy) CanCreate(_ context.Context, caller identity.Identity) policy.Decision {
	return adminOnly(caller)
}

func (p *Policy) CanGet(_ context.Context, caller identity.Identity, _ string) policy.Decision {
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

type Repository interface {
	// Insert сохраняет запись и проставляет ей ID и время создания.
	Insert(ctx context.Context, e *$TITLE) error
	GetByID(ctx context.Context, id string) (*$TITLE, error)
}

type Authorizer interface {
	CanCreate(ctx context.Context, caller identity.Identity) policy.Decision
	CanGet(ctx context.Context, caller identity.Identity, id string) policy.Decision
}

type UsecaseDeps struct {
	fx.In

	Repo   Repository
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

func (uc *Usecase) Create(ctx context.Context, in CreateInput) (*$TITLE, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanCreate(ctx, caller); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	e, err := New$TITLE(in.Name)
	if err != nil {
		return nil, err
	}
	if err := uc.d.Repo.Insert(ctx, e); err != nil {
		return nil, err
	}
	uc.d.Log.Info(ctx, "$NAME created", "${NAME}_id", e.ID)
	return e, nil
}

func (uc *Usecase) Get(ctx context.Context, id string) (*$TITLE, error) {
	caller, ok := identity.From(ctx)
	if !ok {
		return nil, &policy.ForbiddenError{Reason: "no identity"}
	}
	if d := uc.d.Policy.CanGet(ctx, caller, id); !d.IsAllowed {
		return nil, &policy.ForbiddenError{Reason: d.Reason}
	}
	return uc.d.Repo.GetByID(ctx, id)
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

func TestNew$TITLE(t *testing.T) {
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
			got, err := $NAME.New$TITLE(tc.in)
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

func FuzzNew$TITLE(f *testing.F) {
	f.Add("first")
	f.Add("")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, name string) {
		got, err := $NAME.New$TITLE(name)
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
			require.Equal(t, tc.want, p.CanCreate(context.Background(), tc.caller).IsAllowed)
			require.Equal(t, tc.want, p.CanGet(context.Background(), tc.caller, "id-1").IsAllowed)
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
func newUC(t *testing.T, repo *mocks.MockRepository) *$NAME.Usecase {
	t.Helper()
	return $NAME.NewUsecase($NAME.UsecaseDeps{Repo: repo, Policy: $NAME.NewPolicy(), Log: logkit.Nop()})
}

func TestCreate(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Insert(mock.Anything, &$NAME.$TITLE{Name: "first"}).
		RunAndReturn(func(_ context.Context, e *$NAME.$TITLE) error {
			e.ID, e.CreatedAt = "id-1", createdAt
			return nil
		}).Once()

	got, err := newUC(t, repo).Create(identitytest.Admin("a1"), $NAME.CreateInput{Name: "first"})

	require.NoError(t, err)
	require.Equal(t, &$NAME.$TITLE{ID: "id-1", Name: "first", CreatedAt: createdAt}, got)
}

// Отказ policy до репозитория не доходит: строгий мок без ожиданий.
func TestCreate_Forbidden(t *testing.T) {
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
			_, err := newUC(t, mocks.NewMockRepository(t)).Create(tc.ctx, $NAME.CreateInput{Name: "first"})

			var forbidden *policy.ForbiddenError
			require.ErrorAs(t, err, &forbidden)
		})
	}
}

func TestCreate_InvalidName(t *testing.T) {
	t.Parallel()
	_, err := newUC(t, mocks.NewMockRepository(t)).Create(identitytest.Admin("a1"), $NAME.CreateInput{Name: ""})

	require.ErrorIs(t, err, $NAME.ErrInvalidName)
}

func TestCreate_RepositoryError(t *testing.T) {
	t.Parallel()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Insert(mock.Anything, mock.Anything).Return(errors.New("db down")).Once()

	_, err := newUC(t, repo).Create(identitytest.Admin("a1"), $NAME.CreateInput{Name: "first"})

	require.EqualError(t, err, "db down")
}

func TestGet(t *testing.T) {
	t.Parallel()
	want := &$NAME.$TITLE{ID: "id-1", Name: "first", CreatedAt: createdAt}
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetByID(mock.Anything, "id-1").Return(want, nil).Once()
	repo.EXPECT().GetByID(mock.Anything, "id-2").Return(nil, $NAME.ErrNotFound).Once()
	uc := newUC(t, repo)

	got, err := uc.Get(identitytest.Admin("a1"), "id-1")
	require.NoError(t, err)
	require.Equal(t, want, got)
	_, err = uc.Get(identitytest.Admin("a1"), "id-2")
	require.ErrorIs(t, err, $NAME.ErrNotFound)
}

func TestGet_Forbidden(t *testing.T) {
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
			_, err := newUC(t, mocks.NewMockRepository(t)).Get(tc.ctx, "id-1")

			var forbidden *policy.ForbiddenError
			require.ErrorAs(t, err, &forbidden)
		})
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/dto.go" <<EOF
package http

type ${TITLE}CreateInput struct {
	Body struct {
		Name string \`json:"name" required:"true"\`
	}
}

type ${TITLE}CreateOutput struct {
	Body struct {
		Data ${TITLE}View \`json:"data"\`
	}
}

type ${TITLE}GetInput struct {
	ID string \`path:"id"\`
}

type ${TITLE}GetOutput struct {
	Body struct {
		Data ${TITLE}View \`json:"data"\`
	}
}
EOF

cat >"$DOMAIN_DIR/delivery/http/view.go" <<EOF
package http

import (
	"time"

	"$MODULE_PATH/internal/domain/$NAME"
)

type ${TITLE}View struct {
	ID        string    \`json:"id"\`
	Name      string    \`json:"name"\`
	CreatedAt time.Time \`json:"created_at"\`
}

func (v *${TITLE}View) FromEntity(e *$NAME.$TITLE) {
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
	Create(ctx context.Context, in $NAME.CreateInput) (*$NAME.$TITLE, error)
	Get(ctx context.Context, id string) (*$NAME.$TITLE, error)
}

type Handler struct {
	uc Usecase
}

func NewHandler(uc Usecase) *Handler { return &Handler{uc: uc} }

func Register(api huma.API, h *Handler) {
	huma.Register(api, createErrors.Declare(authgate.Secured(huma.Operation{
		OperationID:   "create$TITLE",
		Method:        http.MethodPost,
		Path:          "/api/${ROUTE}",
		Tags:          []string{"${ROUTE}"},
		DefaultStatus: http.StatusCreated,
		Summary:       "Создать $NAME",
	})), h.create)

	huma.Register(api, getErrors.Declare(authgate.Secured(huma.Operation{
		OperationID: "get$TITLE",
		Method:      http.MethodGet,
		Path:        "/api/${ROUTE}/{id}",
		Tags:        []string{"${ROUTE}"},
		Summary:     "Получить $NAME по ID",
	})), h.get)
}

func (h *Handler) create(ctx context.Context, in *${TITLE}CreateInput) (*${TITLE}CreateOutput, error) {
	e, err := h.uc.Create(ctx, $NAME.CreateInput{Name: in.Body.Name})
	if err != nil {
		return nil, ${CAMEL}Errors.Map(err)
	}
	out := &${TITLE}CreateOutput{}
	out.Body.Data.FromEntity(e)
	return out, nil
}

func (h *Handler) get(ctx context.Context, in *${TITLE}GetInput) (*${TITLE}GetOutput, error) {
	e, err := h.uc.Get(ctx, in.ID)
	if err != nil {
		return nil, ${CAMEL}Errors.Map(err)
	}
	out := &${TITLE}GetOutput{}
	out.Body.Data.FromEntity(e)
	return out, nil
}

// Правило каждой ошибки домена — одно. ${CAMEL}Errors переводит в хендлерах их все (ни одна не уходит
// 500 — archtest), таблица операции объявляет в OpenAPI те, что отдаёт её usecase.
var (
	notFoundRule    = httpkit.ErrorRule{Err: $NAME.ErrNotFound, Status: http.StatusNotFound, Code: "${UPPER}_NOT_FOUND", Detail: "$NAME not found"}
	invalidNameRule = httpkit.ErrorRule{Err: $NAME.ErrInvalidName, Status: http.StatusUnprocessableEntity, Code: "${UPPER}_INVALID_NAME", Detail: "invalid name", Field: "body.name"}

	${CAMEL}Errors = httpkit.NewErrorMap(notFoundRule, invalidNameRule, authgate.ForbiddenRule)

	createErrors = httpkit.NewErrorMap(invalidNameRule, authgate.ForbiddenRule)
	getErrors    = httpkit.NewErrorMap(notFoundRule, authgate.ForbiddenRule)
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
		{"create$TITLE", createErrors, []error{$NAME.ErrInvalidName, forbidden}},
		{"get$TITLE", getErrors, []error{$NAME.ErrNotFound, forbidden}},
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

var created = &$NAME.$TITLE{ID: "id-1", Name: "first", CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}

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

func TestCreate_Created(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Create(mock.Anything, $NAME.CreateInput{Name: "first"}).Return(created, nil).Once()

	resp := setup(t, uc).PostCtx(identitytest.Admin("a1"), "/api/${ROUTE}", strings.NewReader(\`{"name":"first"}\`))

	handlertest.AssertStatus(t, resp, http.StatusCreated)
	assertView(t, resp.Body.Bytes())
}

func TestCreate_Errors(t *testing.T) {
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
			uc.EXPECT().Create(mock.Anything, $NAME.CreateInput{Name: "first"}).Return(nil, tc.err).Once()

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

func TestGet_OK(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Get(mock.Anything, "id-1").Return(created, nil).Once()

	resp := setup(t, uc).GetCtx(identitytest.Admin("a1"), "/api/${ROUTE}/id-1")

	handlertest.AssertStatus(t, resp, http.StatusOK)
	assertView(t, resp.Body.Bytes())
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()
	uc := mocks.NewMockUsecase(t)
	uc.EXPECT().Get(mock.Anything, "id-2").Return(nil, $NAME.ErrNotFound).Once()

	resp := setup(t, uc).GetCtx(identitytest.Admin("a1"), "/api/${ROUTE}/id-2")

	handlertest.AssertAPIError(t, resp, http.StatusNotFound, "${UPPER}_NOT_FOUND")
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/queries/${NAME}.sql" <<EOF
-- name: Insert${TITLE} :one
INSERT INTO ${TABLE} (name)
VALUES (\$1)
RETURNING id, created_at;

-- name: Get${TITLE}ByID :one
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

func NewRepository(database *pgkit.DB) $NAME.Repository {
	return &Repository{q: db.New(database)}
}

func (r *Repository) Insert(ctx context.Context, e *$NAME.$TITLE) error {
	row, err := r.q.Insert$TITLE(ctx, e.Name)
	if err != nil {
		return fmt.Errorf("$NAME repo: insert: %w", err)
	}
	e.ID = pgkit.UUIDString(row.ID)
	e.CreatedAt = row.CreatedAt.Time
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*$NAME.$TITLE, error) {
	uid, err := pgkit.ParseUUID(id)
	if err != nil {
		return nil, $NAME.ErrNotFound
	}
	row, err := r.q.Get${TITLE}ByID(ctx, uid)
	if pgkit.IsNoRows(err) {
		return nil, $NAME.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("$NAME repo: select: %w", err)
	}
	return &$NAME.$TITLE{ID: pgkit.UUIDString(row.ID), Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
}
EOF

cat >"$DOMAIN_DIR/infra/postgres/repository_test.go" <<EOF
//go:build integration

package postgres_test

import (
	"context"
	"testing"

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

	e := &$NAME.$TITLE{Name: "first"}
	require.NoError(t, repo.Insert(ctx, e))
	require.NotEmpty(t, e.ID)
	require.False(t, e.CreatedAt.IsZero(), "the database sets the creation time")

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
	require.ErrorIs(t, repo.Insert(canceled, &$NAME.$TITLE{Name: "second"}), context.Canceled)
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
      Repository:
  ${MODULE_PATH}/internal/domain/${NAME}/delivery/http:
    interfaces:
      Usecase:
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

echo "→ make gen"
make --no-print-directory gen

echo "→ go vet: срез и cmd/api собираются"
go vet -tags=integration "./$DOMAIN_DIR/..." ./cmd/api/

completed=1
git add "$DOMAIN_DIR" "${MIGRATION}.up.sql" "${MIGRATION}.down.sql" sqlc.yaml .mockery.yml cmd/api/main.go cmd/api/http.go openapi.json

echo
echo "✓ домен $NAME: срез Create и Get (POST /api/${ROUTE}, GET /api/${ROUTE}/{id}, только admin),"
echo "  таблица ${TABLE}, подключение в cmd/api; созданное — в индексе git"
echo
echo "Дальше:"
echo "  1. make check (нужен Docker)"
echo "  2. Домен — по артефакту strategy: .claude/skills/scaffold/references/domain.md § 4"
echo "     (поля — в миграцию среза до первого коммита; права, операции, события)"
