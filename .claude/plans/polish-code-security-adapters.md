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
- Дополнение дирижёра (16.10): после `git rebase main` с треком gate+tests спецификация мутаций под код трека —
  устаревшие якоря, `RA25`, блоки скелета `-02`, `-09`; `make mutate A=--strict` на них — код 0
  (`N/validation-s7-gate.md` § 6).
- Дополнение дирижёра (15.10), после S14: N4 валидации трека harness (`N/validation-s7-harness.md` § 3) — prune под
  код трека, `.template/identity` и `init-service/references/prune.md` (файлы трека harness — правка минимальная);
  остаток RHAR-10 (`R/template-harness.md`) — блоки «Верификация» четырёх references.

## Не входит
- NADP-03 (closure конверсией и алиасом), view и SQL-функция над чужой таблицей (часть RADP-05) и CTE с именем
  чужой таблицы (часть RADP-11) — правила archkit в golibinfra: строкой отчёта для раздела других репозиториев.
- RADP-10 и спецификация мутаций `scripts/tools/mutants.txt` — трек gate+tests; RCOD-16 — golibinfra.
- Решения продукта и зоны «сначала спросить»: вход с неподтверждённым адресом (RCOD-08), экспорт
  `authgate.SecurityScheme` ради теста (RCOD-14), `pkg/policy`, `go.mod` — вопросом в «Открытые вопросы».

## Критерии приёмки
- [x] **AC1** — NSEC-02: вне dev сервис не стартует с `JWT_SECRET` или `JWT_PREVIOUS_SECRET` из примеров окружения
  (`.env.example`, `test/stack`). Проверка: база — `config.Guard` с `APP_ENV=prod` и секретом из `.env.example` → nil
  (проба D05/D12); ветка — `ErrUnsafe`; `go test ./internal/platform/config/` → PASS
  - Подтверждено: проба AC1 (`N/s7c/exp/ac1-{base,branch}.log`): база — nil у обоих ключей обоих файлов,
    ветка — `ErrUnsafe`; D05 — старт с ключом примера: база стартует, ветка — отказ; D12 — токен на ключе примера → 401
- [x] **AC2** — Ссылки сброса: письма в один ящик с многих адресов — не чаще паузы своей настройки (NSEC-01); срок
  ссылки — не дольше часа от обработки, событие из будущего его не продлевает (NSEC-03); два параллельных
  подтверждения двух ссылок одного пользователя — 204 и отказ, без `40P01` (RCOD-10); ссылка, выданная до смены
  пароля, после неё не меняет пароль (RCOD-18, E10); отметка «отправлено» не задевает чужие ссылки (NTST-01).
  Проверка: на базе дефект, на ветке нет — `TestNSec_ResetMailRate`, `TestNSec_ForgedResetEvent`
  (`N/reaudit/tpl-security`), E4 и E10 (`N/reaudit/tpl-code`), мутация `… WHERE id = $1 OR TRUE` → тест репозитория FAIL
  - Подтверждено: NS1 — 60 писем в ящик за 2 мин 30 с (≈1440 в час) → одно; NS2 — срок ссылки от события из будущего
    721 ч → 1 ч; E4 — `40P01` → 204 и `ErrTokenUsed`; E10 — ссылка до смены пароля 204 → 400; мутация `OR TRUE` → FAIL
    `TestMarkPasswordResetSent_OwnLinkOnly`; занятые соседом выпуски — пауза без ожидания
    (`TestIdentity_ResetIssueBusyIsPause`)
- [x] **AC3** — `seed-admin` — операцией домена user: слабый пароль и не-адрес — код 1 без admin, повышение
  поднимает версию строки, пишет аудит и гасит выданные до него ссылки сброса (RSECT-08, RCOD-12). Проверка: пробы
  M01, M02, E9 на базе — дефект, на ветке — нет; `go test -tags=integration ./cmd/seed-admin/` → PASS
  - Подтверждено: M01 — пароль «1»: код 0 → 1, не-адрес — код 1 без admin; M02 — повышение заблокированной учётной
    записи поднимает версию (1 → 2); E9 — отказ без строки admin и аудита; `TestRun_*` `cmd/seed-admin` → PASS
- [x] **AC4** — `audit_log` со сроком хранения (`AUDIT_RETENTION_DAYS`, job кита), ответы `/api/*` — с
  `Cache-Control: no-store` (RSECT-05, 06). Проверка: проба D14 на ветке → `no-store`; `TestBoot` видит job retention
  - Подтверждено: D14 — `GET /api/users/me` и 401 `/api/users` → `Cache-Control: no-store`; `TestBoot` — job
    `auditkit.retention`
- [x] **AC5** — Контракт: у каждой операции `/api/*` под строгим лимитом в `openapi.json` — 429 `RATE_LIMITED`, у
  операций доменов нет `default`, схемы `Cookie` нет, один отказ — один код у адреса регистрации, `maxLength` у
  скелета, у `changePassword` — описание и гашение cookie, в `UserView` — роль и `email_verified`, `enum`/`format` у
  статуса, роли и id (NCOD-01, RCOD-08 частично, RCOD-09). Проверка: сводка `openapi.json` по операциям; E2 —
  пробельные формы адреса → один код
  - Подтверждено: сводка `openapi.json` (`N/s7c/exp/ac5-{base,branch}.txt`): 429 у всех `/api/auth/*`, `default` и схемы
    `Cookie` нет, `UserView` — `role` (`enum`), `email_verified`, `id` (`uuid`), тело `changePassword` без `user_id`,
    адрес регистрации — `pattern`, `maxLength`; E2 — пробельные и обрамлённые формы адреса → 422 `VALIDATION_FAILED`
- [x] **AC6** — Один способ на правило: пароль проверяет `auth` по хэшу порта, хеширует только `user`
  (RCOD-07); смена пароля — у вызывающего, без `user_id` и ветки admin (RCOD-14); мёртвое снято, мелочи
  единообразия сведены (RCOD-14, 18); образцы конфига домена и job'а — в коде (RCOD-17); комментариев длиннее
  двух строк в коде нет (RCOD-15). Проверка: AST-скрипт `N/reaudit/tpl-code/ast` на ветке → пусто; `rg` по
  символам; `make check` → ✓
  - Подтверждено: AST-скрипт — одна группа длиннее двух строк, `authgate/authn.go:9` (Q2); снятые символы по коду и прозе
    — только в плане; `make check` → ✓
- [x] **AC7** — Правила archtest без ложных отказов и с закрытыми обходами: законные `rafp01`, `rafp01d`, `rafp05`,
  `rafp04`, `fp10b` — зелёные (NADP-01, RADP-11); `ra01`, `ra04` — отказ (NADP-02, 04); порт только для чтения не
  переводит ошибку записи поставщика (T11); `n09f`, `n39`, `n56`, `n03f`, `n30`, `o-j+test`, `n33`, `n31`, `n27`,
  `n25`, `n26`, `n23`, `n19q`, `n21`, `n40`, `n41`, `v09g`, чтение до policy, subject `.v1` — отказ (RADP-05…08,
  T3, T6). Проверка: `python3 N/reaudit/tpl-adp/run.py <wt> --no-full <кейс>` на базе и ветке
  (`N/s7c/exp/{base,branch}.tsv`); законные формы прежнего списка — `✓ check clean`
  - Подтверждено: `N/s7c/exp/branch.tsv`: кейсы списка — отказ правилом (n19q, ra04 — вариантами `-br` под новый API), на
    базе зелёные (v09g, ra05 ловил лишь тест usecase); законные rafp01, rafp01d, rafp05, rafp04, fp10b — тишина (на базе
    rafp01, 01d, 05 и fp10b — ложный отказ); примеры валидатора (`N/val-s7c/rules-examples`) — 17 из 21 как задумано,
    4 — отказ по замыслу (`ProductName`, фейк порта в testkit, таблица переходов, subject конкатенацией)
- [x] **AC8** — RADP-13: у каждого правила файлов трека — свои примеры (нарушение — отказ с текстом, законная
  форма — тишина) в `internal/archtest/testdata/rules`; правило, которое замолчало, роняет тест примеров.
  Проверка: `go test ./internal/archtest/ -run TestRuleExamples` → PASS; ручная поломка правила → FAIL
  - Подтверждено: `TestRuleExamples` → PASS; каждое из 22 правил, онемев, роняет его
    (`N/s7c/exp/ac8-mute-rules-phaseB.tsv`)
- [x] **AC9** — T4: `TestIdentity` не зависит от скорости подписчиков: с задержкой подписчика сброса на базе — FAIL,
  на ветке — PASS. Проверка: инъекция задержки в `N/wt-s7c-t4` → база FAIL, ветка PASS
  - Подтверждено: подписчик сброса на 3 с медленнее (`N/s7c/exp/t4-README.txt`): база — FAIL «Condition never satisfied»,
    ветка — PASS
- [x] **AC10** — T6: образец ресурса с владельцем (видимость — параметр запроса, чужое — 404) в
  `docs/reference/architecture.md` и `scaffold/references/endpoint.md`; `CanGet` скелета без id. Проверка:
  ручная сверка; `make new-domain NAME=order` во временном worktree → `✓ check clean`
  - Подтверждено: образец — `architecture.md` и `endpoint.md`; `make new-domain NAME=order` во временном worktree на итоге
    ветки → `✓ check clean` (929/929), мутации скелета `order-01…24` убиты (`N/s7c/mutate-sk.log`)
- [x] **AC11** — Гейт и handoff: `make check` → `✓ check clean`; блоки INFRA-HANDOFF — в `.claude/plans/handoff.md`.
  Проверка: `flock N/gate.lock make check`; чтение накопителя
  - Подтверждено: `flock N/gate.lock make check` → `✓ check clean` на итоге ветки (`main` с треками harness+lifecycle и
    gate+tests); блоки S1, S3, S4, S6, S7, S9 — в `.claude/plans/handoff.md`
- [x] **AC12** — N4 (валидация трека harness): prune доменов идентичности по тексту `prune.md` на итоге ветки после
  `main` с треком harness+lifecycle — `✓ check clean`: `0007` в `.template/identity`, шаги 2, 3, 6 называют
  `authlimit`, retention аудита и `PASSWORD_RESET_MAIL_PAUSE`. Проверка: опыт AC2 трека harness (`N/s7h/`)
  на итоге ветки
  - Подтверждено: свежий форк на итоге ветки (`N/s7c/prune/shop6`: онбординг external, `product`, prune по тексту
    `prune.md`) → `✓ check clean` (140/140), `make mutate A=--check` — 82 якоря OK; на `shop5` без правки
    `TestMigrationsCheck` — красный (тест читал миграцию идентичности `0003`)
- [x] **AC13** — RHAR-10 (остаток): блоки «Верификация» `scaffold/references/{endpoint,migration,port,event}.md` — без
  пересказа того, что держат гейт и правила. Проверка: ручная сверка; `make check` → ✓
  - Подтверждено: ручная сверка четырёх references — шаг «Готово» без пересказа гейта; `make check` → ✓

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
    `.env.example` — 30); middleware `noStore` в `cmd/api/http.go`; `TestBoot` видит job `auditkit.retention` и
    `no-store` у операций под Security, job `verification.purge_links` — `TestIdentity` (S15), `TestNoStore`
    (`cmd/api/http_test.go`) — `no-store` у ответа, `TestBoot_AuthRateLimit` — и у 429 лимита; handoff-блок;
    `make check` → ✓
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
- [x] **S9** — RCOD-07, RCOD-14: пароль хеширует `user.Trusted.HashPassword`, проверяет `auth`; смена пароля —
  у вызывающего
  - Проверка: `rg` по снятым символам; `make check` → ✓ · Закрывает: AC6 · Зависит от: S7, S8
  - Сделано: `user.Trusted.HashPassword` (правило пароля и хэшер владельца учётной записи, строка `TrustedErrors` —
    `ErrWeakPassword`, `ErrPasswordTooLong`) вместо `VerifyPassword`; `auth` сверяет пароль своим хэшером по хэшу
    порта (`passwordMatches` — вход и смена); `PasswordSetter` обоих потребителей — `HashPassword` и
    `UpdatePassword`, у `verification` хэшера нет; `ChangePassword(ctx, old, new)` — пароль вызывающего, policy
    `auth` и порт `PasswordVerifier` сняты, тело операции без `user_id`; e2e-харнес — без `user_id`, `UserView` — с
    `role` и `email_verified`; `rg 'VerifyPassword|PasswordVerifier|CanChangePassword'` по коду и прозе → пусто
    (кроме `scripts/tools/mutants.txt`); handoff-блок; `make check` → ✓
- [x] **S10** — RCOD-18 и RCOD-14: входы usecase значением, имена DTO, логи успеха, тип статуса во входе HTTP,
  порядок и проекция списка, мёртвый `StatusUnknown`
  - Проверка: `make check` → ✓ · Закрывает: AC6 · Зависит от: S9
  - Сделано: `user.Register(ctx, RegisterInput)` и `auth.Login(ctx, LoginInput)` — значением; `EnsureAdmin` — свой
    `EnsureAdminInput`; итог `Login` и `Refresh` — `auth.Session`; логи успеха `ConfirmEmail` и `Logout`; статус во
    входе правки — `*string`, тип домена — в хендлере; `ListUsers` — `ORDER BY created_at DESC, id DESC`, без
    `password_hash`; `StatusUnknown` и роль по умолчанию у `NewUser` сняты (`RoleUnknown` — `ErrInvalidRole`).
    RED на базе: `TestList` (строка списка с хэшем) и `TestList_SameCreatedAt` (страницы при равном `created_at` —
    не по id) — FAIL (`N/s7c/exp/s10-list-base.log`), на ветке PASS; формулировка `event.md` о подписчике своего
    события — уже на базе; канон `architecture.md`, `field.md`; `make check` → ✓
- [x] **S11** — RADP-13: примеры правил archtest (`testdata/rules`); NADP-01, NADP-02, RADP-11, T3 — правила и
  примеры
  - Проверка: AC8; кейсы AC7 этих находок; `make check` → ✓ · Закрывает: AC7, AC8 · Зависит от: S8
  - Сделано: правила `archtest`, `crossdomain`, `delivery` — функции над деревом (`check`: репо или пример), тест
    правила зовёт его по имени (`runRule`, реестр `rules`); `TestRuleExamples` гоняет `testdata/rules/*.txtar`
    (свой разбор txtar, `base`/`drop` поверх канона) и требует у каждого из 16 правил нарушение и законную форму;
    NADP-01 — сервисы корня от зависимостей (`fx.In`, конструкторы `Usecase`/`Trusted`), моки входа не читаются
    (примеры `delivery-root-sum-type`, `delivery-entity-interface-field`, `delivery-root-sink` — тишина);
    NADP-02 — событие поставщика этого репо, записанное рукой в контракт-тесте, — только в функции с «снять в
    релизе» (`contract-hand-payload` — отказ, `contract-previous-version`, `contract-other-service` — тишина);
    RADP-11 — интерфейсы и типы-функции входа без префикса; T3 — `TestSubjectVersions`; проза: rule
    `events-and-messaging`, `event.md`, `architecture.md`; `make check` → ✓
- [x] **S12** — Новые правила archtest (RADP-05…08, `identity.With`, чтение до policy) с примерами; T6 — образец и
  скелет; RADP-12 — проза
  - Проверка: кейсы AC7; AC10; `make check` → ✓ · Закрывает: AC7, AC8, AC10 · Зависит от: S11
  - Сделано: правила `TestDomainPackages` (подпакеты домена, `infra` ↛ вход, SQL — `infra/postgres`, `natskit` —
    `delivery/nats`), `TestModulesOnlyWire` (`module.go` — только `Module`, адаптеры поверх `Trusted`, без хуков fx,
    литерал — один `return`), `TestDomainDependencies` (порт привязывает свой домен, граф доменов ацикличен),
    `TestDomainDeterminism` (`clockkit.Real`, `crypto/rand`, uuid, переменные корня), `TestRootTypes` (теги
    транспорта в корне, PII в payload), `TestIdentityWithOnlyInAuthgate`; `TestSecuredOperationsAskPolicy` — порядок
    «policy до вызова порта»; у каждого — примеры (60 примеров и канон); T6 — образец «автор или админ» в
    `architecture.md` и `endpoint.md`, `CanGet` скелета без id (`make new-domain NAME=order` → archtest и тесты
    среза зелёные); RADP-12 — `event.md` § Когда НЕ применять, пометки archtest в rule; `make check` → ✓
- [x] **S13** — RCOD-15: комментарии кода — не длиннее двух строк, только WHY
  - Проверка: AST-скрипт → пусто; `make check` → ✓ · Закрывает: AC6 · Зависит от: S12
  - Сделано: десять групп длиннее двух строк сокращены до WHY; WHAT-комментарии outbox сняты, английские
    комментарии `events.go` и `auth/entity.go` — по-русски, как образец `event.md`; rule `comments` — «одна строка,
    редко две». AST-скрипт `N/reaudit/tpl-code/ast/comments`: было 11 групп ≥ 3 строк, стало одна —
    `internal/platform/authgate/authn.go:9` (зона «сначала спросить», не тронута; `N/s7c/exp/s13-comments-*.txt`);
    `make check` → ✓
- [x] **S14** — Приёмка: эксперименты на базе и ветке, гейт, handoff
  - Проверка: `N/s7c/exp/{base,branch}.tsv`; `make check` → `✓ check clean` · Зависит от: S16, S17
  - Закрывает: AC1, AC2, AC3, AC4, AC5, AC6, AC7, AC8, AC9, AC10, AC11
  - Сделано: пробы аудиторов на базе `a4020ee` и ветке (`N/s7c/exp/`: `base.tsv`, `branch.tsv`,
    `compare-base-branch.tsv`; пробы под новый API — `sec-branch-adapted.log`) — дефекты AC1–AC5 на базе есть, на
    ветке нет; кейсы приёмки нашли n32, fp05, fp11, валидация — B1, B2, N1–N14 (S17), все закрыты. Мутации
    (`make mutate A=--strict`, `N/s7c/mutate-*.log`): 47 переписанных блоков, 16 RA, новые блоки трека и скелет
    `order-01…24` — убиты; ветка — на `main` с треками harness+lifecycle и gate+tests; `make check` → ✓
- [x] **S15** — N4: `git rebase main` (трек harness+lifecycle), `0007` в `.template/identity`, шаги 2, 3, 6
  `init-service/references/prune.md` под код трека; prune по тексту на итоге
  - Проверка: опыт AC2 трека harness → `✓ check clean`; `make check` → ✓ · Закрывает: AC12 · Зависит от: S13
  - Сделано: ветка перенесена на `main` (конфликт — строка «Чек» rule `comments`); в `.template/identity` —
    `0007_password_resets_user_version` и `internal/platform/authlimit` (только ради `/api/auth/*`), без
    `cmd/api/http_test.go` (тест `noStore` — не про идентичность); `TestBoot` без знания доменов: `no-store` — у
    каждой операции под Security, job `verification.purge_links` — в `TestIdentity`, 429 лимита с `no-store` — в
    `TestBoot_AuthRateLimit`, `call` — рядом с её пользователями; `prune.md` шаги 2, 3, 6 — `authlimit`, поля
    `App.AuthRateLimit` и `App.PasswordResetMailPause`, retention аудита, `PASSWORD_RESET_MAIL_PAUSE` и
    `AUDIT_RETENTION_DAYS`. Опыт AC2 трека harness (`N/s7c/prune/`: форк, онбординг external, `make new-domain
    NAME=product`, prune по тексту `prune.md` — `prune.py` по образцу трека): `shop0` (ветка до правки) — `0007`
    остаётся, шаг 2 называет `authRateLimit`; `shop1` — lint `call is unused` (исправлено); `shop2` — prune прошёл,
    красный только `TestRuleExamples`: примеры правил написаны с module path шаблона, а de-template его меняет —
    в архивах `{module}`, его подставляет загрузка примера (`fix(archtest)` следом); `shop3` — `✓ check clean`;
    `make check` → ✓
- [x] **S16** — RHAR-10 (остаток): блоки «Верификация» четырёх references — коротко, без пересказа гейта и правил
  - Проверка: ручная сверка; `make check` → ✓ · Закрывает: AC13 · Зависит от: S15
  - Сделано: `endpoint.md`, `port.md`, `event.md` — шаг «Готово» одной строкой: `make gen` (что он подхватит), затем
    `make check` (CLAUDE.md § Готово); `migration.md` — шаги «Проверка компиляции» и «gen-check» сняты (их держит
    гейт), «Тесты» — когда нужен тест и как integration-тест берёт миграцию, без команд; `make check` → ✓
- [x] **S17** — Валидация трека (`N/validation-s7-code.md`): ложные отказы и обходы новых правил archtest (B1, B2) и
  неблокирующие N1–N14 — по коммиту на пункт
  - Проверка: примеры правил в `testdata/rules` и пробы валидатора (`N/val-s7c/`) — RED → GREEN; `make check` → ✓ ·
    Закрывает: AC7, AC8 · Зависит от: S16
  - Сделано: B1 `TestRootTypes` (слово имени поля, payload по `Publish`, теги fx), B2 `adapters.go` видит то же, что
    корень, N1 `releasedSubjects`, N2 выпуск ссылки без ожидания блокировки, N3–N6 обходы и ложные отказы правил, N7
    оговорка rule `comments`, N8 комментарий `0007` и `SECURITY.md`, N9 текст плана, N10 Breaking в «Handoff», N11
    чистка ссылок батчами, N14 шаги 4, 5, 7 prune; N12, N13 — решением (Q3). RED → GREEN — примером или тестом
    (`N/s7c/exp/*-red.log`); примеры валидатора — 17 из 21 как задумано, 4 — отказ по замыслу; `make check` → ✓

## Решения
- RCOD-18 (ссылка сброса переживает смену пароля): ссылка хранит версию учётной записи на момент письма
  (`password_resets.user_version`), подтверждение пишет пароль при ней — изменённая после письма учётная запись
  ссылку гасит; отзыв ссылок из `auth` потребовал бы порта `auth → verification` и транзакции через три домена.
- NSEC-01: пауза писем — условием выпуска ссылки в подписчике (другая ссылка учётной записи моложе паузы, часы БД),
  а не лимитом запроса: письма в ящик ограничены с любых адресов, настройка не делит блокировку входа, запрос
  делает одну работу для любого адреса без ветки «лимит», и адреса анонимных запросов не копятся в
  `bruteforce_attempts`. Лимит запроса на guard входа снят: поток запросов с адреса держит лимит `/api/auth/*`.
- NSEC-01, выпуски пользователя: транзакционная advisory-блокировка по пользователю, а не строка-слот на учётную
  запись: без новой таблицы и миграции, письмо уходит после коммита — блокировка не держится на вводе-выводе mailer'а.
  Блокировка не ждёт (`pg_try_advisory_xact_lock`, N2 валидации): занятые соседом выпуски — та же пауза, а очередь
  потока событий в один ящик держала бы соединения пула и задерживала письма других; без паузы (0) блокировки нет.
- N3 (валидация, `TestIdentityWithOnlyInAuthgate`): `With` без селектора — в файлах самого пакета `identity` и под
  dot-импортом — тот же вызов; имена объявлений, полей и селекторов (`log.With`) — нет. Отказ называет выход служебной
  работы: метод usecase без policy или `Trusted`, а не помощник с личностью.
- N4 (валидация, `TestDomainDeterminism`): значение структуры в переменной корня — только типа корня или `pkg/`:
  литерал `sync` и `sync/atomic` (и любого внешнего пакета) — состояние, которое пишут его методы. Тип корня, внутри
  которого счётчик, правило по тексту не отличит — предел, его держит ревью.
- N5 (валидация, «policy до чтения»): вызов порта с ключом вызывающего в аргументах — чтение своего, а не чтение до
  решения: квота по своим записям спрашивает policy после него, чужого такой вызов не выдаёт (строка
  `architecture.md` с образцом). Ключ — та же метка, что у `Me`: `caller.UserID` из `identity.From` и его переменные.
- N6 (валидация, мелкие ложные отказы): моки адаптера `infra/<адаптер>/mocks` (dir mockery по умолчанию) и адаптеры
  `module/` по файлу на поставщика (`adapters_<поставщик>.go`) — законны; фейк порта в `internal/testkit` с типом
  порта — по-прежнему отказ (привязка мимо домена), а отказ называет выход: фейк отдаёт свой конкретный тип.
- N7 (валидация, rule `comments` и archtest): комментарии тестов-правил и пакета `internal/archtest` — оговоркой в
  правиле, а не сокращением: это текст правила для того, кто получил отказ (что держит, почему, где выход), и
  CLAUDE.md называет перечень правил комментарием пакета. Прод-код — одна группа длиннее двух строк (`authn.go:9`,
  Q2). Правка чужого файла: rule `comments` — абзац исключения.
- N8 (валидация, миграция `0007`): комментарий называет то, что гасит ссылку, — любую запись учётной записи с ростом
  версии (имя, статус, роль, пароль, удаление); следствие для пользователя (переименовался — новая ссылка после паузы)
  — компромиссом в `SECURITY.md`. Миграция не выпущена — правка комментария до релиза.
- N9 (валидация, текст плана): «Сделано» S6 называет тесты по именам кода (`TestNoStore`, `TestBoot_AuthRateLimit`,
  job чистки ссылок — в `TestIdentity` после S15); «Сделано» прочих шагов — история шага, его не переписываю.
- N10 (валидация, Breaking для форков): перечень — разделом «Handoff» плана, его строки дирижёр переносит в
  аннотацию `template/v1.2.0`; INFRA-HANDOFF окружения и миграции — прежние блоки накопителя, не копии.
- N11 (валидация, ёмкость `verification.purge_links`): job удаляет батчами до пустого в пределах своего `Timeout`, а
  не один батч в час: порог «устойчивый выпуск выше батча — хвост растёт» снят; сбой посреди прогона — ошибка с
  числом удалённого (её логирует timerkit), следующий прогон продолжит.
- N14 (валидация, prune): блоки мутаций, чьи строки сняли шаги 2–3, снимает второй фильтр шага 4 по выводу
  `make mutate A=--check`, а не правка руками: на форке валидатора (`vshop1`) — `M04+M05`, `K07`, `SR1`, после фильтра —
  `OK`. Правка файла трека harness — минимальная: команда шага 4. На итоге ветки после `main` с треком gate+tests
  prune по тексту нашёл ещё две вещи: шаг 5 переименовывает миграции своих доменов, и блок мутации миграции скелета
  (`<домен>-24`) ссылался бы на старый путь — шаг 5 переименовывает и пути в `mutants.txt`; шаг 7 называл запись
  `archkit.WithUncheckedPackages(…)`, а после B2 это список `uncheckedPackages`.
- AC12, `TestMigrationsCheck` (файл трека gate+tests, по поручению дирижёра — трек слит): случай «выпущенная миграция
  под другим номером» брал содержимое `migrations/0003_email_verifications_idempotency.up.sql` из дерева, а prune у
  форка с внешней идентичностью этот файл снимает — гейт форка краснел. Содержимое — строкой в тесте (его хэш — в
  `released` скрипта), тест от миграций доменов идентичности не зависит. Правка минимальная: один случай теста.
- N12 (валидация, тайминг запроса сброса): кода не меняю — путь запроса у известного и заблокированного один (поиск и
  событие), медианы классов равны, отклонение заблокированного (z 3,04 → 2,32 → −0,38) не воспроизводится; снятый
  лимитер убрал запись на каждый запрос, и шум хранилища под нагрузкой стал заметнее. Переаудиту — перемерить на
  спокойной машине.
- N13 (валидация, вне трека): пароль из пробелов проходит правило продукта `pkg/policy.PasswordStrong` (только
  длина, как у регистрации) — зона «сначала спросить», вопрос владельцу Q3; текст ошибки mailer'а форка с адресом и
  токеном в логе natskit — ответственность адаптера mailer'а форка, маскирование логов (`LOG_REDACT_*`) у кита есть.
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
- RCOD-07: пароль хеширует только `user` (`Trusted.HashPassword` — правило пароля продукта и хэшер, ими же
  `Register` и `EnsureAdmin`), сверяет только `auth` (`passwordMatches` по хэшу из `UserReader` — вход и смена), а не
  `Trusted.VerifyPassword`: проверка через порт читала учётную запись второй раз и второй раз переводила предел
  хэшера.
- RCOD-14: смена пароля — только своего: policy и так пускала одного вызывающего (админ чужой пароль не меняет), и
  `user_id` в теле был вторым способом назвать себя; policy `auth` снята вместе с портом.
- Правки чужих файлов, S9: e2e-харнес (трек gate) — тело смены пароля без `user_id` (`test/smoke/test_auth.py`,
  `test/client/models.py`) и `UserView` с `role` и `email_verified` (S7): модели e2e не пускают лишних полей;
  `.mockery.yml` — строки снятых портов `ResetLimiter` (S4) и `PasswordVerifier`.
- RCOD-18, входы и имена: вход usecase — значением (`<Op>Input`), у `EnsureAdmin` — свой `EnsureAdminInput`, а не
  `RegisterInput` другой операции; итог `Login` и `Refresh` — один описательный `auth.Session`, а не копия
  `RefreshOutput`; поле HTTP-DTO — тип провода (`*string` статуса, как `string` фильтра списка), тип домена — в
  хендлере.
- RCOD-18, список: проекция без `password_hash` со своим типом строки — забытую в ней колонку сборка не видит, её
  ловит `TestList` (строки целиком; `field.md` шаги 3, 4, 7); второй ключ порядка — `id`: страницы не плывут на
  равном `created_at`.
- RCOD-18, копии `uc.audit` и преамбулы identity → policy — явные в каждом домене и методе: преамбула — три строки
  канона, вызов policy archtest читает в методе операции, а общий помощник аудита — примитив кита (`auditkit`),
  не шаблона.
- RADP-13, примеры правил: канон (дерево, на котором молчит каждое правило) и правка канона (`base: canon`), а не
  копия репо: пример не зависит от доменов идентичности, которые форк снимает prune'ом, и от кода форка. Пример
  нарушения называет одно правило и все его ожидаемые отказы — лишний отказ тоже роняет пример; законная форма —
  тишина. Правила archkit (`TestLayout`) держит кит своими мутантами, правила других файлов archtest — их треки.
- NADP-01: сервисы корня — зависимости (`fx.In`, параметры конструкторов `Usecase` и `Trusted`), а не любой
  интерфейс: интерфейс корня вне зависимостей — данные (sum-type, приёмник); моки входа правило не читает.
- NADP-02: событие, записанное рукой, — литерал `outboxtest.Event` (и элемент `[]outboxtest.Event{…}`); пометка —
  комментарий функции сценария «снять в релизе …»: сценарий прежней версии снимают целиком. Subject, который не
  вычислить по тексту (не литерал и не константа пакета подписки), правило не судит — предел, его держит ревью.
- T3: `.v1` у subject домена этого репо — отказ (`TestSubjectVersions`); subject другого сервиса называет его
  контракт, правило его не трогает.
- RADP-05…08, новые правила: по одному тесту на тему (пакеты домена, `module/`, зависимости доменов, детерминизм,
  типы корня, личность в ctx), а не тест на находку: отказ называет правило, которое форк ищет в прозе. Переменная
  корня — перечень неизменяемых форм (sentinel, `TrustedErrors`, `outcome.New`, `regexp.MustCompile`, `var _`), а не
  поиск записи в неё: мутацию через метод (`atomic.Int64.Add`) по тексту не отличить от чтения.
- RADP-06, порт привязывает свой домен — весь домен, а не только `module/`: конструктор `infra/` возвращает порт
  (`NewRepository`); граф доменов — по импортам корня чужого домена из `module/`.
- T6, порядок policy: первый вызов `Can…` (сам или своим методом) раньше первого вызова порта зависимостей; чтение
  внутри своего метода до policy правило не видит — предел, держит ревью и канон теста usecase на строгом моке.
- Правка чужого файла, S12: `scripts/new-domain.sh` (трек gate) — `CanGet` скелета без id и строка комментария к
  нему (T6).
- RCOD-15: предел — две строки, и rule `comments` говорит то же («одна строка, редко две»): образец и правило
  сходятся; машиной длину не держу — это стиль, его держат правило и ревью. Комментарий `authgate/authn.go`
  (три строки) не тронут: зона «сначала спросить». Правка чужого файла: rule `comments` — строка «Чек».
- N4: тесты сервиса (`TestBoot`) — без знания доменов, как обещает их комментарий: проверки доменов идентичности —
  в их файлах, которые prune снимает целиком; бандл `.template/identity` — только то, что живёт ради них
  (`internal/platform/authlimit` — да, `cmd/api/http_test.go` с тестом `noStore` — нет). Правки файлов трека harness:
  `.template/identity`, `init-service/references/prune.md` (шаги 2, 3, 6).
- Примеры правил — с `{module}` вместо module path шаблона: правила сверяют импорты с константой `module`, которую
  de-template меняет у форка, а архивы txtar — не Go, и его замена их не трогает.
- RHAR-10: блок проверки reference — ссылка на CLAUDE.md § Готово и `make gen` там, где он нужен; команды, которые
  уже гоняет `make check`, не пересказываются: четыре разных набора команд перед общим «Готово» расходились бы.
- RADP-06, n32 (кейс приёмки S14): `adapters.go` знает лишь stdlib и корни доменов — обёртка кита в нём оставалась
  зелёной; правило — в `TestModulesOnlyWire`, пример `module-adapter-over-kit`.
- Ложные отказы новых правил, найденные кейсами приёмки (S14): `fp05` (значение структуры `T{…}` в переменной корня)
  и `fp11` (конструктор job'а в `module.go`) — законные формы прежнего списка. Переменная корня — и значение структуры,
  если функции корня её не пишут; `module.go` не объявляет типов и конструкторов адаптера, а его функции — один
  `return`: правило держит n33, n31, n40, а не любую функцию.
- `scripts/tools/mutants.txt` и блоки мутаций скелета в `scripts/new-domain.sh` (файлы трека gate+tests; по поручению
  дирижёра): блоки, чьи якоря сдвинул код трека (56 прежних и 17 RA), переписаны под код или сняты, если цели нет
  (policy `auth`, лимитер сброса — и `RA42b`, `RA43b`, ветки пустого cookie, роль по умолчанию, отзыв ссылок
  отдельно от погашения); у `RA25` снята строка «=» — мутацию убивает тест NTST-01; `-02`, `-09` скелета — под
  `CanGet` без id. Правка минимальная: формат и прочие блоки не тронуты.
- Перенос на `main` с треком gate+tests — до конца круга валидации, а не после: хуки git общие для worktree (путь
  хуков — основного дерева), и pre-commit `main` зовёт `make vet`, которого у ветки до переноса нет. Правило
  `TestMainOnlyEnters` трека: логгер бинарей `migrate` и `seed-admin` — функция `newLog` с тестом (cover-check судит
  и `cmd/`); строки `generated_test.go` в комментарии пакета archtest, потерянные при сведении конфликта, — на месте.
- SD6 (выжившая мутация): `cmd/seed-admin` проверяет `SEED_ADMIN_EMAIL` и `SEED_ADMIN_PASSWORD` до подключения к БД —
  не второй способ правила (адрес и пароль судит `EnsureAdmin`), а сообщение оператору, какой переменной нет: тест
  держит его текст.
- B1 и B3 (валидация, `TestRootTypes`): персональные данные — слово PII в любом месте имени (CamelCase и `_`, целиком:
  `Membership`, `Zip` — не PII) и слитные формы списком (`Username`, `Surname`, `EMail`); факт — по типу поля (`bool`,
  числа, `time.Time`, `time.Duration`: `EmailVerified bool`, `TokenCount int`), `…ID` последним словом — ID. Только
  последнее слово (B1) пропускало `SecretKey`, `TokenValue`, `PasswordPlain`, `Username` — гейт проверял меньше; пример
  `root-event-secrets` держит секретную половину правила. Строковый «факт» со словом PII (`EmailVerified string`) —
  отказ: факт в каноне типизирован. Payload — тип литерала в `….Publish` корня и типы `events*.go`; теги fx (`name`,
  `group`, `optional`) у структуры с `fx.In` — способ fx. `ProductName` — отказ по замыслу: имя берут по ID.
- B1′ (перепроверка): payload — и литерал в переменной, которую функция корня отдаёт `Publish` (`ev := T{…}`), и типы
  корня в полях payload (под указателем, срезом, map) — транзитивно. Предел — payload, собранный не литералом (вызов
  конструктора, параметр функции): по тексту тип не вычислить, держит ревью.
- B2 (валидация, `TestModulesOnlyWire`): `adapters.go` видит то же, что корень домена, — stdlib без I/O, `rootLibs`,
  корни доменов, пакеты `pkg/` и `internal/platform/` без I/O в импортах: порт несёт `paging.Page`, а клиент с I/O в
  `pkg/` — отказ. Перечни `rootLibs` и `uncheckedPackages` — переменные файла настроек сервиса, их берут и `layout`, и
  правило: один список на «что видит корень». Правка чужого файла: skill `upgrade` — перечень настроек.
- N1 (валидация, `TestSubjectVersions`): `.v1`, выпущенный до канона, — строка `releasedSubjects` файла настроек
  сервиса (его перенос шаблона не трогает), а не пометка в коде: перечень виден ревью, а строка без такого subject —
  отказ, как запись `WithKnownViolations`. Строка `event.md` одна не дала бы выхода: гейт форка оставался бы красным.

## Открытые вопросы
- [ ] **Q1** — Пускать ли вход с неподтверждённым адресом (RCOD-08): решение продукта — блокирует: — — к кому: владелец
- [ ] **Q2** — `authgate.SecurityScheme` экспортирован ради теста (RCOD-14): зона «сначала спросить» — блокирует: — —
  к кому: владелец
- [ ] **Q3** — Пароль из пробелов проходит `PasswordStrong` (только длина; N13 валидации): решение продукта, `pkg/policy`
  — блокирует: — — к кому: владелец

## Handoff
- Аннотация `template/v1.2.0` (дирижёр), Breaking для форков:
  - код доменов идентичности, который зовут домены форка: `user.TrustedErrors` — карта «метод → ошибки» (тест
    адаптера берёт `TrustedErrors[метод]`, порт только для чтения не переводит ошибок записи); `Trusted.VerifyPassword`
    → `HashPassword`, порт `auth.PasswordVerifier` и policy `auth` сняты; `user.CreateInput` → `RegisterInput`,
    `auth.LoginOutput` → `Session`, входы `Register`/`Login` — значением; `ChangePassword(ctx, old, new)`;
  - API: тело смены пароля без `user_id` (лишнее поле — 422), в `UserView` — `role` и `email_verified`;
  - гейт: новые правила archtest (S11, S12, валидация S17) судят и домены форка; `internal/archtest/
    layout_settings_test.go` форка — новые настройки `rootLibs`, `uncheckedPackages` и `releasedSubjects`: без них
    archtest не собирается, свести по образцу шаблона (skill `upgrade`);
  - `TestSubjectVersions`: subject `.v1` своего домена — отказ; уже выпущенный — строкой `releasedSubjects`, новый —
    без суффикса (`event.md` § Versioning);
  - окружение: `AUDIT_RETENTION_DAYS` — required (без неё сервис не стартует), новые `PASSWORD_RESET_MAIL_PAUSE` и
    `HTTP_AUTH_RATE_LIMIT_*` со значениями по умолчанию; миграция `0007` — ссылки сброса до неё пароль не меняют.
    INFRA-HANDOFF этих пунктов — блоки S1, S3, S4, S6, S7, S9 в `.claude/plans/handoff.md`.

## Где мы сейчас
Шаги S1–S17 и критерии AC1–AC13 закрыты; ветка — на `main` `566a3ce`, гейт ✓, prune по тексту на итоге — ✓.
Перепроверка валидатора: B3, B1′ закрыты; дальше N1′, N2′, N3′, N14′ по коммиту. Ждут: ревью дирижёра (строки
`Review: … — ожидает`) и ответы владельца Q1–Q3.
