# Zavod AI

Локальное desktop-приложение для macOS: AI-завод с проектами, чатом, агентом-менеджером и подключаемыми OpenAI-compatible моделями.

## V0.1

- Wails + Go + React + SQLite.
- Проекты по умолчанию: `~/ai_zavod`.
- Код и база: `~/dev_ai_zavod`, `~/dev_ai_zavod/zavod.db`.
- Один агент: `Менеджер`.
- Провайдер моделей: OpenAI-compatible `/v1/chat/completions`.
- Поддержка OpenAI и удаленной Qwen по сети через `base_url`.

## V0.2

- Несколько конфигураций моделей.
- Типы моделей: OpenAI, Remote Qwen, OpenAI-compatible.
- Выбор активной модели.
- Проверка подключения модели с latency и статусом.
- Статусы модели: `unknown`, `checking`, `online`, `offline`.
- Streaming-ответы менеджера в чат через Wails events.

## Запуск

```bash
cd ~/dev_ai_zavod
sh scripts/frontend-install.sh
wails dev
```

Если `wails` CLI еще не установлен:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```
