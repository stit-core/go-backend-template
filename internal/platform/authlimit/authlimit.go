// Package authlimit — строгий лимит клиента на пути входа, регистрации, сессии и паролей: Argon2id процесса считает
// общая очередь, и поток одного клиента в пределах общего лимита httpkit занял бы её для всех.
package authlimit

import (
	"errors"
	"net/http"
	"strings"

	"git.stit.tech/stit-core/golibinfra/httpkit"
)

// Prefix — пути под строгим лимитом: их операции объявляют его отказ правилом Rule в своей таблице.
const Prefix = "/api/auth/"

// Config — token-bucket на клиента (IP, как у лимитера httpkit); RPS = 0 выключает лимит: его держит край.
type Config struct {
	RPS   int `default:"5"  envconfig:"HTTP_AUTH_RATE_LIMIT_RPS"`
	Burst int `default:"10" envconfig:"HTTP_AUTH_RATE_LIMIT_BURST"`
}

// errLimited — отказ лимита: его отдаёт Middleware, а не хендлер — Rule только объявляет код в OpenAPI.
var errLimited = errors.New("authlimit: rate limit exceeded")

// Rule — отказ Middleware в таблице операции под Prefix: код и статус лимитера httpkit.
var Rule = httpkit.ErrorRule{Err: errLimited, Status: http.StatusTooManyRequests, Code: "RATE_LIMITED", Detail: "rate limit exceeded"}

// Middleware — лимит клиента на пути под Prefix поверх общего лимита httpkit; остальные пути он не считает.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	limit := httpkit.RateLimit(httpkit.RateLimitConfig{RPS: cfg.RPS, Burst: cfg.Burst})
	return func(next http.Handler) http.Handler {
		limited := limit(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, Prefix) {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
