# Ecosystem

Короткая карта для этого репо: роль, соседи, как сюда приходят агенты, что репо отдаёт и
берёт. Канон карты и операционного флоу — `docs/reference/ecosystem.md` инфра-репо `<org>/infra`
(форк `project-infra-template-compose`; sibling-checkout — `../infra`), устройство корня проекта —
[project-root.md](https://github.com/stit-core/project-infra-template-compose/blob/main/docs/reference/project-root.md)
там же.

## Роль

`go-backend-template` форкается в `<org>/backend` (второй сервис орги — `<org>/<имя>`) — Go-сервис
проекта: HTTP API, outbox-drain, NATS-подписчики и фоновые jobs в одном бинаре `cmd/api`;
`cmd/migrate` — отдельный образ (init-контейнер). Зона репо — код → образ в registry; выкатка — не здесь.
Имя репо — имя сервиса: путь образа и его имя в хабе.

```
<org>/strategy ──домены (design-time, read-only)──► <org>/backend (этот репо) ◄── golibinfra (go.mod)
                                                         │ make release
                                                         ▼
                                   registry Gitea: <host>/<org>/<репо>{,-migrate}:vX.Y.Z
                                                         │ make bump SVC=<репо> TAG=vX.Y.Z
                                                         ▼
                                   <org>/infra — runtime проекта (docker-compose на VPS) и хаб
```

## Соседи

Шаблоны `stit-core/*` форкаются в репо клиентской орги `<org>/*`. Проект — папка `~/git/<org>/`: в
её корне — карта проекта `CLAUDE.md` и общие настройки сессии, в папках корня — форки орги
sibling-checkout'ами (`../<name>`).

| Форк (шаблон) | Роль |
|---|---|
| `<org>/strategy` (`business-strategy-template`) | design-time source of truth: бизнес, продукты, сервисы и их домены |
| `<org>/infra` (`project-infra-template-compose`) | runtime проекта (docker-compose на VPS) и хаб: программы сквозных задач, выкатка тегов (`make bump`) |
| `<org>/backend` (`go-backend-template`) | этот репо — Go-сервис |
| `<org>/frontend` (`spa-frontend-template`) | React SPA (кабинет, всё за логином) |
| `<org>/site` (`seo-frontend-template`) | Astro SEO-сайт (индексируемый контент) |
| `<org>/<bot>` (`telegram-bot-template`) | Telegram-бот (Python, aiogram) |
| `<org>/mobile` (`mobile-template`) | KMP-приложение Android + iOS; релиз — в магазины |

Библиотеки: `golibinfra` — Go-киты этого сервиса; `pylibinfra` — Python-киты бота.

## Как сюда приходят агенты

- **Из корня проекта** — обычный путь. Проект стартует в `<org>/strategy`: код начинается, когда
  проект продуман. Работу ведёт сессия корня `~/git/<org>/` — сама или фоновым агентом в папке этого
  репо (первый сервис орги — `backend/`). Сквозная задача — программа в `.claude/plans/` хаба: её фаза
  приходит сюда и ведётся планом здесь (`/plan`, `parent: infra:<slug>`). Работа идёт обвязкой этого
  репо: сначала `CLAUDE.md` через Read (с ним подхватываются правила и скиллы репо), дальше — скиллы и
  гейт `make check`. В хаб возвращаются образ в registry (`/release`), INFRA-HANDOFF-блок и дифф на
  ревью: ревью проводит принимающая сторона, строка `Review:` коммитов называет её
  (CLAUDE.md § Готово).
- **Прямой заход** (сессия в папке репо) — тоже нормальный режим: та же обвязка. Последствия для
  соседей (инфра-поверхность — таблица rule `infra-impact`) оформляются INFRA-HANDOFF-блоком для
  хаба — skill `/infra-handoff`.

## Что репо отдаёт

- **Образы** `<host>/<org>/<репо>:vX.Y.Z` (`cmd/api`) и `<host>/<org>/<репо>-migrate:vX.Y.Z`
  (`cmd/migrate` + SQL) одним lockstep-тегом — `make release` (skill `/release`, процедура —
  [release.md](release.md)). Путь выводится из `git remote get-url origin`: образы уходят в container
  registry той же Gitea, где лежит репо (`git.stit.tech`), у каждого сервиса орги — свой. `<org>/infra`
  тянет их из registry `REGISTRY_HOST` (дефолт `git.stit.tech`) как `${PROJECT_NAME}/<svc>`, `<svc>` —
  имя репо.
- **INFRA-HANDOFF-блок**: `<РЕПО>_TAG` с путём образов + накопитель `.claude/plans/handoff.md`
  (`make release`).
- **STOP после `/release`.** Выкатка тега — `make bump SVC=<репо> TAG=vX.Y.Z` в `<org>/infra`
  (правит `<РЕПО>_TAG` в `.env` на VPS + force-recreate; у `<org>/backend` — `BACKEND_TAG`); откат —
  тот же `make bump` с предыдущим тегом. Деплой-команды (`make bump`, `make ship`, `ssh`) отсюда не
  запускаются.

## Что репо берёт

- **Домены — из `../strategy`** (design-time, read-only): `docs/projects/<slug>/services/<svc>/domains/<ctx>/`,
  общие VO — `services/_shared/`, границы сервиса — паспорт `services/<svc>/README.md`. `<slug>`,
  `<svc>` и путь к strategy-репо записаны в `docs/stack/service.md § Identity`; по ним `/scaffold`
  (вариант domain) разворачивает код, маппинг — [domain-mapping.md](domain-mapping.md).
  Проектируются домены там (`/domain-deep-dive`); рантайм-образ от strategy-репо не зависит.
- **Киты — `golibinfra`** (версия в `go.mod`, перечень — README golibinfra § Пакеты). Нужный
  примитив — в `golibinfra`, не локально (`architecture.md § Граница с golibinfra`); новая версия — по
  аннотации тега, skill `/upgrade`.
- **Обвязка — из шаблона `go-backend-template`**: пути из `.template/paths` (CLAUDE.md, `.claude`,
  скрипты, Makefile, Dockerfile'ы, lint-конфиг, archtest, доки шаблона); код форка вне них — по
  запросу тем же переносом (`PATHS=`, разделы `Безопасность` и `Платформа` аннотации). Версия обвязки
  форка — `.template/version`, новые — теги шаблона `template/vX.Y.Z` у remote `template` (его заводит
  финал `/init-service`); новый тег называет `make doctor`, перенос — `make template-update` (skill
  `/upgrade`). Периодический `/upgrade` по сервисам проекта — программа хаба.
- **Конфиг и секреты — из окружения контейнера.** В проде их задаёт `<org>/infra`: `.env` на VPS по
  контракту его `.env.example`, интерполяция в compose `environment:`. Схема ключей — артефакт
  `docs/stack/config.md`; значения в этот репо не попадают.
