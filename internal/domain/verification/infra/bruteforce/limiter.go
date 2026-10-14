// Package bruteforce — порт ResetLimiter домена verification на bruteforcekit.Guard.
package bruteforce

import (
	"context"
	"strings"

	"git.stit.tech/stit-core/go-backend-template/internal/domain/verification"
	"git.stit.tech/stit-core/golibinfra/bruteforcekit"
)

// subjectPrefix отделяет счётчик запросов сброса от счётчика входа (email) в общем хранилище guard'а.
const subjectPrefix = "password_reset:"

type resetLimiter struct {
	guard bruteforcekit.Guard
}

func NewResetLimiter(g bruteforcekit.Guard) verification.ResetLimiter {
	return &resetLimiter{guard: g}
}

// Allow: запрос сброса — попытка guard'а без успеха: после BRUTEFORCE_MAX_FAILURES запросов подряд
// пара (субъект, IP) ждёт BRUTEFORCE_LOCK_DURATION.
func (l *resetLimiter) Allow(ctx context.Context, subject, ip string) (bool, error) {
	// Guard обрезает пробелы по краям ключа целиком: у субъекта за приставкой их снимаем сами, иначе адрес
	// с пробелом впереди — свой счётчик.
	lock, err := l.guard.Attempt(ctx, subjectPrefix+strings.TrimSpace(subject), ip)
	if err != nil {
		return false, err
	}
	return lock == nil, nil
}
