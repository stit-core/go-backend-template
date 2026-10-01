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

Другим доменам поставщик отдаёт **`Trusted`** (`<provider>/trusted.go`): операции без policy —
решение принял вызывающий. Методы `Usecase` с policy — только входам самого поставщика (HTTP,
NATS); `Trusted` берут только адаптеры `module/` потребителей, свой `module/` лишь провайдит
`NewTrusted` (archtest `TestTrustedOnlyInModules`). Нужной операции нет — добавь её в `Trusted`, а
её ошибки — в `TrustedErrors` (тест `Trusted` держит, что других нет); канон —
`internal/domain/user/trusted.go`.

Адаптер — один тип на домен-поставщика, он реализует все порты потребителя к нему; тип **и его
конструктор** — в `adapters.go` (не расщеплять между `module.go` и `adapters.go`); канон —
`internal/domain/auth/module/adapters.go`.

```go
package module

import (
    "context"
    "errors"

    "<module-path>/internal/domain/<consumer>"
    "<module-path>/internal/domain/<provider>"
)

type <provider>Adapter struct {
    <provider>s *<provider>.Trusted
}

func new<Provider>Adapter(t *<provider>.Trusted) *<provider>Adapter {
    return &<provider>Adapter{<provider>s: t}
}

func (a *<provider>Adapter) GetByID(ctx context.Context, id string) (*<consumer>.<DTO>, error) {
    src, err := a.<provider>s.LookupByID(ctx, id)
    if err != nil {
        return nil, map<Provider>Err(err)
    }
    return <provider>To<Consumer>(src), nil
}

// map<Provider>Err переводит каждую ошибку <provider>.Trusted в ошибку <consumer>.
func map<Provider>Err(err error) error {
    switch {
    case errors.Is(err, <provider>.ErrNotFound):
        return <consumer>.Err<Provider>NotFound
    case errors.Is(err, <provider>.ErrConcurrentUpdate):
        return <consumer>.ErrConcurrentUpdate
    }
    return err
}

func <provider>To<Consumer>(s *<provider>.<Entity>) *<consumer>.<DTO> {
    return &<consumer>.<DTO>{
        ID:       s.ID,
        CanLogin: s.CanLogin(), // решение поставщика — его метод, не условие по его полям
        // ...только нужные поля
    }
}
```

`module/adapters.go` имеет право импортить корневой пакет другого домена (не его `infra`/`delivery`). Время в адаптере — только через инжектированный `clockkit.Clock`, не `time.Now()`. Обёртка кита (JWT, brute-force guard) — не здесь: адаптер порта к киту живёт в `<consumer>/infra/<kit>/`.

### 6. Зарегистрировать в `module/module.go` потребителя

```go
var Module = fx.Module(
    "internal/domain/<consumer>/module",
    fx.Provide(
        // ...уже было
        new<Provider>Adapter,
        func(a *<provider>Adapter) <consumer>.<Provider>Reader { return a },
    ),
    // ...
)
```

Тип адаптера реализует несколько портов — каждый привязывает identity-closure над **своим** типом
адаптера (канон fx — `docs/reference/architecture.md § fx`).

Identity-closure (`func(uc *<provider>.Usecase) <consumer>.<Port> { return uc }`) для порта между доменами запрещён (archtest): чужие sentinel'ы ушли бы потребителю без маппинга. Всегда типовой адаптер.

### 7. Проверить границы

```bash
make test PKG=./internal/archtest/...
```

`TestImportBoundaries` красный — провайдер импортирован не из `module/` или не корневым пакетом.
Перенеси код. Новый домен покрыт автоматически.

### 8. Тесты

- `usecase_test.go` потребителя: используй `Mock<Provider>Reader` (свежесгенерированный). Тестируй именно поведение consumer'а.
- `module/adapters_test.go` (white-box, обязателен): адаптер поверх настоящего `<provider>.Trusted` с моком его репозитория — маппинг полей и каждая ошибка `<provider>.TrustedErrors` → sentinel потребителя на каждом методе (чужой sentinel не протекает, новая ошибка без перевода роняет тест); канон — `internal/domain/auth/module/adapters_test.go`.

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
