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

// MinPasswordLen — правило пароля продукта для всех доменов; тег minLength HTTP-схем паролей
// повторяет его, handler-тесты держат их равными.
const MinPasswordLen = 8

func PasswordStrong(plain string) bool { return len(plain) >= MinPasswordLen }
