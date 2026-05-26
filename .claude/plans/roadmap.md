# Roadmap

Единственный артефакт планирования проекта — этот файл. Подпапок, `<slug>.md`,
архивов и changelog'ов нет: они расходятся с правдой и мешают.

**Planning-first.** Изменение идёт через план — секцию в `## Active` ниже.
Прямая правка без плана допустима только для truly-atomic (опечатка, один
очевидный файл) и багфиксов (→ skill `triage-failure`, RED-first). Отклонение
от плана — осознанное и проговаривается с юзером.

**Жизненный цикл.** `planning → in-progress → blocked? → done`. По `done` секция
задачи удаляется целиком — история остаётся в `git log`. Незакрытый follow-up
переезжает в backlog или новой Active-секцией, а не хвостом в удаляемой.

**Current focus:** — (нет активных планов)

## Active

## Backlog (idea state)
- Description-оптимизация (run_loop из skill-creator) для новых/слитых скиллов (`init-service`, `domain-deep-dive`, `stack-deep-dive`, `scaffold`, `de-template`) — эмпирический тюнинг триггеров через `claude -p`. Опционально: descriptions уже канон-pushy; запускать отдельным заходом (тяжёлый), приоритетно `scaffold` (одно description на 5 интентов).
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
- Глубокий slim правил `comments`/`domain-artifacts`/`env-local`/`infra-impact`/`sqlc-and-migrations`/`tests-pipeline` (длинные примеры → docs/skills) — низкий приоритет: структурная диспозиция и фикс `globs` уже сделаны (A1, A11), правила path-scoped и нормального размера.
