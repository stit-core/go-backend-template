// Package outcome — исходы операций для метки result бизнес-метрик: один исход в разных доменах
// называется одинаково.
package outcome

// Result — значение метки result. Исход, особый для домена, — константа Result в его metrics.go.
type Result string

const (
	OK           Result = "ok"
	Error        Result = "error"
	Invalid      Result = "invalid"
	Forbidden    Result = "forbidden"
	WeakPassword Result = "weak_password"
	Expired      Result = "expired"
)
