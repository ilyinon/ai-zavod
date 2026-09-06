# Zavod AI

Локальное desktop-приложение для macOS: AI-завод, где пользователь ставит задачу в чат, а роли-агенты превращают ее в требования, blueprint, код, проверки и ревью.

Стек:

- Wails v2
- Go
- React + TypeScript
- SQLite
- OpenAI-compatible LLM providers

## Возможности

- Новый чат одним кликом, без обязательного проекта или названия.
- Независимые чаты внутри проекта, поиск, закрепление, архив и переименование.
- Выбор рабочей папки, команды и модели прямо из чата.
- Локальный запуск на macOS.
- Мультипроектность: проекты по умолчанию лежат в `~/ai_zavod`.
- Чат с Markdown-ответами и копированием сообщений.
- Роли: Люмен, Продакт, Архитектор, Разработчик, Тестировщик, Ревьюер, ИБ-специалист.
- Autopilot workflow: задача, требования, blueprint, архитектура, разработка, проверки, ревью, итог.
- Подключение OpenAI-compatible моделей: OpenAI, удаленная Qwen, LM Studio, vLLM и похожие API.
- Web research для актуальной информации и ответов с источниками.
- Controlled project writes: код пишется только через структурированные proposed changes.
- Python-проекты запускаются через project-local `.venv` и `requirements.txt`.
- Диагностика проекта через реальные инструменты чтения, поиска и запуска проверок;
  журнал вызовов доступен рядом с диффом.

### Диагностика проекта

В чате с проектом выберите модель с function calling и отметьте разрешение диагностики
над полем ввода. Отправьте «разберись, почему падают тесты». Агент сможет прочитать
разрешённые файлы и выполнить `go test ./...`, `go vet ./...` или pytest в `.venv`,
если это допускает его профиль. Полный workflow и правки кода не запускаются.

Разрешение действует на один запрос: прочитанные файлы и логи получает выбранная модель.
Не используйте недоверенный endpoint для приватного кода. Проверки выполняют код проекта
с правами приложения, это не sandbox. Обычные файлы и логи могут содержать секреты,
даже при исключении `.env` и ключей. Подробности: [контракт Tool Runtime](docs/TOOL_RUNTIME_V1_0_6_1.md).

## Требования

- macOS, основной целевой режим разработки.
- Go `1.26+` для сборки самого Zavod AI.
- Node.js `24+`.
- npm.
- Wails CLI `v2.10+`.

Установка Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Проверка окружения:

```bash
wails doctor
go version
node --version
npm --version
```

## Быстрый старт

```bash
git clone <repo-url> zavod-ai
cd zavod-ai
make dev
```

После запуска откройте настройки моделей в приложении и добавьте provider.
Во вкладке "Интернет" можно включить/выключить web research, ограничить число источников и задать allowlist/blocklist доменов.

Пример OpenAI-compatible Qwen по сети:

```text
Name: Qwen по сети
Provider: OpenAI-compatible
Base URL: http://192.168.50.120:8000/v1
Model: Qwen36:coder
API key: optional
```

Пример OpenAI:

```text
Name: OpenAI ChatGPT
Provider: OpenAI-compatible
Base URL: https://api.openai.com/v1
Model: gpt-5-mini
API key: <OPENAI_API_KEY>
```

## Сборка и установка на macOS

Собрать production `.app`:

```bash
make app
```

Готовое приложение появится здесь:

```text
build/bin/Zavod AI.app
```

Открыть без установки:

```bash
open "build/bin/Zavod AI.app"
```

Установить в `/Applications`:

```bash
cp -R "build/bin/Zavod AI.app" /Applications/
open "/Applications/Zavod AI.app"
```

Если приложение уже было установлено раньше, замените старую копию:

```bash
rm -rf "/Applications/Zavod AI.app"
cp -R "build/bin/Zavod AI.app" /Applications/
open "/Applications/Zavod AI.app"
```

Если macOS блокирует запуск из-за quarantine/Gatekeeper после скачивания или копирования:

```bash
xattr -dr com.apple.quarantine "/Applications/Zavod AI.app"
open "/Applications/Zavod AI.app"
```

Собрать начисто:

```bash
make clean
make app
```

Проверить, что внутри `.app` есть исполняемый файл:

```bash
ls -la "build/bin/Zavod AI.app/Contents/MacOS/"
```

Ожидаемый бинарник:

```text
zavod-ai
```

Собрать `.dmg` для распространения:

```bash
make dmg
```

Готовый образ появится здесь:

```text
build/bin/Zavod-AI.dmg
```

Установка из `.dmg`:

1. Откройте `build/bin/Zavod-AI.dmg`.
2. Перетащите `Zavod AI.app` в `Applications`.
3. Запустите приложение из `/Applications`.

Если macOS блокирует приложение из `.dmg`, снимите quarantine уже с установленной копии:

```bash
xattr -dr com.apple.quarantine "/Applications/Zavod AI.app"
open "/Applications/Zavod AI.app"
```

## GitHub Actions

В репозитории есть workflow:

```text
.github/workflows/macos.yml
```

Он запускается на macOS для `push`, `pull_request` и ручного `workflow_dispatch`.

Что делает CI:

1. Устанавливает Go и Node.js 24.
2. Устанавливает Wails CLI.
3. Ставит frontend dependencies.
4. Запускает `npm run build --prefix frontend`.
5. Запускает `go test ./...`.
6. Собирает macOS-приложение через `wails build`.
7. Упаковывает `build/bin/Zavod AI.app` в `.dmg`.
8. Загружает `.dmg` как GitHub Actions artifact.

Workflow использует Node 24-compatible versions GitHub Actions:

- `actions/checkout@v6`
- `actions/setup-go@v7`
- `actions/setup-node@v7`
- `actions/upload-artifact@v7`

Скачать установочный образ можно в GitHub:

```text
Actions -> macOS -> нужный run -> Artifacts -> Zavod-AI-macOS-*-dmg
```

Приложение из CI не подписано Apple Developer ID. После установки из `.dmg` macOS может потребовать снять quarantine:

```bash
xattr -dr com.apple.quarantine "/Applications/Zavod AI.app"
open "/Applications/Zavod AI.app"
```

## Разработка

Основные команды:

```bash
make help       # список команд
make deps       # установить frontend dependencies
make dev        # dev-режим Wails
make test       # frontend build + go test ./...
make app        # собрать build/bin/Zavod AI.app
make dmg        # собрать build/bin/Zavod-AI.dmg
make build      # полный локальный build: test + .app + .dmg
make install    # установить .app в /Applications
make clean      # удалить build/bin и frontend/dist
```

Backend:

```bash
go test ./...
```

Frontend:

```bash
cd frontend
npm run build
```

Dev mode:

```bash
wails dev
```

Полная локальная проверка перед коммитом:

```bash
make build
```

На чистом checkout `go test ./...` нужно запускать после frontend build, потому что `main.go` встраивает `frontend/dist` через Go embed.

## Структура проекта

```text
.
├── app.go                 # Wails bindings
├── main.go                # Wails entrypoint
├── wails.json             # Wails config
├── frontend/              # React UI
├── internal/app/          # application service layer
├── internal/agents/       # agent specs and prompts
├── internal/artifacts/    # task specs and workflow artifacts
├── internal/blueprint/    # Task Blueprint contract
├── internal/changes/      # proposed changes, apply, diff
├── internal/checks/       # safe test runner
├── internal/llm/          # provider interfaces
├── internal/providers/    # provider implementations
├── internal/reviews/      # review parsing and status
├── internal/router/       # intent router
├── internal/store/        # SQLite store
└── internal/workflow/     # workflow states and steps
```

## Локальные данные

По умолчанию:

```text
~/ai_zavod                 # каталог пользовательских проектов
~/dev_ai_zavod/zavod.db    # SQLite база приложения
```

При обычной разработке `zavod.db`, `frontend/dist`, `frontend/node_modules` и `build/bin` не коммитятся.

## Workflow

Основной coding workflow:

1. Люмен принимает задачу и уточняет смысл.
2. Продакт формирует требования.
3. Архитектор создает Task Blueprint.
4. Архитектор пишет технический план.
5. Разработчик возвращает structured proposed changes.
6. Backend применяет изменения.
7. Тестировщик предлагает и запускает безопасные проверки.
8. Ревьюер принимает результат или возвращает задачу на repair-итерацию.

Autopilot ограничивает число repair-итераций, чтобы не уходить в бесконечный цикл.

## Python policy

Для Python-задач Zavod AI всегда использует project-local virtualenv:

- `requirements.txt` обязателен;
- если внешних зависимостей нет, файл содержит `# standard library only`;
- проверки запускаются через `.venv/bin/python <script.py>`;
- перед проверкой backend создает `.venv` и устанавливает зависимости из `requirements.txt`.

## Security policy

ИБ-специалист работает в defensive-first режиме:

- требует явный scope для активного pentest;
- не дает пошаговые инструкции атаки сторонней цели без разрешения;
- предпочитает threat model, hardening, безопасные проверки и remediation plan.

## Подготовка к публикации на GitHub

Перед первым push:

```bash
go test ./...
cd frontend
npm run build
cd ..
wails build
git status
```

Проверьте, что в коммит не попали:

- `zavod.db`
- `frontend/node_modules`
- `frontend/dist`
- `build/bin`
- секреты API keys

## Лицензия

Лицензию нужно выбрать перед публикацией. Для open source обычно подойдет `MIT` или `Apache-2.0`.
