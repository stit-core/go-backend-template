#!/usr/bin/env bash
# install-tools.sh — поставить pinned-версии dev-инструментов.
# Все версии должны быть pinned. При добавлении CI — синхронизируй.
set -euo pipefail

GOFUMPT_VERSION="${GOFUMPT_VERSION:-v0.10.0}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.12.2}"
SQLC_VERSION="${SQLC_VERSION:-v1.31.1}"
MOCKERY_VERSION="${MOCKERY_VERSION:-v3.7.0}"
# logcli pinned to match Loki server version in stit/infra (compose/loki.yml).
LOGCLI_VERSION="${LOGCLI_VERSION:-v3.7.1}"

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
