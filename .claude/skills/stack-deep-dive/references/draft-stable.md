# Контракт `draft → stable`

Артефакт становится `stable`, когда **все 6 пунктов одновременно** ✅.
Скилл проверяет, архитектор подтверждает, скилл применяет правку `status:`.

## Чек-лист

### 1. Обязательные H2 заполнены

См. `references/field-checklists.md` — по каждому kind перечислены
обязательные H2 и какой placeholder в шаблоне они несут. К stable —
непустое осмысленное содержимое, без placeholder'ов вида `<…>` и
заглушек (`TBD…`, `_TBD_`, `TODO…`, `FIXME`, прочерк, многоточие, `???`,
таблица без строк или из одних `TBD`).

### 2. Секция `Generates` непуста и конкретна

**Главный пункт для стека.** `Generates` — это контракт для
`de-template`: что из артефакта родится в коде (пути в репо, имена
файлов, имена структур, имена env). Пустая / `TBD` `Generates` =
артефакт бесполезен движку.

Что считается конкретным:
- пути в коде: `internal/platform/<x>`, `cmd/<bin>`, `<domain>/delivery/<y>`;
- имена env: `PG_DSN`, `JWT_SECRET`, `LOG_LEVEL`;
- имена fx-модулей / kit-функций: `pgkit.Module`, `outboxkit.Instance`,
  `events.OutboxFor`;
- subjects: `user.events.registered`;
- ldflags-флаги: `-X main.version`.

Что НЕ считается:
- «соответствующий код в platform/» (без пути);
- «нужный kit golibinfra» (без имени);
- «env, который читает kit» (без имени env).

### 3. Cross-kind sync согласован

Решения kit-артефактов (`auth`/`persistence`/`messaging`/`observability`/
`dependencies`) роллапнуты в `config` и `runtime`. См. SKILL.md §
Cross-kind sync — там полная матрица.

Программная проверка:
- каждый env, упомянутый в `Generates` или в теле kit-артефакта, есть
  в `config.md § Required env` или `§ Optional env`;
- каждый секрет — в `config.md § Secrets` (имя env; значение не коммитим);
- каждый subscriber/job/cleanup-job, объявленный в kit-артефакте, есть
  в `runtime.md § Entrypoints` или `§ Lifecycle`;
- каждая внешняя runtime-dep (Postgres/NATS/Redis/S3/…), упомянутая в
  kit-артефакте, есть в `dependencies.md § External`.

Если drift — скилл предлагает автодополнение в `config`/`runtime`
через `AskUserQuestion`, не маркирует stable до согласования.

### 4. Валидатор зелёный

```bash
make validate-stack-docs
```

Проверяет:
- frontmatter (`id`, `kind`, `name`, `status` обязательны;
  `kind ∈ 8 stack-kinds`);
- `basename == name`; `id == "stack.<name>"`;
- все обязательные H2 для kind присутствуют;
- при `status: stable` — в каждой секции решение: плейсхолдер шаблона
  `<…>` (и многострочный), заглушка — строка, пункт, чекбокс или цитата из
  `TBD`, `TODO` или `FIXME` словом (с хвостом или без; `Todoist`,
  `TODO-лист` — слова), прочерка, многоточия или вопросов, — подзаголовок и
  таблица без строк или из строк одних заглушек не в счёт; у `auth` —
  `identity: own | external`.

Конкретность решения («соответствующий код в platform/» — не путь) валидатор
не видит — её держит пункт 2.

Если красный — вернись к проблемным секциям, не маркируй stable.

### 5. Тест-критерий kind'а явно подтверждён

В этой сессии. Скилл предъявляет через `AskUserQuestion` примеры
✅/❌, архитектор соотносит и выбирает «соответствует ✅» / «нет, ближе к ❌»
(через Other).

См. **§ Тест-критерий по kind** ниже.

### 6. State чист по `**blocker**`-вопросам

`state/<kind>.md`:
- не содержит секцию `## Открытые вопросы` ИЛИ
- содержит, но в ней нет вопросов с `**blocker**`.

Не-blocker открытые вопросы при stable допустимы — это «хорошо бы
узнать», не «здесь смысл сломан».

## Тест-критерий по kind

Скилл готовит ✅/❌-варианты под конкретный артефакт. Шаблон ниже.

### service

> Идентичность сервиса полная: имя, module path (`git.stit.tech/<org>/<svc>`),
> схема версионирования, какие бинари собираются. На старте логируется
> component+version+commit?

- ✅ `name=orders`, `module=git.stit.tech/foo/orders`, semver через
  ldflags `-X main.version/-X main.commit`, бинари: `cmd/api` +
  `cmd/migrate`, лог `"starting" component version commit`.
- ❌ `name=service`, `module=git.stit.tech/stit-core/go-backend-template`
  (не сменили), version=`dev` без ldflags.

### runtime

> Один бинарь `cmd/api` поднимает все нужные entrypoints через одну
> fx.Lifecycle; graceful shutdown описан; jobs перечислены с интервалами?

- ✅ `cmd/api` поднимает HTTP (chi+huma) + outbox-drain + 2 NATS-subs
  + 1 cleanup-job; остановка — в бюджете `fx.StopTimeout`: за
  балансировщиком пауза `HTTP_SHUTDOWN_DRAIN_DELAY=5s`, дообработка и
  дренаж NATS — в своих долях; порядок хуков — китов (HTTP первым).
- ❌ `Entrypoints: HTTP server`, остальное «и ещё что-то» — outbox/jobs
  упущены; или свой порядок закрытия, которого шаблон не делает.

### config

> Required env перечислены полностью (старт падает без
> них); Optional env с дефолтами по подключённым kit'ам; секреты —
> отдельной секцией (имя env, значение не коммитим); release-checklist для
> добавления новых env есть?

- ✅ `Required: APP_ENV, PG_DSN, JWT_SECRET`;
  `Optional: HTTP_ADDR (:8080), LOG_LEVEL (info), PG_MAX_CONNS (10), …`;
  `Secrets: JWT_SECRET, пароль в PG_DSN (env, значение не коммитим)`; release-checklist.
- ❌ «Берётся envconfig автоматически» — без перечисления.

### persistence

> Engine + driver + migrations + kit-таблицы + пул описаны конкретно?
> Миграции kit'ов (outbox/audit/bruteforce) не дублируются вручную, а
> подключаются как `pgkit.Source` китов (`pgstore.Migrations` / `TableMigrations`)?

- ✅ Postgres 16, pgx/v5, golang-migrate up/down парные, `cmd/migrate`
  как init-контейнер, `outbox_<domain>`/`audit_log` миграции наследуются из
  golibinfra (`pgstore.Migrations` / `TableMigrations`), пул `PG_MAX_CONNS=50` с метрикой `pg_pool_*`.
- ❌ «Postgres, миграции, пул» — без конкретики.

### messaging

> Все publish'ы и subscribe'ы перечислены с subject'ом;
> delivery/ordering/idempotency явные; версионирование subject'ов при
> breaking-changes описано?

- ✅ Published `user.events.registered`, `user.events.email_changed`;
  Consumed `verification.events.email_verified`; Delivery=at-least-once,
  Ordering=per-aggregate (key=UserID), Idempotency=unique-индекс по
  natural key + `ON CONFLICT DO NOTHING`, versioning через суффикс `.v2`
  при breaking.
- ❌ «Outbox для всего, NATS пока нет» — но в коде уже есть consumers.

### auth

> Идентичность выбрана (own/external); authn механизм явный
> (JWT/session/none); authgate описывает что middleware кладёт в principal;
> роли перечислены; seed-admin путь идемпотентен; brute-force защищает login?

- ✅ `identity: own`; JWT HS256 (authnkit) с секретом из env (`JWT_SECRET`, ≥32 байт); authgate кладёт
  `identity.Identity{UserID, Role}`; роли `client`/`admin`; seed-admin через
  `cmd/seed-admin` (env `SEED_ADMIN_*`), идемпотентен; cleanup попыток —
  `bruteforcepg.Module` кита (`BRUTEFORCE_CLEANUP_INTERVAL=5m`).
- ❌ «JWT, есть middleware» — без указания где принципал, без описания
  ролей.

### observability

> Sink явный (stdout); формат и уровень заданы; старт-лог
> (component/version/commit) есть; где и как логируются ключевые события?

- ✅ Логи в stdout, формат `json`, уровень `LOG_LEVEL` (default `info`);
  старт-лог `component`/`version`/`commit`; ключевые события (старт, ошибки usecase,
  результат обработки эвента) логируются структурно через logkit.
- ❌ «Логирование есть» — без формата, без уровня, без структурных полей.

### dependencies

> Все подключённые kit'ы golibinfra перечислены; что НЕ подключено —
> явно (чтобы не было ложных ожиданий «эта env читается»); внешние
> runtime-deps с режимом отказа; capacity-оценка есть?

- ✅ Kits: httpkit, pgkit, authnkit, outboxkit, natskit, auditkit,
  bruteforcekit, clockkit, logkit; **не подключено**:
  cachekit, flagkit, storagekit; External: Postgres (req),
  email-провайдер (Postmark, fallback=N/A, timeout=5s); Capacity:
  1 CPU/512MB, 2 replicas, pool=50.
- ❌ Список без «что НЕ подключено» — env-table из шаблонов читается,
  но эти kit'ы реально не wired → de-template не понимает.

## Алгоритм проверки скиллом

После любой правки артефакта:

1. `Read` файл артефакта.
2. `Read` `state/<kind>.md`, если есть.
3. Прогнать пункты 1, 2, 3, 4, 6 (программно проверяемые).
4. Если все 5 ✅ — раунд `AskUserQuestion` по тест-критерию kind'а
   (пункт 5) с двумя ✅/❌-вариантами + Other.
5. Если и пункт 5 ✅ — раунд `AskUserQuestion`: «Готов к stable,
   повышаем?» с опциями `Да` / `Нет` (Other).
6. При «Да» — `Edit`'ом `status: draft → stable`. Запустить валидатор
   ещё раз. Красный → откатить.

## Что НЕ делать

- **Не повышать сам**, без подтверждения.
- **Не понижать `stable → draft` сам** — только подсветить и предложить.
- **Не считать `Generates` заполненной**, если она содержит
  «соответствующие пути в коде» без конкретных путей — это TBD,
  не Generates.
- **Не считать cross-kind sync согласованным**, если drift найден,
  но архитектор сказал «потом» — оставить в state как `**blocker**`,
  stable не выдавать.
- **Не игнорировать `**blocker**`-вопросы**.
- **Не маркировать stable при красном валидаторе.**
