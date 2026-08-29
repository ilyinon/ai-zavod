#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
FRONTEND_DIR="$ROOT_DIR/frontend"
CACHE_ROOT="${TMPDIR:-/tmp}/zavod-ai-node"
CACHE_NODE_MODULES="$CACHE_ROOT/node_modules"

mkdir -p "$CACHE_ROOT"

if [ -L "$FRONTEND_DIR/node_modules" ] && [ -d "$FRONTEND_DIR/node_modules" ]; then
  exit 0
fi

if [ -d "$FRONTEND_DIR/node_modules" ] && [ ! -L "$FRONTEND_DIR/node_modules" ]; then
  mv "$FRONTEND_DIR/node_modules" "$CACHE_ROOT/node_modules.$(date +%s)"
fi

if [ ! -d "$CACHE_NODE_MODULES" ]; then
  npm install --prefix "$FRONTEND_DIR"
  mv "$FRONTEND_DIR/node_modules" "$CACHE_NODE_MODULES"
fi

if [ ! -e "$FRONTEND_DIR/node_modules" ]; then
  ln -s "$CACHE_NODE_MODULES" "$FRONTEND_DIR/node_modules"
fi

