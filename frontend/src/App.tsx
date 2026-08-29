import { FormEvent, KeyboardEvent, useEffect, useMemo, useRef, useState } from 'react';
import {
  AgentStatus,
  AgentMessageDelta,
  AppPaths,
  ChatState,
  Message,
  ModelConfig,
  Project,
  ProjectState,
  WorkflowRun,
  WorkflowStep,
  backend,
} from './lib/backend';

const statusLabels: Record<string, string> = {
  idle: 'свободен',
  thinking: 'думает',
  calling_model: 'вызывает модель',
  answering: 'пишет ответ',
  done: 'готово',
  failed: 'ошибка',
  queued: 'в очереди',
  running: 'в работе',
  waiting_user: 'ждет вас',
};

const workflowStatusLabels: Record<string, string> = {
  running: 'в работе',
  waiting_user: 'ждет уточнение',
  done: 'готово',
  failed: 'ошибка',
};

const workflowStepLabels: Record<string, string> = {
  manager_intake: 'Постановка задачи',
  product_requirements: 'Требования',
  architect_plan: 'Архитектурный план',
  manager_final: 'Итог',
};

const modelStatusLabels: Record<string, string> = {
  unknown: 'не проверялась',
  checking: 'проверяется',
  online: 'доступна',
  offline: 'недоступна',
};

const providerLabels: Record<string, string> = {
  'remote-qwen': 'Remote Qwen',
  openai: 'OpenAI',
  'openai-compatible': 'OpenAI-compatible',
};

const emptyModel: ModelConfig = {
  id: '',
  name: 'Qwen по сети',
  provider: 'remote-qwen',
  baseUrl: 'http://192.168.50.120:8000/v1',
  apiKeyRef: '',
  modelName: 'qwen3:8b',
  isActive: false,
  status: 'unknown',
  lastCheckedAt: '',
  lastError: '',
  latencyMs: 0,
  createdAt: '',
  updatedAt: '',
};

type SettingsTab = 'projects' | 'models';

function App() {
  const [paths, setPaths] = useState<AppPaths | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState('');
  const [currentProject, setCurrentProject] = useState<Project | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [workflowRun, setWorkflowRun] = useState<WorkflowRun | null>(null);
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [agents, setAgents] = useState<AgentStatus[]>([]);
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [activeModelId, setActiveModelId] = useState('');
  const [projectQuery, setProjectQuery] = useState('');
  const [newProjectName, setNewProjectName] = useState('');
  const [existingProjectName, setExistingProjectName] = useState('');
  const [existingProjectPath, setExistingProjectPath] = useState('');
  const [showNewProject, setShowNewProject] = useState(false);
  const [showExistingProject, setShowExistingProject] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>('projects');
  const [messageInput, setMessageInput] = useState('');
  const [modelForm, setModelForm] = useState<ModelConfig>(emptyModel);
  const [editingModelId, setEditingModelId] = useState('');
  const [checkingModelId, setCheckingModelId] = useState('');
  const [streamingMessage, setStreamingMessage] = useState<Message | null>(null);
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState('');
  const messagesRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    loadBootstrap();

    const offStatus = backend.onAgentStatusChanged((status) => {
      setAgents((previous) => upsertAgent(previous, status));
    });
    const offChat = backend.onChatStateChanged((state) => {
      applyChatState(state);
      setAgents(state.agents);
      if (state.error) {
        setError(state.error);
      }
    });
    const offDelta = backend.onAgentMessageDelta(handleAgentMessageDelta);
    const offWorkflowRun = backend.onWorkflowRunChanged((run) => {
      setWorkflowRun(run);
    });
    const offWorkflowStep = backend.onWorkflowStepChanged((step) => {
      setWorkflowSteps((previous) => upsertWorkflowStep(previous, step));
    });
    const offModels = backend.onModelsChanged((nextModels) => {
      setModels(nextModels);
      const active = nextModels.find((model) => model.isActive);
      if (active) {
        setActiveModelId(active.id);
      }
    });

    return () => {
      offStatus();
      offChat();
      offDelta();
      offWorkflowRun();
      offWorkflowStep();
      offModels();
    };
  }, []);

  useEffect(() => {
    const messagesElement = messagesRef.current;
    if (!messagesElement) {
      return;
    }
    messagesElement.scrollTop = messagesElement.scrollHeight;
  }, [messages, workflowSteps, sending]);

  useEffect(() => {
    if (editingModelId === 'new') {
      return;
    }
    const nextModel = models.find((model) => model.id === (editingModelId || activeModelId)) ?? models[0];
    if (nextModel) {
      setModelForm(nextModel);
      setEditingModelId(nextModel.id);
    }
  }, [models, activeModelId, editingModelId]);

  const activeModel = useMemo(
    () => models.find((model) => model.id === activeModelId) ?? models.find((model) => model.isActive) ?? null,
    [models, activeModelId],
  );

  const visibleProjects = useMemo(() => {
    const query = projectQuery.trim().toLowerCase();
    if (!query) {
      return projects;
    }
    return projects.filter((project) => {
      return project.name.toLowerCase().includes(query) || project.path.toLowerCase().includes(query);
    });
  }, [projects, projectQuery]);

  const visibleMessages = streamingMessage ? [...messages, streamingMessage] : messages;

  function openSettings(tab: SettingsTab) {
    setSettingsTab(tab);
    setSettingsOpen(true);
  }

  async function loadBootstrap() {
    setLoading(true);
    setError('');
    try {
      const state = await backend.bootstrap();
      setPaths(state.paths);
      setProjects(state.projects);
      setSelectedProjectId(state.selectedProjectId);
      applyProjectState(state.chat);
      setAgents(state.agents);
      setModels(state.models);
      setActiveModelId(state.activeModelId);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
  }

  function applyProjectState(state: ProjectState) {
    if (state.project?.id) {
      setCurrentProject(state.project);
      setSelectedProjectId(state.project.id);
    }
    setMessages(state.messages ?? []);
    setWorkflowRun(state.workflowRun ?? null);
    setWorkflowSteps(state.workflowSteps ?? []);
  }

  function applyChatState(state: ChatState) {
    applyProjectState(state);
  }

  function handleAgentMessageDelta(delta: AgentMessageDelta) {
    if (delta.error) {
      setError(delta.error);
    }
    if (delta.done) {
      setStreamingMessage(null);
      return;
    }
    if (!delta.delta) {
      return;
    }

    setStreamingMessage((previous) => {
      if (!previous || previous.taskId !== delta.taskId || previous.agentId !== delta.agentId) {
        return {
          id: `stream-${delta.taskId}-${delta.agentId}`,
          taskId: delta.taskId,
          role: 'agent',
          agentId: delta.agentId,
          content: delta.delta,
          createdAt: new Date().toISOString(),
        };
      }
      return {
        ...previous,
        content: previous.content + delta.delta,
      };
    });
  }

  async function handleSelectProject(projectId: string) {
    setError('');
    try {
      const state = await backend.selectProject(projectId);
      applyProjectState(state);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleCreateProject(event: FormEvent) {
    event.preventDefault();
    setError('');
    try {
      await backend.createProject(newProjectName);
      setNewProjectName('');
      setShowNewProject(false);
      setSettingsOpen(false);
      await loadBootstrap();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleAddExistingProject(event: FormEvent) {
    event.preventDefault();
    setError('');
    try {
      await backend.addExistingProject(existingProjectName, existingProjectPath);
      setExistingProjectName('');
      setExistingProjectPath('');
      setShowExistingProject(false);
      setSettingsOpen(false);
      await loadBootstrap();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleSendMessage(event?: FormEvent) {
    event?.preventDefault();
    const content = messageInput.trim();
    if (!content || !selectedProjectId || sending) {
      return;
    }

    setMessageInput('');
    setSending(true);
    setStreamingMessage(null);
    setError('');
    setMessages((previous) => [
      ...previous,
      {
        id: `local-${Date.now()}`,
        taskId: '',
        role: 'user',
        agentId: '',
        content,
        createdAt: new Date().toISOString(),
      },
    ]);

    try {
      const state = await backend.sendMessage(selectedProjectId, content);
      applyChatState(state);
      setAgents(state.agents);
      if (state.error) {
        setError(state.error);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSending(false);
    }
  }

  function handleMessageKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      handleSendMessage();
    }
  }

  async function handleSetActiveModel(modelId: string) {
    setError('');
    try {
      const updated = await backend.setActiveModel(modelId);
      setModels(updated);
      setActiveModelId(modelId);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  function handleNewModel(provider = 'remote-qwen') {
    const draft = modelDraft(provider);
    setEditingModelId('new');
    setModelForm(draft);
  }

  function handleProviderChange(provider: string) {
    setModelForm({
      ...modelDraft(provider),
      id: modelForm.id,
      isActive: modelForm.isActive,
      status: modelForm.status,
      lastCheckedAt: modelForm.lastCheckedAt,
      lastError: modelForm.lastError,
      latencyMs: modelForm.latencyMs,
      createdAt: modelForm.createdAt,
      updatedAt: modelForm.updatedAt,
    });
  }

  async function handleSaveModel(event: FormEvent) {
    event.preventDefault();
    setError('');
    try {
      const updated = await backend.saveModelConfig({
        ...modelForm,
      });
      setModels(updated);
      const active = updated.find((model) => model.isActive);
      if (active) {
        setActiveModelId(active.id);
      }
      const saved = updated.find((model) => {
        return (
          model.name === modelForm.name &&
          model.baseUrl === modelForm.baseUrl &&
          model.modelName === modelForm.modelName
        );
      });
      if (saved) {
        setEditingModelId(saved.id);
        setModelForm(saved);
      }
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleCheckModel(modelId: string) {
    if (!modelId) {
      setError('Сначала сохрани модель, потом ее можно проверить.');
      return;
    }
    setCheckingModelId(modelId);
    setError('');
    try {
      const updated = await backend.checkModel(modelId);
      setModels(updated);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setCheckingModelId('');
    }
  }

  return (
    <main className="app-shell">
      <aside className="sidebar projects-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">Zavod AI</p>
            <h1>Проекты</h1>
          </div>
          <div className="header-actions">
            <button className="icon-button" title="Настройки проектов" onClick={() => openSettings('projects')}>
              ⚙
            </button>
            <button className="icon-button" title="Обновить" onClick={loadBootstrap}>
              ↻
            </button>
          </div>
        </div>

        <label className="field-label" htmlFor="project-search">
          Поиск
        </label>
        <input
          id="project-search"
          className="input"
          placeholder="Название или путь"
          value={projectQuery}
          onChange={(event) => setProjectQuery(event.target.value)}
        />

        <div className="project-list">
          {loading && <p className="muted">Загружаю проекты...</p>}
          {!loading &&
            visibleProjects.map((project) => (
              <button
                key={project.id}
                className={`project-item ${project.id === selectedProjectId ? 'active' : ''}`}
                onClick={() => handleSelectProject(project.id)}
              >
                <span className="project-name">{project.name}</span>
                <span className="project-path">{project.path}</span>
              </button>
            ))}
          {!loading && visibleProjects.length === 0 && <p className="muted">Проекты не найдены.</p>}
        </div>

        {paths && (
          <div className="path-note">
            <span>Каталог проектов</span>
            <code>{paths.projectsDir}</code>
          </div>
        )}
      </aside>

      <section className="chat-panel">
        <header className="chat-header">
          <div>
            <p className="eyebrow">Чат с агентом</p>
            <h2>{currentProject?.name ?? 'Выбери проект'}</h2>
          </div>
          <button className="model-pill" type="button" title="Настройки LLM" onClick={() => openSettings('models')}>
            <span>Модель</span>
            <strong>{activeModel?.name ?? 'не выбрана'}</strong>
          </button>
        </header>

        {workflowRun && (
          <div className="workflow-topbar">
            <WorkflowProgress
              agents={agents}
              run={workflowRun}
              steps={workflowSteps}
            />
          </div>
        )}

        {error && <div className="error-banner">{error}</div>}

        <div className="messages" ref={messagesRef}>
          {visibleMessages.length === 0 && (
            <div className="empty-state">
              <h3>Поставь первую задачу</h3>
              <p>Менеджер примет ее, уточнит контекст или предложит первый план действий.</p>
            </div>
          )}

          {visibleMessages.map((message) => (
            <article key={message.id} className={`message ${message.role}`}>
              <div className="message-meta">
                <span>{message.role === 'user' ? 'Вы' : 'Менеджер'}</span>
                <time>{formatTime(message.createdAt)}</time>
              </div>
              <p>{message.content}</p>
            </article>
          ))}
          {sending && !streamingMessage && (
            <article className="message agent pending">
              <div className="message-meta">
                <span>Менеджер</span>
                <time>сейчас</time>
              </div>
              <p>Работаю над ответом...</p>
            </article>
          )}
        </div>

        <form className="composer" onSubmit={handleSendMessage}>
          <textarea
            value={messageInput}
            onChange={(event) => setMessageInput(event.target.value)}
            onKeyDown={handleMessageKeyDown}
            placeholder={selectedProjectId ? 'Опиши задачу для AI-завода' : 'Сначала выбери проект'}
            disabled={!selectedProjectId || sending}
          />
          <button className="send-button" type="submit" disabled={!messageInput.trim() || !selectedProjectId || sending}>
            Отправить
          </button>
        </form>
      </section>

      <aside className="sidebar agents-panel">
        <section className="right-section">
          <div className="panel-heading compact">
            <div>
              <p className="eyebrow">Роли</p>
              <h2>Агенты</h2>
            </div>
          </div>
          <div className="agent-list">
            {agents.map((agent) => (
              <div key={agent.id} className="agent-card">
                <div className="agent-topline">
                  <strong>{agent.name}</strong>
                  <span className={`status-badge ${agent.status}`}>{statusLabels[agent.status] ?? agent.status}</span>
                </div>
                <p>{agent.activity}</p>
                <small>{modelNameById(models, agent.modelId)}</small>
              </div>
            ))}
          </div>
        </section>

        <section className="right-section">
          <div className="panel-heading compact">
            <div>
              <p className="eyebrow">LLM</p>
              <h2>Активная модель</h2>
            </div>
            <button className="icon-button" title="Настройки LLM" onClick={() => openSettings('models')}>
              ⚙
            </button>
          </div>
          <div className="active-model-card">
            <strong>{activeModel?.name ?? 'не выбрана'}</strong>
            <span>{activeModel ? `${providerLabels[activeModel.provider] ?? activeModel.provider} · ${activeModel.modelName}` : 'Открой настройки LLM'}</span>
            {activeModel && (
              <div className="model-meta-row">
                <span className={`model-status ${activeModel.status || 'unknown'}`}>
                  {modelStatusLabels[activeModel.status || 'unknown'] ?? activeModel.status}
                </span>
                {activeModel.latencyMs > 0 && <small>{activeModel.latencyMs} мс</small>}
              </div>
            )}
          </div>
        </section>

        {paths && (
          <section className="right-section storage-note">
            <p className="eyebrow">Хранилище</p>
            <code>{paths.dbPath}</code>
          </section>
        )}
      </aside>

      {settingsOpen && (
        <div className="settings-backdrop" role="presentation" onClick={() => setSettingsOpen(false)}>
          <section className="settings-panel" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <header className="settings-header">
              <div>
                <p className="eyebrow">Настройки</p>
                <h2>Проекты и модели</h2>
              </div>
              <button className="icon-button" title="Закрыть" onClick={() => setSettingsOpen(false)}>
                ×
              </button>
            </header>

            <div className="settings-tabs">
              <button
                className={settingsTab === 'projects' ? 'active' : ''}
                type="button"
                onClick={() => setSettingsTab('projects')}
              >
                Проекты
              </button>
              <button
                className={settingsTab === 'models' ? 'active' : ''}
                type="button"
                onClick={() => setSettingsTab('models')}
              >
                LLM
              </button>
            </div>

            {settingsTab === 'projects' && (
              <div className="settings-content">
                <section className="settings-section">
                  <div className="project-actions">
                    <button
                      className="action-button project-action"
                      type="button"
                      title="Создать новый проект"
                      onClick={() => setShowNewProject((value) => !value)}
                    >
                      <span className="button-icon" aria-hidden="true">
                        +
                      </span>
                      <span className="button-label">Новый</span>
                    </button>
                    <button
                      className="action-button project-action secondary"
                      type="button"
                      title="Добавить существующий проект"
                      onClick={() => setShowExistingProject((value) => !value)}
                    >
                      <span className="button-icon" aria-hidden="true">
                        +
                      </span>
                      <span className="button-label">Из папки</span>
                    </button>
                  </div>

                  {showNewProject && (
                    <form className="inline-form" onSubmit={handleCreateProject}>
                      <input
                        className="input"
                        placeholder="Название проекта"
                        value={newProjectName}
                        onChange={(event) => setNewProjectName(event.target.value)}
                      />
                      <button className="primary-button" type="submit">
                        Создать
                      </button>
                    </form>
                  )}

                  {showExistingProject && (
                    <form className="inline-form" onSubmit={handleAddExistingProject}>
                      <input
                        className="input"
                        placeholder="Название"
                        value={existingProjectName}
                        onChange={(event) => setExistingProjectName(event.target.value)}
                      />
                      <input
                        className="input"
                        placeholder="~/ai_zavod/project"
                        value={existingProjectPath}
                        onChange={(event) => setExistingProjectPath(event.target.value)}
                      />
                      <button className="primary-button" type="submit">
                        Добавить
                      </button>
                    </form>
                  )}
                </section>
              </div>
            )}

            {settingsTab === 'models' && (
              <div className="settings-content settings-models">
                <section className="settings-section">
                  <div className="settings-section-heading">
                    <h3>Модели</h3>
                    <button className="icon-button" title="Новая модель" onClick={() => handleNewModel()}>
                      +
                    </button>
                  </div>
                  <div className="model-list">
                    {models.map((model) => (
                      <div key={model.id} className={`model-item ${model.id === activeModelId ? 'active' : ''}`}>
                        <button
                          className="model-edit-button"
                          onClick={() => {
                            setEditingModelId(model.id);
                            setModelForm(model);
                          }}
                        >
                          <span>{model.name}</span>
                          <small>{providerLabels[model.provider] ?? model.provider} · {model.modelName}</small>
                        </button>
                        <div className="model-meta-row">
                          <span className={`model-status ${model.status || 'unknown'}`}>
                            {modelStatusLabels[model.status || 'unknown'] ?? model.status}
                          </span>
                          {model.latencyMs > 0 && <small>{model.latencyMs} мс</small>}
                        </div>
                        {model.lastError && <p className="model-error">{model.lastError}</p>}
                        <div className="model-actions-row">
                          <button
                            className="small-button"
                            disabled={model.id === activeModelId}
                            onClick={() => handleSetActiveModel(model.id)}
                          >
                            {model.id === activeModelId ? 'Активна' : 'Выбрать'}
                          </button>
                          <button
                            className="small-button secondary"
                            disabled={checkingModelId === model.id}
                            onClick={() => handleCheckModel(model.id)}
                          >
                            {checkingModelId === model.id ? 'Проверяю' : 'Проверить'}
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                </section>

                <form className="model-form settings-section" onSubmit={handleSaveModel}>
                  <label className="field-label" htmlFor="model-provider">
                    Тип
                  </label>
                  <select
                    id="model-provider"
                    className="input"
                    value={modelForm.provider}
                    onChange={(event) => handleProviderChange(event.target.value)}
                  >
                    <option value="remote-qwen">Remote Qwen</option>
                    <option value="openai">OpenAI</option>
                    <option value="openai-compatible">OpenAI-compatible</option>
                  </select>

                  <label className="field-label" htmlFor="model-name">
                    Название
                  </label>
                  <input
                    id="model-name"
                    className="input"
                    value={modelForm.name}
                    onChange={(event) => setModelForm({ ...modelForm, name: event.target.value })}
                  />

                  <label className="field-label" htmlFor="model-base-url">
                    Base URL
                  </label>
                  <input
                    id="model-base-url"
                    className="input"
                    value={modelForm.baseUrl}
                    onChange={(event) => setModelForm({ ...modelForm, baseUrl: event.target.value })}
                  />

                  <label className="field-label" htmlFor="model-name-id">
                    Model
                  </label>
                  <input
                    id="model-name-id"
                    className="input"
                    value={modelForm.modelName}
                    onChange={(event) => setModelForm({ ...modelForm, modelName: event.target.value })}
                  />

                  <label className="field-label" htmlFor="model-key">
                    API key или env
                  </label>
                  <input
                    id="model-key"
                    className="input"
                    value={modelForm.apiKeyRef}
                    onChange={(event) => setModelForm({ ...modelForm, apiKeyRef: event.target.value })}
                  />

                  <label className="checkbox-row">
                    <input
                      type="checkbox"
                      checked={modelForm.isActive}
                      onChange={(event) => setModelForm({ ...modelForm, isActive: event.target.checked })}
                    />
                    <span>Сделать активной</span>
                  </label>

                  <div className="model-form-actions">
                    <button className="primary-button" type="submit">
                      {modelForm.id ? 'Сохранить модель' : 'Добавить модель'}
                    </button>
                    <button
                      className="action-button secondary"
                      type="button"
                      disabled={!modelForm.id || checkingModelId === modelForm.id}
                      onClick={() => handleCheckModel(modelForm.id)}
                    >
                      Проверить
                    </button>
                  </div>
                </form>
              </div>
            )}
          </section>
        </div>
      )}
    </main>
  );
}

type WorkflowProgressProps = {
  agents: AgentStatus[];
  run: WorkflowRun;
  steps: WorkflowStep[];
};

function WorkflowProgress({ agents, run, steps }: WorkflowProgressProps) {
  const orderedStepKeys = ['manager_intake', 'product_requirements', 'architect_plan', 'manager_final'];
  return (
    <section className={`workflow-card ${run.status}`}>
      <div className="workflow-card-header">
        <div>
          <p className="eyebrow">Ход работы</p>
          <h3>AI-завод V0.3</h3>
        </div>
        <span className={`workflow-status ${run.status}`}>
          {workflowStatusLabels[run.status] ?? run.status}
        </span>
      </div>
      <div className="workflow-steps">
        {orderedStepKeys.map((stepKey, index) => {
          const step = steps.find((item) => item.stepKey === stepKey);
          const agent = step ? agents.find((item) => item.id === step.agentId) : null;
          const status = step?.status ?? (run.currentStep === stepKey ? 'running' : 'queued');
          return (
            <div key={stepKey} className={`workflow-step ${status}`}>
              <span className="workflow-step-index">{index + 1}</span>
              <div className="workflow-step-body">
                <div className="workflow-step-title">
                  <strong>{workflowStepLabels[stepKey] ?? stepKey}</strong>
                  <span className={`workflow-step-status ${status}`}>
                    {statusLabels[status] ?? status}
                  </span>
                </div>
                <p>{agent?.name ?? agentNameById(step?.agentId ?? agentForStep(stepKey))}</p>
                {step?.output && <small>{shortPreview(step.output)}</small>}
                {step?.error && <small className="workflow-step-error">{step.error}</small>}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function upsertAgent(items: AgentStatus[], next: AgentStatus): AgentStatus[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) {
    return [...items, next];
  }
  return items.map((item, itemIndex) => (itemIndex === index ? next : item));
}

function upsertWorkflowStep(items: WorkflowStep[], next: WorkflowStep): WorkflowStep[] {
  const index = items.findIndex((item) => item.id === next.id);
  if (index === -1) {
    return [...items, next];
  }
  return items.map((item, itemIndex) => (itemIndex === index ? next : item));
}

function agentForStep(stepKey: string): string {
  if (stepKey === 'product_requirements') {
    return 'product';
  }
  if (stepKey === 'architect_plan') {
    return 'architect';
  }
  return 'manager';
}

function agentNameById(agentId: string): string {
  if (agentId === 'product') {
    return 'Продакт';
  }
  if (agentId === 'architect') {
    return 'Архитектор';
  }
  return 'Менеджер';
}

function shortPreview(value: string): string {
  const compact = value.replace(/\s+/g, ' ').trim();
  if (compact.length <= 140) {
    return compact;
  }
  return `${compact.slice(0, 140)}...`;
}

function modelNameById(models: ModelConfig[], modelId: string): string {
  return models.find((model) => model.id === modelId)?.name ?? 'модель не выбрана';
}

function modelDraft(provider: string): ModelConfig {
  const base: ModelConfig = {
    ...emptyModel,
    provider,
    status: 'unknown',
    lastCheckedAt: '',
    lastError: '',
    latencyMs: 0,
  };

  if (provider === 'openai') {
    return {
      ...base,
      name: 'OpenAI ChatGPT',
      baseUrl: 'https://api.openai.com/v1',
      apiKeyRef: 'OPENAI_API_KEY',
      modelName: 'gpt-5-mini',
    };
  }

  if (provider === 'openai-compatible') {
    return {
      ...base,
      name: 'OpenAI-compatible',
      baseUrl: 'http://127.0.0.1:8000/v1',
      apiKeyRef: '',
      modelName: 'local-model',
    };
  }

  return {
    ...base,
    name: 'Qwen по сети',
    baseUrl: 'http://192.168.50.120:8000/v1',
    apiKeyRef: '',
    modelName: 'qwen3:8b',
  };
}

function formatTime(value: string): string {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
  });
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

export default App;
