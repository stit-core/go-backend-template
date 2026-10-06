package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// healthServer отвечает на GET /health статусом status, на прочие пути — 404.
func healthServer(t *testing.T, status int) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// unreachable — адрес, который никто не слушает: привилегированный порт не займёт чужой непривилегированный
// слушатель, как освобождённый порт тестового сервера.
const unreachable = "127.0.0.1:1"

func port(t *testing.T, addr string) string {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	return p
}

// Подкоманду разбирает main: тест запускает свой бинарь как `api healthcheck` (TestMain зовёт main) и
// смотрит код выхода процесса — проверка не стартует сервис и отвечает за здоровье того, что на HTTP_ADDR.
func TestMainHealthcheck(t *testing.T) {
	t.Parallel()
	live := healthServer(t, http.StatusOK)
	tests := []struct {
		name string
		addr string
		code int
	}{
		{"service answers", ":" + port(t, live), 0},
		{"nobody listens", unreachable, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0])
			cmd.Env = []string{mainArgsEnv + "=healthcheck", "HTTP_ADDR=" + tc.addr}

			out, err := cmd.CombinedOutput()
			require.NotNil(t, cmd.ProcessState, "api healthcheck did not run: %v", err)
			require.Equal(t, tc.code, cmd.ProcessState.ExitCode(), "exit code of api healthcheck, output:\n%s", out)
		})
	}
}

// Переменная перезапуска в окружении самого прогона тестов не подменяет их проверкой здоровья, даже если
// /health на HTTP_ADDR жив: иначе она молча выключила бы все тесты пакета в гейте.
func TestMainArgsEnvKeepsTests(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFxGraphValidates$", "-test.v")
	cmd.Env = []string{mainArgsEnv + "=healthcheck", "HTTP_ADDR=" + healthServer(t, http.StatusOK)}

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "output:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestFxGraphValidates", "the tests ran, not main")
}
