---
status: in-progress
parent: —
blocked-by: —
---

# polish-code-security-adapters — добор кода, security и границ до «чисто»

## Цель
Закрыть в коде-эталоне, его security-поверхности и правилах archtest всё, что после переаудита осталось ниже
девятки и выше нуля: находки NSEC-01…03, NCOD-01, NADP-01, 02, 04, NTST-01, minor бэклога (`minor-code`,
`minor-security`, `minor-adapters`) и трения догфудинга T3, T4, T6, T11 — так, чтобы эксперименты аудиторов на
ветке дефекта не давали, а важное держала машина.

## Контекст
- Программа — `.claude/plans/reaudit-to-nine.md` (S7, трек code+security+adapters, решение владельца 15.10 — и
  minor). Ветка `nine/s7-code`, worktree `N/wt-s7c` от `main` `a4020ee`. `N` = `/home/mm/.cache/stit-audit-program/nine`,
  `R` = `/home/mm/.cache/stit-audit-program/reaudit`; рабочие файлы трека — `N/s7c/`, worktree экспериментов —
  `N/wt-s7c-<x>`.
- Находки и их эксперименты: `N/reaudit/template-{security,code,adapters,tests}.md` (NSEC, NCOD, NADP, NTST; пробы —
  `N/reaudit/tpl-security/`, `tpl-code/`, `tpl-adp/`), прежние minor — `R/template-{code,security,adapters}.md`
  (RCOD, RSECT, RADP; пункты бэклога — `.claude/plans/backlog.md`), трения — `N/dogfood.md` (живой образец — домен
  `note` на ветке `dog/note`).
- Владение: `internal/domain/**`, `internal/platform/**` (кроме `authgate`), `cmd/**`, `pkg/**` (кроме `policy`), новые
  файлы `migrations/`, `openapi.json` (только `make gen`), `SECURITY.md`, `.env.example`,
  `internal/archtest/{archtest,crossdomain,layout,layout_settings,delivery}_test.go` (в `archtest_test.go` — кроме
  `TestTestsAssert` и правила тегов сборки), rules `domain-architecture`, `events-and-messaging`,
  `scaffold/references/{port,endpoint,event,field,env,migration}.md`, `docs/reference/{architecture,domain-mapping}.md`.
  Параллельно идут треки S7 harness+lifecycle и gate+tests.

## Не входит
- NADP-03 (closure конверсией и алиасом), view и SQL-функция над чужой таблицей (часть RADP-05) и CTE с именем
  чужой таблицы (часть RADP-11) — правила archkit в golibinfra: строкой отчёта для раздела других репозиториев.
- RADP-10 и спецификация мутаций `scripts/tools/mutants.txt` — трек gate+tests; RCOD-16 — golibinfra.
- Решения продукта и зоны «сначала спросить»: вход с неподтверждённым адресом (RCOD-08), экспорт
  `authgate.SecurityScheme` ради теста (RCOD-14), `pkg/policy`, `go.mod` — вопросом в «Открытые вопросы».

## Критерии приёмки
- [ ] **AC1** — NSEC-02: вне dev сервис не стартует с `JWT_SECRET` или `JWT_PREVIOUS_SECRET` из примеров окружения
  (`.env.example`, `test/stack`). Проверка: база — `config.Guard` с `APP_ENV=prod` и секретом из `.env.example` → nil
  (проба D05/D12); ветка — `ErrUnsafe`; `go test ./internal/platform/config/` → PASS
- [ ] **AC2** — Ссылки сброса: письма в один ящик с многих адресов — не чаще паузы своей настройки (NSEC-01); срок
  ссылки — не дольше часа от обработки, событие из будущего его не продлевает (NSEC-03); два параллельных
  подтверждения двух ссылок одного пользователя — 204 и отказ, без `40P01` (RCOD-10); ссылка, выданная до смены
  пароля, после неё не меняет пароль (RCOD-18, E10); отметка «отправлено» не задевает чужие ссылки (NTST-01).
  Проверка: на базе дефект, на ветке нет — `TestNSec_ResetMailRate`, `TestNSec_ForgedResetEvent`
  (`N/reaudit/tpl-security`), E4 и E10 (`N/reaudit/tpl-code`), мутация `… WHERE id = $1 OR TRUE` → тест репозитория FAIL
- [ ] **AC3** — `seed-admin` — операцией домена user: слабый пароль и не-адрес — код 1 без admin, повышение
  поднимает версию строки, пишет аудит и гасит выданные до него ссылки сброса (RSECT-08, RCOD-12). Проверка: пробы
  M01, M02, E9 на базе — дефект, на ветке — нет; `go test -tags=integration ./cmd/seed-admin/` → PASS
- [ ] **AC4** — `audit_log` со сроком хранения (`AUDIT_RETENTION_DAYS`, job кита), ответы `/api/*` — с
  `Cache-Control: no-store` (RSECT-05, 06). Проверка: проба D14 на ветке → `no-store`; `TestBoot` видит job retention
- [ ] **AC5** — Контракт: у каждой операции `/api/*` под строгим лимитом в `openapi.json` — 429 `RATE_LIMITED`, у
  операций доменов нет `default`, схемы `Cookie` нет, один отказ — один код у адреса регистрации, `maxLength` у
  скелета, у `changePassword` — описание и гашение cookie, в `UserView` — роль и `email_verified`, `enum`/`format` у
  статуса, роли и id (NCOD-01, RCOD-08 частично, RCOD-09). Проверка: сводка `openapi.json` по операциям; E2 —
  пробельные формы адреса → один код
- [ ] **AC6** — Один способ на правило: пароль проверяет `auth` по хэшу порта, хеширует только `user`
  (RCOD-07); смена пароля — у вызывающего, без `user_id` и ветки admin (RCOD-14); мёртвое снято, мелочи
  единообразия сведены (RCOD-14, 18); образцы конфига домена и job'а — в коде (RCOD-17); комментариев длиннее
  двух строк в коде нет (RCOD-15). Проверка: AST-скрипт `N/reaudit/tpl-code/ast` на ветке → пусто; `rg` по
  символам; `make check` → ✓
- [ ] **AC7** — Правила archtest без ложных отказов и с закрытыми обходами: законные `rafp01`, `rafp01d`, `rafp05`,
  `rafp04`, `fp10b` — зелёные (NADP-01, RADP-11); `ra01`, `ra04` — отказ (NADP-02, 04); порт только для чтения не
  переводит ошибку записи поставщика (T11); `n09f`, `n39`, `n56`, `n03f`, `n30`, `o-j+test`, `n33`, `n31`, `n27`,
  `n25`, `n26`, `n23`, `n19q`, `n21`, `n40`, `n41`, `v09g`, чтение до policy, subject `.v1` — отказ (RADP-05…08,
  T3, T6). Проверка: `python3 N/reaudit/tpl-adp/run.py <wt> --no-full <кейс>` на базе и ветке
  (`N/s7c/exp/{base,branch}.tsv`); законные формы прежнего списка — `✓ check clean`
- [ ] **AC8** — RADP-13: у каждого правила файлов трека — свои примеры (нарушение — отказ с текстом, законная
  форма — тишина) в `internal/archtest/testdata/rules`; правило, которое замолчало, роняет тест примеров.
  Проверка: `go test ./internal/archtest/ -run TestRuleExamples` → PASS; ручная поломка правила → FAIL
- [ ] **AC9** — T4: `TestIdentity` не зависит от скорости подписчиков: с задержкой подписчика сброса на базе — FAIL,
  на ветке — PASS. Проверка: инъекция задержки в `N/wt-s7c-t4` → база FAIL, ветка PASS
- [ ] **AC10** — T6: образец ресурса с владельцем (видимость — параметр запроса, чужое — 404) в
  `docs/reference/architecture.md` и `scaffold/references/endpoint.md`; `CanGet` скелета без id. Проверка:
  ручная сверка; `make new-domain NAME=order` во временном worktree → `✓ check clean`
- [ ] **AC11** — Гейт и handoff: `make check` → `✓ check clean`; блоки INFRA-HANDOFF — в `.claude/plans/handoff.md`.
  Проверка: `flock N/gate.lock make check`; чтение накопителя

## Шаги
- [ ] **S1** — NSEC-02: `config.Guard` отвергает ключи подписи из примеров окружения; тест сверяет их с файлами
  - Проверка: RED на базе → GREEN; `make check` → ✓ · Закрывает: AC1 · Зависит от: —
- [ ] **S2** — T4: `TestIdentity` ждёт письма до правки таблицы `users`, токены берёт до второй регистрации
  - Проверка: эксперимент AC9; `make check` → ✓ · Закрывает: AC9 · Зависит от: —
- [ ] **S3** — Ссылки сброса: срок от `min(At, now)`, погашение своей и соседних одним оператором, ссылка
  привязана к версии учётной записи (новая миграция), битый id записи — ошибка, тест отметки своей строки
  - Проверка: RED на базе → GREEN; пробы AC2 (кроме NSEC-01); `make check` → ✓ · Закрывает: AC2 · Зависит от: —
- [ ] **S4** — NSEC-01 и RCOD-17: пауза писем сброса на учётную запись — своя настройка через `Config` домена
  verification; лимитер на guard входа снят; job чистки отживших ссылок — образец job'а домена
  - Проверка: `TestNSec_ResetMailRate` на ветке; `make check` → ✓ · Закрывает: AC2, AC6 · Зависит от: S3
- [ ] **S5** — `seed-admin` через `user.Usecase.EnsureAdmin`: правило пароля, сущность, аудит, версия; логгер
  входов `cmd/` — из `LOG_*`
  - Проверка: M01, M02, E9; `make check` → ✓ · Закрывает: AC3, AC6 · Зависит от: S3
- [ ] **S6** — Срок хранения аудита (job кита) и `Cache-Control: no-store` у ответов `/api/*`
  - Проверка: D14; `TestBoot`; `make check` → ✓ · Закрывает: AC4 · Зависит от: —
- [ ] **S7** — Контракт API: 429 строгого лимита, таблицы logout и запроса сброса, адрес регистрации, cookie
  строкой, `changePassword`, поля и форматы ответов, ветки пустого cookie
  - Проверка: сводка `openapi.json`; E2; `make check` → ✓ · Закрывает: AC5 · Зависит от: S6
- [ ] **S8** — T11, NADP-04: ошибки `Trusted` — по методам; archtest читает методы `*Trusted` любого файла корня
  - Проверка: `ra04` → отказ; `make check` → ✓ · Закрывает: AC7 · Зависит от: —
- [ ] **S9** — RCOD-07, RCOD-14: пароль хеширует `user.Trusted.HashPassword`, проверяет `auth`; смена пароля —
  у вызывающего
  - Проверка: `rg` по снятым символам; `make check` → ✓ · Закрывает: AC6 · Зависит от: S7, S8
- [ ] **S10** — RCOD-18 и RCOD-14: входы usecase значением, имена DTO, логи успеха, тип статуса во входе HTTP,
  порядок и проекция списка, мёртвый `StatusUnknown`
  - Проверка: `make check` → ✓ · Закрывает: AC6 · Зависит от: S9
- [ ] **S11** — RADP-13: примеры правил archtest (`testdata/rules`); NADP-01, NADP-02, RADP-11, T3 — правила и
  примеры
  - Проверка: AC8; кейсы AC7 этих находок; `make check` → ✓ · Закрывает: AC7, AC8 · Зависит от: S8
- [ ] **S12** — Новые правила archtest (RADP-05…08, `identity.With`, чтение до policy) с примерами; T6 — образец и
  скелет; RADP-12 — проза
  - Проверка: кейсы AC7; AC10; `make check` → ✓ · Закрывает: AC7, AC8, AC10 · Зависит от: S11
- [ ] **S13** — RCOD-15: комментарии кода — не длиннее двух строк, только WHY
  - Проверка: AST-скрипт → пусто; `make check` → ✓ · Закрывает: AC6 · Зависит от: S12
- [ ] **S14** — Приёмка: эксперименты на базе и ветке, гейт, handoff
  - Проверка: `N/s7c/exp/{base,branch}.tsv`; `make check` → `✓ check clean` · Зависит от: S13
  - Закрывает: AC1, AC2, AC3, AC4, AC5, AC6, AC7, AC8, AC9, AC10, AC11

## Решения
- —

## Открытые вопросы
- [ ] **Q1** — Пускать ли вход с неподтверждённым адресом (RCOD-08): решение продукта — блокирует: — — к кому: владелец
- [ ] **Q2** — `authgate.SecurityScheme` экспортирован ради теста (RCOD-14): зона «сначала спросить» — блокирует: — —
  к кому: владелец

## Где мы сейчас
План записан. Следующий шаг — S1 (NSEC-02).
