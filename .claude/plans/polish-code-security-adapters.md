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
- [x] **S1** — NSEC-02: `config.Guard` отвергает ключи подписи из примеров окружения; тест сверяет их с файлами
  - Проверка: RED на базе → GREEN; `make check` → ✓ · Закрывает: AC1 · Зависит от: —
  - Сделано: `exampleJWTSecrets` в `Guard` (и для `JWT_PREVIOUS_SECRET`); `TestGuard_ExampleSecrets` берёт ключи
    из `.env.example` и `test/stack/compose.yml` — на базе FAIL (Guard → nil), на ветке PASS; handoff-блок;
    `make check` → ✓; Review: дирижёр программы reaudit-to-nine через валидатора трека — ожидает
- [x] **S2** — T4: `TestIdentity` ждёт письма до правки таблицы `users`, токены берёт до второй регистрации
  - Проверка: эксперимент AC9; `make check` → ✓ · Закрывает: AC9 · Зависит от: —
  - Сделано: ожидание обоих писем — до переименования `users` и второй регистрации; эксперимент
    `N/s7c/exp/t4-README.txt` (подписчик сброса на 3 с медленнее): база — FAIL «Condition never satisfied», ветка —
    PASS; `make check` → ✓
- [x] **S3** — Ссылки сброса: срок от `min(At, now)`, погашение своей и соседних одним оператором, ссылка
  привязана к версии учётной записи (новая миграция), битый id записи — ошибка, тест отметки своей строки
  - Проверка: RED на базе → GREEN; пробы AC2 (кроме NSEC-01); `make check` → ✓ · Закрывает: AC2 · Зависит от: —
  - Сделано: подписчик сброса берёт `min(At, now)`; `RedeemPasswordReset` — один `UPDATE … RETURNING id`; миграция
    `0007` и `PasswordResetIssue.UserVersion` — подтверждение пишет пароль при версии на момент письма;
    `MarkEmailVerificationUsed` с битым id — ошибка. RED на базе: `TestIssuePasswordReset_RequestFromTheFuture`
    (upsert со сроком от будущего At), подтест `TestSessions` «ссылка до смены пароля» (204 вместо 400),
    `TestRepository_Failures` (sentinel вместо ошибки), E4 — `40P01` (`N/s7c/exp/e4-base.log`); на ветке — PASS,
    E4 — `nil` и `ErrTokenUsed` (`e4-branch.log`); NTST-01: мутация `… OR TRUE` — FAIL
    `TestMarkPasswordResetSent_OwnLinkOnly`, без неё — PASS (`ntst01-*.log`); handoff-блок; `make check` → ✓
- [x] **S4** — NSEC-01 и RCOD-17: пауза писем сброса на учётную запись — своя настройка через `Config` домена
  verification; лимитер на guard входа снят; job чистки отживших ссылок — образец job'а домена
  - Проверка: `TestNSec_ResetMailRate` на ветке; `make check` → ✓ · Закрывает: AC2, AC6 · Зависит от: S3
  - Сделано: `PASSWORD_RESET_MAIL_PAUSE` (5m) → `verification.Config` → условие `UpsertPasswordReset` (другая ссылка
    пользователя моложе паузы); `ResetLimiter` и `verification/infra/bruteforce` сняты; job `verification.purge_links`
    (`Usecase.PurgeLinks`, батч по 5000). RED `TestIdentity_ResetMailPause`: три запроса с трёх адресов — на базе 3
    письма, на ветке 1; handoff-блок; `make check` → ✓. Гонка: подписчик обрабатывает доставки параллельно, и
    выпуски в пересекающихся транзакциях не видели друг друга (эксперимент `N/s7c/exp/pause-overlap-base.log`: оба
    выпущены) — выпуски ссылок пользователя идут по очереди (`LockPasswordResets`, `TestLockPasswordResets`)
- [x] **S5** — `seed-admin` через `user.Usecase.EnsureAdmin`: правило пароля, сущность, аудит, версия; логгер
  входов `cmd/` — из `LOG_*`
  - Проверка: M01, M02, E9; `make check` → ✓ · Закрывает: AC3, AC6 · Зависит от: S3
  - Сделано: `EnsureAdmin` — правило пароля (`hashPassword`, его же берёт `Register`), сущность, `FOR UPDATE` по
    адресу, `Promote` при версии (растёт), закрытие сессий, аудит `seed_admin`; сырого SQL в `cmd/seed-admin` нет;
    логгеры `cmd/migrate` и `cmd/seed-admin` — `logkit.NewConfig`. RED: `TestRun_RejectsWeakPassword` (код 0 с
    паролем «1»), `TestRun_PromotionIsVersionedAndAudited` (версия 1, аудита нет) — на ветке PASS; `make check` → ✓
- [x] **S6** — Срок хранения аудита (job кита) и `Cache-Control: no-store` у ответов `/api/*`
  - Проверка: D14; `TestBoot`; `make check` → ✓ · Закрывает: AC4 · Зависит от: —
  - Сделано: `auditpg.RetentionModule` и `NewRetentionConfig` в графе, `AUDIT_RETENTION_DAYS` — required (в
    `.env.example` — 30); middleware `noStore` в `cmd/api/http.go`; `TestBoot` видит job'ы `auditkit.retention` и
    `verification.purge_links` и `no-store` у ответа `/api/users/me`, `TestMiddlewares_NoStore` — и у отказа лимита;
    handoff-блок; `make check` → ✓
- [x] **S7** — Контракт API: 429 строгого лимита, таблицы logout и запроса сброса, адрес регистрации, cookie
  строкой, `changePassword`, поля и форматы ответов, ветки пустого cookie
  - Проверка: сводка `openapi.json`; E2; `make check` → ✓ · Закрывает: AC5 · Зависит от: S6
  - Сделано: пакет `internal/platform/authlimit` (`Prefix`, `Config`, `Middleware`, `Rule` — 429 `RATE_LIMITED` в таблицах
    операций под префиксом); `TestHTTPOperations` держит 429 у `/api/auth/*` и отсутствие `default`; адрес
    регистрации — `pattern` без пробелов и `maxLength` (все формы не разбора — `VALIDATION_FAILED`,
    `TestRegister_AddressNotCanonical`); cookie — `string`; `changePassword` — описание и гашение cookie;
    `UserView` — `role`, `email_verified`, `enum`/`format`; ветки пустого cookie сняты; скелет — `maxLength`
    имени (правка `scripts/new-domain.sh`, срез `order` — тесты зелёные); handoff-блок; `make check` → ✓
- [x] **S8** — T11, NADP-04: ошибки `Trusted` — по методам; archtest читает методы `*Trusted` любого файла корня
  - Проверка: `ra04` → отказ; `make check` → ✓ · Закрывает: AC7 · Зависит от: —
  - Сделано: `user.TrustedErrors` — карта «метод → ошибки»; тест `Trusted` требует ровно строку метода (и ключи =
    методы); тесты адаптеров переводят ошибки вызываемого метода; `TestTrustedErrors` — по получателю в любом файле
    корня, строка на каждый метод, лишних строк нет. `ra04` (метод в `trusted_rename.go`): без строки и с ошибкой
    вне строки — отказ (на базе archtest читал только `trusted.go`); правка rule `tests-pipeline` (трек gate) —
    две строки таблицы о `TrustedErrors`; `make check` → ✓
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
- RCOD-18 (ссылка сброса переживает смену пароля): ссылка хранит версию учётной записи на момент письма
  (`password_resets.user_version`), подтверждение пишет пароль при ней — изменённая после письма учётная запись
  ссылку гасит; отзыв ссылок из `auth` потребовал бы порта `auth → verification` и транзакции через три домена.
- NSEC-01: пауза писем — условием выпуска ссылки в подписчике (другая ссылка учётной записи моложе паузы, часы БД),
  а не лимитом запроса: письма в ящик ограничены с любых адресов, настройка не делит блокировку входа, запрос
  делает одну работу для любого адреса без ветки «лимит», и адреса анонимных запросов не копятся в
  `bruteforce_attempts`. Лимит запроса на guard входа снят: поток запросов с адреса держит лимит `/api/auth/*`.
- NSEC-01, очередь выпусков: транзакционная advisory-блокировка по пользователю (`pg_advisory_xact_lock`), а не
  строка-слот на учётную запись: без новой таблицы и миграции, письмо уходит после коммита — блокировка не держится
  на вводе-выводе mailer'а.
- RCOD-17: образец конфига домена — `verification.Config` (пауза писем, `config.go` корня), образец job'а —
  `verification.purge_links`: оба — с настоящей нуждой, а не ради образца.
- RCOD-12, RSECT-08: `seed-admin` — метод `user.Usecase.EnsureAdmin` без policy (вход оператора: решает доступ к
  БД), а не `Trusted` (его берут только адаптеры доменов, archtest); событий он не публикует — outbox домена
  монтирует только его `module/` (archkit), а подтверждённому admin письмо не нужно.
- RSECT-05: `AUDIT_RETENTION_DAYS` — required без дефолта шаблона: срок хранения аудита — требование проекта, и кит
  нарочно не даёт его по умолчанию (`RetentionConfig.Validate`).
- NCOD-01: 429 строгого лимита объявляет правило `authlimit.Rule` в таблице операции (канон `Declare`), а не хук
  OpenAPI в `cmd/api`: ответы huma строит из `op.Errors` до хуков, и только таблица снимает `default` у операций
  без доменных ошибок. Лимит, префикс и правило — в одном пакете платформы; совпадение префикса и объявлений
  держит `TestHTTPOperations`. 429 общего лимитера кита (`HTTP_RATE_LIMIT_*`) контракт не объявляет — свойство
  кита: его описание ошибок — в golibinfra (строкой отчёта для раздела других репозиториев).
- NCOD-01, один отказ — один код: адрес регистрации не в форме разбора отвергает схема (`pattern` без пробелов к
  `format:"email"`), сущность держит то же правило для `seed-admin`; запрос сброса принимает адрес мягко, как вход.
- T11, NADP-04: `TrustedErrors` — карта «метод → ошибки» (тест `Trusted` держит точность в обе стороны), а не
  строка в `port.md`: машина держит, что порт только для чтения не переводит ошибку записи.
- Правки чужих файлов: `scripts/new-domain.sh` (трек gate) — `maxLength` и комментарий предела имени скелета; rule
  `tests-pipeline` (трек gate) — строки `module/adapters.go` и `trusted.go` о `TrustedErrors` по методам.
- RCOD-10: своя ссылка и соседние гасятся одним оператором с `RETURNING id`; канон одиночного погашения в rule
  `sqlc-and-migrations` (трек gate) — `MarkEmailVerificationUsed`, строкой там же — погашение с соседями: правка
  чужого файла, иначе правило называло бы снятый метод.
- NSEC-02: список ключей из примеров — в `config`, а их сверку с файлами держит тест: форк, сменивший ключ в
  `.env.example`, получает красный тест, а не молча пропущенный гард.

## Открытые вопросы
- [ ] **Q1** — Пускать ли вход с неподтверждённым адресом (RCOD-08): решение продукта — блокирует: — — к кому: владелец
- [ ] **Q2** — `authgate.SecurityScheme` экспортирован ради теста (RCOD-14): зона «сначала спросить» — блокирует: — —
  к кому: владелец

## Где мы сейчас
S1–S8 закрыты. Следующий шаг — S9 (один способ пароля, смена пароля — у вызывающего).
