# Zavod AI Agent Groups Release Specs

Дата: 2026-09-05

Статус: draft

Связанные документы:

- `docs/HERMES_LIKE_TEAMS_SPEC.md`
- `docs/FEATURE_ROADMAP.md`

## Цель

Этот документ описывает реализацию пяти связанных релизов:

- `V0.7.2` - фундамент групп агентов;
- `V0.7.3` - `soul.md` для каждого агента;
- `V0.7.4` - редактор lifecycle;
- `V0.8.2` - мощный dev-пайплайн для Python/Go;
- `V0.9.2` - отдельная CTF-команда с категориями `web`, `LFI`, `RCE`, `SQLi`, `pwn`, `crypto`, `reverse`, `forensics`.

Главная продуктовая идея: Zavod AI должен перестать быть одним жёстким pipeline. Пользователь должен собирать свои команды агентов, задавать им роли, модели, инструменты, жизненный цикл и поведение через `soul.md`.

## Базовые принципы

- Группа агентов - основная единица настройки workflow.
- Агент внутри группы - редактируемый профиль, а не hardcoded роль.
- `soul.md` описывает личность, стиль и рабочие привычки агента.
- Lifecycle описывает порядок работы, повторы, возвраты и остановки.
- Dev и CTF - два разных направления, а не один общий pipeline.
- В чат выводится только значимая информация; служебные JSON, trace и артефакты скрываются в диагностике.
- Все активные ИБ-действия требуют явного scope, если цель не является локальным CTF/lab-окружением.

## Общая модель данных

### AgentGroup

Группа агентов, которую можно назначить проекту или выбрать для отдельной задачи.

Поля:

- `id`
- `name`
- `slug`
- `kind`: `dev`, `ctf`, `research`, `security`, `custom`
- `description`
- `default_model_id`
- `default_lifecycle_id`
- `status`: `active`, `archived`
- `created_at`
- `updated_at`

### AgentProfile

Профиль конкретного агента внутри группы.

Поля:

- `id`
- `group_id`
- `name`
- `role_key`
- `description`
- `avatar_path`
- `soul_path`
- `model_id`
- `tool_profile_id`
- `temperature`
- `context_budget`
- `enabled`
- `sort_order`
- `created_at`
- `updated_at`

### LifecycleDefinition

Описание жизненного цикла группы.

Поля:

- `id`
- `group_id`
- `name`
- `kind`: `dev`, `ctf`, `research`, `custom`
- `description`
- `max_total_iterations`
- `max_repair_iterations`
- `same_error_limit`
- `status`: `active`, `archived`
- `created_at`
- `updated_at`

### LifecycleStep

Один шаг lifecycle.

Поля:

- `id`
- `lifecycle_id`
- `step_key`
- `title`
- `agent_profile_id`
- `mode`: `llm`, `tool`, `human_gate`, `apply_changes`, `checks`, `review`, `final`
- `required`
- `can_retry`
- `max_retries`
- `on_success_step_key`
- `on_failure_step_key`
- `output_schema`
- `visible_to_user`
- `sort_order`

### ToolProfile

Allowlist инструментов и команд для роли или группы.

Поля:

- `id`
- `name`
- `kind`: `go_dev`, `python_dev`, `ctf_web`, `ctf_pwn`, `ctf_crypto`, `ctf_reverse`, `ctf_forensics`, `research`, `custom`
- `description`
- `allowed_commands_json`
- `blocked_commands_json`
- `requires_scope`
- `created_at`
- `updated_at`

### ProjectGroupBinding

Связь проекта с активной группой.

Поля:

- `id`
- `project_id`
- `group_id`
- `lifecycle_id`
- `is_default`
- `created_at`
- `updated_at`

## V0.7.2 - Agent Groups Foundation

### Цель

Добавить фундамент групп агентов: пользователь может видеть, создавать, редактировать, архивировать и назначать группы проектам.

### Пользовательские сценарии

- Пользователь открывает настройки проекта и выбирает активную группу агентов.
- Пользователь создаёт новую группу на основе шаблона `Dev Squad` или `CTF Cell`.
- Пользователь видит список агентов внутри группы.
- Пользователь может включить/выключить агента.
- Пользователь может назначить группе модель по умолчанию.
- Workflow использует активную группу проекта, а не фиксированный список ролей.

### Backend

Добавить миграции SQLite:

- `agent_groups`
- `agent_profiles`
- `tool_profiles`
- `lifecycle_definitions`
- `lifecycle_steps`
- `project_group_bindings`

Добавить сервисы:

- `AgentGroupService`
- `AgentProfileService`
- `ToolProfileService`
- `ProjectGroupBindingService`

Добавить seed при первом запуске:

- группа `Dev Squad`;
- группа `CTF Cell`;
- текущие роли переносятся в `Dev Squad`;
- ИБ-специалист и CTF-роли добавляются в `CTF Cell`.

### Frontend

Добавить экран `Группы агентов`.

Карточка группы показывает:

- название;
- тип;
- количество агентов;
- активную модель;
- lifecycle;
- статус.

Редактор группы показывает:

- имя;
- описание;
- тип;
- модель по умолчанию;
- список агентов;
- кнопку добавления агента;
- кнопку архивирования.

### Workflow

Текущий hardcoded pipeline не удаляется сразу. Он должен быть адаптирован так, чтобы читать дефолтную группу `Dev Squad`.

Если у проекта нет привязки к группе:

- использовать `Dev Squad`;
- показать пользователю мягкую подсказку в настройках проекта.

### Acceptance Criteria

- Можно создать группу агентов через UI.
- Можно назначить группу проекту.
- Можно добавить агента в группу.
- Можно отключить агента без удаления.
- Старые задачи продолжают работать через `Dev Squad`.
- В БД есть seed-группы `Dev Squad` и `CTF Cell`.
- Workflow выбирает агентов из активной группы проекта.

### Не входит в релиз

- Редактирование полного lifecycle.
- Полноценный `soul.md` редактор.
- Специализированный CTF-runner.

## V0.7.5 - Project Group Choice

### Цель

Проект создаётся сразу с выбранной группой агентов: `Dev Squad`, `CTF Cell` или кастомная группа. Пользователь явно управляет базовым lifecycle проекта, без скрытой перепривязки при первом сообщении.

### Пользовательские сценарии

- Пользователь нажимает `Новый` и выбирает группу проекта до создания.
- Пользователь добавляет проект `Из папки` и выбирает группу проекта до сохранения.
- Если пользователь ничего не выбрал, используется `Dev Squad`.
- Пользователь может позже изменить группу в настройках `Группы`.
- CTF lifecycle запускается только для проекта, который явно привязан к `CTF Cell` или кастомной CTF-группе.

### Backend

- `CreateProjectInput` принимает `group_id` и `lifecycle_id`.
- `AddExistingProjectInput` принимает `group_id` и `lifecycle_id`.
- Service валидирует, что группа существует и не архивная.
- Если `lifecycle_id` пустой, используется `default_lifecycle_id` выбранной группы.
- Если `lifecycle_id` передан, он должен принадлежать выбранной группе.
- При создании проекта сразу создаётся `project_group_binding`.
- `SendMessage` не меняет привязку проекта к группе автоматически.

### Frontend

- В форме `Новый проект` есть selector группы.
- В форме `Из папки` есть selector группы.
- Selector показывает название группы, тип и количество агентов.
- Значение по умолчанию - `Dev Squad`, если группа доступна.
- После создания проекта selector возвращается к дефолтной группе.

### Acceptance Criteria

- Новый проект можно создать сразу с `Dev Squad`.
- Новый проект можно создать сразу с `CTF Cell`.
- Новый проект можно создать сразу с кастомной группой.
- Добавленный из папки проект получает выбранную группу.
- Нельзя создать проект с архивной или несуществующей группой.
- Нельзя передать lifecycle от другой группы.
- CTF-запрос не перепривязывает Dev-проект к `CTF Cell` скрыто.

## V0.7.6 - Agent Group Templates

### Цель

Добавить templates для быстрого создания команд агентов. Шаблон создаёт независимую редактируемую группу с собственными агентами, `soul.md` файлами и lifecycle.

### Шаблоны

- `Dev Squad` - Python/Go разработка, требования, blueprint, код, проверки, ревью.
- `CTF Cell` - CTF/lab задачи с категориями `web`, `LFI`, `RCE`, `SQLi`, `pwn`, `crypto`, `reverse`, `forensics`.
- `Research Desk` - поиск в интернете, проверка источников, аналитика, итог со ссылками.
- `Security Audit` - defensive-аудит, threat model, hardening, remediation.
- `Solo Lumen` - минимальная группа для direct answers и коротких одиночных workflow.

### Backend

- `ListAgentGroupTemplates` возвращает список доступных шаблонов.
- `CreateAgentGroupFromTemplate` создаёт новую группу из шаблона.
- Профили агентов получают новые ID и не связаны с seed-группами.
- Lifecycle создаётся в default lifecycle новой группы.
- Lifecycle steps перепривязываются к новым profile IDs.
- Для каждого агента создаётся `soul.md`.
- Если имя не передано, backend создаёт уникальное имя вида `<template> copy`.

### Frontend

- В настройках `Группы` есть блок `Шаблоны команд`.
- Карточка шаблона показывает название, тип, количество агентов и количество шагов.
- Кнопка `Создать` разворачивает шаблон в новую группу.
- После создания новая группа открывается в редакторе и её можно донастроить.

### Acceptance Criteria

- Пользователь видит 5 шаблонов.
- Пользователь может создать группу из каждого шаблона.
- Созданная группа редактируется как обычная кастомная группа.
- У созданной группы есть агенты, lifecycle и `soul.md` файлы.
- Создание из шаблона не меняет seed-группы `Dev Squad` и `CTF Cell`.

## V0.7.3 - soul.md для каждого агента

### Цель

Сделать поведение агента редактируемым через Markdown-файл `soul.md`.

### Концепция

`soul.md` - это не task spec и не project spec. Это личность и рабочий контракт агента.

Разделы файла:

```markdown
# Soul

## Кто я

## За что отвечаю

## Как принимаю решения

## Как общаюсь с пользователем

## Что никогда не делаю

## Как передаю задачу дальше
```

### Хранение

Файлы лежат в workspace Zavod:

```text
agents/
  dev-squad/
    lumen/soul.md
    product/soul.md
    architect/soul.md
    developer/soul.md
    tester/soul.md
    reviewer/soul.md
  ctf-cell/
    lumen/soul.md
    scout/soul.md
    web-exploiter/soul.md
    pwner/soul.md
    cryptographer/soul.md
    reverser/soul.md
    forensics/soul.md
    validator/soul.md
```

В SQLite хранится путь, хеш и метаданные, но не единственная копия текста.

### Prompt Assembly

Prompt агента собирается в строгом порядке:

1. системные guardrails приложения;
2. `soul.md` агента;
3. project context;
4. task spec;
5. lifecycle step input;
6. compact trace предыдущих шагов.

Правило приоритета:

- safety guardrails имеют самый высокий приоритет;
- `soul.md` не может отменить безопасность;
- project context не может отменить требования конкретной задачи;
- task spec является источником истины по текущей задаче.

### UI

В карточке агента добавить:

- кнопку `Редактировать soul.md`;
- индикатор, что файл изменён;
- кнопку восстановить дефолтный `soul.md`;
- preview первых строк.

Редактор должен быть Markdown-редактором без raw JSON.

### Безопасность

Перед сохранением `soul.md` выполнить простой scan:

- попытки отключить safety;
- инструкции игнорировать пользователя;
- инструкции раскрывать секреты;
- инструкции выполнять произвольный shell;
- инструкции обходить scope в ИБ-задачах.

Scan не блокирует обычный стиль и личность, но показывает предупреждение.

### Acceptance Criteria

- У каждого агента есть `soul.md`.
- Пользователь может открыть и изменить `soul.md`.
- Изменённый `soul.md` реально участвует в prompt.
- Можно восстановить дефолтный `soul.md`.
- `soul.md` хранится как файл, пригодный для Git.
- В чат не попадает полный `soul.md`, если пользователь явно не попросил.

### Не входит в релиз

- Marketplace готовых агентов.
- Совместное редактирование.
- Версионирование через UI.

## V0.7.4 - Lifecycle Editor

### Цель

Дать пользователю возможность редактировать жизненный цикл группы: шаги, исполнителей, повторы и правила возврата.

### Пользовательские сценарии

- Пользователь открывает группу `Dev Squad` и видит список шагов.
- Пользователь добавляет новый шаг между Архитектором и Разработчиком.
- Пользователь меняет исполнителя шага.
- Пользователь задаёт `max_retries`.
- Пользователь указывает, куда возвращать задачу после негативного ревью.
- Пользователь отключает необязательный шаг, например Докера или Релизера.

### Lifecycle Step Modes

- `llm` - обычный шаг агента.
- `tool` - шаг, который вызывает инструмент или набор команд.
- `apply_changes` - применение structured changes.
- `checks` - запуск проверок.
- `review` - обязательный контроль качества.
- `human_gate` - ожидание подтверждения пользователя.
- `final` - финальный ответ.

### Backend Executor

Нужен `LifecycleExecutor`, который:

- получает активную группу проекта;
- загружает lifecycle;
- исполняет шаги по порядку;
- пишет trace;
- умеет возвращаться на указанный шаг;
- уважает лимиты повторов;
- умеет остановиться с понятной причиной.

Стоп-причины:

- `completed`
- `needs_user_scope`
- `needs_user_secret`
- `max_repair_iterations`
- `same_error_repeated`
- `invalid_step_output`
- `tool_unavailable`
- `safety_blocked`

### Frontend

Редактор lifecycle:

- список шагов;
- карточка шага;
- выбор агента;
- выбор режима;
- переключатель `required`;
- поле `max_retries`;
- выбор `on_success`;
- выбор `on_failure`;
- preview всего pipeline.

В основном экране:

- компактная плашка текущего шага;
- popover со всеми шагами;
- динамическое количество шагов;
- рядом с diff-плашкой можно показать progress-плашку lifecycle.

### Acceptance Criteria

- Lifecycle можно открыть и изменить.
- Изменения сохраняются.
- Executor использует lifecycle из БД, а не hardcoded список.
- Негативное ревью может вернуть задачу на нужный шаг.
- При превышении лимита workflow останавливается с понятной причиной.
- UI показывает текущий шаг и общий прогресс.

### Не входит в релиз

- Сложный граф с ветвлениями через визуальный canvas.
- Parallel execution.
- Marketplace lifecycle templates.

## V0.8.2 - Dev Squad 2.0 для Python/Go

### Цель

Сделать `Dev Squad` сильной командой для разработки скриптов и больших проектов на Python и Go.

### Поддерживаемые типы задач

- создать CLI-скрипт;
- исправить существующий код;
- добавить тесты;
- добавить README;
- добавить Makefile;
- собрать приложение;
- провести ревью;
- объяснить проект;
- вывести task spec;
- провести локальную диагностику.

### Dev Agents

Минимальная команда:

- `Люмен` - intent router, task intake, spec keeper;
- `Продакт` - требования и acceptance criteria;
- `Архитектор` - структура решения и риски;
- `Разработчик` - код и structured changes;
- `Тестировщик` - команды проверки;
- `Ревьюер` - обязательная проверка результата;
- `Докер` - README, install, usage;
- `Релизер` - сборка, changelog, release notes.

### Go Policy

Для Go-проектов:

- runtime по умолчанию: `Go 1.25+`;
- если `go.mod` отсутствует, создать его;
- не менять module name без причины;
- после изменения Go-кода запускать `gofmt`;
- базовая проверка: `go test ./...`;
- если есть Makefile, предпочитать `make test`/`make build`, если они явно описаны.

### Python Policy

Для Python-проектов:

- обязательно создавать `requirements.txt`, если есть внешние зависимости;
- обязательно использовать `.venv`;
- тестировщик запускает Python-команды внутри `.venv`;
- не предлагать `pip install` глобально;
- если нет тестов, хотя бы запускать `python -m py_compile` для изменённых файлов;
- если есть pytest, запускать `.venv/bin/python -m pytest`.

Типовой bootstrap:

```text
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
.venv/bin/python -m pytest
```

### Task Spec Format

Для dev-задач `docs/task-spec.md` должен быть человекочитаемым:

```markdown
# Task Spec

## User Request

## Goal

## Scope

## Requirements

## Acceptance Criteria

## Files Expected To Change

## Checks

## Decisions

## Out Of Scope
```

Машинные данные хранятся отдельно:

```text
.zavod/runs/<run_id>/context.json
.zavod/runs/<run_id>/trace.jsonl
.zavod/runs/<run_id>/agent-outputs/
```

### Diff and Changes

В UI показывать общий итог изменений:

- количество уникальных файлов;
- суммарный `+N -M`;
- список уникальных файлов;
- по клику или hover - детальная история по итерациям.

Повторные изменения одного файла в repair-loop должны схлопываться в один итоговый diff относительно начального состояния задачи.

### Reviewer

Ревьюер обязателен.

Статусы:

- `accepted`
- `needs_work`
- `blocked`

Если `needs_work`, reviewer должен вернуть задачу на конкретный шаг:

- `requirements`
- `architecture`
- `development`
- `checks`

Если проверки в последней итерации прошли и reviewer принял работу, финальный статус должен быть успешным, даже если ранние итерации падали.

### Acceptance Criteria

- Dev Squad корректно работает с Go 1.25+.
- Python-задачи используют `.venv`.
- Внешние Python-зависимости попадают в `requirements.txt`.
- Ревьюер обязателен.
- Итоговый diff показывает уникальные файлы, а не все repair-итерации как отдельные файлы.
- По запросу `выведи спеку задания` Люмен показывает `docs/task-spec.md`, а не пересказывает ход работ.
- Служебные артефакты не выводятся как пользовательские файлы задачи.

### Не входит в релиз

- Полноценный cloud CI runner внутри приложения.
- Автоматический publish на GitHub.
- Поддержка языков кроме Python и Go.

## V0.9.2 - CTF Cell MVP

### Цель

Создать отдельную команду для CTF и легитимных лабораторных ИБ-задач.

### Поддерживаемые категории

- `web`
- `LFI`
- `RCE`
- `SQLi`
- `pwn`
- `crypto`
- `reverse`
- `forensics`

### CTF Agents

Минимальная команда:

- `Люмен` - intake, intent router, writeup coordinator;
- `Разведчик` - пассивный сбор информации и структура challenge;
- `Web Exploiter` - общие web challenge в рамках scope;
- `LFI Hunter` - LFI/path traversal challenge;
- `RCE Analyst` - RCE/command injection challenge;
- `SQLi Solver` - SQL injection challenge;
- `Pwner` - binary exploitation для локальных challenge;
- `Криптограф` - crypto tasks;
- `Реверсер` - reverse engineering;
- `Форензик` - файлы, дампы, сетевые артефакты;
- `Валидатор` - проверка flag и writeup.

### CTF Workspace

Для каждой CTF-задачи создавать структуру:

```text
ctf/
  <challenge-slug>/
    challenge.yml
    scope.md
    notes.md
    artifacts/
    evidence/
    solve/
      README.md
    writeup.md
```

### challenge.yml

```yaml
title:
event:
category:
difficulty:
provided_files:
target:
flag_format:
status:
created_at:
updated_at:
```

### scope.md

Обязателен для активных сетевых действий, если цель не является локальным контейнером, локальным файлом или явно CTF-платформой.

```markdown
# Scope

## Target

## Authorization

## Allowed Actions

## Forbidden Actions

## Rate Limits

## Time Window

## Evidence Directory
```

### notes.md

Рабочие заметки:

```markdown
# Notes

## Observations

## Hypotheses

## Attempts

## Dead Ends

## Evidence

## Next Steps
```

### writeup.md

Финальный отчёт:

```markdown
# Writeup

## Challenge

## Category

## Summary

## Approach

## Exploit or Solution

## Flag

## Lessons Learned
```

### CTF Lifecycle

Шаги:

1. `intake` - понять задачу и категорию;
2. `scope_check` - проверить разрешённость действий;
3. `artifact_collection` - сохранить файлы и вводные;
4. `triage` - определить категорию и стратегию;
5. `hypothesis_board` - записать гипотезы;
6. `category_solver` - передать профильному агенту;
7. `validation` - проверить flag или результат;
8. `writeup` - оформить решение.

### Safety Policy

Разрешено:

- локальные CTF-файлы;
- локальные Docker challenge;
- учебные CTF-платформы;
- цели с явным разрешением пользователя;
- анализ предоставленных артефактов.

Требует scope:

- сканирование внешних IP/доменов;
- brute force;
- fuzzing публичного сервиса;
- эксплуатация уязвимости на внешней цели;
- отправка payload на сторонний сервис.

Запрещено без явного разрешения:

- атаки на реальные сторонние системы;
- персистентность;
- скрытность;
- кража секретов;
- разрушительные действия;
- обход лимитов и блокировок.

### Tool Profiles

`ctf_web`:

- `curl`
- локальные Python scripts;
- browser/manual notes;
- wordlists только локально и с rate limit;
- sqlmap только при явном scope.

`ctf_lfi`:

- `curl`
- `.venv/bin/python`
- локальные path traversal/LFI заметки;
- чтение только предоставленных файлов и разрешённых lab-целей.

`ctf_rce`:

- `curl`
- `.venv/bin/python`
- локальные proof-of-concept заметки;
- активные payload только в явном CTF/lab scope.

`ctf_sqli`:

- `curl`
- `.venv/bin/python`
- `sqlmap` только при явном scope;
- ручные SQLi-гипотезы и reproduction notes.

`ctf_pwn`:

- `file`
- `strings`
- `objdump`
- `gdb`
- `checksec`, если установлен;
- `.venv/bin/python` и `pwntools`, если есть зависимость.

`ctf_crypto`:

- `.venv/bin/python`
- локальные solver scripts;
- `sage`, если установлен.

`ctf_reverse`:

- `file`
- `strings`
- `objdump`
- `radare2`, если установлен;
- Ghidra как manual external step.

`ctf_forensics`:

- `file`
- `exiftool`, если установлен;
- `binwalk`, если установлен;
- локальные Python parsers.

### UI

В проекте появляется режим `CTF`.

Показывать:

- challenge title;
- category;
- status;
- current hypothesis;
- artifacts;
- notes;
- writeup;
- scope status.

В чате показывать только:

- что найдено;
- какая гипотеза;
- что пробуем дальше;
- результат;
- ссылку на writeup.

Raw command output и большие логи хранить в `evidence/` и diagnostic trace.

### Acceptance Criteria

- Можно выбрать группу `CTF Cell`.
- CTF-задача создаёт workspace.
- Категория определяется автоматически или выбирается пользователем.
- Для web/LFI/RCE/SQLi/pwn/crypto/reverse/forensics есть отдельный профиль агента.
- Активные сетевые действия требуют `scope.md`, если цель не локальная и не явно CTF.
- Итоговый результат сохраняется в `writeup.md`.
- В чат не вываливаются большие raw логи.

### Не входит в релиз

- Автоматическая эксплуатация реальных внешних целей.
- Интеграция со всеми CTF-платформами.
- Автоматический запуск тяжёлых сканеров без подтверждения.

### Реализовано в V0.9.2

- Seed `CTF Cell` содержит отдельные профили для `web`, `LFI`, `RCE`, `SQLi`, `pwn`, `crypto`, `reverse`, `forensics`.
- Intent router распознает CTF/lab/category-запросы без ожидания LLM-классификатора.
- CTF-запрос автоматически привязывает проект к `CTF Cell` и строит lifecycle-план из группы.
- Backend создает workspace `ctf/<challenge-slug>/` с `challenge.yml`, `scope.md`, `notes.md`, `solve/README.md`, `writeup.md`.
- `category_solver` выбирает профильного агента по категории, а UI показывает фактического solver-агента.
- Для внешних активных целей без явного CTF/lab/scope workflow останавливается на `scope_check`.

## V0.7.7 — роли и capabilities агентов

### Цель

Сделать профиль агента полноценным контрактом исполнения: не только имя, роль и `soul.md`, но и явное описание того, что агент умеет, какие инструменты может использовать, какие файлы может читать/писать и когда обязан передать задачу другой роли.

### Модель данных

`AgentProfile` получает структурированные поля:

- `capabilities` — список рабочих возможностей агента.
- `allowedTools` — список разрешенных инструментов, команд или tool profiles.
- `readPaths` — файловые области, которые агент может читать.
- `writePaths` — файловые области, которые агент может менять.
- `handoffRules` — условия передачи задачи дальше или возврата назад.

Поля хранятся в SQLite как JSON-массивы, чтобы UI и агенты получали один и тот же контракт без парсинга markdown.

### Prompt Assembly

При запуске шага агент получает:

- содержимое своего `soul.md`;
- служебный `Capabilities Contract`, собранный из профиля;
- текущий task/project context.

`soul.md` отвечает за стиль, личность и рабочие принципы. `Capabilities Contract` отвечает за границы полномочий и имеет приоритет над общими пожеланиями из `soul.md`.

### Defaults

Для seed-групп и шаблонов должны существовать дефолтные capabilities:

- `Dev Squad`: manager, product, architect, developer, tester, reviewer, docs, release.
- `CTF Cell`: manager, scout, web, LFI, RCE, SQLi, pwn, crypto, reverse, forensics, validator.
- `Research Desk`: researcher, analyst, source reviewer.
- `Security Audit`: security, threat modeler, remediator, reviewer.
- `Solo Lumen`: manager/direct answer профиль.

Если у существующего агента пустые capability-поля, приложение показывает и использует role/tool defaults, не ломая старые проекты.

### UI

В редакторе группы у каждого агента можно менять:

- возможности;
- разрешенные инструменты;
- read/write paths;
- handoff rules.

Карточка агента показывает краткое резюме capabilities и счетчики `tools/read/write/handoff`, чтобы состав команды был понятен без открытия `soul.md`.

### Acceptance Criteria

- У каждого агента есть явные capabilities в API и UI.
- Capabilities сохраняются и читаются из БД.
- Старые профили получают безопасные дефолты.
- Prompt assembly добавляет capability-контракт к `soul.md`.
- Seed `Dev Squad` и `CTF Cell` не остаются пустыми по полномочиям.
- Пользователь может отредактировать контракт агента без ручного JSON.
- Сборка frontend и Go-тесты проходят.

## V0.7.8 — marketplace/local library агентов

### Цель

Добавить внутри приложения локальную библиотеку готовых агентов, чтобы пользователь мог быстро собирать собственные команды без ручного копирования ролей, capabilities и `soul.md`.

### Основные сценарии

- Пользователь открывает настройки группы и видит библиотеку готовых агентов.
- Пользователь добавляет агента из библиотеки в текущую группу.
- Пользователь копирует уже существующего агента внутри группы.
- Пользователь отключает агента без удаления из группы.
- Пользователь заменяет `soul.md` выбранного агента содержимым из библиотечного пресета.

### Модель поведения

Локальная библиотека не является внешним marketplace. Это встроенный каталог curated presets, который работает offline и версионируется вместе с приложением.

Добавление агента из библиотеки:

- создаёт новый `AgentProfile` в выбранной группе;
- копирует роль, описание, tool profile, capabilities, file access и handoff rules;
- создаёт отдельный `soul.md` для нового агента;
- выбирает модель группы или активную модель приложения.

Копирование агента:

- создаёт независимую копию профиля в той же группе;
- сохраняет capabilities и permissions;
- копирует содержимое исходного `soul.md`, если файл существует;
- включает нового агента по умолчанию.

Замена `soul.md`:

- меняет только текст `soul.md`;
- не меняет capabilities, tools и file access без отдельного явного действия;
- сразу открывает обновленный soul editor, чтобы пользователь видел результат.

### Категории библиотеки

Минимальный набор:

- `Core`: Люмен/manager.
- `Dev`: product, architect, Go developer, Python developer, tester, reviewer, docs.
- `Research`: researcher, analyst.
- `Security`: security specialist, threat modeler.
- `CTF`: scout, web, LFI, RCE, SQLi, pwn, crypto, reverse, forensics, validator.

### UI

В настройках групп появляется секция `Библиотека агентов`.

Для каждого агента показывать:

- имя;
- категорию;
- role key;
- краткое описание;
- теги.

Действия:

- `Добавить` — добавляет агента в текущую группу.
- `soul.md` — заменяет `soul.md` выбранному агенту из текущей группы.

В списке агентов группы добавить действие `Копировать`.

### Acceptance Criteria

- Backend возвращает список библиотечных агентов.
- Можно добавить библиотечного агента в любую группу.
- Добавленный агент получает отдельный профиль и отдельный `soul.md`.
- Можно скопировать существующего агента.
- Отключение агента продолжает работать через existing toggle.
- Замена `soul.md` не меняет permissions/capabilities агента.
- UI показывает библиотеку в настройках групп.
- Go-тесты и frontend build проходят.

## Общая последовательность реализации

Рекомендуемый порядок:

1. Реализовать `V0.7.2`, чтобы появилась сущность группы.
2. Реализовать `V0.7.3`, чтобы агенты стали редактируемыми.
3. Реализовать `V0.7.4`, чтобы убрать hardcoded workflow.
4. Реализовать `V0.7.7`, чтобы у каждого агента появились явные роли и capabilities.
5. Реализовать `V0.7.8`, чтобы группы собирались из локальной библиотеки агентов.
6. Усилить `Dev Squad` в `V0.8.2`.
7. Добавить `CTF Cell` в `V0.9.2`.

Нельзя начинать CTF Cell без lifecycle и tool profiles, иначе получится ещё один hardcoded pipeline.

## Общие Acceptance Criteria пакета

- Пользователь может создать свою группу агентов.
- Пользователь может добавить агентов в группу.
- Пользователь может настроить модель каждого агента.
- Пользователь может настроить lifecycle группы.
- У каждого агента есть редактируемый `soul.md`.
- У каждого агента есть редактируемые capabilities, tool permissions, file access и handoff rules.
- Пользователь может добавлять и копировать агентов из локальной библиотеки.
- Проект может быть привязан к группе.
- Dev и CTF используют разные lifecycle.
- UI не показывает служебные артефакты как пользовательские изменения.
- Workflow trace доступен для диагностики, но не засоряет чат.
