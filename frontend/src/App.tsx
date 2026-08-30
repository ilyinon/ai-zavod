import { FormEvent, KeyboardEvent, ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import {
  AgentStatus,
  AgentMessageDelta,
  AppPaths,
  ChatState,
  Message,
  ModelConfig,
  PendingClarification,
  ProposedChange,
  Project,
  ProjectState,
  TaskBlueprint,
  WorkflowRun,
  WorkflowStep,
  backend,
} from './lib/backend';
import architectAvatar from './assets/avatars/architect.png';
import developerAvatar from './assets/avatars/developer.png';
import managerAvatar from './assets/avatars/manager.png';
import productAvatar from './assets/avatars/product.png';
import reviewerAvatar from './assets/avatars/reviewer.png';
import securityAvatar from './assets/avatars/security.svg';
import testerAvatar from './assets/avatars/tester.png';

const statusLabels: Record<string, string> = {
  idle: 'свободен',
  thinking: 'думает',
  calling_model: 'вызывает модель',
  answering: 'пишет ответ',
  writing_files: 'сохраняет',
  done: 'готово',
  failed: 'ошибка',
  queued: 'в очереди',
  running: 'в работе',
  waiting_user: 'ждет вас',
  needs_work: 'нужна доработка',
  blocked: 'остановлено',
};

const workflowStepLabels: Record<string, string> = {
  manager_intake: 'Постановка задачи',
  product_requirements: 'Требования',
  task_blueprint: 'Blueprint',
  architect_plan: 'Архитектурный план',
  security_analysis: 'ИБ-анализ',
  developer_plan: 'Разработка',
  tester_commands: 'Проверка',
  review: 'Ревью',
  manager_final: 'Итог',
};

const workflowStepOrder = [
  'manager_intake',
  'product_requirements',
  'task_blueprint',
  'architect_plan',
  'developer_plan',
  'tester_commands',
  'review',
  'manager_final',
];

const securityWorkflowStepOrder = ['security_analysis'];

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

const changeStatusLabels: Record<string, string> = {
  pending: 'ожидает',
  applied: 'применено',
  failed: 'ошибка',
};

const changeActionLabels: Record<string, string> = {
  create: 'создать',
  replace: 'заменить',
};

const reviewStatusLabels: Record<string, string> = {
  pending: 'ожидает',
  running: 'в работе',
  accepted: 'принято',
  needs_work: 'нужна доработка',
  blocked: 'остановлено',
  failed: 'ошибка',
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
  const [blueprint, setBlueprint] = useState<TaskBlueprint | null>(null);
  const [clarification, setClarification] = useState<PendingClarification | null>(null);
  const [changes, setChanges] = useState<ProposedChange[]>([]);
  const [agents, setAgents] = useState<AgentStatus[]>([]);
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [activeModelId, setActiveModelId] = useState('');
  const [projectQuery, setProjectQuery] = useState('');
  const [newProjectName, setNewProjectName] = useState('');
  const [existingProjectName, setExistingProjectName] = useState('');
  const [existingProjectPath, setExistingProjectPath] = useState('');
  const [editingProjectId, setEditingProjectId] = useState('');
  const [editingProjectName, setEditingProjectName] = useState('');
  const [editingProjectPath, setEditingProjectPath] = useState('');
  const [confirmDeleteProjectId, setConfirmDeleteProjectId] = useState('');
  const [showNewProject, setShowNewProject] = useState(false);
  const [showExistingProject, setShowExistingProject] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>('projects');
  const [messageInput, setMessageInput] = useState('');
  const [modelForm, setModelForm] = useState<ModelConfig>(emptyModel);
  const [editingModelId, setEditingModelId] = useState('');
  const [checkingModelId, setCheckingModelId] = useState('');
  const [streamingMessage, setStreamingMessage] = useState<Message | null>(null);
  const [expandedDiffIds, setExpandedDiffIds] = useState<string[]>([]);
  const [changesDockOpen, setChangesDockOpen] = useState(false);
  const [applyingChanges, setApplyingChanges] = useState(false);
  const [copiedMessageId, setCopiedMessageId] = useState('');
  const [clarificationAnswers, setClarificationAnswers] = useState<Record<string, string>>({});
  const [submittingClarification, setSubmittingClarification] = useState(false);
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
  }, [messages, workflowSteps, clarification, sending]);

  useEffect(() => {
    setClarificationAnswers({});
  }, [clarification?.workflowRunId]);

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

  const visibleMessages = useMemo(() => {
    const source = streamingMessage ? [...messages, streamingMessage] : messages;
    return source.filter((message) => !isRoutinePipelineMessage(message));
  }, [messages, streamingMessage]);
  const visibleChanges = useMemo(() => aggregateWorkflowChanges(changes), [changes]);
  const pendingChanges = useMemo(() => changes.filter((change) => change.status === 'pending'), [changes]);
  const changeSummary = useMemo(() => summarizeChanges(visibleChanges), [visibleChanges]);

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
    setBlueprint(state.blueprint ?? null);
    setClarification(state.clarification ?? null);
    setChanges(state.changes ?? []);
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

  function startEditProject(project: Project) {
    setEditingProjectId(project.id);
    setEditingProjectName(project.name);
    setEditingProjectPath(project.path);
    setConfirmDeleteProjectId('');
    setError('');
  }

  function cancelEditProject() {
    setEditingProjectId('');
    setEditingProjectName('');
    setEditingProjectPath('');
  }

  async function handleUpdateProject(event: FormEvent) {
    event.preventDefault();
    if (!editingProjectId) {
      return;
    }

    setError('');
    try {
      const updated = await backend.updateProject(editingProjectId, editingProjectName, editingProjectPath);
      setProjects((previous) => previous.map((project) => (project.id === updated.id ? updated : project)));
      if (selectedProjectId === updated.id) {
        setCurrentProject(updated);
      }
      cancelEditProject();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleDeleteProject(project: Project) {
    if (confirmDeleteProjectId !== project.id) {
      setConfirmDeleteProjectId(project.id);
      return;
    }

    setError('');
    try {
      const state = await backend.deleteProject(project.id);
      setProjects(state.projects);
      setSelectedProjectId(state.selectedProjectId);
      setActiveModelId(state.activeModelId);
      setModels(state.models);
      setAgents(state.agents);
      applyProjectState(state.chat);
      setConfirmDeleteProjectId('');
      cancelEditProject();
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

  async function handleSubmitClarification(event: FormEvent) {
    event.preventDefault();
    if (!selectedProjectId || !clarification || submittingClarification) {
      return;
    }
    const answers = clarification.questions
      .map((question) => ({
        questionId: question.id,
        question: question.text,
        answer: (clarificationAnswers[question.id] ?? '').trim(),
      }))
      .filter((item) => item.answer);
    if (answers.length === 0) {
      setError('Ответь хотя бы на один вопрос уточнения.');
      return;
    }

    setSubmittingClarification(true);
    setSending(true);
    setError('');
    try {
      const state = await backend.submitClarification(selectedProjectId, clarification.workflowRunId, answers);
      applyChatState(state);
      setAgents(state.agents);
      if (state.error) {
        setError(state.error);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSubmittingClarification(false);
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

  async function handleApplyChanges() {
    if (!selectedProjectId || !workflowRun?.id || pendingChanges.length === 0 || applyingChanges) {
      return;
    }

    setApplyingChanges(true);
    setError('');
    try {
      const state = await backend.applyWorkflowChanges(selectedProjectId, workflowRun.id);
      applyChatState(state);
      setAgents(state.agents);
      if (state.error) {
        setError(state.error);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setApplyingChanges(false);
    }
  }

  async function handleCopyMessage(message: Message) {
    try {
      await copyToClipboard(message.content);
      setCopiedMessageId(message.id);
      window.setTimeout(() => {
        setCopiedMessageId((current) => (current === message.id ? '' : current));
      }, 1400);
    } catch (err) {
      setError(`Не удалось скопировать сообщение: ${errorMessage(err)}`);
    }
  }

  function toggleDiff(changeId: string) {
    setExpandedDiffIds((previous) => {
      if (previous.includes(changeId)) {
        return previous.filter((id) => id !== changeId);
      }
      return [...previous, changeId];
    });
  }

  return (
    <main className="app-shell">
      <aside className="sidebar projects-panel">
        <div className="panel-heading">
          <div>
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
            visibleProjects.map((project) => {
              const isEditing = editingProjectId === project.id;
              if (isEditing) {
                return (
                  <form
                    key={project.id}
                    className={`project-item project-edit-card ${project.id === selectedProjectId ? 'active' : ''}`}
                    onSubmit={handleUpdateProject}
                  >
                    <input
                      className="input project-edit-input"
                      aria-label="Название проекта"
                      value={editingProjectName}
                      onChange={(event) => setEditingProjectName(event.target.value)}
                    />
                    <input
                      className="input project-edit-input"
                      aria-label="Путь проекта"
                      value={editingProjectPath}
                      onChange={(event) => setEditingProjectPath(event.target.value)}
                    />
                    <div className="project-edit-actions">
                      <button className="project-save-button" type="submit">
                        Сохранить
                      </button>
                      <button className="project-cancel-button" type="button" onClick={cancelEditProject}>
                        Отмена
                      </button>
                    </div>
                  </form>
                );
              }

              return (
                <div key={project.id} className={`project-item ${project.id === selectedProjectId ? 'active' : ''}`}>
                  <button className="project-select-button" type="button" onClick={() => handleSelectProject(project.id)}>
                    <span className="project-name">{project.name}</span>
                    <span className="project-path">{project.path}</span>
                  </button>
                  {confirmDeleteProjectId === project.id ? (
                    <div className="project-delete-confirm">
                      <button
                        className="project-confirm-delete-button"
                        type="button"
                        title="Папка на диске останется"
                        onClick={() => void handleDeleteProject(project)}
                      >
                        Удалить
                      </button>
                      <button className="project-cancel-delete-button" type="button" onClick={() => setConfirmDeleteProjectId('')}>
                        Отмена
                      </button>
                    </div>
                  ) : (
                    <div className="project-item-actions">
                      <button
                        className="project-icon-button"
                        type="button"
                        title="Редактировать проект"
                        aria-label={`Редактировать ${project.name}`}
                        onClick={() => startEditProject(project)}
                      >
                        ✎
                      </button>
                      <button
                        className="project-icon-button danger"
                        type="button"
                        title="Удалить из списка"
                        aria-label={`Удалить ${project.name}`}
                        onClick={() => void handleDeleteProject(project)}
                      >
                        ×
                      </button>
                    </div>
                  )}
                </div>
              );
            })}
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
          <div className="chat-title-area">
            <AgentStrip agents={agents} run={workflowRun} steps={workflowSteps} activeModel={activeModel} />
          </div>
          <div className="chat-header-actions">
            {pendingChanges.length > 0 && (
              <button
                className="header-apply-button"
                type="button"
                disabled={applyingChanges}
                onClick={handleApplyChanges}
              >
                {applyingChanges ? 'Применяю...' : `Ручное применение ${pendingChanges.length}`}
              </button>
            )}
          </div>
        </header>

        {error && <div className="error-banner">{error}</div>}

        <div className="messages" ref={messagesRef}>
          {visibleMessages.length === 0 && (
            <div className="empty-state">
              <h3>Поставь первую задачу</h3>
              <p>Люмен примет ее, уточнит контекст или предложит первый план действий.</p>
            </div>
          )}

          {visibleMessages.map((message) => (
            <article key={message.id} className={`message ${message.role}`}>
              <div className="message-meta">
                <span>{message.role === 'user' ? 'Вы' : agentNameById(message.agentId)}</span>
                <div className="message-meta-actions">
                  <time>{formatTime(message.createdAt)}</time>
                  <button
                    className={`message-copy-button ${copiedMessageId === message.id ? 'copied' : ''}`}
                    type="button"
                    aria-label={copiedMessageId === message.id ? 'Сообщение скопировано' : 'Скопировать сообщение'}
                    title={copiedMessageId === message.id ? 'Скопировано' : 'Скопировать сообщение'}
                    onClick={() => void handleCopyMessage(message)}
                  >
                    {copiedMessageId === message.id ? '✓' : '⧉'}
                  </button>
                </div>
              </div>
              <MarkdownContent content={message.content} />
            </article>
          ))}
          {sending && !streamingMessage && (
            <article className="message agent pending">
              <div className="message-meta">
                <span>Люмен</span>
                <time>сейчас</time>
              </div>
              <p>Работаю над ответом...</p>
            </article>
          )}
        </div>

        {clarification && (
          <form className="clarification-panel" onSubmit={handleSubmitClarification}>
            <div className="clarification-heading">
              <div>
                <p className="eyebrow">Нужно уточнение</p>
                <h3>{clarification.summary || 'Люмен нужно больше контекста'}</h3>
                {clarification.goal && <p>{clarification.goal}</p>}
              </div>
              <span>{clarification.questions.length}</span>
            </div>
            <div className="clarification-questions">
              {clarification.questions.map((question) => (
                <label key={question.id} className="clarification-question">
                  <span>{question.text}</span>
                  <textarea
                    value={clarificationAnswers[question.id] ?? ''}
                    onChange={(event) =>
                      setClarificationAnswers((previous) => ({
                        ...previous,
                        [question.id]: event.target.value,
                      }))
                    }
                    placeholder="Ответь обычным текстом"
                    disabled={submittingClarification}
                  />
                </label>
              ))}
            </div>
            <button
              className="clarification-submit"
              type="submit"
              disabled={submittingClarification || !Object.values(clarificationAnswers).some((value) => value.trim())}
            >
              {submittingClarification ? 'Отправляю...' : 'Ответить и продолжить'}
            </button>
          </form>
        )}

        {visibleChanges.length > 0 && (
          <ChangeSummaryDock
            changes={visibleChanges}
            summary={changeSummary}
            pendingCount={pendingChanges.length}
            isOpen={changesDockOpen}
            expandedDiffIds={expandedDiffIds}
            applyingChanges={applyingChanges}
            onToggleOpen={() => setChangesDockOpen((value) => !value)}
            onToggleDiff={toggleDiff}
            onApplyChanges={handleApplyChanges}
          />
        )}

        <form className="composer" onSubmit={handleSendMessage}>
          <textarea
            value={messageInput}
            onChange={(event) => setMessageInput(event.target.value)}
            onKeyDown={handleMessageKeyDown}
            placeholder={clarification ? 'Сначала ответь на уточнение выше' : selectedProjectId ? 'Опиши задачу для AI-завода' : 'Сначала выбери проект'}
            disabled={!selectedProjectId || sending || Boolean(clarification)}
          />
          <button className="send-button" type="submit" disabled={!messageInput.trim() || !selectedProjectId || sending || Boolean(clarification)}>
            Отправить
          </button>
        </form>
      </section>

      <aside className="sidebar agents-panel">
        <section className="right-section artifacts-section">
          <div className="panel-heading compact">
            <div>
              <h2>Контракт задачи</h2>
            </div>
            {blueprint && (
              <span className={`review-status ${blueprint.confidence}`}>
                {blueprintConfidenceLabel(blueprint.confidence)}
              </span>
            )}
          </div>
          {blueprint ? (
            <div className="blueprint-card">
              <div className="blueprint-grid">
                <span>Стек</span>
                <strong>{blueprint.stack}</strong>
                <span>Runtime</span>
                <strong>{blueprint.runtime || 'не указан'}</strong>
                <span>Тип</span>
                <strong>{blueprint.projectType}</strong>
                <span>Scaffold</span>
                <strong>{blueprint.scaffoldRequired ? 'нужен' : 'не нужен'}</strong>
              </div>
              {blueprint.entrypoints?.length > 0 && (
                <p className="blueprint-line">Entrypoint: <code>{blueprint.entrypoints.join(', ')}</code></p>
              )}
              {blueprint.expectedFiles?.length > 0 && (
                <div className="blueprint-list">
                  <strong>Файлы</strong>
                  {blueprint.expectedFiles.slice(0, 5).map((file) => (
                    <p key={file.path}><code>{file.path}</code> · {file.action}</p>
                  ))}
                </div>
              )}
              {blueprint.openQuestions?.length > 0 && (
                <div className="blueprint-list warning">
                  <strong>Вопросы</strong>
                  {blueprint.openQuestions.map((question) => <p key={question}>{question}</p>)}
                </div>
              )}
            </div>
          ) : (
            <p className="panel-note">После постановки задачи здесь появится стек, scaffold и ожидаемые файлы.</p>
          )}
        </section>
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
                <section className="settings-section model-routing-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Назначение моделей</h3>
                      <p className="muted">Сейчас выбранная модель применяется ко всему пайпу.</p>
                    </div>
                  </div>
                  <div className="model-routing-control" aria-label="Режим назначения моделей">
                    <button className="active" type="button">
                      Одна модель на пайп
                    </button>
                    <button type="button" disabled title="Появится в следующем шаге">
                      По ролям
                    </button>
                  </div>
                </section>

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
  run?: WorkflowRun | null;
  steps: WorkflowStep[];
};

type ChangeSummary = {
  fileCount: number;
  added: number;
  removed: number;
};

type DisplayChange = ProposedChange & {
  displayAction: string;
  revisionCount: number;
};

type AgentInfo = {
  title: string;
  why: string;
  responsibility: string;
};

function AgentStrip({
  agents,
  run,
  steps,
  activeModel,
}: {
  agents: AgentStatus[];
  run?: WorkflowRun | null;
  steps: WorkflowStep[];
  activeModel?: ModelConfig | null;
}) {
  if (agents.length === 0) {
    return null;
  }

  const activeAgent = activeAgentForDisplay(agents);
  const activeStatus = statusLabels[activeAgent.status] ?? activeAgent.status;
  const activeActivity = activeAgent.activity || defaultAgentActivity(activeAgent);
  const activeAgentInfo = agentInfoById(activeAgent.id);

  return (
    <div className="agent-activity-panel with-workflow" aria-label="Активность агентов">
      <div className={`active-agent ${activeAgent.status}`} tabIndex={0} aria-label={`Активный агент: ${activeAgent.name || agentNameById(activeAgent.id)}`}>
        <img
          className="active-agent-avatar"
          src={avatarForAgent(activeAgent.id)}
          alt=""
          aria-hidden="true"
        />
        <div className="active-agent-body">
          <div className="active-agent-title">
            <strong>{activeAgent.name || agentNameById(activeAgent.id)}</strong>
            <span
              className={`active-agent-status ${activeAgent.status}`}
              title={`${activeStatus}. ${activeActivity}`}
            >
              {activeStatus}
            </span>
          </div>
          <p>{activeActivity}</p>
        </div>
        <div className="agent-popover" role="tooltip">
          <div className="agent-popover-header">
            <img src={avatarForAgent(activeAgent.id)} alt="" aria-hidden="true" />
            <div>
              <strong>{activeAgent.name || agentNameById(activeAgent.id)}</strong>
              <span>{activeAgentInfo.title}</span>
            </div>
            <span className={`active-agent-status ${activeAgent.status}`}>
              {activeStatus}
            </span>
          </div>
          <dl className="agent-popover-grid">
            <dt>Зачем</dt>
            <dd>{activeAgentInfo.why}</dd>
            <dt>Отвечает</dt>
            <dd>{activeAgentInfo.responsibility}</dd>
            <dt>Сейчас</dt>
            <dd>{activeActivity}</dd>
          </dl>
        </div>
      </div>
      <WorkflowProgress agents={agents} run={run} steps={steps} />
      <ActiveModelCard model={activeModel} />
    </div>
  );
}

function ActiveModelCard({ model }: { model?: ModelConfig | null }) {
  return (
    <div className="active-model-card header-model" tabIndex={0} aria-label="Активная модель пайпа">
      <strong>{model?.name ?? 'не выбрана'}</strong>
      <span>{model ? `${providerLabels[model.provider] ?? model.provider} · ${model.modelName}` : 'Открой настройки LLM'}</span>
      {model && (
        <div className="model-meta-row">
          <span className={`model-status ${model.status || 'unknown'}`}>
            {modelStatusLabels[model.status || 'unknown'] ?? model.status}
          </span>
          {model.latencyMs > 0 && <small>{model.latencyMs} мс</small>}
        </div>
      )}
      {model && (
        <div className="model-popover" role="tooltip">
          <div className="model-popover-header">
            <strong>{model.name}</strong>
            <span className={`model-status ${model.status || 'unknown'}`}>
              {modelStatusLabels[model.status || 'unknown'] ?? model.status}
            </span>
          </div>
          <dl className="model-popover-grid">
            <dt>Provider</dt>
            <dd>{providerLabels[model.provider] ?? model.provider}</dd>
            <dt>Model</dt>
            <dd>{model.modelName || 'не указана'}</dd>
            <dt>Base URL</dt>
            <dd>{model.baseUrl || 'не указан'}</dd>
            <dt>Задержка</dt>
            <dd>{model.latencyMs > 0 ? `${model.latencyMs} мс` : 'нет данных'}</dd>
            <dt>Проверка</dt>
            <dd>{formatDateTime(model.lastCheckedAt) || 'еще не проверялась'}</dd>
          </dl>
          {model.lastError && <p className="model-health-error">{model.lastError}</p>}
        </div>
      )}
    </div>
  );
}

function ChangeSummaryDock({
  changes,
  summary,
  pendingCount,
  isOpen,
  expandedDiffIds,
  applyingChanges,
  onToggleOpen,
  onToggleDiff,
  onApplyChanges,
}: {
  changes: DisplayChange[];
  summary: ChangeSummary;
  pendingCount: number;
  isOpen: boolean;
  expandedDiffIds: string[];
  applyingChanges: boolean;
  onToggleOpen: () => void;
  onToggleDiff: (id: string) => void;
  onApplyChanges: () => void;
}) {
  return (
    <section className="changes-dock" aria-label="Изменения файлов">
      {isOpen && (
        <div className="changes-dock-popover">
          <div className="changes-dock-header">
            <strong>{changesTitle(summary.fileCount)}</strong>
            <span>
              <span className="diff-added">+{summary.added}</span>
              <span className="diff-removed"> -{summary.removed}</span>
            </span>
          </div>
          <div className="changes-dock-list">
            {changes.map((change) => {
              const stats = diffStats(change.diffText);
              const expanded = expandedDiffIds.includes(change.id);
              return (
                <div key={change.id} className={`changes-dock-file ${change.status}`}>
                  <button className="changes-dock-file-row" type="button" onClick={() => onToggleDiff(change.id)}>
                    <span>{change.filePath}</span>
                    <span>
                      <span className="diff-added">+{stats.added}</span>
                      <span className="diff-removed"> -{stats.removed}</span>
                    </span>
                  </button>
                  <div className="changes-dock-file-meta">
                    <span>{changeActionLabels[change.displayAction] ?? change.displayAction}</span>
                    <span className={`change-status ${change.status}`}>
                      {changeStatusLabels[change.status] ?? change.status}
                    </span>
                    {change.revisionCount > 1 && <span>{change.revisionCount} версии</span>}
                  </div>
                  {change.reason && <p>{change.reason}</p>}
                  {change.error && <p className="change-error">{change.error}</p>}
                  {expanded && <DiffViewer diffText={change.diffText || 'Изменений нет'} />}
                </div>
              );
            })}
          </div>
          {pendingCount > 0 && (
            <button className="apply-changes-button dock-apply" type="button" disabled={applyingChanges} onClick={onApplyChanges}>
              {applyingChanges ? 'Применяю...' : `Применить ${pendingCount}`}
            </button>
          )}
        </div>
      )}
      <button className={`changes-dock-pill ${isOpen ? 'active' : ''}`} type="button" onClick={onToggleOpen}>
        <span>{changesTitle(summary.fileCount)}</span>
        <span>
          <span className="diff-added">+{summary.added}</span>
          <span className="diff-removed"> -{summary.removed}</span>
        </span>
      </button>
    </section>
  );
}

function DiffViewer({ diffText }: { diffText: string }) {
  return (
    <pre className="diff-viewer">
      {diffText.split('\n').map((line, index) => (
        <span key={index} className={`diff-line ${diffLineKind(line)}`}>
          {line || ' '}
        </span>
      ))}
    </pre>
  );
}

function activeAgentForDisplay(agents: AgentStatus[]): AgentStatus {
  const workingStatuses = ['calling_model', 'thinking', 'answering', 'writing_files', 'running'];
  const attentionStatuses = ['waiting_user', 'needs_work', 'blocked', 'failed'];
  const doneWithActivity = agents.find((agent) => {
    return agent.status === 'done' && agent.activity && agent.activity !== 'Ждет задачу';
  });

  return (
    agents.find((agent) => workingStatuses.includes(agent.status)) ??
    agents.find((agent) => attentionStatuses.includes(agent.status)) ??
    doneWithActivity ??
    agents.find((agent) => agent.id === 'manager') ??
    agents[0]
  );
}

function defaultAgentActivity(agent: AgentStatus): string {
  if (agent.id === 'manager') {
    return agent.status === 'idle' ? 'Готова принять задачу' : 'Держит входной контур';
  }
  if (agent.status === 'idle') {
    return 'Ждет свою часть работы';
  }
  return statusLabels[agent.status] ?? agent.status;
}

function WorkflowProgress({ agents, run, steps }: WorkflowProgressProps) {
  const stepOrder = workflowOrderFor(run, steps);
  const completedCount = stepOrder.filter((stepKey) => {
    return steps.find((item) => item.stepKey === stepKey)?.status === 'done';
  }).length;
  const totalCount = stepOrder.length;
  const runStatus = run?.status ?? 'idle';
  const activeStepIndex = stepOrder.findIndex((stepKey) => stepKey === run?.currentStep);
  const displayIndex = run ? (run.status === 'done' ? totalCount : Math.max(1, activeStepIndex + 1 || completedCount + 1)) : 0;
  const currentStepKey = run
    ? activeStepIndex >= 0
      ? stepOrder[activeStepIndex]
      : stepOrder[Math.min(displayIndex - 1, totalCount - 1)]
    : stepOrder[0];
  const currentStep = steps.find((item) => item.stepKey === currentStepKey);
  const currentAgent = currentStep ? agents.find((item) => item.id === currentStep.agentId) : null;
  const runStatusText = workflowRunStatusText(runStatus, completedCount, totalCount);
  const currentAgentName = run ? currentAgent?.name ?? agentNameById(currentStep?.agentId ?? agentForStep(currentStepKey)) : 'Люмен';
  const currentStepTitle = run ? workflowStepLabels[currentStepKey] ?? currentStepKey : 'Ожидает задачу';
  const currentStepLine = run ? `${currentAgentName} · ${workflowStepLabels[currentStepKey] ?? currentStepKey}` : 'Пайплайн готов к запуску';

  return (
    <section className={`workflow-compact ${runStatus}`} tabIndex={0} aria-label="Текущий шаг workflow">
      <div className="workflow-compact-main">
        <div className="workflow-compact-head">
          <span>Ход работы</span>
          <span className={`workflow-status ${runStatus}`}>{runStatusText}</span>
        </div>
        <div className="workflow-compact-title">
          <span className="workflow-compact-index">{displayIndex}/{totalCount}</span>
          <strong>{currentStepTitle}</strong>
        </div>
        <p className={run?.error ? 'workflow-step-error' : ''}>{run?.error || currentStepLine}</p>
      </div>
      <div className="workflow-popover" role="tooltip">
        <div className="workflow-popover-header">
          <strong>Ход работы</strong>
          <span>{displayIndex}/{totalCount}</span>
        </div>
        <div className="workflow-popover-steps">
          {stepOrder.map((stepKey, index) => {
          const step = steps.find((item) => item.stepKey === stepKey);
          const agent = step ? agents.find((item) => item.id === step.agentId) : null;
          const status = step?.status ?? (run?.currentStep === stepKey ? 'running' : 'queued');
          const isActive = run?.currentStep === stepKey && run.status === 'running';
          return (
            <div key={stepKey} className={`workflow-step ${status} ${isActive ? 'active' : ''}`}>
              <span className="workflow-step-index">{status === 'done' ? '✓' : index + 1}</span>
              <div className="workflow-step-body">
                <div className="workflow-step-title">
                  <strong>{workflowStepLabels[stepKey] ?? stepKey}</strong>
                  <span className={`workflow-step-status ${status}`}>
                    {statusLabels[status] ?? status}
                  </span>
                </div>
                <p>{agent?.name ?? agentNameById(step?.agentId ?? agentForStep(stepKey))}</p>
                {step?.output && <small>{workflowStepPreview(step, stepKey)}</small>}
                {step?.error && <small className="workflow-step-error">{step.error}</small>}
              </div>
            </div>
          );
        })}
      </div>
      <p className="workflow-popover-current">
        {run ? `Сейчас: ${currentAgentName} · ${workflowStepLabels[currentStepKey] ?? currentStepKey}` : 'Сейчас: пайплайн не запущен'}
      </p>
      </div>
    </section>
  );
}

function isRoutinePipelineMessage(message: Message): boolean {
  if (message.role !== 'agent') {
    return false;
  }
  const content = message.content.trim();
  return (
    content.startsWith('## Изменения применены автоматически') ||
    content.startsWith('## Применение изменений') ||
    content.startsWith('## Проверки') ||
    content.startsWith('## Результат проверки') ||
    content.startsWith('## Ревью')
  );
}

function workflowRunStatusText(status: string, completedCount: number, totalCount: number): string {
  if (status === 'idle') {
    return 'не запущен';
  }
  if (status === 'done') {
    return `готово ${completedCount}/${totalCount}`;
  }
  if (status === 'waiting_user') {
    return 'уточнение';
  }
  if (status === 'failed') {
    return 'остановлен';
  }
  if (status === 'blocked') {
    return 'вмешательство';
  }
  return `в работе ${completedCount}/${totalCount}`;
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
  if (stepKey === 'security_analysis') {
    return 'security';
  }
  if (stepKey === 'product_requirements') {
    return 'product';
  }
  if (stepKey === 'architect_plan') {
    return 'architect';
  }
  if (stepKey === 'task_blueprint') {
    return 'architect';
  }
  if (stepKey === 'developer_plan') {
    return 'developer';
  }
  if (stepKey === 'tester_commands') {
    return 'tester';
  }
  if (stepKey === 'review') {
    return 'reviewer';
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
  if (agentId === 'developer') {
    return 'Разработчик';
  }
  if (agentId === 'tester') {
    return 'Тестировщик';
  }
  if (agentId === 'reviewer') {
    return 'Ревьюер';
  }
  if (agentId === 'security') {
    return 'ИБ-специалист';
  }
  return 'Люмен';
}

function avatarForAgent(agentId: string): string {
  if (agentId === 'security') {
    return securityAvatar;
  }
  if (agentId === 'product') {
    return productAvatar;
  }
  if (agentId === 'architect') {
    return architectAvatar;
  }
  if (agentId === 'developer') {
    return developerAvatar;
  }
  if (agentId === 'tester') {
    return testerAvatar;
  }
  if (agentId === 'reviewer') {
    return reviewerAvatar;
  }
  return managerAvatar;
}

function agentInfoById(agentId: string): AgentInfo {
  if (agentId === 'product') {
    return {
      title: 'формулирует требования',
      why: 'Чтобы задача стала понятной и проверяемой до разработки.',
      responsibility: 'Пользовательские сценарии, ограничения, критерии готовности и границы задачи.',
    };
  }
  if (agentId === 'architect') {
    return {
      title: 'проектирует решение',
      why: 'Чтобы разработчик не гадал про стек, файлы и порядок изменений.',
      responsibility: 'Task Blueprint, архитектурный план, scaffold, риски и технические решения.',
    };
  }
  if (agentId === 'developer') {
    return {
      title: 'пишет изменения',
      why: 'Чтобы превратить требования и blueprint в реальные файлы проекта.',
      responsibility: 'Кодовые изменения, go.mod/package files, тестовые файлы и исправления по ревью.',
    };
  }
  if (agentId === 'tester') {
    return {
      title: 'проверяет результат',
      why: 'Чтобы поймать ошибки сборки и регрессии до финального ответа.',
      responsibility: 'Безопасные команды проверки, allowlist, stdout/stderr и итог статуса тестов.',
    };
  }
  if (agentId === 'reviewer') {
    return {
      title: 'контролирует качество',
      why: 'Чтобы не принимать недоделанный код после разработки и тестов.',
      responsibility: 'Diff, результаты проверок, соответствие требованиям и возврат на доработку.',
    };
  }
  if (agentId === 'security') {
    return {
      title: 'разбирает ИБ-задачи',
      why: 'Чтобы security и pentest-запросы проходили через отдельный защитный контур.',
      responsibility: 'Scope, риски, threat model, безопасные проверки и remediation plan.',
    };
  }
  return {
    title: 'входной контур завода',
    why: 'Чтобы понять намерение пользователя и выбрать: ответить сразу или запустить пайплайн.',
    responsibility: 'Прием задачи, уточнения, маршрутизация, итоговый ответ и связь между ролями.',
  };
}

function workflowOrderFor(run: WorkflowRun | null | undefined, steps: WorkflowStep[]): string[] {
  if (run?.currentStep === 'security_analysis' || steps.some((step) => step.stepKey === 'security_analysis')) {
    return securityWorkflowStepOrder;
  }
  return workflowStepOrder;
}

type MarkdownBlock =
  | { type: 'heading'; level: number; text: string }
  | { type: 'paragraph'; text: string }
  | { type: 'unorderedList'; items: string[] }
  | { type: 'orderedList'; items: string[] }
  | { type: 'code'; language: string; code: string };

function MarkdownContent({ content }: { content: string }) {
  const blocks = parseMarkdown(content);

  return (
    <div className="message-markdown">
      {blocks.map((block, index) => {
        if (block.type === 'heading') {
          const HeadingTag = `h${Math.min(Math.max(block.level, 2), 4)}` as 'h2' | 'h3' | 'h4';
          return <HeadingTag key={index}>{renderInlineMarkdown(block.text)}</HeadingTag>;
        }
        if (block.type === 'unorderedList') {
          return (
            <ul key={index}>
              {block.items.map((item, itemIndex) => (
                <li key={itemIndex}>{renderInlineMarkdown(item)}</li>
              ))}
            </ul>
          );
        }
        if (block.type === 'orderedList') {
          return (
            <ol key={index}>
              {block.items.map((item, itemIndex) => (
                <li key={itemIndex}>{renderInlineMarkdown(item)}</li>
              ))}
            </ol>
          );
        }
        if (block.type === 'code') {
          return (
            <pre key={index} className="markdown-code-block">
              <code>{block.code}</code>
            </pre>
          );
        }
        return (
          <p key={index}>
            {block.text.split('\n').map((line, lineIndex) => (
              <FragmentWithBreak key={lineIndex} needsBreak={lineIndex > 0}>
                {renderInlineMarkdown(line)}
              </FragmentWithBreak>
            ))}
          </p>
        );
      })}
    </div>
  );
}

function FragmentWithBreak({ children, needsBreak }: { children: ReactNode; needsBreak: boolean }) {
  return (
    <>
      {needsBreak && <br />}
      {children}
    </>
  );
}

function parseMarkdown(content: string): MarkdownBlock[] {
  const lines = content.replace(/\r\n/g, '\n').split('\n');
  const blocks: MarkdownBlock[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index];
    if (line.trim() === '') {
      index++;
      continue;
    }

    const fence = line.match(/^```([A-Za-z0-9_-]*)\s*$/);
    if (fence) {
      const language = fence[1] ?? '';
      const codeLines: string[] = [];
      index++;
      while (index < lines.length && !lines[index].match(/^```\s*$/)) {
        codeLines.push(lines[index]);
        index++;
      }
      if (index < lines.length) {
        index++;
      }
      blocks.push({ type: 'code', language, code: codeLines.join('\n') });
      continue;
    }

    const heading = line.match(/^(#{1,4})\s+(.+)$/);
    if (heading) {
      blocks.push({ type: 'heading', level: heading[1].length, text: heading[2].trim() });
      index++;
      continue;
    }

    if (/^\s*[-*]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length && /^\s*[-*]\s+/.test(lines[index])) {
        items.push(lines[index].replace(/^\s*[-*]\s+/, '').trim());
        index++;
      }
      blocks.push({ type: 'unorderedList', items });
      continue;
    }

    if (/^\s*\d+[.)]\s+/.test(line)) {
      const items: string[] = [];
      while (index < lines.length && /^\s*\d+[.)]\s+/.test(lines[index])) {
        items.push(lines[index].replace(/^\s*\d+[.)]\s+/, '').trim());
        index++;
      }
      blocks.push({ type: 'orderedList', items });
      continue;
    }

    const paragraphLines: string[] = [];
    while (
      index < lines.length &&
      lines[index].trim() !== '' &&
      !lines[index].match(/^```/) &&
      !lines[index].match(/^(#{1,4})\s+/) &&
      !/^\s*[-*]\s+/.test(lines[index]) &&
      !/^\s*\d+[.)]\s+/.test(lines[index])
    ) {
      paragraphLines.push(lines[index]);
      index++;
    }
    blocks.push({ type: 'paragraph', text: paragraphLines.join('\n') });
  }

  return blocks;
}

function renderInlineMarkdown(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const parts = text.split(/(`[^`]+`)/g);
  parts.forEach((part, index) => {
    if (part.startsWith('`') && part.endsWith('`') && part.length > 1) {
      nodes.push(<code key={index}>{part.slice(1, -1)}</code>);
      return;
    }
    nodes.push(...renderBoldMarkdown(part, `text-${index}`));
  });
  return nodes;
}

function renderBoldMarkdown(text: string, keyPrefix: string): ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return parts.map((part, index) => {
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
      return <strong key={`${keyPrefix}-${index}`}>{part.slice(2, -2)}</strong>;
    }
    return part;
  });
}

function shortPreview(value: string): string {
  const compact = value.replace(/\s+/g, ' ').trim();
  if (compact.length <= 140) {
    return compact;
  }
  return `${compact.slice(0, 140)}...`;
}

function summarizeChanges(items: DisplayChange[]): ChangeSummary {
  return items.reduce(
    (summary, item) => {
      const stats = diffStats(item.diffText);
      summary.fileCount += 1;
      summary.added += stats.added;
      summary.removed += stats.removed;
      return summary;
    },
    { fileCount: 0, added: 0, removed: 0 },
  );
}

function aggregateWorkflowChanges(items: ProposedChange[]): DisplayChange[] {
  const byPath = new Map<string, ProposedChange[]>();
  const ordered = [...items].sort(compareChangesByTime);
  for (const item of ordered) {
    const key = item.filePath.trim().toLowerCase();
    if (!key) {
      continue;
    }
    byPath.set(key, [...(byPath.get(key) ?? []), item]);
  }
  return Array.from(byPath.values()).map(buildDisplayChange);
}

function buildDisplayChange(group: ProposedChange[]): DisplayChange {
  const materialChanges = group.filter((change) => change.status !== 'failed');
  const first = materialChanges[0] ?? group[0];
  const latest = materialChanges[materialChanges.length - 1] ?? group[group.length - 1];
  const status = group[group.length - 1]?.status ?? latest.status;
  const beforeContent = first.beforeContent ?? '';
  const afterContent = firstNonEmptyString(latest.afterContent, latest.content);
  const displayAction = first.action === 'create' ? 'create' : latest.action;
  const diffText = buildDisplayDiff(latest.filePath, beforeContent, afterContent, latest.diffText);

  return {
    ...latest,
    id: `display:${latest.workflowRunId}:${latest.filePath}`,
    status,
    beforeContent,
    afterContent,
    diffText,
    displayAction,
    revisionCount: group.length,
    reason: summarizeChangeReason(first, latest, group.length),
  };
}

function compareChangesByTime(a: ProposedChange, b: ProposedChange): number {
  return changeTime(a) - changeTime(b);
}

function changeTime(change: ProposedChange): number {
  const raw = change.appliedAt || change.createdAt;
  const value = new Date(raw).getTime();
  return Number.isNaN(value) ? 0 : value;
}

function summarizeChangeReason(first: ProposedChange, latest: ProposedChange, revisionCount: number): string {
  const reason = first.reason || latest.reason;
  if (revisionCount <= 1) {
    return reason;
  }
  if (!reason) {
    return `${revisionCount} версии в ходе workflow`;
  }
  return `${reason} · ${revisionCount} версии`;
}

function buildDisplayDiff(filePath: string, beforeContent: string, afterContent: string, fallbackDiff: string): string {
  if (beforeContent === afterContent) {
    return fallbackDiff;
  }
  const beforeLines = splitDiffLines(beforeContent);
  const afterLines = splitDiffLines(afterContent);
  if (beforeLines.length === 0) {
    return unifiedDiffHeader('/dev/null', filePath) + afterLines.map((line) => `+${line}`).join('\n');
  }
  if (afterLines.length === 0) {
    return unifiedDiffHeader(filePath, '/dev/null') + beforeLines.map((line) => `-${line}`).join('\n');
  }
  if (beforeLines.length * afterLines.length > 250000) {
    return fallbackDiff || unifiedDiffHeader(filePath, filePath) + `@@\n-${beforeLines.length} строк до\n+${afterLines.length} строк после`;
  }
  return unifiedDiffHeader(filePath, filePath) + diffLineOps(beforeLines, afterLines).join('\n');
}

function unifiedDiffHeader(beforePath: string, afterPath: string): string {
  return `--- ${beforePath}\n+++ ${afterPath}\n@@\n`;
}

function splitDiffLines(content: string): string[] {
  if (!content) {
    return [];
  }
  return content.replace(/\r\n/g, '\n').replace(/\n$/, '').split('\n');
}

function diffLineOps(beforeLines: string[], afterLines: string[]): string[] {
  const table: number[][] = Array.from({ length: beforeLines.length + 1 }, () => Array(afterLines.length + 1).fill(0));
  for (let i = beforeLines.length - 1; i >= 0; i--) {
    for (let j = afterLines.length - 1; j >= 0; j--) {
      table[i][j] = beforeLines[i] === afterLines[j] ? table[i + 1][j + 1] + 1 : Math.max(table[i + 1][j], table[i][j + 1]);
    }
  }

  const result: string[] = [];
  let i = 0;
  let j = 0;
  while (i < beforeLines.length && j < afterLines.length) {
    if (beforeLines[i] === afterLines[j]) {
      result.push(` ${beforeLines[i]}`);
      i++;
      j++;
    } else if (table[i + 1][j] >= table[i][j + 1]) {
      result.push(`-${beforeLines[i]}`);
      i++;
    } else {
      result.push(`+${afterLines[j]}`);
      j++;
    }
  }
  while (i < beforeLines.length) {
    result.push(`-${beforeLines[i]}`);
    i++;
  }
  while (j < afterLines.length) {
    result.push(`+${afterLines[j]}`);
    j++;
  }
  return result;
}

function diffStats(diffText: string): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  diffText.split('\n').forEach((line) => {
    if (line.startsWith('+++') || line.startsWith('---')) {
      return;
    }
    if (line.startsWith('+')) {
      added += 1;
    } else if (line.startsWith('-')) {
      removed += 1;
    }
  });
  return { added, removed };
}

function diffLineKind(line: string): string {
  if (line.startsWith('+++') || line.startsWith('---')) {
    return 'file';
  }
  if (line.startsWith('@@')) {
    return 'hunk';
  }
  if (line.startsWith('+')) {
    return 'added';
  }
  if (line.startsWith('-')) {
    return 'removed';
  }
  return 'context';
}

function firstNonEmptyString(...values: string[]): string {
  for (const value of values) {
    if (value) {
      return value;
    }
  }
  return '';
}

function changesTitle(count: number): string {
  if (count % 10 === 1 && count % 100 !== 11) {
    return `Изменен ${count} файл`;
  }
  if ([2, 3, 4].includes(count % 10) && ![12, 13, 14].includes(count % 100)) {
    return `Изменено ${count} файла`;
  }
  return `Изменено ${count} файлов`;
}

function blueprintConfidenceLabel(value: string): string {
  switch (value) {
    case 'high':
      return 'уверенность высокая';
    case 'medium':
      return 'уверенность средняя';
    case 'low':
      return 'уверенность низкая';
    default:
      return value || 'уверенность не указана';
  }
}

function workflowStepPreview(step: WorkflowStep, stepKey: string): string {
  const structured = parseStructuredPreview(step.output);
  if (structured) {
    if (stepKey === 'task_blueprint') {
      const parts = [structured.stack, structured.runtime, structured.project_type].filter(Boolean);
      const expectedFiles = Array.isArray(structured.expected_files) ? structured.expected_files.length : 0;
      if (expectedFiles > 0) {
        parts.push(`файлов: ${expectedFiles}`);
      }
      return parts.length > 0 ? shortPreview(parts.join(' · ')) : 'Blueprint готов';
    }
    if (stepKey === 'tester_commands') {
      const commands = Array.isArray(structured.commands) ? structured.commands.length : 0;
      const summary = textValue(structured.summary) || 'Проверки подготовлены';
      return commands > 0 ? shortPreview(`${summary} · команд: ${commands}`) : shortPreview(summary);
    }
    if (stepKey === 'review') {
      const status = textValue(structured.status);
      const summary = textValue(structured.summary) || 'Ревью готово';
      const label = status ? reviewStatusLabels[status] ?? status : '';
      return shortPreview([label, summary].filter(Boolean).join(' · '));
    }
    const summary = textValue(structured.summary) || textValue(structured.goal) || textValue(structured.recommended_next_step);
    if (summary) {
      return shortPreview(summary);
    }
    return 'Структурный ответ сохранен';
  }
  return shortPreview(cleanMarkdownPreview(step.output));
}

function parseStructuredPreview(value: string): Record<string, unknown> | null {
  const trimmed = stripMarkdownFence(value).trim();
  if (!trimmed.startsWith('{')) {
    return null;
  }
  try {
    const parsed = JSON.parse(trimmed);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return null;
  }
  return null;
}

function stripMarkdownFence(value: string): string {
  let trimmed = value.trim();
  if (trimmed.startsWith('```json')) {
    trimmed = trimmed.slice('```json'.length);
  } else if (trimmed.startsWith('```')) {
    trimmed = trimmed.slice('```'.length);
  }
  if (trimmed.endsWith('```')) {
    trimmed = trimmed.slice(0, -3);
  }
  return trimmed.trim();
}

function cleanMarkdownPreview(value: string): string {
  return value
    .replace(/^#{1,6}\s+/gm, '')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/^\s*[-*]\s+/gm, '')
    .trim();
}

function textValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
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

function formatDateTime(value: string): string {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
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

async function copyToClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }

  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', 'true');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand('copy');
  document.body.removeChild(textarea);
  if (!copied) {
    throw new Error('clipboard API недоступен');
  }
}

export default App;
