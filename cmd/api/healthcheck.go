package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"git.stit.tech/stit-core/golibinfra/httpkit"
	"git.stit.tech/stit-core/golibinfra/logkit"
)

// healthcheckTimeout: /health отвечает без проб зависимостей — дольше отвечает только зависший процесс.
const healthcheckTimeout = 3 * time.Second

// healthcheck — `api healthcheck`, проверка здоровья контейнера: в образе нет shell и wget, бинарь
// сам спрашивает /health своего HTTP_ADDR; код выхода 0 — сервис ответил 200.
func healthcheck(ctx context.Context) int {
	log, err := logkit.New(logkit.Config{Level: "info"})
	if err != nil {
		return 1
	}
	cfg, err := httpkit.NewConfig()
	if err == nil {
		err = checkHealth(ctx, cfg.Addr)
	}
	if err != nil {
		log.Error(ctx, "healthcheck failed", "err", err)
		return 1
	}
	return 0
}

// checkHealth: сервер на адресе без хоста или на 0.0.0.0 слушает и loopback — туда и идёт запрос.
func checkHealth(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR %q: %w", addr, err)
	}
	if host == "" || net.ParseIP(host).IsUnspecified() {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, port) + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return nil
}
