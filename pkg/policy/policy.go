package policy

type Decision struct {
	IsAllowed bool
	Reason    string
}

func Allow() Decision             { return Decision{IsAllowed: true} }
func Deny(reason string) Decision { return Decision{IsAllowed: false, Reason: reason} }

// ForbiddenError — отказ policy; Reason — причина решения, клиенту не отдаётся.
type ForbiddenError struct{ Reason string }

func (e *ForbiddenError) Error() string {
	if e.Reason == "" {
		return "forbidden"
	}
	return "forbidden: " + e.Reason
}

// minPasswordLen — правило пароля продукта: одно для всех доменов, где пароль задаётся.
const minPasswordLen = 8

func PasswordStrong(plain string) bool { return len(plain) >= minPasswordLen }
