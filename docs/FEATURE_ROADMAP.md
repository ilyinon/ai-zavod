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
- группы агентов, `soul.md` и редактор lifecycle;
- сильный dev-пайплайн: Go 1.25+, `gofmt`, Python `.venv` + `requirements.txt` enforcement;
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
- категории web/LFI/RCE/SQLi/pwn/crypto/reverse/forensics;
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

Детальная спека по пакетам `V0.7.2` - `V0.9.2`: `docs/AGENT_GROUPS_RELEASE_SPECS.md`.

## V0.8.4 - Code Execution Policy

### Цель

Команды агентов должны проходить через единый policy gate: `auto`, `confirm`, `deny`.

### Фичи

- Профили выполнения:
  - `dev` - автопроверки Go/Python/frontend;
  - `ctf` - локальные CTF-инструменты автоматически, сетевые/активные только после scope и подтверждения;
  - `research` - только встроенный web research provider, без shell-команд.
- Backend-runner принимает в автозапуск только `auto` команды.
- `confirm` команды не попадают в Autopilot-проверки.
- `deny` команды блокируются до запуска.
- Shell-операторы, destructive-команды, docker/kubernetes/infrastructure и активные attack tooling вне CTF scope запрещены.

### Acceptance Criteria

- `go test ./...`, `go vet ./...`, `npm run build`, `.venv/bin/python -m pytest` могут идти автоматически при подходящей структуре проекта.
- `make build`, `wails build`, `go mod tidy`, `npm install`, `pip install` требуют подтверждения.
- `curl` в dev не запускается автоматически.
- `curl` в CTF требует scope/подтверждения.
- В research shell-команды не запускаются.

## V0.8.5 - Smart Repair Loop

### Цель

Autopilot должен чинить исправимые проблемы сам: падение тестов, ошибки применения diff и замечания ревью не должны превращаться в “нужно вмешательство”.

### Фичи

- Synthetic review по результатам проверок до обязательного ревью.
- Автоматический возврат:
  - упавшие проверки -> Разработчик;
  - заблокированные/неподходящие проверки -> Тестировщик;
  - неверный blueprint/scaffold -> Архитектор;
  - неполные требования -> Продакт.
- Нормализация ложного `blocked` от ревьюера в `needs_work`, если причина исправима агентами.
- Пользовательский блокер только для scope, секретов/API key, конфликтующих требований или недоступной внешней инфраструктуры.
- Repair-loop продолжает работать до lifecycle `max_repair_iterations`.

### Acceptance Criteria

- Синтаксическая ошибка или failed `go test ./...` автоматически возвращает задачу Разработчику.
- Неприменимая команда проверки автоматически возвращает задачу Тестировщику.
- Ревьюерский `blocked` про исправимую ошибку кода не показывает пользователю “нужно вмешательство”.
- Отсутствие токена/API key или CTF scope останавливает workflow и просит пользователя.

## V0.8.6 - Review Gate 2.0

### Цель

Ревью должно быть обязательным контрольным gate перед итогом workflow, а не просто текстовым мнением модели.

### Фичи

- Ревьюер получает единый контекст: живую task spec, Task Blueprint, workflow summary, примененные changes, unified diff и последние результаты проверок.
- Backend строит deterministic checklist по категориям:
  - `spec` - цель, требования, acceptance criteria;
  - `blueprint` - стек, runtime, scaffold, ожидаемые файлы;
  - `diff` - примененные/непримененные changes, лишние файлы, полнота diff;
  - `tests` - только последние результаты проверок по каждой команде;
  - `security` - секреты, `.env`, опасные команды, scope и Code Execution Policy;
  - `quality` - patch/JSON/transcript-мусор, escaped `\n`, заглушки и грубый rewrite без причины.
- Если модель-ревьюер принимает работу, но deterministic gate нашел `critical` или `major`, итог принудительно становится `needs_work`.
- Gate выбирает маршрут возврата:
  - `product` - неполная/противоречивая спека;
  - `architect` - неверный blueprint, runtime, scaffold или список файлов;
  - `developer` - код, diff, безопасность кода, качество;
  - `tester` - проверки не выбраны, не завершены или не применимы;
  - `user` - только настоящий блокер: scope, секрет/API key, конфликт требований или внешняя инфраструктура.

### Acceptance Criteria

- Ревью не может принять код с захардкоженным секретом, `.env`, записанным patch вместо файла или отсутствующим diff.
- Упавшие/незавершенные/отсутствующие проверки после изменений возвращают workflow к тестировщику или разработчику, а не требуют вмешательства пользователя.
- Старые failed-проверки не блокируют итог, если последняя попытка той же команды прошла.
- Findings имеют `category`, чтобы UI мог показывать причину и маршрут без парсинга текста.

## V0.9.0 - Research Squad

### Цель

Интернет-поиск и аналитика должны идти через отдельную команду с проверкой источников и сохранением заметок, а не через один монолитный ответ Люмен.

### Фичи

- Seed-группа `Research Squad` с ролями:
  - `researcher` - план поиска, сбор источников, evidence;
  - `source_reviewer` - freshness/trust/link/citation review;
  - `analyst` - сравнение, синтез, выводы;
  - `manager` - рамка задачи и финальный итог.
- Lifecycle:
  - `web_research`;
  - `source_review`;
  - `research_synthesis`;
  - `research_notes`;
  - `manager_final`.
- Источники сохраняются в `web_sources`.
- Research notes сохраняются в `docs/research-notes.md` и регистрируются как artifact.
- В чат выводится только значимая информация: краткий ответ, ограничения и источники.

### Acceptance Criteria

- Research intent не запускает dev pipeline и не теряет steps в UI.
- У research workflow видны отдельные агенты: Исследователь, Проверяющая источники, Аналитик, Люмен.
- Ответ не содержит сырой JSON и не выдумывает ссылки.
- Notes остаются в проекте для будущих задач и Project Memory.

## V0.9.1 - Web Sources UI

### Цель

Источники должны быть отдельным проверяемым UI-артефактом, а не частью болтливого сообщения или JSON-дампа.

### Фичи

- Нижняя плашка `Источников N` рядом с diff/step dock.
- Popover со списком источников текущего workflow run.
- Для каждого источника показываются:
  - активная ссылка;
  - домен;
  - тип источника;
  - дата получения;
  - trust badge;
  - краткая выжимка;
  - кнопки `Открыть` и `Копировать`.
- Дедупликация по URL на уровне UI.
- Финальный research answer не дописывает отдельный markdown-раздел `Источники`.

### Acceptance Criteria

- Источники не отображаются в правом сайдбаре как дубль.
- В чате нет сырого JSON/YAML/dump источников.
- Ссылки открываются через Wails `BrowserOpenURL`.
- Пользователь может скопировать URL источника одной кнопкой.

## V0.9.3 - CTF Workspace UI

### Цель

CTF-задача должна иметь отдельный рабочий экран с состоянием challenge, а не растворяться в чате и отдельных markdown-файлах.

### Фичи

- `ProjectState.ctfWorkspace` как агрегированный DTO для UI.
- Экран над чатом для CTF workflow:
  - категория;
  - scope status;
  - workspace root;
  - paths для `artifacts`, `evidence`, `solve`, `writeup`.
- Карточки workspace:
  - challenge/category;
  - scope;
  - artifacts;
  - hypotheses;
  - attempts;
  - evidence;
  - solver scripts;
  - writeup.
- Список файлов CTF workspace с копированием relative path.
- Данные берутся из `ctf/<slug>/challenge.yml`, `scope.md`, `notes.md`, `writeup.md`, directories и outputs CTF lifecycle.

### Acceptance Criteria

- Dev/research workflow не показывает CTF workspace.
- CTF workflow показывает workspace сразу после появления CTF steps/artifacts.
- Секции не показывают raw JSON и не требуют читать `.zavod`.
- Длинные секции не ломают высоту чата.
- UI остается читаемым на узком окне.

## V0.9.4 - CTF Tool Profiles

### Цель

CTF Cell должен выбирать инструменты по категории задачи, а не работать одним общим security allowlist.

### Фичи

- Tool profile на каждую категорию: `web`, `LFI`, `RCE`, `SQLi`, `pwn`, `crypto`, `reverse`, `forensics`.
- Маппинг `ctf.ToolProfileID(category)`.
- Проверка команд через `executionpolicy.EvaluateToolProfile(profileID, command)`.
- Runner/checker API: `checks.ValidateCommandWithToolProfile` и `checks.RunWithToolProfile`.
- Seed/upsert `tool_profiles` для новых и существующих локальных БД.
- Solver scripts запускаются только через project `.venv`.
- Примеры профилей:
  - `pwn`: `pwntools` через `.venv`, `checksec/readelf/objdump/nm`, debugger с подтверждением;
  - `forensics`: `binwalk/exiftool/xxd`, extract/tshark с подтверждением;
  - `SQLi`: `sqlmap` только с явным scope и подтверждением.

### Acceptance Criteria

- У каждой CTF-категории свой allowlist и свой profile id.
- Лишние инструменты не протекают между категориями.
- Активные сетевые проверки требуют explicit CTF scope.
- Старые проекты получают обновленные profiles при запуске seed.

## V0.9.5 - CTF Evidence Store

### Цель

Отделить доказательства и сырые outputs CTF-работы от чата. Чат должен показывать только значимую выжимку и ссылки на evidence/writeup.

### Фичи

- `ctf/<challenge>/evidence/index.md` как человекочитаемый индекс.
- `ctf/<challenge>/evidence/events.jsonl` как машинный журнал.
- Отдельные markdown-записи для:
  - command outputs;
  - found files;
  - payload notes;
  - screenshots;
  - pcap analysis;
  - solver outputs;
  - validation evidence.
- `ctf.RecordEvidence` для безопасной записи evidence внутри project workspace.
- CTF workflow автоматически пишет outputs шагов в evidence store.
- `CTFWorkspaceDTO.evidenceIndex/evidenceEvents`.
- UI Evidence читает `evidence/index.md`.

### Acceptance Criteria

- Новая CTF-задача сразу содержит `evidence/index.md` и `evidence/events.jsonl`.
- Solver output хранится отдельной evidence-записью.
- Чат не обязан содержать raw outputs целиком.
- JSONL не отображается как текст сообщения.
- Evidence paths защищены от path traversal.

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
