# Cookie-only auth + golibinfra v0.20 bump

**Slug:** `cookie-only-auth`
**Создан:** 2026-05-13
**Статус:** in-progress
**Последний чекпоинт:** —

## Контекст

В `ROC/admin-panel-backend` закрыта auth-эпопея: cookie-only flow (`httpOnly access_token`/`refresh_token`), drop Authorization-header fallback, OpenAPI security scheme переведена на `apiKey/cookie/access_token` через fx-группу `httpkit.security_scheme` (golibinfra v0.20.0). См. ROC-коммиты `6286b2e` (cookie scheme + drop header), `21eb7ab` (/refresh с rotation), `d4f936f` (cookie names → `access_token`/`refresh_token`).

Шаблон `stit/go-backend-template` сейчас:
- `auth`-домен уже **впереди** ROC по refresh-логике: `Refresh()` с rotation + `writeRefreshReusedAudit` + sentinel `ErrRefreshReused` + revoke-all-on-reuse (см. `internal/domain/auth/usecase.go`).
- Но всё ещё на **JSON-bearer**: `LoginOutput.AccessToken/RefreshToken` в response body, `authgate.Extractor.Extract` читает только `Authorization: Bearer …` (`internal/platform/authgate/jwt.go:23`), security schemes `{"Bearer": {}}` в 6 операциях (auth: 1, user: 5; verification — public).
- golibinfra на v0.19.0 (hardcoded Bearer scheme в httpkit). Тег v0.20.0 в golibinfra уже опубликован (`457d969 refactor(httpkit)!: drop hardcoded Bearer scheme, accept SecuritySchemeEntry via fx group`).

Цель: довести template до cookie-only auth, чтобы будущие проекты из шаблона стартовали с правильной модели. Reuse-detection в шаблоне **не трогаем** (она богаче ROC-варианта — рекомендована as-is к обратному backport'у в ROC отдельной задачей).

## Не входит в скоуп

- Backport шаблонной reuse-detection (audit + `ErrRefreshReused`) в ROC. Отдельная задача.
- Рефакторинг user/verification domain'ов (касаемся только их huma security-метаданных).
- Изменение API auth-usecase (Login/Refresh/Logout сигнатуры остаются — меняется только delivery/http слой).
- Добавление CSRF/SameSite-strict — оставляем дефолты, которые в ROC оказались рабочими.
- DDD-артефакты в `docs/domains/auth/` (не у этого PR'а).

## Открытые вопросы

- [x] Q1: iam→auth mapping. Template уже на `auth` — никакого переименования, просто адаптируем идеи из ROC-iam-diff'ов к шаблонному `auth`.
- [x] Q2: golibinfra состояние. Тег `v0.20.0` уже опубликован — bump через `go get @v0.20.0`.
- [x] Q3: Reuse-policy. Оставляем шаблонную (audit + `ErrRefreshReused`).
- [x] Q4: Plan location. В template-репо (этот файл).
- [x] Q5: Smoke-script. Портируем `iam-smoke.sh` → `auth-smoke.sh` (cookie-flow).
- [x] Q6: Branch. Работаем прямо на `main` шаблона, commit-by-commit.
- [x] Q7: Bearer-security охват. 6 ссылок: `auth/delivery/http/handler.go:58` (login) + `user/delivery/http/handler.go` строки 44/53/62/71/80. `verification` без security.

## Атомы

- [ ] **A1:** Bump golibinfra v0.19.0 → v0.20.0 + регистрация Cookie scheme через fx group + cookie-constants.
  - Что делаем:
    - `go get git.stit.tech/stit/golibinfra@v0.20.0 && go mod tidy`
    - Создать `internal/platform/authgate/cookie.go` с константами `CookieAccess = "access_token"` / `CookieRefresh = "refresh_token"` и helper'ами `SetAccess(w, token, exp)` / `SetRefresh(w, token, exp)` / `Clear(w)` (по образцу `ROC/internal/platform/authgate/cookie.go`).
    - В `internal/platform/authgate/module.go` добавить provider `fx.Annotated{Group: "httpkit.security_scheme", Target: func() httpkit.SecuritySchemeEntry { ... apiKey/cookie/access_token ... }}`.
    - При необходимости поправить вызов `httpkit.NewServer` в `cmd/api/` под новую сигнатуру (variadic schemes — параметр опциональный, ничего не должно сломаться).
  - Файлы: `go.mod`, `go.sum`, `internal/platform/authgate/cookie.go` (новый), `internal/platform/authgate/module.go`, возможно `cmd/api/server.go`.
  - Verify: `scripts/agent-tools/build.sh` → OK; `scripts/agent-tools/test.sh ./internal/platform/...` → PASS.
  - Зависит от: —

- [ ] **A2:** Authgate `Extractor`: cookie-only (drop Authorization header).
  - Что делаем:
    - В `internal/platform/authgate/jwt.go` заменить парсинг `Authorization: Bearer …` на чтение `r.Cookie(CookieAccess)`. Без fallback.
    - Сохранить структуру `Extractor` + конструктор `NewExtractor`. Error messages обновить под cookie.
  - Файлы: `internal/platform/authgate/jwt.go`.
  - Verify: `scripts/agent-tools/test.sh ./internal/platform/authgate/...` → PASS (тесты переписать если они мокают Authorization header).
  - Зависит от: A1.

- [ ] **A3:** Auth-домен delivery/http: cookie-in/cookie-out для login/refresh/logout + huma Security → Cookie.
  - Что делаем:
    - `dto.go`:
      - `LoginInput` — без изменений.
      - `LoginOutput` — убрать поля `AccessToken`/`RefreshToken` из JSON body; добавить два `header:"Set-Cookie"`-поля (cookie-attrs формируются хелперами из A1 на handler-уровне). Альтернатива: оставить тонкий wrapper и устанавливать cookies через `huma.Context.SetCookie` — выбрать что чище в huma/v2 (как в ROC iam-dto).
      - `RefreshInput` — убрать JSON `RefreshToken`, заменить на `RefreshCookie string` с huma `cookie:"refresh_token"`.
      - `RefreshOutput` — то же что `LoginOutput` (Set-Cookie на access+refresh).
      - `LogoutInput` — `RefreshCookie string` с huma `cookie:"refresh_token"`.
      - `LogoutOutput` — Set-Cookie с пустыми/expired для clear.
    - `handler.go`:
      - login/refresh: вызвать usecase → результат → `authgate.SetAccess` / `SetRefresh` хелперы (или ручная сборка cookies в LoginOutput.SetCookie).
      - logout: вызвать usecase → `authgate.Clear`.
      - В `huma.Register` поменять `Security: [{"Bearer": {}}]` → `[{"Cookie": {}}]`.
    - `handler_test.go`: переписать на cookie I/O (httptest.Cookie в Request, проверять `Set-Cookie` headers в Response). Сценарии: login → cookies, refresh с cookie → rotate, refresh без cookie → 400, refresh с revoked jti → 401 (reuse-detect path), logout → clears cookies.
    - `view.go` — если есть mapping, проверить что не зависит от убранных JSON полей.
  - Файлы: `internal/domain/auth/delivery/http/dto.go`, `handler.go`, `handler_test.go`, `view.go`.
  - Verify: `scripts/agent-tools/test.sh ./internal/domain/auth/...` → PASS.
  - Зависит от: A2.

- [ ] **A4:** Switch `Security: [{"Bearer": {}}]` → `[{"Cookie": {}}]` в `user`-домене.
  - Что делаем: заменить 5 строк в `internal/domain/user/delivery/http/handler.go` (44/53/62/71/80). `verification` не трогаем (public endpoints).
  - Файлы: `internal/domain/user/delivery/http/handler.go`.
  - Verify: `scripts/agent-tools/build.sh` → OK; `scripts/agent-tools/test.sh ./internal/domain/user/...` → PASS.
  - Зависит от: A1 (нужна зарегистрированная Cookie scheme).

- [ ] **A5:** Smoke `scripts/smoke/auth-smoke.sh` — адаптация ROC iam-smoke.sh под template/auth.
  - Что делаем: создать `scripts/smoke/` если нет, портировать `ROC/scripts/smoke/iam-smoke.sh` с заменой `/api/auth/*` маршрутов (одинаковые) и cookie-names (одинаковые). Сценарии: login → cookies set, /me с cookie → 200, refresh → rotates, replay-of-old-refresh → 401 (reuse-detect — в шаблоне это путь через ErrRefreshReused, ожидаем 401), logout → cookies cleared.
  - Файлы: `scripts/smoke/auth-smoke.sh` (новый), возможно `scripts/smoke/README.md`.
  - Verify: ручной прогон против локально-поднятого `bin/api` (опционально для атома — закрываем когда юзер прогнал; иначе синтаксический `bash -n` PASS).
  - Зависит от: A3.

- [ ] **A6:** Финальный gate + commit-chain в template-main.
  - Что делаем: `make pre-merge` (или `scripts/agent-tools/test.sh && lint.sh && build.sh`). Серия Conventional Commits, по одному на атом (или разумная агрегация — обсудим перед коммитом):
    1. `chore(deps): bump golibinfra v0.20.0 + register Cookie security scheme` (A1)
    2. `refactor(authgate)!: cookie-only extractor (drop Authorization header)` (A2)
    3. `refactor(auth)!: cookie-in/out for login/refresh/logout` (A3)
    4. `refactor(user): switch Security to Cookie scheme` (A4)
    5. `chore(smoke): add auth-smoke.sh for cookie-flow` (A5)
  - Verify: `make pre-merge` → OK; `git log --oneline main` показывает 5 коммитов в правильном порядке.
  - Зависит от: A1..A5.

## Чекпоинты

—
