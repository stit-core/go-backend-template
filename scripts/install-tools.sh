#!/usr/bin/env bash
# install-tools.sh — поставить pinned-версии dev-инструментов.
# Все версии должны быть pinned. При добавлении CI — синхронизируй.
set -euo pipefail

GOFUMPT_VERSION="${GOFUMPT_VERSION:-v0.6.0}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v1.62.2}"
SQLC_VERSION="${SQLC_VERSION:-v1.31.1}"
MOCKERY_VERSION="${MOCKERY_VERSION:-v3.7.0}"

GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"

install() {
  local pkg="$1" name
  name="$(basename "${pkg%@*}")"
  if command -v "$name" >/dev/null 2>&1 && "$name" --version 2>/dev/null | grep -q "${pkg##*@}"; then
    echo "✓ $name (${pkg##*@}) уже установлен"
    return
  fi
  echo "→ installing $pkg"
  GOBIN="$GOBIN" go install "$pkg"
}

install "mvdan.cc/gofumpt@${GOFUMPT_VERSION}"
install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}"
install "github.com/vektra/mockery/v3@${MOCKERY_VERSION}"

# golangci-lint ставится через свой installer (статическая сборка,
# не через `go install`).
if command -v golangci-lint >/dev/null 2>&1; then
  echo "✓ golangci-lint уже установлен ($(golangci-lint --version | head -1))"
else
  echo "→ installing golangci-lint ${GOLANGCI_LINT_VERSION}"
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
    | sh -s -- -b "$GOBIN" "${GOLANGCI_LINT_VERSION}"
fi

echo
echo "Готово. Проверь, что \$GOBIN ($GOBIN) в \$PATH:"
echo "  echo \$PATH | grep -q '$GOBIN' && echo OK || echo 'добавь export PATH=\$PATH:$GOBIN'"
