#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
FRONTEND_DIR="$ROOT_DIR/frontend"
CACHE_ROOT="${TMPDIR:-/tmp}/zavod-ai-node"
CACHE_NODE_MODULES="$CACHE_ROOT/node_modules"

mkdir -p "$CACHE_ROOT"

if [ -d "$FRONTEND_DIR/node_modules" ] && [ ! -L "$FRONTEND_DIR/node_modules" ]; then
  mv "$FRONTEND_DIR/node_modules" "$CACHE_ROOT/node_modules.$(date +%s)"
fi

if [ ! -d "$CACHE_NODE_MODULES" ] || ! cmp -s "$FRONTEND_DIR/package-lock.json" "$CACHE_ROOT/installed-package-lock.json"; then
  cp "$FRONTEND_DIR/package.json" "$FRONTEND_DIR/package-lock.json" "$CACHE_ROOT/"
  (
    cd "$CACHE_ROOT"
    npm ci --workspaces=false
  )
  cp "$FRONTEND_DIR/package-lock.json" "$CACHE_ROOT/installed-package-lock.json"
fi

if [ ! -e "$FRONTEND_DIR/node_modules" ]; then
  ln -s "$CACHE_NODE_MODULES" "$FRONTEND_DIR/node_modules"
fi
