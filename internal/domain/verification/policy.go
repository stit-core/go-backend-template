package verification

import "time"

// Сроки жизни ссылок из писем: подтверждение email ждёт пользователя сутки, сброс пароля — час.
const (
	emailVerificationTTL = 24 * time.Hour
	passwordResetTTL     = time.Hour
)
