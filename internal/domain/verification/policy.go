package verification

import "time"

// Сроки жизни ссылок из писем: подтверждение email ждёт пользователя сутки, сброс пароля — час. Истёкшую сутки назад
// ссылку удаляет job: запрос в полёте старше часа ссылки не получает.
const (
	emailVerificationTTL = 24 * time.Hour
	passwordResetTTL     = time.Hour
	linkRetention        = 24 * time.Hour
)
