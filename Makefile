SHELL := /bin/sh

APP_NAME := Zavod AI
APP_PATH := build/bin/$(APP_NAME).app
DMG_PATH := build/bin/Zavod-AI.dmg
GO ?= go
GOCACHE ?= $(CURDIR)/.cache/go-build
WAILS ?= $(shell command -v wails 2>/dev/null || printf "%s" "$$HOME/go/bin/wails")

.PHONY: help deps frontend-build test dev app dmg build open install reinstall clean doctor

help:
	@echo "Zavod AI"
	@echo ""
	@echo "Targets:"
	@echo "  make deps            Install frontend dependencies"
	@echo "  make dev             Start Wails dev mode"
	@echo "  make test            Build frontend and run Go tests"
	@echo "  make app             Build macOS .app"
	@echo "  make dmg             Build macOS .dmg"
	@echo "  make build           Run full local build: test + app + dmg"
	@echo "  make open            Open built .app"
	@echo "  make install         Copy built .app to /Applications"
	@echo "  make reinstall       Rebuild and copy .app to /Applications"
	@echo "  make clean           Remove build outputs"
	@echo "  make doctor          Run Wails environment diagnostics"

deps:
	sh scripts/frontend-install.sh

frontend-build: deps
	npm run build --prefix frontend

test: frontend-build
	GOCACHE="$(GOCACHE)" $(GO) test ./...

dev: deps
	$(WAILS) dev

app: test
	GOCACHE="$(GOCACHE)" $(WAILS) build

dmg: app
	sh scripts/package-dmg.sh "$(APP_PATH)" "$(DMG_PATH)"

build: dmg

open:
	open "$(APP_PATH)"

install: app
	rm -rf "/Applications/$(APP_NAME).app"
	cp -R "$(APP_PATH)" /Applications/
	xattr -dr com.apple.quarantine "/Applications/$(APP_NAME).app" 2>/dev/null || true
	open "/Applications/$(APP_NAME).app"

reinstall: install

clean:
	rm -rf build/bin frontend/dist

doctor:
	$(WAILS) doctor
