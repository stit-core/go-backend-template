// Package outcome — исходы операций для метки result бизнес-метрик: один исход в разных доменах
// называется одинаково.
package outcome

// Result — значение метки result; структура, а не строка: литерал вместо исхода не компилируется.
type Result struct{ name string }

// New — исход, особый для домена: переменная в его metrics.go.
func New(name string) Result { return Result{name: name} }

func (r Result) String() string { return r.name }

var (
	OK           = New("ok")
	Error        = New("error")
	Invalid      = New("invalid")
	Forbidden    = New("forbidden")
	WeakPassword = New("weak_password")
	Expired      = New("expired")
)
