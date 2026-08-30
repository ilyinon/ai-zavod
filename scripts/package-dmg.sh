#!/usr/bin/env sh
set -eu

APP_PATH="${1:-build/bin/Zavod AI.app}"
DMG_PATH="${2:-build/bin/Zavod-AI.dmg}"
VOL_NAME="${3:-Zavod AI}"

if [ ! -d "$APP_PATH" ]; then
  echo "App bundle not found: $APP_PATH" >&2
  echo "Run 'wails build' first." >&2
  exit 1
fi

if ! command -v hdiutil >/dev/null 2>&1; then
  echo "hdiutil not found. DMG packaging is supported on macOS only." >&2
  exit 1
fi

DMG_DIR=$(dirname "$DMG_PATH")
mkdir -p "$DMG_DIR"

STAGING_DIR=$(mktemp -d "${TMPDIR:-/tmp}/zavod-ai-dmg.XXXXXX")
cleanup() {
  rm -rf "$STAGING_DIR"
}
trap cleanup EXIT INT TERM

cp -R "$APP_PATH" "$STAGING_DIR/"
ln -s /Applications "$STAGING_DIR/Applications"

rm -f "$DMG_PATH"
hdiutil create \
  -volname "$VOL_NAME" \
  -srcfolder "$STAGING_DIR" \
  -ov \
  -format UDZO \
  "$DMG_PATH"

echo "DMG created: $DMG_PATH"
