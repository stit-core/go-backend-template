package archtest

import "git.stit.tech/stit-core/golibinfra/archkit"

// layout — настройки archkit этого сервиса (TestLayout). Файл — сервиса: в .template/paths он под
// :(exclude), перенос обвязки шаблона его не трогает.
var layout = []archkit.Option{
	archkit.WithInternalDirs("internal/archtest", "internal/testkit"),
	archkit.WithPurePackages("internal/platform/identity", "pkg/policy"),
	// identity.IPFrom читает IP клиента из ctx запроса через httpkit — транспортный кит под корнем
	// домена: правило в этот пакет не заглядывает, за его чистотой следит ревью.
	archkit.WithUncheckedPackages("internal/platform/identity"),
	// Внешние пакеты, которые видит корень домена: порты и значения китов (аудит, хеширование, сессии,
	// метрики, часы, лог, outbox, pgkit.Transactor), чистые киты без I/O (ID, указатели), DI. Это
	// перечень разрешённого: драйвер, транспорт или кит с I/O в корне падает без правки списка.
	archkit.WithRootLibs(
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
	),
	// Известное нарушение — «нарушение из строки отказа → ID находки»; запись без нарушения — отказ.
	archkit.WithKnownViolations(map[string]string{}),
}
