# scaffold › endpoint — новый HTTP-метод в существующем домене

Расширение `delivery/http/` существующего домена новым HTTP-методом. НЕ для нового домена (вариант `domain`) и НЕ для перепиливания существующего метода (точечная правка, без scaffold).

## Когда применять

- "Добавь POST /api/users/{id}/avatar"
- "Нужен endpoint для смены email"
- "Добавь GET /api/sessions"

## Когда НЕ применять

- Endpoint требует новых usecase-методов и крупных изменений domain — это уже фича, лучше через `plan`.
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

```go
type NewMethodInput struct {
    ID   string `path:"id" doc:"User ID"`
    Body struct {
        Field string `json:"field" minLength:"1" doc:"…"`
    }
}

type NewMethodOutput struct {
    Body struct {
        Data View `json:"data"`
    }
}
```

Используй huma-теги: `path`, `query`, `header`, валидаторы (`minLength`, `format:"email"` и т.д.).

### 5. Handler-метод в `handler.go`

```go
func (h *Handler) newMethod(ctx context.Context, in *NewMethodInput) (*NewMethodOutput, error) {
    res, err := h.uc.NewMethod(ctx, /* mapping */)
    if err != nil {
        return nil, mapErr(err)
    }
    out := &NewMethodOutput{}
    out.Body.Data.FromEntity(res)
    return out, nil
}
```

### 6. Зарегистрировать в `Register`

```go
huma.Register(api, huma.Operation{
    OperationID:   "newMethod",
    Method:        http.MethodPost,
    Path:          "/api/users/{id}/...",
    Tags:          []string{"users"},
    Security:      []map[string][]string{{"Bearer": {}}},  // если требуется auth
    DefaultStatus: http.StatusOK,                           // или 201/204
    Summary:       "Краткое описание для OpenAPI",
}, h.newMethod)
```

### 7. Error mapping

Если usecase возвращает новые sentinel-ошибки — добавь их в `mapErr()`:

```go
httpkit.ErrorMapping{
    Match:  func(e error) bool { return errors.Is(e, user.ErrXyz) },
    Code:   "USER_XYZ",
    Status: http.StatusBadRequest,
    Detail: "...",
},
```

`ForbiddenError` уже покрыт — не дублируй.

### 8. Тест в `handler_test.go`

Минимум:
- Happy path (201/200)
- Forbidden (mock policy.Deny → 403)
- Validation error (huma сам отдаёт 422 / 400)
- Один specific error mapping (например, `ErrAlreadyExists` → 409)

Используй `MockUsecase` (уже регенерирован после шага 3).

### 9. Верификация

```bash
scripts/tools/test.sh ./internal/domain/<name>/...
scripts/tools/lint.sh ./internal/domain/<name>/...
scripts/tools/build.sh
```

### 10. Коммит

`feat(<domain>): add <METHOD> <path>` — например, `feat(user): add POST /api/users/{id}/avatar`.

## Anti-patterns

- ❌ Бизнес-логика в handler-методе. Handler только: парсит вход, вызывает usecase, мапит ошибки.
- ❌ Прямое использование `pgx`/`sqlc` из handler. Только через usecase.
- ❌ Свой формат ошибок мимо `httpkit.MapErr` — нарушит единый shape ответа.
- ❌ Забыть `Security:` для авторизованного endpoint — он окажется анонимным в swagger.
- ❌ `_, _ = ctx, in` в handler без вызова usecase — компилятор пропустит, но это dead-handler.
