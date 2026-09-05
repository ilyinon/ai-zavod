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
      const model = { id: 'm', name: 'Qwen по сети', modelName: 'qwen3:coder', provider: 'openai-compatible', isActive: true, status: 'online', latencyMs: 12 };
      const listeners = {};
      const emit = (event, data) => (listeners[event] || []).forEach(fn => fn(structuredClone(data)));
      const state = id => ({ project: projects.find(p => p.id === chats.find(t => t.id === id)?.projectId) || {}, task: chats.find(t => t.id === id), messages: messages[id] || [], workflowSteps: [], planSteps: [], artifacts: [], changes: [], testRuns: [], reviews: [], webSources: [], agents: [] });
      window.__emit = emit;
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
        SendMessage: async ({ taskId, content }) => {
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
    assert.equal(await page.getByText('Ответ Люмен', { exact: true }).count(), 0);
    await page.evaluate(() => window.__emit('chat_state_changed', { task: { id: 'other', title: 'Background', projectId: '' }, messages: [{ id: 'leak', role: 'agent', content: 'WRONG CHAT' }], agents: [] }));
    assert.equal(await page.getByText('WRONG CHAT', { exact: true }).count(), 0);
    await page.getByRole('button', { name: 'Привет', exact: true }).click();
    assert.equal(await composer.inputValue(), 'Сохранённый черновик');
    await page.getByRole('button', { name: 'Новый чат в Telegram-бот' }).click();
    assert.equal(await page.getByLabel('Проект чата', { exact: true }).inputValue(), 'p');
    assert.equal(await page.getByText('Ответ Люмен', { exact: true }).count(), 0);
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
    console.log('PASS: new chats, project chats, isolated events, drafts, workspace gate, folder picker, rename, archive, mobile layout');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
