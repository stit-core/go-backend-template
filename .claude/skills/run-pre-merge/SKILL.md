---
name: run-pre-merge
description: Полная проверка кода перед claim "готово" / коммитом / PR. Активируй когда пользователь спрашивает "проверь готовность", "перед коммитом", "запусти все проверки".
---

# Skill: run-pre-merge

Канонический pre-merge checklist для этого шаблона. Соответствует `make pre-merge` target в Makefile.

## Когда применять

- "Проверь что всё готово к коммиту"
- "Запусти все проверки"
- "Готово ли это к merge?"
- Автоматически перед claim "task done" в долгих задачах.

## Шаги

### 1. Форматирование

```bash
make fmt
```

Или через `gofumpt -w .`. PostToolUse-хук уже гонит `gofumpt` на каждый Edit, но `make fmt` страхует от ручных правок не через Edit.

### 2. Vet

```bash
go vet ./...
```

Уже включается в `make pre-merge`. Если падает — исправь и заново.

### 3. Lint

```bash
scripts/agent-tools/lint.sh
```

Или `make lint`. Wrapper даёт сжатый вывод.

Топ ошибок которые встретишь:
- `forbidigo` — `fmt.Print*` или `slog.*`. Замени на `logkit.Logger`.
- `depguard` — прямой импорт между доменами. Сделай порт + адаптер.
- `errcheck` — забытая проверка `err`.
- `gosec` — небезопасные паттерны (например, `crypto/md5`).

### 4. Тесты

```bash
scripts/agent-tools/test.sh
```

Или `make test`. Wrapper показывает только падающие.

Если есть integration-тесты которые задевают твои правки:
```bash
make test-int
```

(Нужен Docker.)

### 5. Build

```bash
scripts/agent-tools/build.sh
```

Или `make build && make build-worker`.

### 6. Сгенерированный код актуален

Если меняли `migrations/` или `infra/postgres/queries/`:
```bash
make sqlc-diff
```

Если меняли интерфейсы из `.mockery.yml`:
```bash
make mocks-check
```

Оба должны выйти с кодом 0.

### 7. Coverage не упал

```bash
make test-cover
```

CI требует 60% floor. Локально проверь что не уронил.

### 8. Краткое summary

Если все шаги зелёные:
```
✓ fmt        gofumpt clean
✓ vet        no issues
✓ lint       no issues
✓ test       N tests passed
✓ build      ok
✓ sqlc-diff  in sync
✓ mocks      in sync
```

Готово к коммиту / PR.

### 9. Если что-то красное

1. Не пытайся коммитить через `--no-verify` — PreToolUse-хук это блокирует.
2. Разбирай по одному:
   - Lint: исправь по сообщению линтера.
   - Test: TDD — посмотри какой тест падает, разбери почему.
   - Build: первая ошибка обычно главная.
3. Поправил → повторяй с шага 1.

## Когда НЕ запускать всё

- Простой docs-only commit (`docs:`-prefix) → достаточно `make fmt` и проверки markdown.
- Обновление одного skill / rule → лишь `make fmt` если задело какие-то скрипты.
- В CI всё это и так гонится — pre-merge нужен чтобы поймать рано локально.

## Коммит после успеха

Не делай коммит за пользователя без явного OK. Только покажи что готово к коммиту и предложи conventional-commit-сообщение.
