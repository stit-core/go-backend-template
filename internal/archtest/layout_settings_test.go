package archtest

import "git.stit.tech/stit-core/golibinfra/archkit"

// layout — настройки archkit этого сервиса (TestLayout). Файл — сервиса: в .template/paths он под
// :(exclude), перенос обвязки шаблона его не трогает.
var layout = []archkit.Option{
	archkit.WithInternalDirs("internal/archtest", "internal/testkit"),
	archkit.WithPurePackages("internal/platform/identity", "pkg/policy"),
	archkit.WithUncheckedPackages(uncheckedPackages...),
	archkit.WithRootLibs(rootLibs...),
	// Известное нарушение — «нарушение из строки отказа → ID находки»; запись без нарушения — отказ.
	archkit.WithKnownViolations(map[string]string{}),
}

// uncheckedPackages — пакеты модуля под корнем домена, в чьи импорты правило не заглядывает: identity.IPFrom
// читает IP клиента из ctx запроса через httpkit — транспортный кит; за чистотой пакета следит ревью.
var uncheckedPackages = []string{"internal/platform/identity"}

// rootLibs — внешние пакеты, которые видят корень домена и адаптер module/: порты и значения китов, чистые киты без
// I/O, DI. Перечень разрешённого: драйвер, транспорт или кит с I/O в корне падает без правки списка.
var rootLibs = []string{
	"git.stit.tech/stit-core/golibinfra/auditkit",
	"git.stit.tech/stit-core/golibinfra/authnkit",
	"git.stit.tech/stit-core/golibinfra/authnkit/session",
	"git.stit.tech/stit-core/golibinfra/bizmetric",
	"git.stit.tech/stit-core/golibinfra/clockkit",
	"git.stit.tech/stit-core/golibinfra/idkit",
	"git.stit.tech/stit-core/golibinfra/logkit",
	"git.stit.tech/stit-core/golibinfra/outboxkit",
	"git.stit.tech/stit-core/golibinfra/pgkit",
	"git.stit.tech/stit-core/golibinfra/ptrkit",
	"go.opentelemetry.io/otel/attribute",
	"go.opentelemetry.io/otel/metric",
	"go.uber.org/fx",
}

// deliveryKits — пакеты golibinfra, которые видит вход домена (TestDeliveryReachesUsecaseOnly): транспорт, лог и
// helpers без I/O. Хранилища, клиенты, хэшер, аудит и метрики домена — за портами usecase; кит, который законно
// нужен входу сервиса, — строкой здесь.
var deliveryKits = []string{"httpkit", "natskit", "grpckit", "logkit", "clockkit", "idkit", "ptrkit", "mapkit", "tzkit"}
