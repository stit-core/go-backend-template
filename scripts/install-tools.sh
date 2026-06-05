#!/usr/bin/env bash
# install-tools.sh — поставить pinned-версии dev-инструментов.
# Все версии должны быть pinned. При добавлении CI — синхронизируй.
set -euo pipefail

GOFUMPT_VERSION="${GOFUMPT_VERSION:-v0.10.0}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.12.2}"
SQLC_VERSION="${SQLC_VERSION:-v1.31.1}"
MOCKERY_VERSION="${MOCKERY_VERSION:-v3.7.0}"
# logcli pinned to match Loki server version in corp-infra-template (compose/loki.yml).
LOGCLI_VERSION="${LOGCLI_VERSION:-v3.7.1}"

GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"

# Хост приватных deps берём из module path в go.mod — после /de-template форк
# может жить на своей Gitea, а не на git.sftbmb.ru. golibinfra и прочие приватные
# модули с этого хоста не зарегистрированы в sum/proxy.golang.org: без GOPRIVATE
# Go идёт через proxy → 404 → direct VCS → запрос пароля → fail.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_PATH="$(awk '/^module /{print $2; exit}' "$ROOT/go.mod" 2>/dev/null || true)"
MODULE_HOST="${MODULE_PATH%%/*}"
: "${MODULE_HOST:=git.sftbmb.ru}"   # fallback, если go.mod ещё не прочитан

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

go_install() {
  local pkg="$1" name="${2:-}"
  # basename даёт неверное имя для версионированных module-path'ов
  # (mockery/v3 → "v3"); для таких передаём имя бинаря вторым аргументом.
  [ -z "$name" ] && name="$(basename "${pkg%@*}")"
  if command -v "$name" >/dev/null 2>&1 && "$name" --version 2>/dev/null | grep -q "${pkg##*@}"; then
    echo "✓ $name (${pkg##*@}) уже установлен"
    return
  fi
  echo "→ installing $pkg"
  GOBIN="$GOBIN" go install "$pkg"
}

go_install "mvdan.cc/gofumpt@${GOFUMPT_VERSION}"
go_install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}"
go_install "github.com/vektra/mockery/v3@${MOCKERY_VERSION}" "mockery"

# golangci-lint ставится через свой installer (статическая сборка,
# не через `go install`). Сравниваем версию — апгрейдим если pin изменился.
GOLANGCI_LINT_PIN="${GOLANGCI_LINT_VERSION#v}"
if command -v golangci-lint >/dev/null 2>&1 \
   && golangci-lint --version 2>/dev/null | grep -q "version $GOLANGCI_LINT_PIN "; then
  echo "✓ golangci-lint ($GOLANGCI_LINT_VERSION) уже установлен"
else
  echo "→ installing golangci-lint ${GOLANGCI_LINT_VERSION}"
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
    | sh -s -- -b "$GOBIN" "${GOLANGCI_LINT_VERSION}"
fi

# logcli ставится из release-зипа grafana/loki (Go-бинарь, но с CGO-зависимостями,
# через `go install` собирается не всегда чисто). Версия совпадает с Loki-сервером.
LOGCLI_PIN="${LOGCLI_VERSION#v}"
if command -v logcli >/dev/null 2>&1 \
   && logcli --version 2>/dev/null | grep -qE "(version|GitVersion).+$LOGCLI_PIN"; then
  echo "✓ logcli ($LOGCLI_VERSION) уже установлен"
else
  case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)   LOGCLI_ASSET="logcli-linux-amd64.zip" ;;
    Linux-aarch64)  LOGCLI_ASSET="logcli-linux-arm64.zip" ;;
    Darwin-x86_64)  LOGCLI_ASSET="logcli-darwin-amd64.zip" ;;
    Darwin-arm64)   LOGCLI_ASSET="logcli-darwin-arm64.zip" ;;
    *) echo "✗ logcli: неподдерживаемая платформа $(uname -s)-$(uname -m)"; exit 1 ;;
  esac
  echo "→ installing logcli ${LOGCLI_VERSION} ($LOGCLI_ASSET)"
  TMPDIR=$(mktemp -d)
  trap 'rm -rf "$TMPDIR"' EXIT
  curl -sSfL -o "$TMPDIR/logcli.zip" \
    "https://github.com/grafana/loki/releases/download/${LOGCLI_VERSION}/${LOGCLI_ASSET}"
  unzip -q -o "$TMPDIR/logcli.zip" -d "$TMPDIR"
  # Внутри зипа бинарь называется logcli-<os>-<arch>; переименовываем.
  install -m 0755 "$TMPDIR"/logcli-* "$GOBIN/logcli"
fi

echo
echo "Готово. Проверь, что \$GOBIN ($GOBIN) в \$PATH:"
echo "  echo \$PATH | grep -q '$GOBIN' && echo OK || echo 'добавь export PATH=\$PATH:$GOBIN'"
