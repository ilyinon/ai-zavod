// Browser smoke tests use an in-memory Wails bridge; Go tests exercise persistence.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(() => {
      const projects = [{ id: 'p', name: 'Telegram-бот', path: '/tmp/bot', createdAt: '', lastOpenedAt: '' }];
      const chats = [{ id: 'old', projectId: 'p', title: 'Архитектура', status: 'active', pinned: false, createdAt: '', updatedAt: '', groupId: '', modelId: '' }];
      const messages = { old: [{ id: 'm0', taskId: 'old', role: 'agent', agentId: 'manager', content: 'Существующая архитектура проекта', createdAt: '' }] };
      const model = { id: 'm', name: 'Qwen по сети', baseUrl: 'http://127.0.0.1:1234/v1', modelName: 'qwen3:coder', provider: 'openai-compatible', isActive: true, status: 'online', latencyMs: 12 };
      const toolInvocations = [{ id: 'tool1', loopId: 'loop', callId: 'call', projectId: 'p', taskId: 'old', agentId: 'agent_dev_developer', agentName: 'Разработчик', workingDir: '/tmp/bot', toolProfileId: 'tool_go_dev', tool: 'run_check', arguments: '{"command":"go test ./..."}', result: { status: 'failed', output: '--- FAIL: TestBroken (0.00s)\n    broken_test.go:3: fixture diagnostic failure\nFAIL', exitCode: 1, truncated: false }, startedAt: '2026-09-06T12:00:00Z', finishedAt: '2026-09-06T12:00:02Z' }];
      const listeners = {};
      const emit = (event, data) => (listeners[event] || []).forEach(fn => fn(structuredClone(data)));
      const state = id => ({ project: projects.find(p => p.id === chats.find(t => t.id === id)?.projectId) || {}, task: chats.find(t => t.id === id), messages: messages[id] || [], toolInvocations: toolInvocations.filter(item => item.taskId === id), workflowSteps: [], planSteps: [], artifacts: [], changes: [], testRuns: [], reviews: [], webSources: [], agents: [] });
      window.__emit = emit;
      window.__state = state;
      window.runtime = { EventsOn: (event, fn) => { (listeners[event] ||= []).push(fn); return () => { listeners[event] = listeners[event].filter(f => f !== fn); }; } };
      window.go = { main: { App: {
        Bootstrap: async () => ({ projects, chats, chat: state(''), selectedProjectId: '', agents: [], models: [model], activeModelId: 'm', agentGroups: [], agentGroupTemplates: [], agentLibrary: [] }),
        ListChats: async () => chats,
        CreateChat: async ({ projectId }) => { const id = 'chat' + chats.length; chats.push({ id, projectId, title: 'Новый чат', status: 'active', pinned: false, createdAt: '', updatedAt: '', groupId: '', modelId: '' }); return state(id); },
        SelectChat: async id => state(id),
        UpdateChat: async data => { const task = chats.find(t => t.id === data.taskId); Object.assign(task, data, { status: data.archived ? 'archived' : 'active' }); return task; },
        DeleteChat: async id => { chats.splice(chats.findIndex(t => t.id === id), 1); delete messages[id]; },
        CreateProject: async ({ name }) => { const p = { id: 'p' + projects.length, name, path: '/tmp/' + name }; projects.push(p); return p; },
        AddExistingProject: async ({ name, path }) => { const p = { id: 'p' + projects.length, name, path }; projects.push(p); return p; },
        ChooseProjectFolder: async () => '/tmp/existing',
        SendMessage: async ({ taskId, content, toolConsentModelId }) => {
          window.__lastToolConsent = toolConsentModelId;
          const task = chats.find(t => t.id === taskId);
          task.title = content.slice(0, 45);
          messages[taskId] ||= [];
          messages[taskId].push({ id: 'u' + Date.now(), taskId, role: 'user', content, createdAt: '' });
          if (content.includes('Напиши') && !task.projectId) task.pendingRequest = content;
          else messages[taskId].push({ id: 'a' + Date.now(), taskId, role: 'agent', agentId: 'manager', content: 'Ответ Люмен', createdAt: '' });
          emit('chat_state_changed', state(taskId)); return state(taskId);
        },
      } } };
    });
    await page.goto(process.env.CHATS_TEST_URL || 'http://127.0.0.1:5179');
    await page.getByRole('heading', { name: 'С чего начнём?' }).waitFor();
    await page.getByRole('button', { name: 'Новый чат', exact: true }).first().click();
    const composer = page.getByPlaceholder('Задача, вопрос или идея...');
    await composer.fill('Привет');
    await page.getByRole('button', { name: 'Отправить', exact: true }).click();
    await page.getByText('Ответ Люмен', { exact: true }).waitFor();
    await composer.fill('Сохранённый черновик');
    await page.getByRole('button', { name: 'Архитектура', exact: true }).click();
    await page.getByText('Существующая архитектура проекта', { exact: true }).waitFor();
    const codeSample = 'def bubble_sort(arr):\n    n = len(arr)\n    for i in range(n):\n        for j in range(n - i - 1):\n            if arr[j] > arr[j + 1]:\n                arr[j], arr[j + 1] = arr[j + 1], arr[j]\n    return arr\n\n# ' + 'long comment '.repeat(30);
    await page.evaluate(code => {
      const state = window.__state('old');
      state.messages.push({ id: 'formatted-code', taskId: 'old', role: 'agent', agentId: 'manager', content: 'Пример:\n```python\n' + code + '\n```\nВызови `bubble_sort(numbers)`.', createdAt: '' });
      for (const [index, fence] of ['```', '```python', '```\n```', '```python\n  \n\t\n```'].entries()) {
        state.messages.push({ id: 'empty-code-' + index, taskId: 'old', role: 'agent', agentId: 'manager', content: 'Ответ без кода ' + index + '.\n' + fence, createdAt: '' });
      }
      window.__emit('chat_state_changed', state);
    }, codeSample);
    const codeBlock = page.locator('.markdown-code-block code');
    await codeBlock.waitFor();
    assert.equal(await page.locator('.markdown-code-block').count(), 1, 'empty fences rendered dark strips');
    assert.equal(await page.getByText(/^Ответ без кода \d\.$/).count(), 4, 'text before empty fences disappeared');
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 1000 });
      assert.equal(await codeBlock.textContent(), codeSample, 'code newlines or indentation lost in DOM');
      assert.equal(await codeBlock.evaluate(el => getComputedStyle(el).whiteSpace), 'pre', 'code must preserve whitespace');
      const layout = await codeBlock.evaluate(el => {
        const text = el.firstChild;
        const at = offset => { const range = document.createRange(); range.setStart(text, offset); range.setEnd(text, offset + 1); return range.getBoundingClientRect(); };
        const first = at(0), second = at(text.textContent.indexOf('    n') + 4);
        return { lineBreak: second.y > first.y, indented: second.x > first.x, scrolls: el.parentElement.scrollWidth > el.parentElement.clientWidth };
      });
      assert.ok(layout.lineBreak && layout.indented, 'Python lines/indentation visually collapsed');
      assert.ok(layout.scrolls, 'long code should scroll within its block');
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'code caused page overflow');
      await page.screenshot({ path: '/tmp/zavod-code-' + width + '.png' });
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.evaluate(() => {
      const state = window.__state('old');
      state.agents = [{ id: 'manager', name: 'Люмен', status: 'idle', activity: 'Жду задачу', modelId: 'm', elapsedMs: 0, totalTokens: 0 }];
      state.requestState = { id: 'req1', sequence: 1, mode: 'workflow', workflowRunId: 'run' };
      state.projectionRevision = '10';
      state.workflowRun = { id: 'run', taskId: 'old', status: 'running', currentStep: 'developer_plan' };
      state.workflowPlan = { id: 'plan', taskId: 'old', workflowRunId: 'run', currentStepId: 's3', status: 'running', title: 'Пять настоящих шагов' };
      state.planSteps = Array.from({ length: 5 }, (_, i) => ({ id: 's' + (i + 1), planId: 'plan', stepKey: 'step' + i, title: 'Шаг плана ' + (i + 1), agentId: 'manager', status: i < 2 ? 'done' : i === 2 ? 'running' : 'queued', sortOrder: i, description: 'Проверенное описание' }));
      window.__workflowFixture = state;
      window.__emit('chat_state_changed', state);
    });
    await page.getByRole('button', { name: 'Шаг 3 / 5', exact: true }).waitFor();
    assert.equal(await page.locator('.workflow-compact-index').innerText(), '3/5');
    await page.getByRole('button', { name: 'Шаг 3 / 5', exact: true }).hover();
    await page.waitForFunction(() => getComputedStyle(document.querySelector('.step-dock-popover')).opacity === '1');
    await page.screenshot({ path: '/tmp/zavod-reliability-desktop.png' });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole('button', { name: 'Шаг 3 / 5', exact: true }).click();
    const stepPanel = await page.locator('.step-dock-popover').boundingBox();
    assert.ok(stepPanel.width > 300 && stepPanel.x >= 0 && stepPanel.x + stepPanel.width <= 390 && stepPanel.y >= 0, 'step popover outside mobile viewport');
    await page.screenshot({ path: '/tmp/zavod-reliability-mobile.png' });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.evaluate(() => {
      const done = structuredClone(window.__workflowFixture);
      done.projectionRevision = '11';
      done.workflowPlan.status = 'done';
      done.workflowPlan.currentStepId = 's5';
      done.planSteps.forEach(step => { step.status = 'done'; });
      window.__emit('chat_state_changed', done);
      window.__emit('chat_state_changed', window.__workflowFixture);
    });
    await page.getByRole('button', { name: 'Шаг 5 / 5', exact: true }).waitFor();
    assert.equal(await page.locator('.workflow-compact-index').innerText(), '5/5', 'stale revision replaced completed progress');
    await page.evaluate(() => {
      const state = structuredClone(window.__workflowFixture);
      state.requestState = { id: 'req2', sequence: 2, mode: 'direct' };
      state.projectionRevision = '20';
      window.__emit('chat_state_changed', state);
      window.__emit('chat_state_changed', window.__workflowFixture);
      window.__emit('workflow_run_changed', { run: window.__workflowFixture.workflowRun });
    });
    assert.equal(await page.locator('.workflow-compact, .step-dock').count(), 0, 'old plan resurrected for direct answer');
    await page.evaluate(() => {
      const state = window.__state('old');
      state.requestState = { id: 'req3', sequence: 3, mode: 'clarify', original: 'Нужна сортировка' };
      state.projectionRevision = '30';
      window.__emit('chat_state_changed', state);
    });
    await page.getByRole('button', { name: 'Показать в чате', exact: true }).waitFor();
    assert.equal(await page.locator('.step-dock').count(), 0, 'clarification created plan');
    const toolButton = page.getByRole('button', { name: 'Инструменты · 1', exact: true });
    await toolButton.hover();
    await page.getByRole('region', { name: 'Вызовы инструментов' }).waitFor();
    await page.locator('.tool-invocation > summary').click();
    await page.getByText('Код завершения: 1', { exact: true }).waitFor();
    await page.screenshot({ path: '/tmp/zavod-tools-desktop.png' });
    await page.setViewportSize({ width: 390, height: 844 });
    await toolButton.click();
    await page.screenshot({ path: '/tmp/zavod-tools-mobile.png' });
    const panel = await page.locator('.tool-activity-popover').boundingBox();
    assert.ok(panel.x >= 0 && panel.x + panel.width <= 390 && panel.y >= 0, 'tool popover outside mobile viewport');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'tool UI overflow');
    await page.keyboard.press('Escape');
    await page.setViewportSize({ width: 1440, height: 1000 });
    const consent = page.getByRole('checkbox', { name: /Разрешить диагностику/ });
    assert.equal(await consent.isChecked(), false, 'tool permission default must be off');
    await consent.check();
    assert.equal(await page.getByText('Ответ Люмен', { exact: true }).count(), 0);
    await page.evaluate(() => window.__emit('chat_state_changed', { task: { id: 'other', title: 'Background', projectId: '' }, messages: [{ id: 'leak', role: 'agent', content: 'WRONG CHAT' }], agents: [] }));
    assert.equal(await page.getByText('WRONG CHAT', { exact: true }).count(), 0);
    await page.getByRole('button', { name: 'Привет', exact: true }).click();
    assert.equal(await page.getByRole('button', { name: 'Инструменты · 1', exact: true }).count(), 0, 'tool log leaked across chats');
    assert.equal(await composer.inputValue(), 'Сохранённый черновик');
    await page.getByRole('button', { name: 'Новый чат в Telegram-бот' }).click();
    assert.equal(await page.getByLabel('Проект чата', { exact: true }).inputValue(), 'p');
    assert.equal(await page.getByText('Ответ Люмен', { exact: true }).count(), 0);
    assert.equal(await page.getByRole('checkbox', { name: /Разрешить диагностику/ }).isChecked(), false, 'consent leaked across chats');
    await page.getByRole('checkbox', { name: /Разрешить диагностику/ }).check();
    await composer.fill('Разберись, почему падают тесты');
    await page.getByRole('button', { name: 'Отправить', exact: true }).click();
    await page.getByText('Ответ Люмен', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => window.__lastToolConsent), 'm', 'consent not bound to selected model');
    assert.equal(await page.getByRole('checkbox', { name: /Разрешить диагностику/ }).isChecked(), false, 'consent was not consumed');
    await page.getByRole('button', { name: 'Новый чат', exact: true }).first().click();
    await composer.fill('Напиши скрипт на Go');
    await page.getByRole('button', { name: 'Отправить', exact: true }).click();
    await page.getByText('Для выполнения задачи нужна рабочая папка.').waitFor();
    await page.screenshot({ path: '/tmp/zavod-chats-desktop.png' });
    await page.getByRole('button', { name: 'Создать рабочую папку' }).click();
    await page.getByRole('dialog', { name: 'Рабочая папка', exact: true }).waitFor();
    await page.getByRole('button', { name: 'Выбрать на диске' }).click();
    assert.equal(await page.getByLabel('Существующая папка (необязательно)').inputValue(), '/tmp/existing');
    await page.getByRole('button', { name: 'Закрыть', exact: true }).click();
    await page.getByLabel('Действия: Напиши скрипт на Go').click();
    await page.getByRole('button', { name: 'Изменить', exact: true }).click();
    await page.getByLabel('Название', { exact: true }).fill('Проверка HTTP');
    await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
    await page.getByRole('button', { name: 'Проверка HTTP', exact: true }).waitFor();
    await page.getByRole('button', { name: 'В архив', exact: true }).click();
    await page.getByRole('button', { name: 'Восстановить', exact: true }).click();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: '/tmp/zavod-chats-mobile.png' });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'horizontal page overflow');
    assert.equal(errors.length, 0, errors.join('\n'));
    console.log('PASS: chats, isolated events and tool logs, tool popover, one-request model consent, drafts, workspace gate, rename, archive, desktop/mobile layout');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
