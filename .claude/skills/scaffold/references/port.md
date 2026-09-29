# scaffold › port — cross-domain порт + адаптер

Кросс-доменный вызов через порт + адаптер. Прямой импорт между бизнес-доменами запрещает `internal/archtest` — это enforce-ит границу.

См. `.claude/rules/domain-architecture.md` для бэкграунда (автогрузится при работе с `internal/domain/`).

> `<module-path>` в импортах ниже — module path из `go.mod` (после `/de-template` — свой
> `git.stit.tech/<org>/backend`). Подставляй фактический, не копируй буквально.

## Когда применять

- "auth-домену нужно прочитать user.Email"
- "verification должен уметь обновлять password в user"
- "Новый домен X требует данных из Y"

## Когда НЕ применять

- Вызов внутри одного домена — просто метод usecase.
- Вызов в `golibinfra/*` — это уже port-слой.
- Вызов из адаптера в `module/adapters.go` в другой домен — ОК: `module/` вправе импортировать корневой пакет чужого домена (archtest).

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
    GetByID(ctx context.Context, id string) (*<DTO>, error)
}
```

Имя — что-то вроде `<Provider>Reader`, `<Provider>Writer`, `<Resource><Verb>er`. Сужай интерфейс до **минимально нужных методов**, не копируй весь Repository.

DTO — **локальные** типы консьюмера, не utility-типы провайдера. Если auth нужны 3 поля от user — заведи `auth.User` с этими 3 полями. Не возвращай `*user.User`. Порт переносит решения, а не сырьё: вычисленное свойство (`CanLogin bool`) вместо сырого статуса чужого enum'а. Ошибки порта — sentinel'ы **потребителя** (`Err<Provider>NotFound` в его `errors.go`): чужие sentinel'ы потребитель не может `errors.Is` (archtest).

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
<module-path>/internal/domain/<consumer>:
  interfaces:
    <Provider>Reader:
```

`make gen` → `internal/domain/<consumer>/mocks/<provider>_reader.go` (пакет `mocks`, тип `Mock<Provider>Reader`).

### 5. Адаптер в `module/adapters.go` потребителя

Тип адаптера **и его конструктор** — в `adapters.go` (не расщеплять между `module.go` и
`adapters.go`); канон — `internal/domain/auth/module/adapters.go`.

```go
package module

import (
    "context"
    "errors"

    "<module-path>/internal/domain/<consumer>"
    "<module-path>/internal/domain/<provider>"
)

type <provider>ReaderAdapter struct {
    uc *<provider>.Usecase
}

func new<Provider>ReaderAdapter(uc *<provider>.Usecase) <consumer>.<Provider>Reader {
    return &<provider>ReaderAdapter{uc: uc}
}

func (a *<provider>ReaderAdapter) GetByID(ctx context.Context, id string) (*<consumer>.<DTO>, error) {
    src, err := a.uc.LookupByID(ctx, id)
    if err != nil {
        return nil, map<Provider>Err(err)
    }
    return <provider>To<Consumer>(src), nil
}

// Ports carry consumer-domain sentinels: callers can't errors.Is against <provider>'s (archtest).
func map<Provider>Err(err error) error {
    if errors.Is(err, <provider>.ErrNotFound) {
        return <consumer>.Err<Provider>NotFound
    }
    return err
}

func <provider>To<Consumer>(s *<provider>.<Entity>) *<consumer>.<DTO> {
    return &<consumer>.<DTO>{
        ID:    s.ID,
        Email: s.Email,
        // ...только нужные поля
    }
}
```

`module/adapters.go` имеет право импортить корневой пакет другого домена (не его `infra`/`delivery`). Время в адаптере — только через инжектированный `clockkit.Clock`, не `time.Now()`.

Если в провайдере нет `LookupByID`-метода (т.е. publicly не выставлен) — добавь его в `<provider>/usecase.go`. Lookup-методы без проверки policy — нормальная практика для cross-domain (вызывающий уже сделал свою проверку).

### 6. Зарегистрировать в `module/module.go` потребителя

```go
var Module = fx.Module(
    "<consumer>",
    fx.Provide(
        // ...уже было
        new<Provider>ReaderAdapter,
    ),
    // ...
)
```

Inline identity-closure (`func(uc *<provider>.Usecase) <consumer>.<Port> { return uc }`) — только когда провайдер удовлетворяет порт как есть, без маппинга.

### 7. Проверить границы

```bash
make test PKG=./internal/archtest/...
```

`TestImportBoundaries` красный — провайдер импортирован не из `module/` или не корневым пакетом.
Перенеси код. Новый домен покрыт автоматически.

### 8. Тесты

- `usecase_test.go` потребителя: используй `Mock<Provider>Reader` (свежесгенерированный). Тестируй именно поведение consumer'а.
- `module/adapters_test.go` (опционально): тест маппинг-функции `<provider>To<Consumer>` — pure function, тривиально тестируется.
- Integration: уже покрыто existing integration-тестами модуля, если адаптер не делает ничего хитрого.

### 9. Верификация

```bash
make lint
make test
make build
make gen-check
```

### 10. Коммит

`feat(<consumer>): add <Provider>Reader port + adapter`

## Anti-patterns

- ❌ Возвращать из адаптера тип провайдера (`*user.User` в auth-коде). Адаптер маппит в локальный DTO потребителя.
- ❌ Широкий порт «на вырост» (10 методов "вдруг пригодится"). Сузь до того что реально нужно сейчас.
- ❌ Двусторонняя зависимость A↔B через два порта. Это сигнал, что границы доменов кривые. Подумай о третьем домене или объединении.
- ❌ Адаптер в `<consumer>/adapters.go` (в корне домена) вместо `<consumer>/module/adapters.go`. archtest заблокирует импорт другого домена из корня.
- ❌ Mockery-конфиг не обновлён → моков нет → юниты не пишутся, или пишутся через ручные fake'и.
