# Hermes-like Teams Spec for Zavod AI

Дата: 2026-09-05

## Цель

Прокачать Zavod AI от фиксированного набора ролей к системе настраиваемых групп агентов:

- пользователь создаёт группы под разные режимы работы;
- в группу добавляются агенты с ролью, моделью, инструментами и личностью;
- для группы задаётся жизненный цикл: какие шаги идут, кто их выполняет, когда повторять, когда останавливаться;
- у каждого агента есть `soul.md`;
- у проекта есть контекстные инструкции;
- основные направления первого релиза: разработка и CTF.

## Почему так

Сейчас Zavod AI похож на один жёстко прошитый конвейер. Это удобно для MVP, но мешает:

- делать разные команды под разные задачи;
- настраивать роли без изменения Go-кода;
- иметь разные модели у разных агентов;
- запускать CTF workflow отдельно от разработки;
- хранить “личность” и рабочие привычки агента как редактируемый файл.

Нужна новая абстракция: `AgentGroup`.

## Концепция

```text
Project
  -> AgentGroup
    -> Agents
      -> soul.md
      -> tools
      -> model routing
    -> Lifecycle
      -> steps
      -> transitions
      -> stop rules
      -> repair policy
```

Пользователь больше не выбирает “один встроенный завод”. Он выбирает группу:

- `Dev Squad` для разработки;
- `CTF Cell` для CTF;
- позже: `Research Desk`, `Security Audit`, `Docs Team`.

## Основные сущности

### AgentGroup

```text
id
name
kind                 dev | ctf | research | security | custom
description
default_model_id
status               active | archived
created_at
updated_at
```

### AgentProfile

```text
id
group_id
name                 Люмен, Архитектор, Реверсер, Криптограф
role_key             intake, product, architect, developer, tester, reviewer, security, custom
short_name
avatar_path
soul_path
model_id
tool_profile_id
temperature
context_budget
enabled
sort_order
```

### AgentSoul

Хранится как обычный Markdown-файл:

```text
~/dev_ai_zavod/agents/<group>/<agent>/soul.md
```

Содержит:

- кто агент;
- стиль общения;
- область ответственности;
- что агент никогда не делает;
- как агент принимает решения;
- как агент передаёт задачу дальше.

`soul.md` не должен содержать проектные инструкции. Проектные инструкции живут отдельно.

### ProjectContext

Для каждого проекта завод ищет контекстные файлы:

```text
.zavod/PROJECT.md
AGENTS.md
README.md
SPEC.md
.cursorrules
.cursor/rules/*.mdc
```

Правило:

- `soul.md` = личность агента;
- project context = правила конкретного проекта;
- task spec = требования конкретной задачи.

### Lifecycle

```text
id
group_id
name
kind                 dev | ctf | custom
max_iterations
stop_policy
created_at
updated_at
```

### LifecycleStep

```text
id
lifecycle_id
step_key
title
agent_profile_id
mode                 llm | tool | human_gate | branch | review | final
required
can_retry
max_retries
on_success
on_failure
input_template
output_schema
sort_order
```

## Направление 1: Dev Factory

### Цель

Делать скрипты и большие проекты на Python/Go устойчиво, не ломая файлы и не гоняя пользователя по кнопкам.

### Рекомендуемая группа

`Dev Squad`

Агенты:

- `Люмен` — intake/router/spec keeper;
- `Продакт` — требования и acceptance criteria;
- `Архитектор` — blueprint, структура проекта, риски;
- `Разработчик` — proposed changes;
- `Тестировщик` — команды, venv/go test/build;
- `Ревьюер` — обязательный diff review;
- `Докер` — README, usage, install notes;
- `Релизер` — changelog, git branch, commit message, package.

### Lifecycle

```text
1. intake
2. requirements
3. task_blueprint
4. architecture
5. implementation
6. apply_changes
7. checks
8. review
9. repair_loop
10. final_summary
```

### Repair Loop

Ревьюер может вернуть задачу:

- к `requirements`, если неверно понята задача;
- к `task_blueprint`, если неверный стек/scaffold/files;
- к `architecture`, если неверный технический план;
- к `implementation`, если ошибка в коде;
- к `checks`, если проверки выбраны неверно.

Ограничения:

```text
max_repair_iterations = 3
same_error_limit = 2
no_progress_limit = 2
```

Если ошибка повторяется без прогресса, workflow останавливается с понятной причиной.

### Dev Tool Profiles

`go_dev`:

- read files;
- write proposed changes;
- apply changes;
- `gofmt`;
- `go test ./...`;
- `go vet ./...`;
- `go mod tidy`;
- `make test`;
- `make app` только для проекта Zavod AI.

`python_dev`:

- read files;
- write proposed changes;
- apply changes;
- `python3 -m venv .venv`;
- `.venv/bin/python -m pip install -r requirements.txt`;
- `.venv/bin/python -m pytest`;
- `.venv/bin/python <entrypoint>`;
- запрещён системный `python`.

### Acceptance Criteria

- Пользователь может создать Dev-группу через UI.
- У каждого агента можно выбрать модель.
- У каждого агента редактируется `soul.md`.
- Go-задачи создают `go.mod`, если его нет.
- Python-задачи создают `requirements.txt` и `.venv` policy.
- Workflow сам применяет изменения, запускает проверки и ревью.
- Пользователь вмешивается только при scope/секретах/критическом падении.

## Направление 2: CTF Cell

### Цель

Сделать команду для CTF-задач: web, LFI, RCE, SQLi, pwn, crypto, reverse, forensics.

### Рекомендуемая группа

`CTF Cell`

Агенты:

- `Люмен` — принимает задачу, определяет категорию, ведёт writeup;
- `Разведчик` — passive recon, чтение условий, endpoints, файлов;
- `Web Exploiter` — web, LFI, SSRF, SQLi, auth bugs;
- `Pwner` — binary exploitation, gdb/checksec/pwntools;
- `Криптограф` — crypto, math, attacks, scripts;
- `Реверсер` — reverse engineering, strings/objdump/radare2;
- `Форензик` — файлы, образы, PCAP, stego;
- `Валидатор` — проверяет флаг, чистит writeup.

### CTF Project Structure

```text
challenge.yml
notes.md
scope.md
artifacts/
evidence/
solve/
solve/README.md
writeup.md
```

### CTF Lifecycle

```text
1. intake
2. classify_category
3. scope_check
4. collect_artifacts
5. hypothesis_board
6. category_solver
7. validate_flag
8. writeup
```

### Scope Policy

CTF считается разрешённым, если:

- пользователь явно указал, что это CTF/lab;
- target относится к локальному challenge, docker, файлам задачи или CTF-платформе;
- нет просьбы атаковать стороннюю публичную цель без разрешения.

Для активных сетевых действий нужен `scope.md`:

```text
target:
authorization:
allowed_actions:
forbidden_actions:
rate_limits:
time_window:
evidence_dir:
```

### CTF Tool Profiles

`ctf_web`:

- `curl`;
- `httpx` optional;
- `python scripts`;
- `sqlmap` только при explicit scope;
- wordlists только локальные и с rate limit.

`ctf_pwn`:

- `file`;
- `strings`;
- `checksec`;
- `gdb`;
- `python/pwntools`;
- запуск локальных бинарей.

`ctf_crypto`:

- `.venv/bin/python`;
- `sage` optional;
- локальные scripts.

`ctf_reverse`:

- `file`;
- `strings`;
- `objdump`;
- `radare2` optional;
- `ghidra` manual note.

### Acceptance Criteria

- На CTF-запрос не запускается обычный Dev pipeline.
- Люмен создаёт/обновляет `challenge.yml`.
- Все гипотезы и попытки пишутся в `notes.md`.
- Найденный флаг фиксируется отдельно.
- Итоговый `writeup.md` понятен человеку.
- Потенциально опасные действия требуют scope.

## UI

### Groups Screen

Новая вкладка в настройках:

```text
Группы
  + Новая группа
  - Dev Squad
  - CTF Cell
  - Research Desk
```

Карточка группы:

- имя;
- тип;
- сколько агентов;
- жизненный цикл;
- модель по умолчанию;
- кнопки: открыть, дублировать, архивировать.

### Group Editor

Секции:

- `Агенты`;
- `Жизненный цикл`;
- `Инструменты`;
- `Модели`;
- `Контекст`;
- `Экспорт/импорт`.

### Agent Editor

Поля:

- имя;
- короткое имя;
- роль;
- аватар;
- модель;
- tool profile;
- `soul.md` editor;
- включён/выключен.

### Lifecycle Editor

Визуально:

```text
[Люмен: intake] -> [Продакт: requirements] -> [Архитектор: blueprint] -> ...
```

MVP можно сделать без drag-and-drop:

- список шагов;
- добавить шаг;
- удалить шаг;
- выбрать агента;
- выбрать mode;
- задать retry/stop policy.

## Storage

SQLite таблицы:

```text
agent_groups
agent_profiles
agent_souls
tool_profiles
lifecycle_definitions
lifecycle_steps
project_group_bindings
workflow_trace_events
security_scopes
ctf_challenges
```

Файлы:

```text
~/dev_ai_zavod/agents/<group>/<agent>/soul.md
~/dev_ai_zavod/groups/<group>/lifecycle.yaml
~/dev_ai_zavod/tool_profiles/*.yaml
```

SQLite хранит индексы, метаданные и активные связи. Markdown/YAML остаются редактируемыми руками.

## Import / Export

Группа экспортируется как папка:

```text
group.yaml
agents/<agent>/soul.md
lifecycle.yaml
tool_profiles/*.yaml
avatars/*
```

Это позволит делиться “командами” через GitHub.

## MVP Implementation Plan

### V0.7.2 - Agent Groups Foundation

- Добавить таблицы `agent_groups`, `agent_profiles`, `lifecycle_definitions`, `lifecycle_steps`.
- Seed groups:
  - `Dev Squad`;
  - `CTF Cell`.
- Сохранить текущий фиксированный pipeline как `Dev Squad`.
- UI: список групп и просмотр состава.
- Workflow пока использует активную группу проекта.

### V0.7.3 - soul.md

- Для каждого агента создать `soul.md`.
- Добавить редактор `soul.md`.
- При вызове агента prompt собирается:
  1. system safety;
  2. agent `soul.md`;
  3. project context;
  4. task spec;
  5. step input.
- Добавить сканирование prompt-injection паттернов в `soul.md`.

### V0.7.4 - Lifecycle Editor

- UI редактирования шагов.
- Backend lifecycle executor вместо hardcoded step order.
- Поддержка `required`, `max_retries`, `on_failure`.
- Trace каждого перехода.

### V0.8.2 - Dev Squad 2.0

- Улучшить Dev-группу:
  - Go project mode;
  - Python project mode;
  - diff-aware developer;
  - обязательный reviewer;
  - docs/release agents optional.

### V0.9.2 - CTF Cell MVP

- Создание CTF workspace.
- Категории challenge.
- `challenge.yml`, `notes.md`, `writeup.md`.
- Scope gate.
- Tool profiles по категориям.
- CTF-specific final summary.

## Нефункциональные требования

- Локальный запуск на macOS.
- Все настройки работают без ручного редактирования БД.
- Группы можно экспортировать/импортировать.
- Нет бесконечного цикла: все lifecycle имеют лимиты.
- Raw JSON не показывается пользователю в обычном чате.
- Служебные артефакты скрыты от обычного списка изменений.
- В CTF/ИБ задачах все активные действия логируются.

## Важные решения

- Не копировать Hermes один-в-один. Взять сильную идею: редактируемые личности, проектный контекст, профили и группы.
- Не хранить весь `soul.md` только в SQLite. Файл должен быть обычным Markdown, чтобы его можно было версионировать.
- Не делать CTF как подвид разработки. Это отдельный тип группы и lifecycle.
- Не отдавать агентам произвольный shell. Только tool profiles и allowlist.

## References

- Hermes SOUL.md: `SOUL.md` как основная identity агента, глобальный файл instance-level: https://hermes-agent.nousresearch.com/docs/guides/use-soul-with-hermes
- Hermes configuration: project context files `.hermes.md`, `AGENTS.md`, `CLAUDE.md`, `.cursorrules`, priority and truncation: https://hermes-agent.nousresearch.com/docs/user-guide/configuration/
- Hermes AI team guide: SOUL, memory, vault, skills, kanban, group chats and team coordination: https://github.com/smfworks/hermes-ai-team
- OWASP WSTG for web security/CTF checklists: https://owasp.org/www-project-web-security-testing-guide/
- CTFd challenge workflow and API: https://docs.ctfd.io/docs/api/getting-started/
