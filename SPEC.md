# AI Завод: Спека V0.1

## Планируемое исправление V1.0.6.1

[Intent Routing и надёжность Lifecycle](docs/INTENT_AND_LIFECYCLE_RELIABILITY_FIX.md):
вопросы и примеры без dev workflow, отдельный бюджет сетевых повторов, защита
от циклических возвратов, сохранение причины остановки и единый прогресс в UI.
Статус: спецификация к реализации.

## V1.0.6.1 - Базовый Tool Runtime

Диагностика проекта использует `list_files`, `read_file`, `search_files`, `run_check`
в цикле function calling. Права и политика проверяются до запуска; результаты
привязаны к чату и доступны в поповере инструментов. Для передачи файлов и логов
модели требуется разрешение на текущий запрос. Детали, лимиты и границы реализации:
[Tool Runtime V1.0.6.1](docs/TOOL_RUNTIME_V1_0_6_1.md).

## V0.8.6 - Review Gate 2.0

Ревьюер является обязательным gate перед финальным ответом. Он проверяет не только текстовый итог агента, но и фактический набор данных workflow: живую task spec, Task Blueprint, примененные изменения, unified diff, последние проверки, безопасность и качество.

Backend дополнительно строит deterministic Review Gate checklist. Если модель-ревьюер ошибочно отвечает `accepted`, но gate находит `critical` или `major` замечания, результат принудительно переводится в `needs_work` и возвращается нужной роли: `product`, `architect`, `developer`, `tester` или `user`.

Категории замечаний:

- `spec` - цель, требования, acceptance criteria;
- `blueprint` - стек, runtime, scaffold и ожидаемые файлы;
- `diff` - лишние файлы, непримененные changes, пустой или неполный diff;
- `tests` - последние результаты проверок;
- `security` - секреты, `.env`, опасные команды, scope и Code Execution Policy;
- `quality` - patch/JSON/transcript-мусор, escaped `\n`, заглушки и грубый rewrite без причины.

Acceptance criteria:

- работа с захардкоженным секретом, `.env`, patch-текстом вместо файла или отсутствующим diff не принимается;
- отсутствие проверок после изменений возвращает задачу тестировщику;
- failed/blocked проверки возвращают задачу разработчику или тестировщику, если проблема исправима агентами;
- пользовательское вмешательство требуется только для scope, секрета/API key, конфликта требований или недоступной внешней инфраструктуры.

## V0.9.0 - Research Squad

Интернет-поиск и аналитика выполняются отдельной командой, а не одиночной Люмен. Research Squad используется для запросов типа “загугли”, “найди актуальное”, “сравни источники”, “дай аналитику” и “проверь свежую информацию”.

Роли команды:

- `researcher` / Исследователь - формирует поисковые запросы, собирает публичные источники и evidence;
- `source_reviewer` / Проверяющая источники - проверяет свежесть, доверие, прямые ссылки и противоречия;
- `analyst` / Аналитик - сравнивает источники, отделяет факты от выводов и собирает ответ;
- `manager` / Люмен - держит рамку запроса и финальный пользовательский итог.

Lifecycle:

1. `web_research` - план поиска и сбор источников.
2. `source_review` - проверка качества источников.
3. `research_synthesis` - аналитика, сравнение и ответ.
4. `research_notes` - сохранение заметок исследования.
5. `manager_final` - финализация workflow.

Данные исследования:

- найденные источники сохраняются в SQLite `web_sources`;
- research notes сохраняются в проектный файл `docs/research-notes.md`;
- `research_notes` регистрируется как artifact задачи;
- в чат выводится только значимая информация: краткий ответ, ограничения и ссылки.

Acceptance criteria:

- новая установка получает seed-группу `Research Squad`;
- шаблон `Research Squad` доступен в библиотеке групп;
- research workflow показывает отдельные шаги поиска, проверки источников, аналитики и notes;
- ответы с актуальными фактами ссылаются на источники обычным Markdown `[название](https://example.com)`;
- старые/слабые/противоречивые источники должны быть явно отмечены в source review или ограничениях ответа.

## V0.9.1 - Web Sources UI

Источники research workflow отображаются отдельным UI-блоком, а не как служебный JSON или длинный список в сообщении Люмен.

Поведение:

- источники берутся из `web_sources` текущего workflow run;
- повторяющиеся URL схлопываются в один источник;
- в чате остается только значимый ответ: факты, выводы, ограничения;
- полный список источников показывается компактной плашкой над полем ввода рядом с diff/steps;
- по нажатию плашка раскрывает popover со всеми источниками.

Карточка источника:

- активная ссылка с названием;
- домен, тип источника и дата получения;
- уровень доверия с цветовым бейджем;
- краткая выжимка из snippet/content excerpt;
- действия `Открыть` и `Копировать`.

Acceptance criteria:

- в сообщениях нет сырого JSON/YAML/dump источников;
- ссылки открываются во внешнем браузере;
- URL можно скопировать без ручного выделения;
- слабые/неизвестные trust level визуально отличаются от `high`;
- источники не дублируются в правом сайдбаре и в теле сообщения.

## V0.9.3 - CTF Workspace UI

CTF-задачи отображаются отдельным рабочим экраном, а не только сообщениями в чате. Экран собирает состояние из CTF lifecycle, `ctf/<challenge>` workspace-файлов и task artifacts.

Структура экрана:

- верхняя сводка: title, category, scope status, root path;
- пути workspace: `artifacts`, `evidence`, `solve`, `writeup`;
- карточки:
  - category/challenge;
  - scope;
  - artifacts;
  - hypotheses;
  - attempts;
  - evidence;
  - solver scripts;
  - writeup;
- отдельный список файлов workspace с копированием относительного пути.

Backend:

- `ProjectState` содержит `ctfWorkspace`;
- DTO собирается только для CTF workflow или CTF artifacts;
- source of truth остается файловым: `challenge.yml`, `scope.md`, `notes.md`, `writeup.md`, директории `artifacts/`, `evidence/`, `solve/`;
- outputs шагов используются как live-preview до появления содержимого в файлах;
- UI не парсит сырые workflow dumps.

Acceptance criteria:

- обычный dev/research проект не показывает CTF workspace;
- CTF-проект сразу показывает категорию, scope и основные workspace-пути;
- hypotheses/attempts/evidence/solver/writeup видны отдельными секциями;
- файлы CTF workspace отображаются без служебных `.zavod` артефактов;
- длинный markdown в секциях скроллится внутри карточки и не ломает layout.

## V0.9.4 - CTF Tool Profiles

CTF-агенты используют разные allowlist-инструменты по категориям challenge. Tool profile выбирается по категории workspace и передается агенту как часть контекста, чтобы `web`, `LFI`, `RCE`, `SQLi`, `pwn`, `crypto`, `reverse` и `forensics` не работали одним общим набором команд.

Backend:

- `ctf.ToolProfileID(category)` возвращает profile id для текущей CTF-категории;
- `executionpolicy.EvaluateToolProfile(profileID, command)` проверяет команду по конкретному tool profile;
- `checks.ValidateCommandWithToolProfile` и `checks.RunWithToolProfile` дают runner-путь для CTF-команд с теми же правилами;
- `tool_profiles` seed обновляется через upsert, чтобы существующие локальные БД получали новые allowlist;
- CTF solver input содержит текущий tool profile;
- CTF Validator использует отдельный `tool_ctf_validator`.

Профили:

- `tool_ctf_web`, `tool_ctf_lfi`, `tool_ctf_rce`: `.venv` solver scripts и локальный `file/strings` автоматически, HTTP/network действия только при явном CTF scope и подтверждении;
- `tool_ctf_sqli`: `.venv` solver scripts и локальный анализ автоматически, `curl/sqlmap` только с подтверждением и scope;
- `tool_ctf_pwn`: `file`, `strings`, `checksec`, `readelf`, `objdump`, `nm` автоматически; `pwntools` только через project `.venv`; `gdb/ROPgadget/one_gadget` с подтверждением;
- `tool_ctf_crypto`: solver scripts через `.venv` автоматически, `sage` с подтверждением;
- `tool_ctf_reverse`: static triage (`file/strings/readelf/objdump/nm`) автоматически, `radare2/r2/ghidra` с подтверждением;
- `tool_ctf_forensics`: `file`, `strings`, `exiftool`, `binwalk` без extract, `xxd` автоматически; `binwalk -e`, `foremost`, `tshark` с подтверждением.

Acceptance criteria:

- каждая CTF-категория имеет собственный profile id и человекочитаемый allowlist;
- pwn-профиль содержит `pwntools` через `.venv`, но не разрешает forensic tools автоматически;
- forensics-профиль содержит `binwalk/exiftool`, но extract-операции требуют подтверждения;
- solver scripts запускаются только через project `.venv`;
- shell-операторы, destructive-команды и активные security tools вне scope запрещены.

## V0.9.5 - CTF Evidence Store

CTF workflow хранит доказательства отдельно от чата. Чат остается кратким: статус, выводы и ссылки на workspace-файлы. Сырые outputs, найденные файлы, payload notes, screenshots, pcap-разборы и solver outputs живут в `ctf/<challenge>/evidence/`.

Файловая структура:

- `evidence/index.md` - человекочитаемый индекс evidence entries;
- `evidence/events.jsonl` - машинный append-only журнал evidence entries;
- `evidence/<timestamp>-<step>-<kind>-<title>.md` - отдельная запись с metadata, summary и полным content;
- `solve/` остается местом для solver scripts, но outputs solver записываются в evidence.

Backend:

- `ctf.EvidenceEntry` описывает тип, источник, step, агента, summary, content, metadata и relative path;
- `ctf.RecordEvidence` безопасно пишет markdown entry, обновляет `index.md` и дописывает `events.jsonl`;
- `PrepareWorkspace` создает `evidence/index.md` и `evidence/events.jsonl` сразу при создании CTF workspace;
- CTF workflow сохраняет outputs ключевых шагов как evidence entries;
- `CTFWorkspaceDTO` содержит `evidenceIndex` и `evidenceEvents`;
- UI-секция Evidence строится из `evidence/index.md`, а не из сырого workflow dump.

Типы evidence:

- `command_output`;
- `found_file`;
- `payload_note`;
- `screenshot`;
- `pcap_analysis`;
- `solver_output`;
- `validation`;
- `agent_output`;
- `note`.

Acceptance criteria:

- новая CTF-задача сразу получает evidence store;
- outputs CTF workflow не обязаны попадать в чат целиком и доступны через evidence files;
- `evidence/index.md` содержит ссылки на отдельные записи;
- `events.jsonl` можно использовать для будущего UI/экспорта;
- пути evidence не могут выйти за пределы project workspace.

## V1.0.0 - Group Lifecycle Runtime

Кастомная группа агентов должна исполняться не как hardcoded список шагов, а как runtime lifecycle: шаги могут ветвиться, ждать пользователя, возвращать задачу назад, ретраиться, запускать независимые ветки параллельно и завершаться по критериям.

Source of truth:

- `agentgroups.LifecycleDefinition` задает лимиты runtime: `max_total_iterations`, `max_repair_iterations`, `same_error_limit`;
- `agentgroups.LifecycleStep` задает step key, агента, mode, required/retry/transition поля;
- расширенный runtime config хранится в `LifecycleStep.output_schema` как JSON object, чтобы не ломать старую БД и UI;
- пустой или не-JSON `output_schema` трактуется как обычная output schema без runtime-настроек.

Поддерживаемые modes:

- `llm` - обычный шаг модели;
- `tool` - инструментальный шаг;
- `checks` - проверки/тесты;
- `review` - review gate;
- `artifact` - запись файлов/артефактов;
- `final` - финальный шаг, завершает lifecycle после успеха;
- `human_gate` - ожидание явного ввода/подтверждения пользователя;
- `branch` - выбор следующего шага по conditions;
- `parallel` - запуск набора независимых шагов;
- `join` - ожидание завершения parallel-группы.

Runtime config fields:

```json
{
  "condition": {"field": "var:intent", "operator": "equals", "value": "ctf"},
  "conditions": [],
  "branches": [
    {"when": {"field": "output", "operator": "contains", "value": "needs_work"}, "next": "developer_plan"},
    {"default": true, "next": "manager_final"}
  ],
  "parallel": ["lint", "tests"],
  "parallelSteps": ["lint", "tests"],
  "parallelWait": "all",
  "join": "review",
  "joinStepKey": "review",
  "humanGate": {
    "reason": "Подтверди scope",
    "requiredInputs": ["target", "authorization"]
  },
  "completion": [
    {"when": {"field": "output", "operator": "contains", "value": "accepted"}, "status": "done"}
  ],
  "returnTo": "developer_plan",
  "return_to": "developer_plan",
  "returnToStepKey": "developer_plan",
  "critical": true
}
```

Runtime decisions:

- `run` - выполнить текущий шаг;
- `run_parallel` - выполнить набор parallel targets;
- `retry` - повторить текущий шаг, если есть retry budget;
- `jump` - перейти к указанному шагу;
- `wait_human` - остановиться до ввода пользователя;
- `skip` - пропустить шаг, если condition false;
- `blocked` - остановить workflow из-за ошибки runtime/required step;
- `complete` - завершить lifecycle.

Acceptance criteria:

- runtime валидирует ссылки `on_success`, `on_failure`, `returnTo`, `join`, `branches.next`, `parallel`;
- `human_gate` не запускает модель сам по себе, а ждет пользователя и после approval продолжает workflow;
- `parallel` возвращает список шагов для независимого запуска и умеет ждать `all` или `any`;
- failures сначала расходуют retry budget, затем уходят в `returnTo`/`on_failure`, и только потом становятся настоящим blocker;
- `final` и `completion` rules умеют завершать workflow;
- frontend editor поддерживает modes `branch`, `parallel`, `join`.

## V1.0.1 - Visual Lifecycle Editor

Lifecycle editor должен показывать кастомный pipeline как визуальный граф, а не только как форму и плоский список. Пользователь должен быстро видеть, кто выполняет шаг, какой у шага режим, можно ли его повторять, обязательный он или нет, и куда workflow пойдет при успехе/ошибке/ветвлении.

Source of truth:

- визуальный редактор использует существующие `LifecycleStep` записи;
- переходы берутся из `on_success_step_key`, `on_failure_step_key` и runtime JSON в `output_schema`;
- отдельная UI-only схема lifecycle не создается;
- редактирование карточки открывает существующую форму шага, чтобы не было двух разных способов сохранить разные данные.

Карточка шага показывает:

- номер в sort order;
- title и `step_key`;
- назначенного агента и role key;
- mode: `llm`, `tool`, `checks`, `review`, `artifact`, `final`, `human_gate`, `branch`, `parallel`, `join`;
- `required` или `optional`;
- retry policy: `retry N` или `no retry`;
- скрыт ли шаг из хода работы;
- human gate marker, если есть `humanGate`;
- validation issues по этому шагу.

Связи:

- default next стрелка идет к следующему шагу по sort order;
- success стрелка идет к `on_success_step_key`, если он задан;
- failure стрелка идет к `on_failure_step_key` или `returnTo`;
- branch стрелки читаются из `branches[].next` / `branches[].nextStepKey`;
- parallel стрелки читаются из `parallel` / `parallelSteps`;
- join стрелка читается из `join` / `joinStepKey`.

Acceptance criteria:

- в настройках группы есть визуальный блок lifecycle с карточками шагов;
- карточка шага открывает редактирование шага;
- из карточки можно удалить шаг;
- UI показывает mode, agent, retries, required/optional и runtime links;
- runtime validation issues видны рядом с графом;
- старый lifecycle editor остается совместимым и сохраняет те же `LifecycleStep`.

## V1.0.2 - Agent Runtime Dashboard

Верхняя панель должна показывать не только текущего агента, но и runtime-контекст: кто сейчас работает, что именно делает, почему ожидает, какая модель, tool profile и soul используются, сколько времени и токенов ушло.

Source of truth:

- runtime telemetry хранится в `agents.Status` и приходит через существующие `agent_status_changed` / `chat_state_changed`;
- отдельная таблица telemetry в V1.0.2 не требуется;
- LLM usage берется из OpenAI-compatible `usage`, если provider его вернул;
- если provider не вернул usage, UI пишет `нет данных`, а не показывает выдуманные токены.

Backend telemetry fields:

- `stepKey` - текущий lifecycle/workflow step;
- `modelId` - модель, с которой работает агент;
- `toolId` - tool profile/runtime context, если известен;
- `soulPath` - файл `soul.md`, использованный при prompt assembly;
- `startedAt`, `elapsedMs` - время активной работы агента;
- `inputTokens`, `outputTokens`, `totalTokens` - usage LLM-вызова.

UI dashboard показывает:

- активного runtime-агента;
- текущий step;
- model/tool/soul;
- elapsed time;
- token usage;
- сколько агентов сейчас работают и сколько ждут пользователя/доработки.

Acceptance criteria:

- dashboard виден рядом с активным агентом;
- при LLM-шаге отображается актуальная модель и `soul.md`;
- после ответа модели отображаются токены, если provider прислал usage;
- при ожидании пользователя видно, что агент ждет;
- старый popover агента остается про роль/зачем/ответственность, а dashboard - про runtime.

## V1.0.3 - Skills per Agent

К каждому агенту можно подключить skills по умолчанию: например `pony-tail`, `security`, `research`, `ctf` или кастомный skill. Skills становятся частью `AgentProfile`, а не глобальной скрытой настройкой пайпа.

Source of truth:

- `AgentProfile.defaultSkills` - список skills агента;
- `agent_profiles.skills_json` - persisted storage в SQLite;
- `soul.md` отвечает за личность/стиль агента, `defaultSkills` отвечает за подключенные рабочие режимы;
- prompt assembly добавляет отдельный блок `Default Skills` рядом с `soul.md` и capability contract.

Нормализация:

- `$pony-tail` и `pony-tail` считаются одним skill;
- регистр не важен;
- пустые строки и дубли удаляются;
- если список пустой, backend подставляет role-based default.

Role defaults:

- manager/product/custom: `pony-tail`;
- developer/tester/reviewer/architect/docs/release: `pony-tail`, `dev`;
- researcher/source_reviewer/analyst: `pony-tail`, `research`;
- security/threat_modeler/remediator: `pony-tail`, `security`;
- CTF-роли: `pony-tail`, `ctf`.

UI:

- в карточке агента видны skill-чипы;
- в редакторе агента есть быстрые переключатели `pony-tail`, `dev`, `research`, `security`, `ctf`;
- можно вписать кастомные skills по одному на строку;
- библиотека агентов показывает skills и переносит их при добавлении агента или замене контракта.

Acceptance criteria:

- skills сохраняются и восстанавливаются после перезапуска;
- skills из библиотеки и шаблонов не теряются при создании/копировании/замене агента;
- prompt конкретного агента содержит именно его default skills;
- `$skill` в UI и API не создает дубль рядом с `skill`;
- старые профили без `skills_json` автоматически получают дефолт по роли.

## V1.0.4 - Hermes-like Orchestration

Люмен становится явным orchestrator: перед ответом или запуском workflow она принимает structured decision, выбирает режим исполнения, группу агентов и lifecycle, пропускает лишние шаги и может объяснить это решение.

Source of truth:

- `router.Decision` классифицирует intent пользователя;
- `orchestration.Decision` решает execution mode, группу, lifecycle и skipped steps;
- выбранная группа хранится в `project_group_bindings`;
- lifecycle берется из `AgentGroup.defaultLifecycleID`;
- наличие живой спеки и Project Memory учитывается в orchestration decision как сигнал, но содержимое Project Memory не отправляется во внешнюю модель без отдельной политики безопасной фильтрации.

Execution modes:

- `direct` - для вопросов, объяснений, вывода спеки/памяти, workflow control и общего чата;
- `workflow` - для разработки, research, CTF и defensive security задач.

Group selection:

- coding/clarification -> `Dev Squad`;
- internet/research -> `Research Squad`;
- explicit CTF/lab/challenge/flag/writeup -> `CTF Cell`;
- security audit/threat model/plain vulnerability request -> `Security Audit`;
- direct answers сохраняют текущую группу проекта и не запускают новый workflow.

No extra steps:

- direct answer пропускает requirements/blueprint/development/checks/review;
- research workflow не гоняет dev requirements/blueprint/checks/review;
- CTF workflow не гоняет обычный dev pipeline;
- security workflow не уходит в CTF без явного CTF/lab контекста.

Explainability:

- orchestration decision содержит `reason`, `explanation`, `groupName`, `groupKind`, `lifecycleId`, `skippedSteps`, `usedMemory`;
- runtime dashboard показывает статус `orchestrating`;
- direct answer context получает краткое объяснение orchestration decision, чтобы Люмен могла отвечать на вопросы “почему пошла так”.

Acceptance criteria:

- вопрос “выведи спеку” отвечает напрямую и не запускает workflow;
- research-запрос выбирает `Research Squad`;
- CTF-запрос с явным CTF/lab/challenge выбирает `CTF Cell`;
- обычный SQLi/security audit без CTF-контекста выбирает `Security Audit`;
- coding-задача выбирает `Dev Squad`;
- выбранная workflow-группа автоматически привязывается к проекту перед запуском;
- решение покрыто unit-тестами отдельно от UI.

## V1.0.7 - Chats & Projects UX

Реализован отдельный жизненный цикл чатов: быстрый старт без проекта, история
внутри проектов, выбор рабочей папки перед выполнением файловой задачи,
настройки команды/модели на чат и очередь выполнения внутри проекта.
Контракт и ограничения: [Chats & Projects UX](docs/CHAT_PROJECT_UX.md).

## V1.0.5 - Universal Lifecycle Runtime

Сейчас в приложении есть lifecycle editor/executor, но реальные пайплайны частично исполняются отдельными ветками backend-кода: dev, research, CTF и security живут разными функциями. V1.0.5 должна сделать один универсальный runtime, который исполняет любой `LifecycleDefinition` и `LifecycleStep`, выбранный Люмен в orchestration decision.

Цель:

- один execution engine для Dev, Research, Security, CTF и кастомных групп;
- lifecycle из UI должен быть не “картинкой/планом”, а реальным source of truth исполнения;
- специальные возможности dev/research/CTF/security подключаются через step mode, tool profile и step handlers, а не через отдельные hardcoded workflows;
- пользователь вмешивается только на `human_gate` или настоящем blocker.

Source of truth:

- `AgentGroup` выбирает команду;
- `ProjectGroupBinding` хранит выбранную группу и lifecycle проекта;
- `LifecycleDefinition` задает лимиты итераций, retries и общий режим;
- `LifecycleStep` задает `stepKey`, `agentProfileId`, `mode`, `required`, `canRetry`, `maxRetries`, `onSuccessStepKey`, `onFailureStepKey`, `outputSchema`, `visibleToUser`;
- `LifecycleExecutor` вычисляет следующий шаг, retry/return/branch/join/completion;
- `WorkflowRun`, `WorkflowStep`, `WorkflowPlan`, `WorkflowPlanStep` фиксируют фактическое исполнение и UI-прогресс.

Step modes:

- `llm` - вызвать агента с его `soul.md`, default skills, capabilities и контекстом задачи;
- `tool` - выполнить backend tool handler по `stepKey`/`toolProfileId`;
- `checks` - запустить test/check runner через Code Execution Policy;
- `review` - запустить Review Gate 2.0;
- `artifact` - сохранить/обновить артефакт задачи;
- `human_gate` - остановить workflow в `waiting_user` с понятным вопросом и resume после ответа;
- `final` - собрать итог и закрыть workflow.

Control flow:

- Если шаг успешен, runtime идет в `onSuccessStepKey`, если он задан, иначе в следующий visible/ordered step.
- Если шаг упал и `canRetry=true`, runtime повторяет его до `maxRetries`.
- Если retry исчерпан и задан `onFailureStepKey`, runtime возвращается туда с причиной.
- Если failure похож на настоящий blocker: scope, секрет, конфликт требований, внешний недоступный сервис, runtime ставит `waiting_user` или `blocked`.
- Условия/ветки читаются из structured runtime config в `outputSchema`:
  - `condition`;
  - `branches`;
  - `parallelSteps`;
  - `joinStepKey`;
  - `completionRules`;
  - `humanGate`.
- Parallel steps в V1.0.5 допускаются как runtime abstraction: можно исполнять последовательно с единым join, но trace/UI должны показывать их как parallel group.

Нужно заменить hardcoded paths:

- `runV03Workflow` должен стать adapter'ом или исчезнуть: dev lifecycle исполняется через Universal Lifecycle Runtime;
- `runWebResearchWorkflow` должен быть набором handlers для modes/stepKeys `web_research`, `source_review`, `research_synthesis`, `research_notes`, `manager_final`;
- `runCTFWorkflow` должен быть набором handlers для `ctf_intake`, `scope_check`, `artifact_collection`, `triage`, `hypothesis_board`, `category_solver`, `validation`, `writeup`;
- `runSecurityWorkflow` должен исполняться lifecycle steps `security_scope`, `security_analysis`, `threat_model`, `remediation_plan`, `review`, `manager_final`;
- legacy fallback остается только для миграции, если у проекта нет lifecycle.

Runtime context для шага:

- user request;
- live Task Spec;
- accepted answers;
- project memory summary по безопасной policy;
- latest relevant workflow outputs;
- selected group/lifecycle/agent;
- step runtime config;
- tool profile;
- agent `soul.md`;
- agent default skills;
- capability contract;
- allowed read/write paths.

Outputs:

- каждый step сохраняет raw output в `WorkflowStep`;
- user-visible summary очищается от JSON и служебного шума;
- structured outputs парсятся step handler'ом;
- изменения файлов идут только через controlled changes/apply;
- evidence/source/research/CTF artifacts сохраняются в свои stores, не в чат.

UI:

- плашка lifecycle рядом с diff показывает реальный runtime graph;
- popover показывает все dynamic steps, текущий step, retries, return links, skipped/parallel/join;
- если workflow ждет пользователя, видно какой `human_gate` и какие input нужны;
- кастомный lifecycle из editor исполняется тем же runtime без отдельной backend-ветки.

Acceptance criteria:

- Dev Squad проходит через lifecycle steps, а не через отдельный hardcoded dev loop;
- Research Squad исполняется тем же runtime и вызывает web research handlers;
- CTF Cell исполняется тем же runtime и сохраняет CTF workspace/evidence;
- Security Audit исполняется тем же runtime;
- кастомная группа с 2-3 `llm` шагами реально исполняется без добавления Go-кода;
- `onFailureStepKey` возвращает задачу нужному шагу;
- `human_gate` ставит run в `waiting_user` и resume продолжает с нужного места;
- retries считаются по step/lifecycle лимитам;
- UI показывает один фактический progress graph без дубликатов;
- существующие тесты dev/research/CTF/security остаются зелеными.

Implementation plan:

1. Добавить `internal/lifecycleruntime` с `Runner`, `StepHandler`, `RuntimeContext`, `StepResult`.
2. Зарегистрировать handlers для базовых modes: `llm`, `tool`, `checks`, `review`, `artifact`, `human_gate`, `final`.
3. Перенести dev/research/CTF/security функции в handlers, оставив старые функции как thin compatibility wrappers на время миграции.
4. На старте workflow брать `orchestration.Decision.GroupID/LifecycleID`, загружать `LifecycleExecutor` и запускать `Runner`.
5. Сделать resume для `waiting_user`: accepted answers пишутся в Task Spec Store, runtime продолжает с gate или configured next step.
6. Добавить trace events и нормальные stop reasons на уровне runtime.
7. Обновить UI progress/popover, чтобы он читал фактические `WorkflowPlanStep`/`WorkflowStep`.
8. Покрыть unit/integration тестами branching, retries, return, human gate, custom lifecycle, dev/research/CTF/security adapters.

Implementation baseline:

- `internal/lifecycleruntime.Runner` исполняет `LifecycleExecutor.NextAction` через registry step handlers.
- Runner поддерживает ordered execution, retries, return/jump, blocked/done/waiting statuses, human gate stop, parallel abstraction with join semantics.
- App service использует Runner для dev/custom lifecycle вместо собственного цикла `runV03RuntimeLifecycle`.
- Resume из `waiting_user` переиспользует существующий `WorkflowRun`, восстанавливает runtime state из сохраненных `WorkflowStep` и продолжает следующий шаг.
- Return target forced rerun переисполняет уже пройденный шаг, чтобы repair-loop не застревал на старом успешном результате.
- Research/CTF/Security специализированные функции остаются compatibility wrappers до полного переноса их внутренних действий в `StepHandler`.

## Цель

Локальное macOS desktop-приложение для управления AI-агентами через чат. Пользователь выбирает проект, пишет задачу, а входной агент "Люмен" принимает ее и отвечает через выбранную модель.

Стек первой версии:

```text
Wails + Go + React + SQLite
```

## Основные требования пользователя

- Приложение запускается локально на macOS.
- Основной интерфейс - чат.
- В чате отображаются сообщения пользователя и агентов.
- Сообщения чата отображаются как Markdown: заголовки, списки, inline-code и code blocks рендерятся визуально, а не показываются сырым текстом.
- У каждого сообщения есть компактная кнопка копирования исходного текста сообщения в clipboard.
- Каждый агент имеет роль и видимый статус.
- Если агент занят, видно, чем он занимается.
- В верхней части чата показывается только активный агент: крупная аватара, имя, статус и расшифровка текущей активности.
- В верхней части чата рядом с активным агентом показываются текущий шаг workflow и компактная карточка активной модели. Карточка модели не дублирует заголовок "Модель пайпа"; она показывает только название модели, provider/model id, health и latency.
- Можно подключать разные модели.
- Для начала нужны OpenAI/ChatGPT и удаленная локальная Qwen-модель по API.
- Разработка идет пошагово.
- На каждом шаге приложение остается минимально рабочим.
- Первый рабочий шаг: интерфейс и один агент, который принимает задание и отвечает.
- Интерфейс по умолчанию на русском языке.
- Нужна мультипроектность.

## Модель подключения LLM

Приложение не привязывается к конкретному способу запуска локальной модели. Используется общий OpenAI-compatible provider.

```text
OpenAI:
  base_url: https://api.openai.com/v1
  api_key: OPENAI_API_KEY
  model: gpt-...

Remote Qwen:
  base_url: http://192.168.x.x:PORT/v1
  api_key: optional
  model: qwen...
```

Ожидаемый формат для удаленной Qwen:

```text
POST /v1/chat/completions
```

Если Qwen API окажется не OpenAI-compatible, нужно добавить отдельный provider-adapter.

## UX V0.1

Основной экран:

```text
+----------------------+----------------------------+----------------------+
| Проекты              | Чат                        | Агенты               |
|                      |                            |                      |
| Поиск проекта        | Пользователь: задача       | Люмен                |
| + Новый проект       | Люмен: ответ               | status: thinking     |
| + Добавить проект    |                            | model: qwen-remote   |
|                      | Поле ввода                 |                      |
| project-1            |                            |                      |
| project-2            |                            |                      |
+----------------------+----------------------------+----------------------+
```

## Проекты

Проекты по умолчанию лежат в каталоге `~/ai_zavod`. Каждый проект хранится в отдельной папке, например:

```text
~/ai_zavod/project-1
~/ai_zavod/project-2
~/ai_zavod/project-3
```

В приложении должны быть:

- список проектов;
- поиск по названию и пути;
- кнопка "Новый проект";
- кнопка "Добавить существующий";
- редактирование названия и пути проекта прямо из списка;
- удаление проекта из списка приложения без удаления каталога на диске;
- запоминание последнего открытого проекта.

В V0.1 проект является контекстом задачи, но агент еще не читает файлы проекта.

## Рабочие каталоги

Код приложения хранится в каталоге:

```text
~/dev_ai_zavod
```

Каталог пользовательских проектов по умолчанию:

```text
~/ai_zavod
```

## Локальное хранилище

SQLite база приложения по умолчанию:

```text
~/dev_ai_zavod/zavod.db
```

В этой базе хранятся проекты, задачи, сообщения, настройки моделей, статусы запусков и история работы агентов.

## Основные сущности

```text
Project
  id
  name
  path
  created_at
  last_opened_at

Task
  id
  project_id
  title
  status
  created_at
  updated_at

Message
  id
  task_id
  role
  agent_id
  content
  created_at

Agent
  id
  role
  name
  status
  model_id

ModelConfig
  id
  name
  provider
  base_url
  api_key_ref
  model_name

AgentRun
  id
  task_id
  agent_id
  status
  started_at
  finished_at
  error
```

## Статусы агента

Для V0.1:

```text
idle
thinking
calling_model
answering
done
failed
```

Для будущих версий:

```text
reading_files
writing_files
running_tests
waiting_user
blocked
```

## Архитектура приложения

```text
React UI
  -> Wails bindings
    -> AppService
      -> Workflow Engine
      -> Agent Runtime
      -> LLM Gateway
      -> SQLite Store
      -> Tool Runtime
```

Wails является оболочкой и мостом между UI и Go. Основная бизнес-логика должна жить в Go-модулях внутри `internal/*`, чтобы позже можно было добавить CLI, HTTP API или другой UI без переписывания ядра.

## Структура проекта

```text
/frontend              React UI
/app.go                Wails bindings

/internal/app          application services для Wails
/internal/project      project catalog
/internal/chat         messages/tasks
/internal/agents       agent runtime
/internal/workflow     task flow
/internal/llm          provider interface
/internal/providers    openai-compatible clients
/internal/store        sqlite repositories/migrations
/internal/config       app config
/internal/tools        future tools: files, git, tests
/internal/artifacts    future markdown/json outputs
```

## LLM Gateway

Единый интерфейс для всех моделей:

```go
type Provider interface {
    Generate(ctx context.Context, req Request) (*Response, error)
    Stream(ctx context.Context, req Request) (<-chan Event, error)
    Capabilities() Capabilities
}
```

Первая реализация:

- OpenAI-compatible provider;
- настройки `base_url`, `api_key`, `model_name`;
- поддержка OpenAI;
- поддержка удаленной Qwen по сети, если она совместима с OpenAI API.

## Agent Runtime V0.1

В первой версии есть один агент:

```text
Люмен
```

Люмен — женский персонаж. В ответах от первого лица и пользовательских текстах, связанных с ее действиями, используется женский род: "поняла", "готова", "приняла", "собрала".

Задачи Люмен:

- принять задачу пользователя;
- кратко подтвердить понимание;
- задать уточняющий вопрос, если задача непонятна;
- если вопрос понятен, дать полезный ответ или первичный план;
- не читать и не изменять файлы проекта в V0.1.

## Workflow V0.1

```text
1. Пользователь выбирает проект.
2. Пользователь пишет задачу в чат.
3. Создается Task.
4. Создается пользовательское Message.
5. ManagerAgent получает задачу.
6. Статус агента меняется:
   idle -> thinking -> calling_model -> answering -> done
7. Ответ агента добавляется в чат.
8. Task, Message и AgentRun сохраняются в SQLite.
9. После перезапуска приложения история остается доступной.
```

## План версий

### V0.1

- Wails app.
- React layout.
- SQLite.
- Список проектов.
- Поиск проектов.
- Создание нового проекта.
- Добавление существующего проекта.
- Чат.
- Один ManagerAgent.
- OpenAI-compatible model endpoint.
- Сохранение сообщений.
- Сохранение истории после перезапуска.

### V0.2

- Экран настроек моделей.
- Несколько model configs.
- Переключение OpenAI / remote Qwen.
- Панель статусов агентов.
- Streaming ответа.

### V0.3

- Несколько ролей:
  - Люмен;
  - Продакт;
  - Архитектор.
- Workflow:
  - задача;
  - требования;
  - план;
  - итоговый ответ.

### V0.4

- Сохранение артефактов workflow в каталог выбранного проекта.
- Главный пользовательский файл:
  - `docs/task-spec.md`.
- Полный след запуска:
  - `.zavod/runs/<workflow-id>/01-manager-task-brief.md`;
  - `.zavod/runs/<workflow-id>/02-product-requirements.md`;
  - `.zavod/runs/<workflow-id>/03-architecture-plan.md`;
  - `.zavod/runs/<workflow-id>/04-manager-summary.md`.
- Метаданные артефактов в SQLite:
  - project_id;
  - task_id;
  - workflow_run_id;
  - agent_id;
  - kind;
  - title;
  - path;
  - relative_path;
  - created_at.
- В UI показывается блок "Артефакты" со списком сохраненных файлов.
- Агент получает статус `writing_files`, пока приложение сохраняет результат.
- Агент не утверждает, что файл создан, если приложение реально не записало файл.

### V0.4.x

- Read-only доступ к проекту:
  - list files;
  - search;
  - read file.

### V0.5

#### V0.5.0

- Новая роль:
  - Разработчик.
- Новый шаг workflow:
  - `developer_plan` после Архитектора и до итогового ответа Люмен.
- Разработчик готовит:
  - developer summary;
  - предлагаемые файлы;
  - план изменений;
  - кодовые блоки или псевдопатчи как текст;
  - проверки;
  - риски.
- Разработчик не изменяет файлы проекта, не запускает команды и не утверждает, что код применен.
- Главный файл `docs/task-spec.md` получает раздел "План разработки".
- Полный след запуска V0.5.0:
  - `.zavod/runs/<workflow-id>/01-manager-task-brief.md`;
  - `.zavod/runs/<workflow-id>/02-product-requirements.md`;
  - `.zavod/runs/<workflow-id>/03-architecture-plan.md`;
  - `.zavod/runs/<workflow-id>/04-developer-plan.md`;
  - `.zavod/runs/<workflow-id>/05-manager-summary.md`.
- В UI верхний workflow показывает пять шагов:
  - Постановка задачи;
  - Требования;
  - Архитектурный план;
  - Разработка;
  - Итог.
- В правой панели появляется агент "Разработчик".
- В SQLite артефакт плана разработки хранится как:
  - `kind = developer_plan`;
  - `agent_id = developer`;
  - `title = План разработки`.

#### V0.5.1

- Controlled edits: приложение пишет кодовые файлы только после явного действия пользователя.
- Разработчик в конце ответа возвращает structured proposed changes:
  - `file_path`;
  - `action = create | replace`;
  - `reason`;
  - `content`.
- Pending changes сохраняются в SQLite-таблицу `proposed_changes`:
  - project_id;
  - task_id;
  - workflow_run_id;
  - agent_id;
  - file_path;
  - action;
  - content;
  - reason;
  - status = pending | applied | failed;
  - error;
  - backup_path;
  - created_at;
  - applied_at.
- UI показывает блок "Изменения":
  - список файлов;
  - действие `создать` или `заменить`;
  - статус;
  - причину;
  - кнопку "Применить изменения".
- Кнопка активна только при наличии `pending` changes.
- Перед применением пользователь видит confirm со списком файлов.
- Без нажатия кнопки кодовые файлы проекта не меняются.
- На V0.5.1 поддерживаются только:
  - создание нового файла;
  - полная замена существующего файла.
- На V0.5.1 не поддерживаются:
  - partial patch hunks;
  - delete;
  - rename;
  - binary files;
  - запуск тестов.
- Правила безопасности:
  - пути только относительные к проекту;
  - запрещены абсолютные пути;
  - запрещен `../`;
  - запрещена запись в `.git`;
  - запрещена запись в `.zavod`;
  - запрещена запись в `zavod.db`;
  - `create` не перезаписывает существующий файл;
  - `replace` требует существующий файл и создает backup в `.zavod/backups/<change-id>/...`;
  - лимит содержимого одного файла: 200 KB.

#### V0.5.2

- Diff viewer после применения controlled edits.
- Для каждого `proposed_change` сохраняются:
  - `before_content`;
  - `after_content`;
  - `diff_text`.
- Для `create`:
  - `before_content = ""`;
  - `after_content = content`;
  - diff показывает создание файла от `/dev/null`.
- Для `replace`:
  - `before_content` берется из существующего файла до записи;
  - `after_content = content`;
  - diff показывает удаленные и добавленные строки;
  - backup продолжает сохраняться в `.zavod/backups/<change-id>/...`.
- UI в блоке "Изменения":
  - для `applied` показывает кнопку "Показать diff";
  - раскрывает monospace diff-блок;
  - показывает backup path для `replace`;
  - для `pending` и `failed` сохраняет компактное отображение статуса и ошибки.
- Ограничения V0.5.2:
  - простой line-based unified diff;
  - нет side-by-side view;
  - нет syntax highlight;
  - нет rollback из UI.

#### V0.5.3

- Добавляется роль "Тестировщик".
- После нажатия "Применить изменения" и успешной записи хотя бы одного файла приложение запускает шаг `tester_commands`.
- Тестировщик анализирует:
  - постановку задачи;
  - требования;
  - архитектурный план;
  - developer plan;
  - список примененных изменений;
  - структуру проекта: наличие `go.mod`, `package.json`, `frontend/package.json`, Python-файлов в корне.
- Тестировщик не запускает команды сам. Он только предлагает JSON:
  - `summary`;
  - `commands[].command`;
  - `commands[].working_dir`;
  - `commands[].reason`.
- Команды сохраняются в SQLite в таблицу `test_runs`.
- Статусы проверок:
  - `pending`;
  - `running`;
  - `passed`;
  - `failed`;
  - `blocked`.
- UI показывает блок "Проверки":
  - команду;
  - рабочий каталог;
  - причину;
  - статус;
  - кнопку "Запустить" или "Повторить";
  - раскрываемый stdout/stderr/error после запуска.
- Приложение запускает команды автоматически только если Code Execution Policy возвращает `auto`.
- Backend запускает команды без shell через `exec.CommandContext`.
- Backend проверяет не только allowlist, но и соответствие команды структуре проекта:
  - Go-команды доступны только при наличии `go.mod` в рабочем каталоге;
  - npm-команды доступны только при наличии `package.json` в рабочем каталоге;
  - Python-команды доступны только если указанный `.py` файл существует внутри рабочего каталога.
- Timeout одной команды: 180 секунд.
- stdout/stderr ограничены 50 KB на поток.
- Code Execution Policy V0.8.4 / `dev` / auto:
  - `go test ./...`;
  - `go test <package starting with ./ >`;
  - `go vet ./...`;
  - `npm test`;
  - `npm run test`;
  - `npm run build`;
  - `npm run lint`.
  - `.venv/bin/python <relative-script.py>`;
  - `.venv/bin/python -m pytest`;
  - `.venv/bin/python -m py_compile <relative-script.py>`.
- Code Execution Policy V0.8.4 / `dev` / confirm:
  - `go mod tidy`, `go get`, `go run`, `go build`, `go generate`;
  - `npm install`, `npm ci`, `npm exec`;
  - `pip install`;
  - `make build`, `make test`, `wails build`;
  - запуск приложения или долгоживущего dev server.
- Code Execution Policy V0.8.4 / `CTF` / auto:
  - локальный анализ challenge-файлов: `file`, `strings`, `objdump`;
  - локальные solver-проверки через `.venv/bin/python`;
  - `.venv/bin/python -m py_compile <relative-script.py>`.
- Code Execution Policy V0.8.4 / `CTF` / confirm:
  - сетевые команды `curl`, `dig`, `whois`;
  - `sqlmap`, `gdb`, `radare2`, `binwalk`, `exiftool`;
  - требуется явный CTF/lab scope и подтверждение пользователя.
- Code Execution Policy V0.8.4 / `research`:
  - auto: только встроенный web research provider;
  - любые shell-команды запрещены.
- Всегда блокируются:
  - shell-операторы `&&`, `||`, `;`, `|`, `>`, `<`, `$()`, backticks;
  - абсолютный `working_dir`;
  - выход `working_dir` за пределы проекта;
  - Python-аргументы кроме одного относительного `.py` файла;
  - `rm/mv/cp/dd/chmod/chown`, `sudo/su`, shell-интерпретаторы, `docker/kubectl/helm/terraform`;
  - активные security-инструменты вне CTF scope;
  - любые команды вне policy.
- Если Тестировщик не вернул валидный JSON, приложение использует fallback:
  - `go test ./...` при наличии `go.mod`;
  - `npm run build` в `frontend` при наличии `frontend/package.json`;
  - `npm run build` в корне при наличии `package.json`.
- Для Python-only проекта fallback выбирает один корневой скрипт:
  - сначала `check.py`;
  - затем `site_health_check.py`, `main.py`, `app.py`;
  - затем первый найденный корневой `.py` файл.
- Неподходящие предложения модели отфильтровываются до создания `test_runs`; если после фильтрации ничего не осталось, применяется fallback по структуре проекта.
- Ограничения V0.5.3:
  - команды выполняются по одной;
  - нет интерактивного terminal view;
  - нет отмены running-команды из UI;
  - нет автозапуска всех проверок.

#### V0.5.4

- Добавляется роль "Ревьюер".
- Ревью запускается вручную после controlled edits и проверок кнопкой "Запустить ревью".
- Ревьюер анализирует только сохраненный контекст:
  - task brief;
  - требования;
  - архитектурный план;
  - developer plan;
  - примененные изменения;
  - unified diff;
  - результаты тестов.
- Ревьюер не читает файлы проекта напрямую, не пишет файлы и не запускает команды.
- Ответ ревьюера должен быть JSON:
  - `status = accepted | needs_work`;
  - `summary`;
  - `findings[]`;
  - `required_changes[]`;
  - `recommended_next_step`.
- `findings[]` содержит:
  - `severity = critical | major | minor | note`;
  - `file_path`;
  - `message`;
  - `suggestion`.
- Результаты сохраняются в SQLite в таблицу `review_runs`.
- Статусы review run:
  - `pending`;
  - `running`;
  - `accepted`;
  - `needs_work`;
  - `failed`.
- UI показывает блок "Ревью":
  - статус;
  - summary;
  - findings;
  - обязательные доработки;
  - recommended next step;
  - кнопку повторного запуска ревью.
- Если `status = accepted`:
  - карточка ревью подсвечивается зеленым;
  - Ревьюер получает статус "Принял работу".
- Если `status = needs_work`:
  - карточка ревью подсвечивается желтым;
  - Разработчик получает статус "Нужна доработка по ревью";
  - в чат добавляется сообщение Ревьюера с замечаниями.
- Если модель не ответила или JSON не распарсился:
  - review run получает `failed`;
  - ошибка сохраняется и отображается в UI.
- Ограничения V0.5.4:
  - автоматический повторный проход Разработчика не запускается;
  - кнопка "Вернуть Разработчику" не создает новый developer iteration;
  - нет inline-комментариев по строкам diff;
  - нет блокирующего правила "ревью только после всех тестов passed" — Ревьюер сам отмечает pending/failed/blocked проверки.

#### V0.6.0 — Autopilot workflow

Цель: убрать обязательные ручные шаги "Применить изменения", "Запустить проверку", "Запустить ревью" из основного сценария. Пользователь формулирует задачу в чат, приложение само ведет workflow до результата или понятной остановки.

- Основной режим по умолчанию: `autopilot`.
- Ручной режим остается как настройка проекта/запуска: `manual`.
- В `autopilot` приложение автоматически выполняет цепочку:
  1. Люмен уточняет задачу или принимает ее в работу.
  2. Продакт формирует требования.
  3. Архитектор формирует технический план.
  4. Разработчик предлагает изменения.
  5. Приложение автоматически применяет безопасные изменения.
  6. Тестировщик предлагает проверки.
  7. Приложение автоматически запускает разрешенные проверки.
  8. Ревьюер анализирует diff и результаты проверок.
  9. Если ревью принято — пользователь получает итоговый отчет.
  10. Если нужны доработки и они безопасны — запускается следующая developer iteration.
- После V0.6.1 эта цепочка получает обязательный `task_blueprint` между требованиями и архитектурой:
  - blueprint фиксирует стек, scaffold, ожидаемые файлы и проверки;
  - Архитектор, Разработчик и Тестировщик обязаны следовать blueprint;
  - если blueprint противоречит задаче или проекту, workflow останавливается в `waiting_user` или `blocked`.

Поведение применения изменений:

- Автозапись использует тот же механизм controlled edits, что и V0.5.1:
  - запись только внутри каталога выбранного проекта;
  - запрет записи в `.zavod`, `zavod.db`, служебные каталоги и файлы вне проекта;
  - `create` не перезаписывает существующий файл с другим содержимым;
  - если `create` указывает на уже существующий файл с идентичным содержимым, это считается успешным no-op применением;
  - `replace` требует существующий файл и сохраняет backup;
  - лимит размера одного файла сохраняется.
- Кнопка "Применить изменения" в `autopilot` не нужна для продолжения workflow.
- UI показывает факт автоприменения:
  - "Изменения применены автоматически";
  - список файлов;
  - diff;
  - backup path для `replace`.
- Если Разработчик вернул секцию `## Proposed changes`, но JSON внутри невалиден, workflow останавливается с явной причиной парсинга.
- Если Task Blueprint ожидает файлы, но Разработчик не вернул применимые `proposed_changes`, workflow останавливается как некорректный developer output.
- Перед остановкой backend делает одну короткую repair-попытку Разработчика: вернуть только валидный `## Proposed changes` JSON без длинного плана и текста после массива.
- Если изменение не проходит safety validation, Autopilot передает ошибку Разработчику на repair-итерацию.
- Если repair-итерации исчерпаны или Разработчик повторяет тот же некорректный результат, workflow останавливается в статусе `blocked` и показывает причину.
- При `blocked` финальный ответ формируется детерминированно приложением: запрещено писать, что работа завершена, все файлы применены или проверки прошли.

Поведение проверок:

- Приложение автоматически запускает только команды из allowlist V0.5.3.
- Перед запуском команда повторно проверяется по структуре проекта:
  - `go test/go vet` только при наличии `go.mod`;
  - `npm` только при наличии `package.json`;
  - `python3/python` только если `.py` файл существует.
- Команды выполняются последовательно, без shell, через `exec.CommandContext`.
- Если команда `blocked`, она не считается падением проекта, но попадает в отчет как "проверка не применима".
- Если команда `failed`, workflow передает stderr/stdout Разработчику на следующую итерацию, если лимит итераций не исчерпан.
- Если подходящих проверок нет, workflow не падает автоматически; Ревьюер получает явный факт "проверки не найдены".

Review and repair loop:

- Ревьюер возвращает `accepted | needs_work | blocked`.
- `accepted` завершает workflow статусом `done`.
- `needs_work` запускает новую итерацию Разработчика, если:
  - есть конкретные `required_changes`;
  - проблема относится к измененным файлам или явно связана с задачей;
  - лимиты итераций не исчерпаны;
  - предыдущая итерация не создала идентичный diff.
- `blocked` останавливает workflow и просит пользователя вмешаться.

Ограничители от бесконечного цикла:

- `max_iterations` по умолчанию: 2 repair-итерации после первой разработки.
- Hard limit: 3 repair-итерации на workflow run.
- `max_changed_files` по умолчанию: 10 файлов за весь workflow.
- `max_total_written_bytes` по умолчанию: 500 KB за весь workflow.
- Запрещен повтор идентичного diff hash в рамках одного workflow.
- Если два раза подряд падает одна и та же проверка с тем же error fingerprint, workflow останавливается.
- Если Ревьюер два раза подряд возвращает однотипное `needs_work`, workflow останавливается и показывает накопленный отчет.
- Если модель три раза подряд возвращает невалидный JSON на обязательном structured-шаге, workflow останавливается.
- Пользователь может нажать "Остановить" в любой момент; running-команда завершается штатно или по timeout.

Статусы workflow run:

- `running` — завод работает автоматически;
- `waiting_user` — нужны уточнения пользователя;
- `blocked` — автопродолжение небезопасно или невозможно;
- `done` — работа завершена и принята;
- `failed` — критическая техническая ошибка приложения/модели.

Критические причины остановки:

- safety validation изменения не прошла;
- попытка изменить файл вне проекта;
- конфликт записи: файл изменился между генерацией diff и применением;
- исчерпан лимит итераций;
- повторяется тот же diff или та же ошибка проверки;
- нет активной модели;
- модель недоступна;
- обязательный JSON не парсится после retry;
- тестовая команда зависла до timeout;
- Ревьюер вернул `blocked`.

UI V0.6:

- Основная кнопка в чате: "Запустить завод".
- Во время работы: кнопка "Остановить".
- Вместо ручных кнопок workflow показывает compact timeline:
  - роль;
  - текущий шаг;
  - статус;
  - количество итераций;
  - последние значимые события.
- Блоки "Изменения", "Проверки", "Ревью" остаются как раскрываемые детали, но не требуют кликов для продолжения.
- Финальный экран workflow показывает:
  - итоговый статус;
  - что было изменено;
  - какие проверки запускались;
  - результат ревью;
  - что пользователь может сделать дальше.
- Для опасных/неподдержанных операций UI показывает остановку с понятной причиной и кнопками:
  - "Продолжить вручную";
  - "Создать новую задачу с уточнением";
  - "Показать детали".

Данные и аудит:

- В `workflow_runs` добавить:
  - `mode = autopilot | manual`;
  - `iteration`;
  - `max_iterations`;
  - `status_reason`;
  - `stopped_by`.
- В `proposed_changes`, `test_runs`, `review_runs` сохранять `iteration`.
- Все автоматические действия логируются как события workflow:
  - `auto_apply_started`;
  - `auto_apply_done`;
  - `test_started`;
  - `test_done`;
  - `review_started`;
  - `review_done`;
  - `repair_iteration_started`;
  - `workflow_blocked`;
  - `workflow_done`.
- Артефакты `.zavod/runs/<workflow-id>/...` продолжают сохраняться автоматически.

Критерии готовности V0.6.0:

- Пользователь отправляет задачу и не нажимает дополнительные кнопки для apply/test/review.
- Простая задача с созданием одного файла проходит полный цикл до финального отчета.
- Python-only проект не получает `go test ./...`.
- Падающая проверка запускает не более разрешенного числа repair-итераций.
- При повторе той же ошибки workflow останавливается с понятным сообщением.
- Все изменения, проверки и ревью видны в UI и сохранены в SQLite.

#### V0.6.1 — Task Blueprint

Цель: завод должен сначала явно решить, что именно строится, на каком стеке, какие файлы должны быть созданы или изменены, нужен ли scaffold, и какими командами это проверяется. Это убирает проблему, когда Разработчик создает Python-файл, а Тестировщик предлагает Go-проверки, или наоборот.

Место в workflow:

- `task_blueprint` создается после требований Продакта и до архитектурного плана.
- Blueprint строится на основе:
  - сообщения пользователя;
  - всей истории уточнений;
  - требований Продакта;
  - структуры выбранного рабочего проекта;
  - уже существующих файлов в проекте;
  - ограничений безопасности приложения.
- Архитектор получает blueprint как обязательный вход.
- Разработчик получает blueprint как контракт на файлы и стек.
- Тестировщик получает blueprint как главный источник test commands.
- Ревьюер проверяет соответствие результата blueprint.

Ответ blueprint-шага:

```json
{
  "stack": "python",
  "runtime": "Python 3",
  "project_type": "single_script",
  "scaffold_required": false,
  "entrypoints": ["check.py"],
  "expected_files": [
    {
      "path": "check.py",
      "action": "create",
      "purpose": "CLI-скрипт проверки доступности сайта"
    }
  ],
  "forbidden_files": ["go.mod", "package.json"],
  "dependencies": {
    "policy": "standard_library_only",
    "items": []
  },
  "test_commands": [
    {
      "command": "python3 check.py",
      "working_dir": ".",
      "reason": "запускает созданный скрипт"
    }
  ],
  "open_questions": [],
  "confidence": "high"
}
```

Поддерживаемые `stack` на первом этапе:

- `python`;
- `go`;
- `node`;
- `mixed`;
- `unknown`.

Правила определения стека:

- Если пользователь явно просит Python-скрипт:
  - `stack = python`;
  - `project_type = single_script` или `python_project`;
  - `go.mod` не создается;
  - проверки начинаются с `python3 <entrypoint>.py`.
- Если пользователь явно просит Go-утилиту или Go-приложение:
  - `stack = go`;
  - `runtime = Go 1.25+`;
  - `scaffold_required = true`, если в рабочем проекте нет `go.mod`;
  - ожидаемые файлы включают `go.mod` и минимум один `.go` entrypoint;
  - проверки включают `go test ./...`.
- Если пользователь просит frontend/React/Node:
  - `stack = node`;
  - `package.json` обязателен, если нужны npm-команды;
  - проверки выбираются из `npm test`, `npm run build`, `npm run lint` только при наличии соответствующего scaffold.
- Если пользователь не указал стек:
  - blueprint использует существующую структуру проекта;
  - если структура пуста или неоднозначна, Люмен задает уточняющий вопрос;
  - завод не должен молча выбирать Go/Python/Node без основания.

Scaffold rules:

- Scaffold создается только если он нужен выбранному стеку и задаче.
- Python single-script задача не требует scaffold по умолчанию.
- Go-задача в пустом каталоге требует:
  - `go.mod` с версией `go 1.25` или выше;
  - `main.go` или другой entrypoint;
  - при необходимости `_test.go`.
- Node-задача в пустом каталоге требует:
  - `package.json`;
  - source entrypoint;
  - scripts для команд, которые Тестировщик будет запускать.
- Blueprint явно указывает `scaffold_required = true|false`.
- Разработчик не имеет права создавать scaffold другого стека, если blueprint запрещает его.

Backend validation:

- Blueprint сохраняется в SQLite как отдельный workflow artifact и как структурированное поле workflow run.
- Разработчик получает компактный snapshot релевантных корневых файлов проекта (`go.mod`, `*.go`, `*.py`, `package.json`) и должен использовать его как основу для `replace`.
- Перед применением changes backend сверяет proposed changes с blueprint:
  - файл из `forbidden_files` блокируется;
  - неожиданный scaffold блокируется;
  - создание файла вне `expected_files` требует объяснения от Разработчика;
  - если объяснение пустое или слабое, workflow переходит в `blocked`.
- Перед созданием `test_runs` backend сверяет команды с `test_commands` blueprint:
  - сначала используются команды из blueprint;
  - затем допускается fallback по структуре проекта;
  - команды другого стека отфильтровываются.
- Если blueprint говорит `stack = go`, но после применения нет `go.mod`, Ревьюер обязан вернуть `needs_work`.
- Если blueprint говорит `stack = python`, команда `go test ./...` считается `not_applicable`, а не ошибкой проекта.

Prompt changes:

- Люмен:
  - обязан учитывать ответы пользователя на уточняющие вопросы;
  - не задает повторно вопросы, ответы на которые уже есть в истории;
  - если стек неясен, задает один короткий вопрос до запуска разработки.
- Продакт:
  - явно фиксирует runtime, entrypoint, dependency policy и acceptance criteria.
- Архитектор:
  - не выбирает стек заново, а проектирует внутри blueprint;
  - если blueprint ошибочен, возвращает `blocked_reason`, а не продолжает.
- Разработчик:
  - обязан создать все `expected_files`, если они нужны;
  - обязан не создавать `forbidden_files`;
  - в summary пишет "предложено/применено" в зависимости от фактического статуса changes.
- Тестировщик:
  - берет команды из blueprint;
  - не предлагает команды другого стека;
  - если команда из blueprint стала невозможной, пишет причину.
- Ревьюер:
  - проверяет соответствие diff blueprint;
  - отдельно отмечает "stack/scaffold mismatch".

UI V0.6.1:

- В шапке workflow показывается компактный блок:
  - "Стек: Python/Go/Node";
  - "Тип: script/app/library/frontend";
  - "Entrypoint: ...";
  - "Scaffold: нужен/не нужен".
- При наведении или раскрытии показываются:
  - expected files;
  - forbidden files;
  - test commands;
  - open questions.
- Если blueprint не уверен (`confidence = low`) или есть `open_questions`, завод останавливается до ответа пользователя.
- В финальном отчете показывается:
  - какой blueprint был принят;
  - какие файлы по нему созданы;
  - какие проверки соответствовали blueprint.

Данные:

- Добавить таблицу или JSON-поле `task_blueprints`:
  - `id`;
  - `project_id`;
  - `task_id`;
  - `workflow_run_id`;
  - `iteration`;
  - `stack`;
  - `runtime`;
  - `project_type`;
  - `scaffold_required`;
  - `entrypoints_json`;
  - `expected_files_json`;
  - `forbidden_files_json`;
  - `dependencies_json`;
  - `test_commands_json`;
  - `open_questions_json`;
  - `confidence`;
  - `created_at`.
- Для `proposed_changes` добавить проверяемую связь с blueprint:
  - `blueprint_file_status = expected | explained_extra | forbidden`.
- Для `test_runs` добавить:
  - `source = blueprint | fallback | manual`;
  - `applicability = applicable | not_applicable`.

Критерии готовности V0.6.1:

- Для запроса "создай Python-скрипт check.py" blueprint запрещает `go.mod` и предлагает `python3 check.py`.
- Для запроса "создай Go CLI" blueprint требует `go.mod` и предлагает `go test ./...`.
- Разработчик создает scaffold только когда blueprint требует scaffold.
- Тестировщик не предлагает команды другого стека.
- Ревьюер ловит mismatch между blueprint и diff.
- UI явно показывает, что именно завод решил строить.

#### V0.6.2 — Native Clarification UI

Цель: пользователь отвечает на уточнения нативно в интерфейсе, без цитирования вопросов, markdown-синтаксиса и ручного структурирования ответа.

- Люмен по-прежнему возвращает structured JSON:
  - `summary`;
  - `goal`;
  - `constraints`;
  - `open_questions`;
  - `needs_clarification`.
- Если `needs_clarification = true`, workflow переходит в `waiting_user`.
- В чат добавляется только короткое сообщение:
  - задача требует уточнения;
  - ответить можно в форме ниже.
- Вопросы не дублируются длинным текстом в сообщении чата.
- Backend восстанавливает pending clarification из последнего `manager_intake` workflow step.
- `ProjectState` возвращает `clarification`:
  - `workflowRunId`;
  - `summary`;
  - `goal`;
  - `questions[]`.
- UI показывает форму уточнений над composer:
  - один textarea на каждый вопрос;
  - кнопку "Ответить и продолжить";
  - обычный composer блокируется, пока есть active clarification.
- Пользователь пишет обычный текст в поля формы.
- Frontend вызывает `SubmitClarification`.
- Backend сохраняет ответы как обычное user-сообщение в формате Q/A:
  - вопрос;
  - ответ.
- Новый workflow получает эти Q/A из истории и продолжает задачу.
- Prompt Люмен требует:
  - учитывать ответы на предыдущие вопросы;
  - не задавать повторно вопросы, на которые уже ответили;
  - возвращать вопросы только в `open_questions`;
  - не просить пользователя отвечать специальным синтаксисом.

Ограничения V0.6.2:

- Ответы пока текстовые, без radio/select controls.
- Вопросы восстанавливаются из workflow step, отдельная таблица clarification не добавляется.
- Нет частичного submit по одному вопросу.
- Нет nested follow-up questions внутри одной формы.

Критерии готовности V0.6.2:

- При `needs_clarification=true` пользователь видит форму с вопросами.
- Можно ответить без цитирования и markdown.
- После отправки формы завод запускает новый workflow.
- Люмен видит ответы в истории и не повторяет те же вопросы.
- Обычный composer заблокирован до отправки уточнений.

#### V0.6.3 — Model Health Monitor

Цель: приложение само отслеживает доступность активной LLM во время работы, особенно для локальной/сетевой Qwen, и показывает пользователю, если модель отвалилась.

- При старте приложения backend запускает background monitor.
- Monitor проверяет только активную модель.
- Проверка легкая:
  - `GET /v1/models` для OpenAI-compatible endpoint;
  - без chat/completion запроса;
  - `200 OK` считается признаком доступности endpoint;
  - health monitor не требует, чтобы `/v1/models` перечислял ровно выбранный `model_name`, потому что локальные OpenAI-compatible серверы часто возвращают alias, пустой список или другой id;
  - timeout 5 секунд.
- Интервалы:
  - первый check примерно через 1 секунду после запуска;
  - базовый interval: 10 секунд;
  - если модель offline: 5 секунд;
  - если модель online стабильно несколько проверок подряд: 30 секунд.
- Результат сохраняется в `model_configs`:
  - `status = online | offline`;
  - `last_checked_at`;
  - `last_error`;
  - `latency_ms`.
- UI получает обновления через существующее событие `models_changed`.
- В карточке активной модели показывается:
  - статус;
  - latency;
  - последняя ошибка, если модель недоступна.
- Если модель была offline и восстановилась, статус агентов обновляется на "модель снова доступна".
- Если модель стала offline, пользователь видит это в карточке модели без ручного нажатия "Проверить".

Ограничения V0.6.3:

- Monitor не пишет отдельное сообщение в чат на каждый сбой, чтобы не шуметь.
- Monitor не останавливает уже выполняющийся model request; текущий workflow получит обычную ошибку вызова модели.
- Нет отдельного статуса workflow `waiting_model`; это кандидат для следующего шага.
- Проверяется только активная модель, не весь список моделей.

Критерии готовности V0.6.3:

- После запуска приложения статус активной модели обновляется автоматически.
- Если endpoint локальной модели выключить, UI показывает `недоступна` и последнюю ошибку.
- Если endpoint снова включить, UI возвращает `доступна`.
- Ручная кнопка "Проверить" продолжает работать.

### V0.6.4 — Mandatory Reviewer Gate

Цель:

- Ревьюер становится обязательным шагом Autopilot workflow перед итогом Люмен.
- Финальный ответ пользователю разрешен только после статуса ревью `accepted` или честной остановки workflow.
- Если Ревьюер нашел проблему, workflow не должен писать "готово": задача возвращается на доработку или блокируется.

Контракт Ревьюера:

- Ревьюер получает task brief, требования, Task Blueprint, архитектурный план, developer plan, applied changes, diff и результаты тестов.
- Ревьюер возвращает только JSON:
  - `status`: `accepted | needs_work | blocked`;
  - `summary`: краткий итог;
  - `return_to`: `product | architect | developer | tester | user`;
  - `blocking_reason`: причина остановки, если `blocked`;
  - `findings`: список замечаний;
  - `required_changes`: обязательные исправления;
  - `recommended_next_step`: следующий шаг.

Правила маршрутизации:

- `accepted`: workflow переходит к итоговому ответу Люмен.
- `needs_work + developer`: Autopilot запускает repair-итерацию Разработчика, применяет изменения, повторяет проверки и ревью.
- `needs_work + product`: Autopilot пересобирает требования, затем Task Blueprint, архитектуру, developer changes, применяет изменения, повторяет проверки и ревью.
- `needs_work + architect`: Autopilot пересобирает Task Blueprint, архитектуру, developer changes, применяет изменения, повторяет проверки и ревью.
- `needs_work + tester`: Autopilot повторяет проверочный контур и снова запускает ревью.
- `needs_work + user` или `blocked`: workflow останавливается и показывает причину.
- Если Ревьюер ошибочно вернул `blocked`, но указал `return_to=developer/product/architect/tester`, backend трактует это как исправимый `needs_work`, а не как вмешательство пользователя.
- Ошибки компиляции, синтаксические ошибки, упавшие тесты и недостающие проектные файлы считаются исправимыми проблемами для ролей завода.

Защита от бесконечного цикла:

- Максимум 2 repair-итерации после первичной разработки.
- Если Разработчик повторяет тот же ответ или не возвращает применимые structured changes, workflow блокируется.
- Если Ревьюер снова не принимает работу после лимита, workflow блокируется.
- Если workflow блокируется из-за лимита repair-итераций, итог Люмен обязан показать последнее summary ревьюера, required changes и ключевые findings. Нельзя оставлять только `needs_work` без объяснения.
- Repair-итерация Разработчика получает не только замечания Ревьюера, но и текущий snapshot файлов проекта после примененных изменений.

UI:

- В прогрессе workflow Ревью отображается до Итога.
- В панели ревью показываются статус, маршрут возврата и причина остановки.
- Кнопка ручного ревью остается как debug/manual fallback, но основной сценарий идет автоматически.
- Верхняя зона чата является рабочей панелью, а не заголовком: подпись "Чат с агентом" и дублирующее имя проекта в центре не показываются.
- Левая панель проектов не дублирует бренд приложения; заголовок панели — только "Проекты".
- В рабочей панели показываются активный агент, карточка текущего шага workflow и карточка активной модели пайпа.
- Карточка активного агента имеет hover/focus popover с описанием роли: зачем агент нужен, за что отвечает и что делает сейчас.
- Карточка workflow видна всегда; до запуска задачи она показывает `0/N`, статус "не запущен" и будущие шаги в hover/focus подсказке.
- Карточка workflow сопоставима по размеру с карточкой агента и показывает текущий шаг, статус и ответственного агента. Подробный preview шага показывается в hover/focus подсказке, а не в компактной карточке.
- Карточки активного агента, workflow и модели имеют одинаковую высоту и общий визуальный стиль; длинные тексты workflow не меняют высоту шапки и не выходят за границы карточки.
- Вторичный текст вроде "Ждет задачу" и подписи модели использует единый muted-тон, а цветовые акценты остаются только у статусов и левого бордера.
- Полная лента шагов не занимает отдельную полосу; список всех этапов и позиция текущего шага показываются в hover/focus подсказке карточки workflow.
- В подсказке workflow не показывается сырой JSON внутренних structured-ответов; UI выводит человекочитаемый summary/status/stack.
- Служебные workflow-артефакты `.zavod/runs/...` не показываются в основном результате и не дописываются в финальный ответ Люмен.
- Пользовательский результат по коду показывается как компактная плашка прямо над composer: количество измененных файлов, суммарные `+/-` строки и раскрываемый список diff по файлам.
- Если один файл менялся в нескольких repair-итерациях, плашка изменений показывает его один раз по последнему состоянию. История итераций сохраняется в БД, но не дублируется в основном UX.
- В чат выводится только значимая информация: запрос пользователя, уточнения, итог Люмен и критические остановки. Рутинные события Autopilot, тестов, ревью и применения изменений остаются в структурных блоках UI, но не засоряют диалог.
- Информация об активной модели показывается один раз: в верхней рабочей панели рядом с активным агентом и workflow. Дублирующая LLM-секция в правой панели не используется.
- Карточка активной модели имеет hover/focus popover с деталями: provider, model, base URL, статус, задержка, время последней проверки и ошибка доступности.
- Правая панель показывает только контекст задачи, в первую очередь "Контракт задачи". В UI не дублируются заголовки "Blueprint" и "Контракт задачи"; confidence показывается понятной русской подписью, например "уверенность высокая".
- В Blueprint-карточке показываются стек, runtime, тип, scaffold, entrypoint и ожидаемые файлы. Команды проверок не выводятся в правой панели постоянно; они доступны через workflow/test state и итоговые сообщения.

Проверки:

- Тестировщик предлагает команды только для стеков, затронутых текущими примененными изменениями.
- Python-проверки не запускаются, если в текущем workflow не менялись Python-файлы.
- npm-проверки не запускаются, если в текущем workflow не менялись frontend/package файлы.
- Go-проверки запускаются для изменений `.go`, `go.mod`, `go.sum`.
- Если Тестировщик или Task Blueprint не вернули команд, backend подставляет релевантные дефолтные проверки по типам измененных файлов.
- Финальный Autopilot summary показывает результаты последней проверочной итерации, а не сумму всех исторических попыток.
- Ревьюер принимает решение по последнему результату каждой команды проверки; старые failed-попытки не должны блокировать итог, если последняя попытка прошла.

### V0.6.5 — Intent Router / Direct Answers

Цель: Люмен должна понимать, что именно хочет пользователь, до запуска Autopilot. Вопросы, справка, анализ текущего состояния, просьбы показать спеку или объяснить логику проекта не должны прогонять весь пайплайн заново.

Поток:

- `SendMessage` сохраняет сообщение пользователя.
- Backend получает активный проект, активную задачу и последний workflow.
- `internal/router` классифицирует intent локальными правилами.
- Если локальная уверенность низкая, Люмен делает короткую LLM-классификацию в JSON.
- Если `needs_workflow=false`, Люмен отвечает напрямую, без создания `workflow_run`.
- Если `needs_workflow=true`, запускается обычный Autopilot.

Intent types:

- `direct_answer`: вопрос по уже существующим данным, например "опиши спеку по которой работала", "почему тесты упали", "что дальше".
- `project_analysis`: нужно читать/объяснять проект, но не менять файлы.
- `coding_task`: создать, исправить, изменить, перенести, реализовать, собрать или применить изменения.
  Просьбы вида "напиши на Go/Python/JS", "сделай скрипт", "создай программу", "реализуй код" всегда относятся к `coding_task`, даже если модель могла бы ответить примером прямо в чат.
- `clarification_answer`: ответ пользователя на active clarification.
- `workflow_control`: показать diff, повторить проверку, запустить ревью, продолжить/остановить workflow.
- `pentest_task`: security/pentest/threat-model запросы с обязательным учетом scope и разрешения.
- `general_chat`: общий вопрос не про проект.

Direct Answer:

- Люмен отвечает как единственный активный агент.
- Новый workflow не создается.
- Не запускаются Продакт, Blueprint, Архитектор, Разработчик, Тестировщик и Ревьюер.
- Люмен получает компактный контекст:
  - последние сообщения;
  - последний workflow и шаги;
  - Task Blueprint;
  - proposed changes;
  - последние результаты проверок;
  - последние ревью;
  - релевантные сохраненные артефакты, включая `docs/task-spec.md`;
  - snapshot файлов проекта, если intent требует проектный контекст.
- Ответ не должен выводить сырой JSON и служебные `.zavod/runs` пути без явной просьбы.

Правила высокого приоритета:

- "опиши спеку по которой работала" => `direct_answer`.
- "почему тесты упали?" => `direct_answer`.
- "исправь чтобы тесты проходили" => `coding_task`.
- "Ответы на уточнения: ..." => `clarification_answer`.
- "покажи diff" => `workflow_control`.
- "проверь проект на уязвимости" => `pentest_task`.

Критерии готовности V0.6.5:

- Вопрос про спеку не создает новый workflow.
- Запрос "выведи/покажи спеку по которой работала" возвращает сохраненный `docs/task-spec.md` как источник истины, а не LLM-пересказ.
- Общий вопрос не создает новый workflow.
- Coding-запрос продолжает запускать Autopilot.
- Ответы native clarification не перехватываются как справка.
- Люмен отвечает по последним сохраненным артефактам и состоянию workflow.
- В чате остаётся только значимая информация.

### V0.6.6 — Web Research

Цель: научить Люмен пользоваться интернетом для задач, где нужны актуальные данные, источники, документация, релизы, CVE/advisory или проверка внешних фактов. Web research не должен запускать полный dev-пайплайн, если пользователь просит только найти/объяснить информацию.

Маршрутизация:

- Добавляется intent `research_task`.
- Локальный router выбирает `research_task` для явных запросов: "найди в интернете", "поищи", "актуальные данные", "с источниками", "проверь в интернете".
- Если запрос одновременно просит изменить код, приоритет остается у `coding_task`; web research позже может стать подготовительным шагом coding workflow.
- LLM-классификатор знает про `research_task` и возвращает `needs_workflow=true`.

Поток:

- `SendMessage` создает короткий `workflow_run`.
- Шаг `web_research` выполняет Люмен.
- Люмен формирует JSON research plan: summary + 1-3 query.
- Backend выполняет поиск и загрузку публичных страниц.
- Найденные источники сохраняются в SQLite в `web_sources`.
- Люмен формирует итоговый Markdown-ответ по найденным источникам.
- Workflow завершается без Продакта, Blueprint, Архитектора, Разработчика, Тестировщика и Ревьюера.

Backend:

- Новый пакет `internal/webresearch`.
- Базовый поисковый провайдер без ключей: DuckDuckGo Instant Answer API.
- Поддержка прямых `http/https` URL из запроса пользователя.
- Для страниц извлекаются title и короткий текстовый фрагмент.
- Для защиты от SSRF web research не ходит в `localhost`, `.local`, loopback/private/link-local IP.
- Настройки хранятся в `app_settings`:
  - enabled;
  - maxResults;
  - maxPagesPerWorkflow;
  - timeoutSeconds;
  - allowedDomains;
  - blockedDomains.

UI:

- В настройках появляется вкладка "Интернет".
- В чате показывается только значимый ответ Люмен.
- Источники отображаются отдельным компактным блоком в правой панели, только если они есть.
- Верхняя плашка workflow показывает один шаг: "Поиск в сети".

Критерии готовности V0.6.6:

- Запрос "найди в интернете ..." не запускает полный coding/autopilot workflow.
- Источники сохраняются в БД и переживают перезапуск приложения.
- Ответ Люмен не содержит сырой JSON.
- При отключенном web research пользователь получает понятное сообщение, что поиск выключен.
- Сетевые лимиты не дают workflow уйти в бесконечный обход страниц.

### V0.6.7 — Dynamic Step Dock

Цель: рядом с плашкой code diff показывать компактный пользовательский план выполнения: `Шаг X / N`. При наведении раскрывается popover со всеми шагами, статусами, агентами и короткими описаниями. Количество шагов динамическое: Люмен формирует план при обработке задачи.

Проблема:

- Технические `workflow_steps` полезны backend-у, но не всегда понятны человеку.
- Пользователь хочет видеть ход работы прямо над диалогом, рядом с измененными файлами.
- В чат не нужно выводить служебный список шагов.

Решение:

- Добавить отдельный пользовательский слой `workflow_plans` / `workflow_plan_steps`.
- Не заменять старые `workflow_steps`: они остаются техническим логом.
- Новый план используется только для UX и восстановления состояния после перезапуска.

Backend:

- Добавить `StepUserPlan = "user_plan"`.
- Люмен формирует JSON-план до основного workflow.
- Если LLM не вернула валидный JSON, backend создает fallback-план по intent:
  - `research_task`: источники + ответ;
  - `pentest_task`: scope/security-анализ + итог;
  - `coding_task`: постановка, требования, blueprint, архитектура, разработка, проверка, ревью, итог.
- Максимум 8 шагов.
- Каждый шаг содержит:
  - `step_key`;
  - `title`;
  - `description`;
  - `agent_id`;
  - `status`;
  - `sort_order`;
  - timestamps и error.
- При запуске технического шага backend обновляет ближайший пользовательский шаг.
- При завершении workflow весь план переводится в `done`, `failed` или `blocked`.

SQLite:

- `workflow_plans`:
  - `id`;
  - `project_id`;
  - `task_id`;
  - `workflow_run_id`;
  - `title`;
  - `status`;
  - `current_step_id`;
  - `created_at`;
  - `updated_at`.
- `workflow_plan_steps`:
  - `id`;
  - `plan_id`;
  - `step_key`;
  - `title`;
  - `description`;
  - `agent_id`;
  - `status`;
  - `started_at`;
  - `finished_at`;
  - `error`;
  - `sort_order`.

Frontend:

- Добавить `StepDock` над composer в одной строке с `ChangeSummaryDock`.
- Компактная плашка:
  - `Шаг X / N`;
  - status-dot по текущему состоянию.
- Popover:
  - title плана;
  - список всех шагов;
  - иконка статуса;
  - название шага;
  - агент;
  - описание;
  - error, если шаг упал.
- Список шагов не выводится отдельным сообщением в чат.

Критерии готовности V0.6.7:

- После отправки задачи план появляется сразу после создания workflow.
- Плашка стоит прямо над окном ввода рядом с diff dock.
- При наведении открывается popover со всеми шагами.
- Количество шагов может отличаться по типу задачи.
- После перезапуска приложение восстанавливает план из SQLite.
- При failed/blocked состоянии popover показывает проблемный шаг.
- Если динамический план не удалось получить от модели, используется fallback без остановки workflow.

### V0.7.0 — ИБ-специалист / Security Workflow

Цель: добавить отдельного агента для задач по информационной безопасности, pentest, threat modeling, security review и анализу уязвимостей. ИБ-задачи не должны случайно запускать обычный dev/autopilot-flow и не должны автоматически выполнять активные сетевые проверки.

Роль:

- id: `security`;
- name: `ИБ-специалист`;
- workflow step: `security_analysis`;
- отвечает в чате отдельным сообщением от ИБ-специалиста;
- отображается в верхней панели как активный агент, когда выполняет security-задачу.

Маршрутизация:

- `пентест`, `pentest`, `penetration test`, `уязвимости`, `security audit`, `аудит безопасности`, `threat model`, `sql injection`, `xss`, `cve` => `pentest_task`.
- `pentest_task` запускает отдельный security workflow.
- Security workflow не запускает Продакта, Blueprint, Архитектора, Разработчика, Тестировщика и Ревьюера.
- Если ИБ-специалист выявил необходимость изменить код, он формулирует security requirements и следующий шаг. Сам код в V0.7.0 не меняет.

Security workflow:

1. Люмен принимает сообщение пользователя и роутит intent.
2. Backend создает `workflow_run`.
3. Запускается шаг `security_analysis`.
4. ИБ-специалист получает:
   - запрос пользователя;
   - название и путь проекта;
   - project signals;
   - последние сообщения;
   - безопасный snapshot корневых source-файлов.
5. ИБ-специалист возвращает Markdown-ответ:
   - ИБ-анализ;
   - scope и допущения;
   - риски;
   - рекомендации;
   - проверки;
   - следующий шаг.
6. Ответ сохраняется в чат.
7. Workflow завершается без автозаписи файлов.

Safety rules:

- Для внешних целей, активного пентеста, эксплуатации, brute force, обхода доступа, stealth, persistence, credential harvesting и вредоносной автоматизации сначала нужен явный scope и подтверждение разрешения.
- Агент не утверждает, что запускал сканеры или эксплуатировал цель, если backend не предоставил результаты таких проверок.
- Разрешены defensive threat model, checklist, безопасные проверки конфигурации, рекомендации по hardening, unit/integration проверки и remediation plan.
- Запрещены пошаговые инструкции по атаке сторонней цели без подтвержденного scope.

Критерии готовности V0.7.0:

- В списке агентов есть ИБ-специалист.
- Security/pentest-запросы роутятся в `pentest_task`.
- `pentest_task` запускает `security_analysis`, а не обычный coding Autopilot.
- В UI активным агентом показывается ИБ-специалист.
- Ответ ИБ-специалиста приходит в Markdown и не содержит сырого JSON.
- По умолчанию security workflow не пишет файлы и не запускает сетевые команды.

### Spec-driven development

- `docs/task-spec.md` является пользовательским контрактом задачи и source of truth для ответа "по какой спеке работал завод".
- `docs/task-spec.md` пишется в человеко-читаемом формате: запрос пользователя, цель, требования, технический контракт, критерии готовности, ограничения и ссылки на поддерживающие материалы.
- `docs/task-spec.md` не должен быть transcript-склейкой ответов ролей и не должен показывать сырой JSON из Task Brief или Task Blueprint.
- Служебные подробности сохраняются отдельно в `.zavod/runs/<workflow_id>/`: task brief, требования продакта, task blueprint, архитектурный план, developer plan и итог.
- Task Blueprint уточняет технический контракт: stack, runtime, project type, scaffold, expected files, forbidden files, dependency policy и проверки.
- Архитектор и Разработчик должны опираться на task spec + blueprint, а не только на свободный пересказ предыдущего агента.
- Ревьюер проверяет результат против task spec + blueprint + diff + последних проверок.
- Если пользователь просит вывести уже использованную спеку, приложение не запускает Autopilot и не просит модель пересказывать ее по памяти; оно достает сохраненный spec artifact.

### Default skills

- Завод добавляет `$pony-tail` в системные инструкции всех рабочих ролей по умолчанию.
- `$pony-tail` трактуется как базовый рабочий skill/стиль: ясные практичные ответы, spec-driven reasoning, аккуратные изменения и короткий вывод без служебного шума.
- Если подключенная модель или remote runtime не умеет вызывать skills напрямую, инструкция остается обычным system prompt hint и не должна ломать workflow.
- Явный выбор другого skill пользователем имеет приоритет над `$pony-tail`.

### Python project policy

- Для Python-задач проект всегда считается virtualenv-based: runtime `Python 3 + venv`.
- Task Blueprint для Python всегда включает `requirements.txt` в `expected_files`.
- Если внешних зависимостей нет, `requirements.txt` создается с комментарием `# standard library only`.
- Если нужны внешние зависимости, например `python-telegram-bot`, они фиксируются в `dependencies.items` и записываются в `requirements.txt`.
- Разработчик не должен писать пользователю `pip install ...` как основной способ установки зависимости; source of truth — `requirements.txt`.
- Тестировщик запускает Python-проверки только через `.venv/bin/python <script.py>`.
- Backend перед Python-проверкой автоматически создает `.venv` внутри проекта и выполняет `.venv/bin/python -m pip install -r requirements.txt`, если `requirements.txt` есть.
- Системный `python`/`python3` не используется для запуска Python-проекта напрямую; он нужен только для первичного создания `.venv`.

### V0.8.3 Dev Workspace

- Для Go-задач blueprint нормализуется до runtime `Go 1.25+`, ожидает `go.mod` и проверки `go test ./...` + `go vet ./...`.
- Для Python-задач blueprint нормализуется до runtime `Python 3 + .venv`, ожидает `requirements.txt`, а проверки запускаются только через `.venv/bin/python`.
- Для Go/Python задач workspace дополняется файлами `.gitignore`, `Makefile`, `README.md`, `.github/workflows/ci.yml`, если их еще нет.
- Существующий `go.mod` обновляется только если его директива ниже `go 1.25`; более новая версия не понижается.
- Существующий `requirements.txt` не затирается: зависимости из blueprint сливаются с уже сохраненными строками.
- Makefile предназначен для локальной разработки, а Autopilot продолжает запускать безопасные прямые проверки из blueprint.
- GitHub Actions workflow генерируется как базовый CI для Go/Python проекта.
- Примененные изменения можно откатить через rollback: файл возвращается к `before_content`, а созданный файл удаляется, если он не был изменен после применения.
- Rollback останавливается для конкретного файла, если текущий контент отличается от сохраненного `after_content`.
- Плашка diff показывает агрегированный список файлов, статусы `ожидает`/`применено`/`ошибка`/`откатано`, суммарные `+/-` и кнопку отката для примененных изменений.

### V0.8.5 Smart Repair Loop

- Autopilot сам возвращает задачу нужному агенту, если кодовые изменения не применились, проверки упали или ревью вернуло `needs_work`.
- Упавшие проверки до обязательного ревью трактуются как synthetic review:
  - `failed` проверки возвращают задачу Разработчику;
  - `blocked`/неприменимые проверки возвращают задачу Тестировщику;
  - отсутствие scope/секрета/API key в ошибке проверки останавливает workflow как `blocked + return_to=user`.
- Ревью `blocked + return_to=user` считается настоящим пользовательским блокером только если причина про scope, секреты, конфликт требований или внешнюю инфраструктуру.
- Если ревьюер ошибочно блокирует исправимую ошибку кода, тестов, blueprint или scaffold, backend нормализует результат в `needs_work` и продолжает repair-loop.
- Возврат по ролям:
  - `developer` - исправить файлы/код/синтаксис/падающие тесты;
  - `tester` - подобрать корректные auto-проверки из Code Execution Policy;
  - `architect` - пересобрать blueprint и архитектурный план;
  - `product` - уточнить требования без участия пользователя, если нет противоречия;
  - `user` - только настоящий блокер.
- Пользователь видит “нужно вмешательство” только когда backend не может безопасно продолжать без новых данных пользователя.

#### V0.5 UX rules

- В ручном режиме кодовые изменения не записываются без controlled apply.
- В Autopilot режиме безопасные structured changes применяются автоматически, затем автоматически запускаются проверки и обязательное ревью.
- Общая плашка изменений показывает агрегированный итог по файлам за весь workflow: один файл отображается один раз, diff считается от исходного состояния к финальному результату, repair-итерации не дублируют файл в списке.
- Если файл был создан, а затем исправлялся ревью-итерациями, общий diff показывает создание файла с финальным содержимым, а не только последнюю маленькую правку.
- Если есть `pending` изменения:
  - в шапке чата показывается заметная кнопка "Применить N";
  - в блоке "Изменения" кнопка "Применить изменения" остается видимой при прокрутке;
  - UI явно пишет, что файлы еще не записаны в проект.
- Агентам запрещено писать "файл создан" до controlled apply. Корректная формулировка: "предложено создать/изменить".
- Артефакты спеки и кодовые изменения должны визуально различаться:
  - артефакты сохраняются автоматически;
  - кодовые файлы пишутся только после подтверждения пользователя.
- Люмен обязана учитывать ответы пользователя на уточняющие вопросы из истории и не задавать те же вопросы повторно.

#### V0.5.x

- Роли:
  - Тестировщик;
  - Ревьюер.
- Controlled edits.
- Test runner.
- Diff viewer.
- Review loop.

## Критерии готовности V0.1

- Приложение запускается на macOS.
- Открывается desktop window через Wails.
- Интерфейс на русском языке.
- Можно добавить проект.
- Можно выбрать проект.
- Можно написать задачу в чат.
- Люмен отвечает через выбранную модель.
- Виден статус Люмен.
- История сохраняется в SQLite.
- После перезапуска приложения видны прошлые проекты и сообщения.
- Удаленная Qwen подключается через `base_url`.

## Неблокирующие вопросы

- У удаленной Qwen API точно OpenAI-compatible или нужен отдельный adapter?
- Нужен ли в V0.1 импорт существующего git-проекта, или достаточно обычной папки?
