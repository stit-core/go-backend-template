---
name: scaffold
description: Добавить новый строительный блок по конвенциям шаблона — domain (бизнес-домен в internal/domain/), endpoint (HTTP-метод существующего домена), event (outbox-событие + subscriber), migration (схема БД up/down + sqlc), field (поле сущности от колонки до ответа API), env (env-переменная конфига), port (cross-domain порт + адаптер), job (периодическая задача домена). Используй, когда задача — завести такой блок: новый домен, ручку API, асинхронную реакцию на событие, изменение схемы БД, новое поле, настройку из окружения, данные одного домена для другого, работу по расписанию. Правка уже существующего блока — прямая правка, не scaffold.
---

# Skill: scaffold (роутер)

Добавляет новый строительный блок в существующий проект по конвенциям шаблона. Скилл —
роутер: общий ритм работы держится здесь, специфика каждого варианта — в
`references/<variant>.md`. Открой нужный reference и веди по нему, не пересказывая его сюда.

## Выбор варианта

| Что добавляешь | Вариант | Reference | Сигналы |
|---|---|---|---|
| Новый бизнес-домен (слои, миграция, sqlc, моки, fx-wiring) | `domain` | `references/domain.md` | «добавь домен `order`», «scaffold a domain» |
| HTTP-метод в существующий домен | `endpoint` | `references/endpoint.md` | «добавь POST /api/...», «новый route/ручка» |
| Outbox-эвент + subscriber | `event` | `references/event.md` | «после X асинхронно Y», «новый outbox-эвент/NATS subscriber» |
| Миграция схемы БД | `migration` | `references/migration.md` | «измени схему», «добавь индекс/таблицу/FK» |
| Поле сущности через слои: колонка → sqlc → сущность → API | `field` | `references/field.md` | «добавь колонку `phone` в users и поле в ответ» |
| Env-переменная конфига | `env` | `references/env.md` | «вынеси TTL в env», «нужен ключ API провайдера» |
| Cross-domain порт + адаптер | `port` | `references/port.md` | «домену A нужны данные из B», «сделай adapter» |
| Периодическая задача домена (`timerkit`) | `job` | `references/job.md` | «раз в час чисти …», «по расписанию» |

Несколько вариантов в одной задаче (новый домен с событием) — веди по основному (`domain`),
затем подключи нужные шаги из соседнего reference (`event`).

## Когда НЕ применять

- **Таблица «Маршрут задачи»** (CLAUDE.md) ведёт в план — сначала `/plan`, его шаги-блоки — здесь.
- **Правка существующего** блока (переписать метод, поменять существующий route/запрос) —
  точечная правка без scaffold.
- **Проектирование нового домена с нуля** (бизнес-смысл ещё не зафиксирован) — сначала
  `/domain-deep-dive` в соседнем strategy-репо орга (домены —
  `../strategy/docs/projects/<slug>/services/<svc>/domains/`), scaffold — потом по stable-артефактам
  оттуда.
- **Онбординг форка** (развернуть фундамент проекта) — это `init-service` → `de-template`.

## Общий ритм (применим ко всем вариантам)

Reference даёт variant-специфичные шаги; этот ритм обрамляет любой из них:

1. **Подтвердить вход.** Уточни недостающее одним блоком до начала (что именно, имя/path, права,
   коды ошибок) — детали уточняющих вопросов в reference.
2. **Сделать изменение** по шагам reference (scaffold-файлы / правка слоёв).
3. **`make gen`** — обязателен, если тронул `migrations/`, `queries/*.sql`, интерфейсы из
   `.mockery.yml` или HTTP-операции (`openapi.json`). Сгенерированное (`infra/postgres/db/*`,
   `**/mocks/*`, `openapi.json`) руками не правь.
4. **Тесты по RED→GREEN.** Где появляется логика — сначала тест (red по нужной причине), потом
   код до green. Reference перечисляет слои в порядке кода, тест слоя пишется до его кода.
5. **Готово** — по CLAUDE.md § Готово. По ходу — `make test PKG=./internal/domain/<name>/...`,
   `make lint PKG=...`.
6. **Conventional Commit:** `feat(<scope>): <subject>` (точный scope/subject — в reference), в
   теле — строка `Review:` (CLAUDE.md § Готово).
   Тронул инфра-поверхность (таблица rule `infra-impact`) → оформи INFRA-HANDOFF (skill `infra-handoff`)
   до коммита.

## Anti-patterns (общие; variant-специфичные — в reference)

- Бизнес-логика в handler / прямой `pgx`/`sqlc` мимо usecase.
- Прямой импорт другого бизнес-домена — только порт + адаптер (`references/port.md`).
- Править сгенерированное руками вместо `make gen`.
- `fmt.Print*`/`slog.*` напрямую — только `logkit.Logger`.
- Claim «готово» мимо CLAUDE.md § Готово.
- Scaffold ради правки существующего блока — это прямая правка.

## См. также

- `references/{domain,endpoint,event,migration,field,env,port,job}.md` — процедуры вариантов.
- Правила `domain-architecture`, `sqlc-and-migrations`, `events-and-messaging`,
  `tests-pipeline`, `infra-impact` — каждый reference читает свои шагом 0.
