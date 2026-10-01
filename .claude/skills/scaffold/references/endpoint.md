# scaffold › endpoint — новый HTTP-метод в существующем домене

**Шаг 0** — прочитай инструментом Read `.claude/rules/domain-architecture.md` и
`.claude/rules/tests-pipeline.md`.

Расширение `delivery/http/` существующего домена новым HTTP-методом. Новый домен — вариант `domain`; переделка существующего метода — точечная правка без scaffold.

## Когда применять

- "Добавь POST /api/users/{id}/avatar"
- "Нужен endpoint для смены email"
- "Добавь GET /api/sessions"

## Когда НЕ применять

- Ручка не укладывается в один коммит или ломает контракт — сначала `/plan` (таблица «Маршрут
  задачи», CLAUDE.md).
- Меняешь существующий route — точечная правка, без scaffold.

## Шаги

### 1. Уточнить контракт

Спроси одним блоком до начала:
- Метод (GET/POST/PATCH/DELETE)?
- Path?
- Кто имеет право вызывать (anonymous / любой авторизованный / только self / только admin)?
- Какие коды ошибок специфичны для этого endpoint?
- Идемпотентный?
- Принимает body / query / path-параметры?

### 2. Добавить usecase-метод (если нужен)

Если бизнес-логика новая — добавь в `internal/domain/<name>/usecase.go`. Сигнатура: `func (uc *Usecase) <Verb>(ctx, in) (*out, error)`.

Если в `policy.go` нет метода для этого действия — добавь `Can<Verb>` в interface `Authorizer` и в `Policy`. Не забудь `policy_test.go`.

### 3. Обновить порт `Usecase` в `delivery/http/handler.go`

Расширь interface `Usecase` новым методом:

```go
type Usecase interface {
    // ...
    NewMethod(ctx context.Context, in NewInput) (*NewOutput, error)
}
```

После этого `make gen` (mockery поднимет порт из `.mockery.yml`, мок-файл регенерируется).

### 4. DTO в `delivery/http/dto.go`

Имена — `<Domain><Op>Input/Output` (huma собирает все DTO в одну OpenAPI-registry по имени типа;
`NewMethodInput` в двух доменах → panic), тело ответа — envelope `data`; держит archtest
`TestHTTPTypes`:

```go
type UserNewMethodInput struct {
    ID   string `path:"id" doc:"User ID"`
    Body struct {
        Field string `json:"field" minLength:"1" doc:"…"`
    }
}

type UserNewMethodOutput struct {
    Body struct {
        Data UserView `json:"data"`
    }
}
```

Используй huma-теги: `path`, `query`, `header`, валидаторы (`minLength`, `format:"email"` и т.д.).
Список — встрой `paging.Page` (`pkg/paging`): query `page` и `size` с общими дефолтом и пределом.

### 5. Handler-метод в `handler.go`

```go
func (h *Handler) newMethod(ctx context.Context, in *UserNewMethodInput) (*UserNewMethodOutput, error) {
    res, err := h.uc.NewMethod(ctx, /* mapping */)
    if err != nil {
        return nil, userErrors.Map(err) // таблица домена: любая ошибка домена — свой код, не 500
    }
    out := &UserNewMethodOutput{}
    out.Body.Data.FromEntity(res)
    return out, nil
}
```

### 6. Зарегистрировать в `Register`

```go
huma.Register(api, newMethodErrors.Declare(authgate.Secured(huma.Operation{
    OperationID:   "newMethod",
    Method:        http.MethodPost,
    Path:          "/api/users/{id}/...",
    Tags:          []string{"users"},
    DefaultStatus: http.StatusOK, // или 201/204
    Summary:       "Краткое описание для OpenAPI",
})), h.newMethod)
```

`authgate.Secured` — операция под cookie: `Security` и коды 401 сессии в OpenAPI. Публичная
операция — без него и в `publicOperations` (`cmd/api/api_test.go`).

### 7. Ошибки: правило домена и таблица операции

Правило каждой ошибки `errors.go` — одно, переменной пакета (`notFoundRule`). Таблица домена
`userErrors` переводит их все — хендлер отдаёт ошибку только через неё. Таблица операции из тех же
правил объявляет при регистрации (`Declare`) коды, которые отдаёт её usecase (`openapi.json`):

```go
var (
    xyzRule = httpkit.ErrorRule{Err: user.ErrXyz, Status: http.StatusConflict, Code: "USER_XYZ", Detail: "..."}

    userErrors      = httpkit.NewErrorMap(notFoundRule /* …правило каждой ошибки домена… */, xyzRule, authgate.ForbiddenRule)
    newMethodErrors = httpkit.NewErrorMap(notFoundRule, xyzRule, authgate.ForbiddenRule)
)
```

- Новая ошибка домена — правило в `userErrors`, иначе archtest `TestDomainErrorTables` красный: она
  ответила бы 500.
- Код — `<DOMAIN>_<WHAT>`; разный смысл — разная ошибка домена; ошибка поля запроса — 422 с
  `Field: "body.<field>"`.
- 429 — `Retry-After` из метода `RetryAfter() time.Duration` ошибки в цепочке (`auth.LockedError`).
- Первое совпавшее правило выигрывает; правило, которое затеняет правило выше, — panic при сборке.
- Операция без доменных ошибок таблицы операции не заводит; заведённая таблица без `Declare` в
  `huma.Register` — archtest красный: её коды не попали бы в OpenAPI.

Контракт ошибок целиком — `docs/reference/architecture.md` § Контракт ошибок API.

### 8. Тесты

`handler_test.go`, минимум:
- Happy path (201/200)
- Forbidden (мок usecase возвращает `&policy.ForbiddenError{…}` → 403)
- Validation error (huma сам отдаёт 422)
- Каждый код таблицы операции (`handlertest.AssertAPIError`)

`errors_test.go` (white-box) — строка операции в `TestErrorMaps`: таблица операции объявляет ошибки
её usecase (список — по коду usecase, `Unmapped` пуст).

Используй `MockUsecase` (уже регенерирован после шага 3).

### 9. Верификация

```bash
make gen          # openapi.json — контракт API — подхватывает новую операцию
make test PKG=./internal/domain/<name>/...
make lint PKG=./internal/domain/<name>/...
make build
```

Готово и коммит — по CLAUDE.md § Готово.

### 10. Коммит

`feat(<domain>): add <METHOD> <path>` — например, `feat(user): add POST /api/users/{id}/avatar`.

## Anti-patterns

- ❌ Бизнес-логика в handler-методе. Handler только: парсит вход, вызывает usecase, мапит ошибки.
- ❌ Прямое использование `pgx`/`sqlc` из handler. Только через usecase.
- ❌ Свой формат ошибок мимо `httpkit.ErrorMap` — нарушит единый контракт ошибок (problem+json с `code`).
- ❌ Забыть `Security:` для авторизованного endpoint — он публичен в рантайме; `TestHTTPOperations` (`cmd/api`) не пропустит операцию без `Security` вне `publicOperations`.
- ❌ `_, _ = ctx, in` в handler без вызова usecase — компилятор пропустит, но это dead-handler.
