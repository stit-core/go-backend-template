#!/usr/bin/env bash
# install-tools.sh — поставить pinned-версии dev-инструментов.
# Все версии должны быть pinned. При добавлении CI — синхронизируй.
set -euo pipefail

GOFUMPT_VERSION="${GOFUMPT_VERSION:-v0.10.0}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.12.2}"
SQLC_VERSION="${SQLC_VERSION:-v1.31.1}"
MOCKERY_VERSION="${MOCKERY_VERSION:-v3.7.0}"
# logcli pinned to match Loki server version in stit-infra/infra (compose/loki.yml).
LOGCLI_VERSION="${LOGCLI_VERSION:-v3.7.1}"

GOBIN="$(go env GOPATH)/bin"
mkdir -p "$GOBIN"

# golibinfra (и прочие stit-private deps) хостятся на git.stit.tech, который не
# зарегистрирован в sum.golang.org / proxy.golang.org. Без GOPRIVATE Go идёт
# через proxy → 404 → direct VCS → запрос пароля → fail. SSH-rewrite для
# git.stit.tech ставится разово вручную: `git config --global url.git@git.stit.
# tech:.insteadOf https://git.stit.tech/` (см. README setup).
GOPRIVATE_PIN='git.stit.tech/*'
CURRENT_GOPRIVATE="$(go env GOPRIVATE)"
if [ "$CURRENT_GOPRIVATE" != "$GOPRIVATE_PIN" ]; then
  echo "→ go env -w GOPRIVATE=$GOPRIVATE_PIN (было: '$CURRENT_GOPRIVATE')"
  go env -w "GOPRIVATE=$GOPRIVATE_PIN"
else
  echo "✓ GOPRIVATE=$GOPRIVATE_PIN уже задан"
fi

# Без insteadOf-rewrite Go (с уже выставленным GOPRIVATE) попробует
# direct HTTPS на git.stit.tech, упрётся в auth-prompt и упадёт.
# git config --global здесь не правим (one-time per-developer setup);
# только warn если ещё не настроено.
if ! git config --global --get-regexp '^url\..*\.insteadof$' 2>/dev/null \
    | grep -q 'git.stit.tech'; then
  echo "⚠ git config: SSH-rewrite для git.stit.tech не настроен."
  echo "  Без него go mod download для приватных deps упадёт. Выполни один раз:"
  echo "    git config --global url.git@git.stit.tech:.insteadOf https://git.stit.tech/"
fi

go_install() {
  local pkg="$1" name
  name="$(basename "${pkg%@*}")"
  if command -v "$name" >/dev/null 2>&1 && "$name" --version 2>/dev/null | grep -q "${pkg##*@}"; then
    echo "✓ $name (${pkg##*@}) уже установлен"
    return
  fi
  echo "→ installing $pkg"
  GOBIN="$GOBIN" go install "$pkg"
}

go_install "mvdan.cc/gofumpt@${GOFUMPT_VERSION}"
go_install "github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION}"
go_install "github.com/vektra/mockery/v3@${MOCKERY_VERSION}"

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
