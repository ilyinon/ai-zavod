# Zavod AI Feature Roadmap

Дата: 2026-08-30

## Цель

Сделать Zavod AI локальным AI-заводом для:

- разработки скриптов и больших проектов на Python и Go;
- CTF и легитимных ИБ-задач в явно заданном scope;
- поиска в интернете, анализа источников и подготовки кратких выводов;
- ответов на вопросы по проекту, коду, логам, требованиям и общей логике.

Ключевой принцип: Люмен должна выбирать кратчайший корректный путь. Простые вопросы отвечаются сразу, инженерные задачи идут через spec-driven workflow, опасные/ИБ-задачи требуют scope и guardrails.

## Текущее состояние

Уже есть:

- локальное macOS-приложение на Wails + Go + React + SQLite;
- мультипроектность;
- чат с Markdown, копированием сообщений и внешним открытием ссылок;
- роли: Люмен, Продакт, Архитектор, Разработчик, Тестировщик, Ревьюер, ИБ-специалист;
- Autopilot workflow;
- Task Blueprint;
- structured proposed changes;
- автоматическое применение изменений;
- проверки и ревью;
- web research с источниками;
- Python `.venv` + `requirements.txt` policy;
- сборка `.app` и `.dmg`.

## Product Direction

### 1. Agent Core

Главный риск сейчас не отсутствие ролей, а качество управления задачей. Нужны:

- единая память задачи;
- строгий task contract;
- трассировка шагов;
- tool guardrails;
- нормальная диагностика, почему workflow остановился;
- защита от бесконечного ремонта.

### 2. Software Factory

Для Python и Go приложение должно уметь не только писать файл, а вести проект:

- scaffold проекта;
- зависимости;
- тесты;
- README;
- запуск;
- локальная диагностика;
- последующие изменения без разрушения структуры.

### 3. CTF / Security Lab

ИБ-агент должен работать в режиме разрешённых лабораторных задач:

- CTF challenge workspace;
- категории web/pwn/crypto/reverse/forensics;
- заметки, гипотезы, payload lab-only;
- запуск инструментов только из allowlist;
- явный scope перед активными проверками.

### 4. Research & Analytics

Web research должен стать не просто поиском, а исследовательским пайпом:

- источники;
- цитирование;
- дедупликация;
- доверие к источникам;
- свежесть;
- сравнение;
- короткий вывод.

## Roadmap

## V0.7.0 - Task Memory & Spec Store

### Цель

Завод должен помнить ответы пользователя, текущую спеку задачи и решения предыдущих шагов. Агент не должен повторно спрашивать то, что уже известно.

### Фичи

- `TaskSpec` как первая сущность задачи.
- Отдельные файлы:
  - `docs/task-spec.md` - человекочитаемая спека текущей задачи;
  - `.zavod/runs/<run>/context.json` - машинный контекст;
  - `.zavod/runs/<run>/decisions.md` - принятые решения;
  - `.zavod/runs/<run>/questions.json` - открытые/закрытые уточнения.
- Слияние ответов пользователя в spec store.
- Перед каждым агентом передавать актуальную спеку, а не сырой чат.

### Acceptance Criteria

- Если пользователь ответил на уточнения, Люмен больше не задаёт их повторно.
- По запросу `выведи спеку этого задания` Люмен показывает именно `docs/task-spec.md`.
- Служебные JSON/логи не показываются пользователю как “артефакты задачи”.

## V0.7.1 - Workflow Trace & Explainability

### Цель

Пользователь должен понимать, что сделал завод и где застрял, без чтения внутренних артефактов.

### Фичи

- Trace store: LLM call, tool call, file write, check, review, stop reason.
- Компактный UI:
  - текущий шаг;
  - последний meaningful event;
  - popover со всеми шагами;
  - подробный trace только по клику.
- Отдельная вкладка `Диагностика` для raw логов.

### Acceptance Criteria

- При остановке видно одну главную причину, а не список внутренних статусов.
- Если Autopilot исправил проблему позже, старые ошибки не участвуют в финальном статусе.

## V0.8.0 - Go Project Mode

### Цель

Нормальная разработка Go CLI/library/service проектов.

### Фичи

- Определение типа Go-проекта:
  - script-like CLI;
  - module CLI;
  - library;
  - HTTP service.
- Scaffold:
  - `go.mod`;
  - `main.go` или `cmd/<name>/main.go`;
  - `internal/*` при необходимости;
  - тесты;
  - README.
- Go runtime policy: `Go 1.25+`.
- Проверки:
  - `go test ./...`;
  - `go vet ./...`;
  - `gofmt`;
  - optional `golangci-lint`.
- Diff-aware developer: не перезаписывать весь файл без причины.

### Acceptance Criteria

- Для новой Go-задачи создаётся валидный `go.mod`.
- `go test ./...` применим и проходит либо даёт actionable ошибку.
- Ревьюер проверяет не только синтаксис, но и соответствие blueprint.

## V0.8.1 - Python Project Mode

### Цель

Нормальная разработка Python scripts/apps/bots с изолированным окружением.

### Фичи

- Всегда создавать/обновлять `requirements.txt`, если нужны зависимости.
- Всегда использовать project-local `.venv`.
- Scaffold:
  - single script;
  - package;
  - CLI через `argparse` или `typer`;
  - bot/service.
- Проверки:
  - `python3 -m venv .venv`;
  - `.venv/bin/python -m pip install -r requirements.txt`;
  - `.venv/bin/python -m pytest` если есть тесты;
  - `.venv/bin/python <entrypoint>` для smoke test.
- Secrets policy:
  - токены только через env;
  - `.env.example`;
  - `.gitignore`.

### Acceptance Criteria

- Python-бот не получает токен в коде.
- Тестировщик не запускает системный `python`, только `.venv/bin/python`.
- Если dependency уже в `requirements.txt`, ревьюер не требует “установить pip install” в ответе.

## V0.9.0 - CTF Workspace Mode

### Цель

Сделать удобный режим участия в CTF и лабораторных задачах.

### Фичи

- Тип проекта `ctf_workspace`.
- Категории:
  - web;
  - crypto;
  - pwn;
  - reverse;
  - forensics;
  - misc.
- Структура:
  - `challenge.yml`;
  - `notes.md`;
  - `solve/`;
  - `artifacts/`;
  - `evidence/`;
  - `writeup.md`.
- Для каждой задачи:
  - цель;
  - scope;
  - известные endpoints/files;
  - гипотезы;
  - попытки;
  - найденные флаги;
  - writeup.
- Импорт CTFd challenge metadata при наличии URL/token.

### Acceptance Criteria

- На вопрос по CTF Люмен не запускает общий software workflow, а создаёт CTF карточку.
- Для каждой категории есть стартовый checklist.
- Финальный результат можно экспортировать в `writeup.md`.

## V0.9.1 - Security Guardrails & Scope

### Цель

ИБ-функции должны быть полезными для CTF и разрешённых тестов, но не превращать приложение в неуправляемый атакующий инструмент.

### Фичи

- `SecurityScope`:
  - target;
  - owner/authorization;
  - allowed actions;
  - forbidden actions;
  - rate limits;
  - time window;
  - evidence path.
- Перед активным тестом требовать scope.
- Tool allowlist по категориям:
  - passive: `curl`, `dig`, `whois`, чтение файлов challenge;
  - web lab: `sqlmap` только при explicit scope;
  - pwn lab: `gdb`, `checksec`, `pwntools`;
  - crypto lab: Python scripts, Sage optional;
  - reverse: `file`, `strings`, `objdump`, `radare2` optional.
- Red-team режим только для локальных/CTF/lab целей.
- Safety stop reason вместо молчаливого отказа.

### Acceptance Criteria

- Если пользователь просит атаковать публичную цель без разрешения, Люмен просит scope.
- Если это CTF/lab, агент помогает полноценно.
- Все активные команды логируются в evidence.

## V1.0.0 - Research Desk

### Цель

Сделать поиск в интернете и аналитику устойчивыми.

### Фичи

- Несколько providers:
  - direct URL fetch;
  - deterministic APIs для погоды/валют;
  - RSS/search fallback;
  - пользовательские API-ключи для search provider.
- Research plan:
  - что ищем;
  - какие источники нужны;
  - freshness;
  - conflict detection.
- Source cards:
  - домен;
  - дата;
  - тип источника;
  - trust level;
  - короткая цитата/фрагмент.
- Ответ:
  - краткий вывод;
  - детали;
  - источники;
  - что не удалось подтвердить.

### Acceptance Criteria

- Коммерческие запросы не падают из-за пустого instant answer.
- Ответы по актуальным темам всегда содержат ссылки.
- Если источники конфликтуют, Люмен явно пишет расхождения.

## V1.1.0 - Project Q&A / Code Intelligence

### Цель

Ответы на вопросы по проекту без запуска полного pipeline.

### Фичи

- Index project files:
  - file tree;
  - symbols;
  - README/SPEC;
  - recent diffs.
- Intent router:
  - `direct_answer`;
  - `project_question`;
  - `implementation_task`;
  - `research_task`;
  - `ctf_task`;
  - `security_task`.
- Для вопросов вида `почему так работает?`, `где это реализовано?`, `покажи спеку` отвечать сразу.
- Source references в ответах: файл + строка.

### Acceptance Criteria

- Вопрос `что делает этот проект?` не запускает разработку.
- Вопрос `где хранится web settings?` отвечает с файлом и строкой.

## V1.2.0 - Tool Runtime & Sandbox Profiles

### Цель

Дать агентам инструменты, но управлять риском.

### Фичи

- Tool registry:
  - read file;
  - write proposed change;
  - run command;
  - web fetch;
  - package install;
  - CTF tools.
- Sandbox profiles:
  - `read_only`;
  - `code_write`;
  - `test_only`;
  - `ctf_lab`;
  - `security_passive`;
  - `security_active_requires_scope`.
- Approval policy:
  - auto;
  - ask;
  - deny.
- Timeouts, output limits, structured errors.

### Acceptance Criteria

- Агент не может случайно запустить команду вне профиля.
- Пользователь видит, почему команда разрешена или заблокирована.
- Нет бесконечного цикла tool calls.

## V1.3.0 - Multi-Model Routing

### Цель

Разные задачи должны уходить на разные модели.

### Фичи

- Model routing per role:
  - Люмен: быстрая модель;
  - Архитектор/Ревьюер: сильная модель;
  - Разработчик: coding model;
  - ИБ: модель с хорошим reasoning.
- Fallback model при недоступности.
- Health monitor.
- Cost/latency display.
- Per-task override.

### Acceptance Criteria

- Если Qwen недоступна, задача может перейти на OpenAI fallback.
- В UI видно, какая модель используется сейчас и почему.

## V1.4.0 - Long-Running Jobs & Queue

### Цель

Большие проекты не должны блокировать UI.

### Фичи

- Очередь задач.
- Background jobs.
- Pause/resume/cancel.
- Retry с checkpoint.
- Job timeline.
- Уведомления о завершении/падении.

### Acceptance Criteria

- Пользователь может запустить задачу, уйти в другой проект и вернуться.
- После перезапуска приложения workflow продолжает или корректно показывает checkpoint.

## V1.5.0 - Git Integration

### Цель

Сделать изменения управляемыми как в нормальной разработке.

### Фичи

- Git status.
- Branch per task.
- Commit message generation.
- Apply/revert file.
- Diff by workflow/run.
- PR description.

### Acceptance Criteria

- Каждая задача может быть оформлена как отдельная ветка.
- Можно откатить изменения конкретного run.

## V1.6.0 - Reports & Export

### Цель

Для исследования, CTF и разработки нужен красивый итог.

### Фичи

- Export:
  - Markdown;
  - HTML;
  - PDF optional.
- CTF writeup.
- Security report:
  - scope;
  - findings;
  - severity;
  - evidence;
  - remediation.
- Dev report:
  - files changed;
  - checks;
  - review;
  - known limitations.

### Acceptance Criteria

- После CTF задачи можно получить `writeup.md`.
- После ИБ анализа можно получить отчёт без внутренних JSON-артефактов.

## Recommended Priority

### Сначала

1. V0.7.0 Task Memory & Spec Store.
2. V0.7.1 Workflow Trace & Explainability.
3. V1.1.0 Project Q&A / Code Intelligence.

Почему: это исправляет повторные вопросы, неправильные workflow для простых вопросов и непрозрачные остановки.

### Затем

4. V0.8.0 Go Project Mode.
5. V0.8.1 Python Project Mode.
6. V1.2.0 Tool Runtime & Sandbox Profiles.

Почему: это делает разработку устойчивой и уменьшает количество сломанных файлов.

### Потом

7. V0.9.0 CTF Workspace Mode.
8. V0.9.1 Security Guardrails & Scope.
9. V1.0.0 Research Desk.

Почему: CTF/ИБ и research требуют хорошего tool runtime и прозрачного scope.

### После стабилизации

10. V1.3.0 Multi-Model Routing.
11. V1.4.0 Long-Running Jobs & Queue.
12. V1.5.0 Git Integration.
13. V1.6.0 Reports & Export.

## MVP Next Step

Следующий практический шаг: V0.7.0 + V0.7.1.

Минимальная реализация:

- добавить `task_specs` в SQLite;
- добавить `task_questions`;
- сохранять ответы пользователя как resolved facts;
- передавать агентам compact task memory;
- сделать команду/intent `выведи спеку этого задания`;
- показывать пользователю только task artifacts, а служебные артефакты держать в `.zavod`;
- добавить trace events и нормальный stop reason.

## References

- OpenAI Agents SDK описывает агентов как модель с instructions, tools, handoffs, guardrails и structured outputs: https://openai.github.io/openai-agents-python/agents/
- OpenAI Agents SDK guardrails: input/output/tool guardrails и tripwire-поведение: https://openai.github.io/openai-agents-js/guides/guardrails/
- OpenAI Agents SDK tracing: trace/span модель для LLM calls, tool calls, handoffs и guardrails: https://openai.github.io/openai-agents-js/guides/tracing/
- NIST SSDF SP 800-218: практики secure software development: https://csrc.nist.gov/pubs/sp/800/218/final
- OWASP Web Security Testing Guide: база для web security/CTF чеклистов: https://owasp.org/www-project-web-security-testing-guide/
- CTFd API docs и challenge workflow: https://docs.ctfd.io/docs/api/getting-started/
- CTFd `challenge.yml` формат через ctfcli: https://docs.ctfd.io/docs/management/ctfcli/challenges/
