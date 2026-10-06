# Backlog

Кандидаты в работу, ещё не взятые в план: `- **<id>** — что и зачем. Триггер: когда браться.` Форма и жизненный цикл — skill `/plan`.

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
- **new-domain-names** — `make new-domain` принимает имена, на которых срез не проходит гейт: `repo`
  (не компилируется), инициализмы (`api_key` — замечания линтера), существующая таблица
  (`password_reset` — `make gen` падает, дерево в полусостоянии). Триггер: S13–S14
  `fix-reference-code` или первая жалоба.
- **vuln-scan** — govulncheck и сканирование образов не входят в гейт: им нужна сеть (база
  уязвимостей), а `make check` от сети не зависит. Место — `release-preflight` перед тегом. Триггер:
  `repair-harness` S9 (гейт) или первый релиз форка (SEC-11).
- **authntest-broken-hasher** — тесты сбоя хэширования (регистрация user, подтверждение сброса
  verification) держат свой `brokenHasher` поверх `authntest.DeterministicHasher`: хэшера со сбоем в
  authntest нет. Триггер: партия 3 golibinfra (v1.4.0) — фейк в authntest, локальные снять.
- **logout-audit** — выход (`auth.Logout`) не пишет аудит: `session.Store.Revoke` не отдаёт, чью
  сессию закрыл, а запись без учётной записи на любой токен — шум. Триггер: golibinfra отдаёт subject
  закрытой сессии (партия 3) — аудит `logout` с учётной записью.
- **clientip-test-setter** — `identitytest.Anon` кладёт IP клиента прогоном запроса через
  `httpkit.ClientIP`: сеттера IP в ctx у httpkit нет. Триггер: сеттер в httpkit (партия 3 golibinfra) —
  взять его.
