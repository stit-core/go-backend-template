# Roadmap

Индекс активных планов. Каждый детальный план — в `<slug>.md` рядом. Управляется skill'ом `plan`.

План удаляется сразу по завершении — история остаётся в `git log`. Индекс не держит секцию завершённого. Открытый follow-up из закрытой задачи переезжает строкой в `## Active`.

## Active

- [e2e-exhaustive-coverage](./e2e-exhaustive-coverage.md) — coverage-матрица: authz/bruteforce-lock/write/негативы/list (План B) — `in-progress` 0/10 atoms

## Backlog (idea state)
- MinIO-примитив (`test/infra/objstore.py` + minio-сервис в compose) — завести когда форк добавит реального потребителя объектного хранилища (сейчас потребителя нет).
