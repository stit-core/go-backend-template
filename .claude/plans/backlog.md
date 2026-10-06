# Backlog

Кандидаты в работу, ещё не взятые в план: `- **<id>** — что и зачем. Триггер: когда браться.` Форма и жизненный цикл — skill `/plan`.

- **publisher-stream-check** — старт не проверяет стрим домена-публикатора: без него сервис здоров,
  а строки outbox копятся без PubAck. Покрытие subject'ов стримом видит JetStream API natskit и
  outboxkit (в v1.2.0 проверки нет), эквивалент в шаблоне не пишется. Триггер: кит проверяет стрим
  публикатора на старте — подключить в `events.OutboxFor`, тексты `.env.example` и
  `architecture.md` (§ События) — «старт падает».
- **infra-touch-migrations** — `make status` молчит на staged-переименовании закоммиченной
  миграции и шумит на правке ещё не выпущенной: сравнивать с тегом последнего релиза образа (без
  тегов обвязки — `git describe --tags --abbrev=0 --exclude 'template/*'`), а не с HEAD. Триггер:
  первая ложная тревога или повторный аудит (`repair-harness` S10).
- **reset-timing** — запрос сброса пароля для существующей учётной записи дольше на транзакцию и
  синхронное письмо: с настоящим mailer время ответа выдаёт учётную запись. Канон — письмо через
  outbox-консьюмер с `sent_at`, как подтверждение email. Триггер: настоящий Mailer в форке или
  повторный аудит (SEC-06).
- **events-v1-reader** — прежний durable `verification-user-registered` дочитывает
  `user.events.registered` (события до `.v2`, `verification/delivery/nats`): снять подписку и
  `subjectUserRegisteredV1`, в накопитель — INFRA-HANDOFF «удалить durable». Триггер: следующий релиз
  после выкатки релиза с `.v2`.
- **healthcheck-kit** — проверка здоровья контейнера без shell (`api healthcheck`: `GET /health` по
  `HTTP_ADDR`) живёт в `cmd/api/healthcheck.go`, а нужна каждому сервису на httpkit в distroless-образе:
  место — httpkit, рядом с `/health`. Триггер: golibinfra v1.3.0 (`harden-authn-release` S10) —
  снимает `fix-reference-code` S9.
- **new-domain-names** — `make new-domain` принимает имена, на которых срез не проходит гейт: `repo`
  (не компилируется), инициализмы (`api_key` — замечания линтера), существующая таблица
  (`password_reset` — `make gen` падает, дерево в полусостоянии). Триггер: S13–S14
  `fix-reference-code` или первая жалоба.
- **vuln-scan** — govulncheck и сканирование образов не входят в гейт: им нужна сеть (база
  уязвимостей), а `make check` от сети не зависит. Место — `release-preflight` перед тегом. Триггер:
  `repair-harness` S9 (гейт) или первый релиз форка (SEC-11).
