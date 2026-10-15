package verification

import "time"

// Config — настройки домена из окружения: его собирает модуль домена из *config.App.
type Config struct {
	// ResetMailPause — после ссылки сброса учётная запись столько новых писем сброса не получает.
	ResetMailPause time.Duration
}
