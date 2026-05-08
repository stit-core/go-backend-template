#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"
SRC_DIR="$REPO_ROOT/scripts/git-hooks"

mkdir -p "$HOOKS_DIR"

ln -sf "$SRC_DIR/pre-commit" "$HOOKS_DIR/pre-commit"
chmod +x "$SRC_DIR/pre-commit"

echo "Git hooks installed:"
echo "  $HOOKS_DIR/pre-commit -> $SRC_DIR/pre-commit"
