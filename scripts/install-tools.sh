#!/usr/bin/env bash
# install-tools.sh — поставить закреплённые версии dev-инструментов (scripts/tools/versions.sh): инструмент, у
# которого стоит закреплённая, не трогается — сеть нужна только тому, чья версия не та.
set -euo pipefail

GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"

# Хост приватных deps берём из module path в go.mod — после /de-template форк
# может жить на своей Gitea, а не на git.stit.tech. golibinfra и прочие приватные
# модули с этого хоста не зарегистрированы в sum/proxy.golang.org: без GOPRIVATE
# Go идёт через proxy → 404 → direct VCS → запрос пароля → fail.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_PATH="$(awk '/^module /{print $2; exit}' "$ROOT/go.mod" 2>/dev/null || true)"
MODULE_HOST="${MODULE_PATH%%/*}"
: "${MODULE_HOST:=git.stit.tech}"   # fallback, если go.mod ещё не прочитан

# Append, не overwrite: сохраняем существующий GOPRIVATE (у разработчика могут
# быть другие приватные модули), добавляем свой хост только если его там нет.
GOPRIVATE_PIN="${MODULE_HOST}/*"
CURRENT_GOPRIVATE="$(go env GOPRIVATE)"
if printf '%s' ",${CURRENT_GOPRIVATE}," | grep -qF ",${GOPRIVATE_PIN},"; then
  echo "✓ GOPRIVATE уже содержит $GOPRIVATE_PIN"
else
  NEW_GOPRIVATE="${CURRENT_GOPRIVATE:+$CURRENT_GOPRIVATE,}$GOPRIVATE_PIN"
  echo "→ go env -w GOPRIVATE=$NEW_GOPRIVATE (append $GOPRIVATE_PIN)"
  go env -w "GOPRIVATE=$NEW_GOPRIVATE"
fi

# Без insteadOf-rewrite Go (с уже выставленным GOPRIVATE) попробует
# direct HTTPS на $MODULE_HOST, упрётся в auth-prompt и упадёт.
# git config --global здесь не правим (one-time per-developer setup);
# только warn если ещё не настроено.
if ! git config --global --get-regexp '^url\..*\.insteadof$' 2>/dev/null \
    | grep -qF "$MODULE_HOST"; then
  echo "⚠ git config: SSH-rewrite для $MODULE_HOST не настроен."
  echo "  Без него go mod download для приватных deps упадёт. Выполни один раз:"
  echo "    git config --global url.git@$MODULE_HOST:.insteadOf https://$MODULE_HOST/"
fi

versions() { bash "$ROOT/scripts/tools/versions.sh" "$@"; }

# go_install <инструмент> <пакет без версии>
go_install() {
  local tool=$1 pkg=$2 pin
  pin=$(versions --pin "$tool")
  if versions "$tool" >/dev/null; then
    echo "✓ $tool ($pin) уже установлен"
    return
  fi
  echo "→ installing $pkg@$pin"
  GOBIN="$GOBIN" go install "$pkg@$pin"
}

go_install gofumpt mvdan.cc/gofumpt
go_install sqlc github.com/sqlc-dev/sqlc/cmd/sqlc
go_install mockery github.com/vektra/mockery/v3
go_install govulncheck golang.org/x/vuln/cmd/govulncheck

# golangci-lint ставится через свой installer (статическая сборка, не через `go install`), взятый с тега той же
# версии, а не с HEAD.
GOLANGCI_LINT_PIN=$(versions --pin golangci-lint)
if versions golangci-lint >/dev/null; then
  echo "✓ golangci-lint ($GOLANGCI_LINT_PIN) уже установлен"
else
  echo "→ installing golangci-lint $GOLANGCI_LINT_PIN"
  curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/$GOLANGCI_LINT_PIN/install.sh" \
    | sh -s -- -b "$GOBIN" "$GOLANGCI_LINT_PIN"
fi

echo
echo "Готово. Проверь, что \$GOBIN ($GOBIN) в \$PATH:"
echo "  echo \$PATH | grep -q '$GOBIN' && echo OK || echo 'добавь export PATH=\$PATH:$GOBIN'"
