---
name: query-logs
description: Прочитать или последить логи задеплоенного бэка через корп-Loki (адрес из `STIT_LOKI_ADDR`) под bearer-токеном из `./.env.local`. Активируй когда юзер просит "посмотри логи", "что в логах", "следи за деплоем", "почему упало в проде", "/query-logs".
---

# Skill: query-logs

Чтение логов через корп-Loki read API. Адрес берётся из `STIT_LOKI_ADDR` (managed: `https://loki.stit.tech`; self-hosted: свой Loki из форка `corp-infra-template`) — без переменной `logs.sh` падает с понятным `FAIL`, не уходит молча на managed-хост. Tenant-изоляция — на стороне `tenant-auth` ForwardAuth (см. infra-repo `docs/project-secrets.md`). Этот repo даёт обёртку `scripts/tools/logs.sh` + правила, ничего к корпу не патчит.

**Инструмент один:** `scripts/tools/logs.sh` (три режима). Tempo (trace API) — отдельно через `curl`, в этот skill пока не входит.

## Когда применять

- "Посмотри логи API за последний час" / "что там по сервису auth"
- "Почему юзер X не залогинился вчера в 15:00"
- "Что упало в проде" → mode `errors`
- "Запусти мониторинг логов на 5 минут после деплоя" → mode `watch` (run_in_background + Monitor)

## Когда НЕ применять

- **Локальный dev без OTLP-коллектора.** Логи всегда в stdout (`LOG_STDOUT=true` по умолчанию), а экспорт в Loki молча дропается — в Loki пусто. Смотри прямо в терминал/stdout контейнера.
- **Юзер хочет метрики (latency / RPS / errors-per-second)** — это VictoriaMetrics, наружу не опубликована. Скажи юзеру смотреть в Grafana.
- **Юзер хочет трейсы по trace-id** — Tempo read API существует (`https://tempo.stit.tech/api/traces/<id>`), но в этом skill'е не покрыт, и обёртки-инструмента под него нет. **Не** дёргай `curl` с inline-подстановкой токена из `.env.local` (`grep ... .env.local`) — значение утечёт в transcript (см. Anti-patterns ниже). Нужны трейсы — заведи agent-tool по образцу `scripts/tools/logs.sh` (читает токен в bash-процессе, не эхо-ит), затем зови его.

## Pre-flight

- [ ] `command -v logcli` — установлен? Если нет → `make setup` (или скажи юзеру).
- [ ] `./.env.local` существует и содержит строку `STIT_OBS_TOKEN=...`? Сам файл читать не нужно — запусти `logs.sh`, он прочитает токен через bash (значение не попадёт в чат), или вернёт `FAIL: no STIT_OBS_TOKEN` с инструкцией.
- [ ] `STIT_LOKI_ADDR` задан (env или строка в `./.env.local`)? Обязателен — managed `https://loki.stit.tech`, self-hosted — свой Loki. Без него `logs.sh` вернёт `FAIL: no STIT_LOKI_ADDR` (Tempo trace API — тот же принцип: свой endpoint, когда под него появится обёртка-инструмент).
- [ ] Знаешь `service` label? Дефолт в шаблоне — имя бинаря (`api` для runtime, `migrate` для init-контейнера миграций); маппинг — `OTEL_SERVICE_NAME` env при деплое.
- [ ] Для `watch` — оценил длительность? Не больше 10m по дефолту, юзер может расширить.

## Режимы

### `query` — точечный поиск

```bash
scripts/tools/logs.sh query --service api --match 'panic' --since 1h --limit 200
# или raw LogQL:
scripts/tools/logs.sh query --raw '{service="api",env="prod"} |~ "user_id=abc-123"' --since 6h
```

Возвращает совпавшие строки. По умолчанию `--since 1h --limit 200`. Усечение при > 25k символов.

**Когда:** юзер дал конкретное уточнение ("найди ошибку с user_id=X" / "что было в момент Y").

### `errors` — триаж недавних ошибок

```bash
scripts/tools/logs.sh errors --service api --since 15m --top 10
```

Группирует совпавшие error-строки по сигнатуре (числа/uuid/hex/timestamps нормализованы), выдаёт топ-N групп с count'ом. Полезно когда юзер спрашивает "что упало" — а не "найди конкретное".

**Сигнатуры ловит**: `level=error`, `panic:`, `fatal`, `"level":"error"` (JSON-логи logkit).

### `watch` — мониторинг N минут

```bash
scripts/tools/logs.sh watch --service api --for 5m
```

**Запускай через `run_in_background: true`**, потом следи через `Monitor` tool. Стримит `logcli query --tail` за указанное окно, в конце — сводка (total lines, errors count). По SIGTERM/timeout — `OK` exit.

**Когда:** юзер сказал "инфра выкатывает релиз, последи 5 минут" или "следи пока я тестирую". Если ожидаемое окно > 10m — спроси, точно ли надо столько (cache TTL у Anthropic 5m, долгое чтение мониторинга через Monitor тянет токены).

## После

- Если нашёл проблему → переключайся в `triage-failure` (TDD-first для багфикса).
- Если ничего значимого → отчёт юзеру в 1-2 предложениях, не пересказывай весь output.

## Anti-patterns

- ❌ `cat ./.env.local` / `Read` на него ради токена — значение утечёт в transcript/память. Доступ к токену — только через `logs.sh` (держит его в bash-процессе).
- ❌ Цитировать токен в чат, даже частично. Если случайно подсмотрел в exec output — НЕ повторяй.
- ❌ Гонять `watch --for 30m` без явного OK юзера. Cache miss + большой transcript.
- ❌ Звать `query --raw` без `{}-matcher` — Loki потребует label selector, выдаст 400.
- ❌ Искать в Loki логи из локального dev без коллектора — экспорт дропнут, в Loki пусто (логи в stdout).

## См. также

- `.claude/rules/env-local.md` — формат `.env.local`, какие ключи там живут.
- `scripts/tools/logs.sh --help` — точный синтаксис флагов.
- infra-repo `docs/project-secrets.md` → секция «Observability read API» — где брать токен, как ротировать.
- infra-repo `.claude/plans/observability-read-api.md` — design / verification API.
