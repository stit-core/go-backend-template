---
name: add-port
description: Добавить cross-domain порт + адаптер с проверкой depguard. Активируй когда юзер просит "домену A нужны данные из B", "добавь port", "сделай adapter", "auth должен видеть user".
---

# Skill: add-port

Кросс-доменный вызов через порт + адаптер. Прямой импорт между бизнес-доменами заблокирован depguard'ом — это enforce-ит границу.

См. `.claude/rules/domain-architecture.md` для бэкграунда (автогрузится при работе с `internal/domain/`).

## Когда применять

- "auth-домену нужно прочитать user.Email"
- "verification должен уметь обновлять password в user"
- "Новый домен X требует данных из Y"

## Когда НЕ применять

- Вызов внутри одного домена — просто метод usecase.
- Вызов в `golibinfra/*` — это уже port-слой.
- Вызов из адаптера в `module/adapters.go` в другой домен — ОК, файлы `module/` исключены из depguard.

## Шаги

### 1. Понять направление зависимости

Кто **владеет** контрактом — тот определяет порт.
- auth нуждается в данных user → порт `UserReader` живёт в `auth/usecase.go`. Не в user.
- verification нуждается в setpassword → порт `PasswordSetter` в `verification/usecase.go`.

Это инверсия зависимостей: высокоуровневый домен не зависит от деталей низкоуровневого, но низкоуровневый поставляет адаптер реализующий контракт высокоуровневого.

### 2. Объявить порт в потребляющем домене

В `internal/domain/<consumer>/usecase.go`:

```go
type <Provider>Reader interface {
    GetByID(ctx context.Context, id string) (*<consumer>.<DTO>, error)
}
```

Имя — что-то вроде `<Provider>Reader`, `<Provider>Writer`, `<Resource><Verb>er`. Сужай интерфейс до **минимально нужных методов**, не копируй весь Repository.

DTO — **локальные** типы консьюмера, не utility-типы провайдера. Если auth нужны 3 поля от user — заведи `auth.User` с этими 3 полями. Не возвращай `*user.User`.

### 3. Инжектируй порт в usecase

`UsecaseDeps`:

```go
type UsecaseDeps struct {
    fx.In

    Users <Provider>Reader  // вот этот порт
    // ...остальные
}
```

### 4. Добавить в `.mockery.yml`

```yaml
git.stit.tech/akadmin/go-backend-template/internal/domain/<consumer>:
  interfaces:
    <Provider>Reader:
```

`make mocks` → `internal/domain/<consumer>/mocks/<provider>_reader.go` (пакет `mocks`, тип `Mock<Provider>Reader`).

### 5. Адаптер в `module/adapters.go` потребителя

```go
package module

import (
    "context"

    "git.stit.tech/akadmin/go-backend-template/internal/domain/<consumer>"
    "git.stit.tech/akadmin/go-backend-template/internal/domain/<provider>"
)

type <provider>ReaderAdapter struct {
    uc *<provider>.Usecase
}

func (a *<provider>ReaderAdapter) GetByID(ctx context.Context, id string) (*<consumer>.<DTO>, error) {
    src, err := a.uc.LookupByID(ctx, id)
    if err != nil {
        return nil, err
    }
    return <provider>To<Consumer>(src), nil
}

func <provider>To<Consumer>(s *<provider>.<DTO>) *<consumer>.<DTO> {
    return &<consumer>.<DTO>{
        ID:    s.ID,
        Email: s.Email,
        // ...только нужные поля
    }
}
```

`module/adapters.go` имеет право импортить другой домен — depguard для пути `module/**` исключён.

Если в провайдере нет `LookupByID`-метода (т.е. publicly не выставлен) — добавь его в `<provider>/usecase.go`. Lookup-методы без проверки policy — нормальная практика для cross-domain (вызывающий уже сделал свою проверку).

### 6. Зарегистрировать в `module/module.go` потребителя

```go
var Module = fx.Module(
    "<consumer>",
    fx.Provide(
        // ...уже было
        func(uc *<provider>.Usecase) <consumer>.<Provider>Reader {
            return &<provider>ReaderAdapter{uc: uc}
        },
    ),
    // ...
)
```

### 7. Проверить depguard

```bash
scripts/agent-tools/lint.sh ./internal/domain/<consumer>/...
```

Если ругается — значит ты импортировал провайдер не из `module/`, а из самого домена. Перенеси код.

### 8. Тесты

- `usecase_test.go` потребителя: используй `Mock<Provider>Reader` (свежесгенерированный). Тестируй именно поведение consumer'а.
- `module/adapters_test.go` (опционально): тест маппинг-функции `<provider>To<Consumer>` — pure function, тривиально тестируется.
- Integration: уже покрыто existing integration-тестами модуля, если адаптер не делает ничего хитрого.

### 9. Верификация

```bash
scripts/agent-tools/lint.sh
scripts/agent-tools/test.sh
scripts/agent-tools/build.sh
make mocks-check
```

### 10. Коммит

`feat(<consumer>): add <Provider>Reader port + adapter`

## Anti-patterns

- ❌ Возвращать из адаптера тип провайдера (`*user.User` в auth-коде). Адаптер маппит в локальный DTO потребителя.
- ❌ Широкий порт «на вырост» (10 методов "вдруг пригодится"). Сузь до того что реально нужно сейчас.
- ❌ Двусторонняя зависимость A↔B через два порта. Это сигнал, что границы доменов кривые. Подумай о третьем домене или объединении.
- ❌ Адаптер в `<consumer>/adapters.go` (в корне домена) вместо `<consumer>/module/adapters.go`. depguard заблокирует импорт другого домена из корня.
- ❌ Bypass depguard через `//nolint:depguard`. Это не способ. Сделай порт.
- ❌ Mockery-конфиг не обновлён → моков нет → юниты не пишутся, или пишутся через ручные fake'и.
