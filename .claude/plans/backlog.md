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
- **commit-msg-pick-continue** — `git cherry-pick --continue` и `git rebase --continue` после конфликта
  зовут `commit-msg` с сообщением переносимого коммита: давний коммит не по Conventional Commits хук
  отклоняет. Триггер: первый такой перенос в форке или повторный аудит (`repair-harness` S10).
- **generated-header-fakes** — рукописный пакет `pkg/` целиком под заголовком «Code generated … DO NOT
  EDIT.» и такой же файл в каталоге вывода sqlc выпадают из покрытия и lint (видно в диффе).
  Триггер: повторный аудит (`repair-harness` S10) или первая такая подделка.
- **strategy-schema** — у артефактов strategy нет версии схемы: `docs/reference/domain-mapping.md`
  сверен с текущим `business-strategy-template` (паспорт, worker, integration-client) вручную.
  Триггер: правка схемы артефактов в strategy — версия во frontmatter там и строка соответствия здесь.
