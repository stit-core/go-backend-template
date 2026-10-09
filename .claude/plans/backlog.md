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
  (`password_reset` — `make gen` падает, дерево в полусостоянии). Триггер: повторный аудит
  (`repair-harness` S10) или первая жалоба.
- **testkit-failing-fakes** — тесты держат свои фейки поверх `authntest.DeterministicHasher`: `brokenHasher`
  (сбой хэширования — auth, user, verification) и `countingHasher` (счёт проверок пароля, холостых тоже —
  auth), и свой `failingAudit` (сбой записи аудита): таких нет в authntest и audittest. Триггер: партия 3
  golibinfra (v1.4.0) — фейки в testkit, локальные снять.
- **logout-audit** — выход (`auth.Logout`) не пишет аудит: `session.Store.Revoke` не отдаёт, чью
  сессию закрыл, а запись без учётной записи на любой токен — шум. Триггер: golibinfra отдаёт subject
  закрытой сессии (партия 3) — аудит `logout` с учётной записью.
- **clientip-test-setter** — `identitytest.Anon` кладёт IP клиента прогоном запроса через
  `httpkit.ClientIP`: сеттера IP в ctx у httpkit нет. Триггер: сеттер в httpkit (партия 3 golibinfra) —
  взять его.
- **commit-msg-pick-continue** — `git cherry-pick --continue` и `git rebase --continue` после конфликта
  зовут `commit-msg` с сообщением переносимого коммита: давний коммит не по Conventional Commits хук
  отклоняет. Триггер: первый такой перенос в форке или повторный аудит (`repair-harness` S10).
- **generated-header-fakes** — рукописный пакет `pkg/` целиком под заголовком «Code generated … DO NOT
  EDIT.» и такой же файл в каталоге вывода sqlc выпадают из покрытия и lint (видно в диффе).
  Триггер: повторный аудит (`repair-harness` S10) или первая такая подделка.
- **strategy-schema** — у артефактов strategy нет версии схемы: `docs/reference/domain-mapping.md`
  сверен с текущим `business-strategy-template` (паспорт, worker, integration-client) вручную.
  Триггер: правка схемы артефактов в strategy — версия во frontmatter там и строка соответствия здесь.
- **nul-guard-key** — вход с NUL в адресе отвечает 500: `bruteforcekit` (`NormalizeKey`) принимает
  subject с NUL, и запись попытки падает в Postgres (TEXT). Кит отвергает такой ключ `ErrInvalidKey`,
  как `session.CheckSubject`, — адаптер входа уже переводит его в 401. Триггер: партия 3 golibinfra
  (v1.4.0) — bump.
