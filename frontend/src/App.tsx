import { FormEvent, KeyboardEvent, MouseEvent, ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import {
  AgentGroup,
  AgentGroupTemplate,
  AgentLibraryItem,
  AgentProfile,
  AgentSoul,
  AgentStatus,
  AgentMessageDelta,
  AppPaths,
  ChatState,
  CTFWorkspace,
  CTFWorkspaceFile,
  CTFWorkspaceSection,
  LifecycleDefinition,
  LifecycleRuntimeIssue,
  LifecycleStep,
  Message,
  ModelConfig,
  PendingClarification,
  ProposedChange,
  Project,
  ProjectGroupBinding,
  ProjectState,
  TaskBlueprint,
  WebSource,
  WebSettings,
  WorkflowRun,
  WorkflowPlan,
  WorkflowPlanStep,
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
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';

const statusLabels: Record<string, string> = {
  idle: 'свободен',
  thinking: 'думает',
  calling_model: 'вызывает модель',
  answering: 'пишет ответ',
  searching_web: 'ищет',
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
  web_research: 'Поиск в сети',
  source_review: 'Источники',
  research_synthesis: 'Аналитика',
  research_notes: 'Research notes',
  intake: 'Постановка CTF',
  scope_check: 'Scope',
  artifact_collection: 'Артефакты',
  triage: 'Категория',
  hypothesis_board: 'Гипотезы',
  category_solver: 'Решение',
  validation: 'Проверка flag',
  writeup: 'Writeup',
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
const researchWorkflowStepOrder = ['web_research', 'source_review', 'research_synthesis', 'research_notes', 'manager_final'];
const ctfWorkflowStepOrder = [
  'intake',
  'scope_check',
  'artifact_collection',
  'triage',
  'hypothesis_board',
  'category_solver',
  'validation',
  'writeup',
];

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
  rolled_back: 'откатано',
};

const changeActionLabels: Record<string, string> = {
  create: 'создать',
  replace: 'заменить',
};

function listToLines(items: string[] = []): string {
  return items.join('\n');
}

function linesToList(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split('\n')
    .map((item) => item.trim())
    .filter((item) => {
      if (!item) {
        return false;
      }
      const key = item.toLowerCase();
      if (seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    });
}

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

const defaultWebSettings: WebSettings = {
  enabled: true,
  maxResults: 5,
  maxPagesPerWorkflow: 8,
  timeoutSeconds: 8,
  allowedDomains: [],
  blockedDomains: [],
};

const lifecycleModes = ['llm', 'tool', 'checks', 'review', 'artifact', 'final', 'human_gate', 'branch', 'parallel', 'join'];

type SettingsTab = 'projects' | 'groups' | 'models' | 'web';

function App() {
  const [paths, setPaths] = useState<AppPaths | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState('');
  const [currentProject, setCurrentProject] = useState<Project | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [workflowRun, setWorkflowRun] = useState<WorkflowRun | null>(null);
  const [workflowSteps, setWorkflowSteps] = useState<WorkflowStep[]>([]);
  const [workflowPlan, setWorkflowPlan] = useState<WorkflowPlan | null>(null);
  const [planSteps, setPlanSteps] = useState<WorkflowPlanStep[]>([]);
  const [blueprint, setBlueprint] = useState<TaskBlueprint | null>(null);
  const [clarification, setClarification] = useState<PendingClarification | null>(null);
  const [changes, setChanges] = useState<ProposedChange[]>([]);
  const [webSources, setWebSources] = useState<WebSource[]>([]);
  const [ctfWorkspace, setCTFWorkspace] = useState<CTFWorkspace | null>(null);
  const [agents, setAgents] = useState<AgentStatus[]>([]);
  const [agentGroups, setAgentGroups] = useState<AgentGroup[]>([]);
  const [agentGroupTemplates, setAgentGroupTemplates] = useState<AgentGroupTemplate[]>([]);
  const [agentLibrary, setAgentLibrary] = useState<AgentLibraryItem[]>([]);
  const [currentAgentGroup, setCurrentAgentGroup] = useState<AgentGroup | null>(null);
  const [groupBinding, setGroupBinding] = useState<ProjectGroupBinding | null>(null);
  const [selectedGroupEditorId, setSelectedGroupEditorId] = useState('');
  const [groupProfiles, setGroupProfiles] = useState<AgentProfile[]>([]);
  const [groupLifecycles, setGroupLifecycles] = useState<LifecycleDefinition[]>([]);
  const [lifecycleSteps, setLifecycleSteps] = useState<LifecycleStep[]>([]);
  const [lifecycleRuntimeIssues, setLifecycleRuntimeIssues] = useState<LifecycleRuntimeIssue[]>([]);
  const [selectedLifecycleId, setSelectedLifecycleId] = useState('');
  const [lifecycleForm, setLifecycleForm] = useState<LifecycleDefinition | null>(null);
  const [lifecycleStepForm, setLifecycleStepForm] = useState<LifecycleStep | null>(null);
  const [models, setModels] = useState<ModelConfig[]>([]);
  const [activeModelId, setActiveModelId] = useState('');
  const [webSettings, setWebSettings] = useState<WebSettings>(defaultWebSettings);
  const [savingWebSettings, setSavingWebSettings] = useState(false);
  const [projectQuery, setProjectQuery] = useState('');
  const [newProjectName, setNewProjectName] = useState('');
  const [newProjectGroupId, setNewProjectGroupId] = useState('');
  const [existingProjectName, setExistingProjectName] = useState('');
  const [existingProjectPath, setExistingProjectPath] = useState('');
  const [existingProjectGroupId, setExistingProjectGroupId] = useState('');
  const [editingProjectId, setEditingProjectId] = useState('');
  const [editingProjectName, setEditingProjectName] = useState('');
  const [editingProjectPath, setEditingProjectPath] = useState('');
  const [confirmDeleteProjectId, setConfirmDeleteProjectId] = useState('');
  const [showNewProject, setShowNewProject] = useState(false);
  const [showExistingProject, setShowExistingProject] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>('projects');
  const [groupForm, setGroupForm] = useState({
    id: '',
    name: '',
    kind: 'custom',
    description: '',
    defaultModelId: '',
  });
  const [agentForm, setAgentForm] = useState<AgentProfile | null>(null);
  const [soulEditor, setSoulEditor] = useState<AgentSoul | null>(null);
  const [savingSoul, setSavingSoul] = useState(false);
  const [savingGroup, setSavingGroup] = useState(false);
  const [savingAgent, setSavingAgent] = useState(false);
  const [libraryTargetProfileId, setLibraryTargetProfileId] = useState('');
  const [addingLibraryAgentId, setAddingLibraryAgentId] = useState('');
  const [savingLifecycle, setSavingLifecycle] = useState(false);
  const [savingLifecycleStep, setSavingLifecycleStep] = useState(false);
  const [messageInput, setMessageInput] = useState('');
  const [modelForm, setModelForm] = useState<ModelConfig>(emptyModel);
  const [editingModelId, setEditingModelId] = useState('');
  const [checkingModelId, setCheckingModelId] = useState('');
  const [streamingMessage, setStreamingMessage] = useState<Message | null>(null);
  const [expandedDiffIds, setExpandedDiffIds] = useState<string[]>([]);
  const [changesDockOpen, setChangesDockOpen] = useState(false);
  const [sourcesDockOpen, setSourcesDockOpen] = useState(false);
  const [applyingChanges, setApplyingChanges] = useState(false);
  const [rollingBackChanges, setRollingBackChanges] = useState(false);
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

  useEffect(() => {
    if (!settingsOpen || settingsTab !== 'groups') {
      return;
    }
    const nextGroupId = selectedGroupEditorId || currentAgentGroup?.id || agentGroups[0]?.id || '';
    if (!nextGroupId) {
      return;
    }
    if (nextGroupId !== selectedGroupEditorId) {
      setSelectedGroupEditorId(nextGroupId);
    }
    void loadGroupDetails(nextGroupId);
  }, [settingsOpen, settingsTab, selectedGroupEditorId, currentAgentGroup?.id, agentGroups]);

  useEffect(() => {
    const fallbackGroupId = defaultProjectGroupId(agentGroups);
    if (!fallbackGroupId) {
      return;
    }
    if (!newProjectGroupId) {
      setNewProjectGroupId(fallbackGroupId);
    }
    if (!existingProjectGroupId) {
      setExistingProjectGroupId(fallbackGroupId);
    }
  }, [agentGroups, newProjectGroupId, existingProjectGroupId]);

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
  const visibleWebSources = useMemo(() => compactWebSources(webSources), [webSources]);
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
      setAgentGroups(state.agentGroups ?? []);
      setAgentGroupTemplates(state.agentGroupTemplates ?? []);
      setAgentLibrary(state.agentLibrary ?? []);
      setModels(state.models);
      setActiveModelId(state.activeModelId);
      setWebSettings(state.webSettings ?? defaultWebSettings);
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
    setWorkflowPlan(state.workflowPlan ?? null);
    setPlanSteps(state.planSteps ?? []);
    setBlueprint(state.blueprint ?? null);
    setClarification(state.clarification ?? null);
    setChanges(state.changes ?? []);
    setWebSources(state.webSources ?? []);
    setCTFWorkspace(state.ctfWorkspace ?? null);
    setCurrentAgentGroup(state.agentGroup ?? null);
    setGroupBinding(state.groupBinding ?? null);
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
      await backend.createProject(newProjectName, newProjectGroupId, lifecycleForGroup(agentGroups, newProjectGroupId));
      setNewProjectName('');
      setNewProjectGroupId(defaultProjectGroupId(agentGroups));
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
      await backend.addExistingProject(
        existingProjectName,
        existingProjectPath,
        existingProjectGroupId,
        lifecycleForGroup(agentGroups, existingProjectGroupId),
      );
      setExistingProjectName('');
      setExistingProjectPath('');
      setExistingProjectGroupId(defaultProjectGroupId(agentGroups));
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

  async function handleSaveWebSettings(event: FormEvent) {
    event.preventDefault();
    setSavingWebSettings(true);
    setError('');
    try {
      const saved = await backend.saveWebSettings(webSettings);
      setWebSettings(saved);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingWebSettings(false);
    }
  }

  async function loadGroupDetails(groupId: string, groupsSource = agentGroups) {
    if (!groupId) {
      setGroupProfiles([]);
      setGroupLifecycles([]);
      setLifecycleSteps([]);
      setLifecycleRuntimeIssues([]);
      setSelectedLifecycleId('');
      setLifecycleForm(null);
      setLifecycleStepForm(null);
      return;
    }
    setError('');
    try {
      const [profiles, lifecycles] = await Promise.all([
        backend.listAgentProfiles(groupId),
        backend.listLifecycleDefinitions(groupId),
      ]);
      setGroupProfiles(profiles);
      setLibraryTargetProfileId(profiles[0]?.id ?? '');
      setGroupLifecycles(lifecycles);
      const selectedGroup = groupsSource.find((group) => group.id === groupId);
      setGroupForm({
        id: selectedGroup?.id ?? '',
        name: selectedGroup?.name ?? '',
        kind: selectedGroup?.kind ?? 'custom',
        description: selectedGroup?.description ?? '',
        defaultModelId: selectedGroup?.defaultModelId || activeModelId,
      });
      const lifecycleID = selectedGroup?.defaultLifecycleId || lifecycles[0]?.id || '';
      if (lifecycleID) {
        setSelectedLifecycleId(lifecycleID);
        setLifecycleForm(lifecycles.find((item) => item.id === lifecycleID) ?? lifecycles[0] ?? null);
        const [steps, issues] = await Promise.all([
          backend.listLifecycleSteps(lifecycleID),
          backend.validateLifecycleRuntime(lifecycleID),
        ]);
        setLifecycleSteps(steps);
        setLifecycleRuntimeIssues(issues);
      } else {
        setSelectedLifecycleId('');
        setLifecycleForm(null);
        setLifecycleSteps([]);
        setLifecycleRuntimeIssues([]);
      }
      setAgentForm(null);
      setSoulEditor(null);
      setLibraryTargetProfileId('');
      setLifecycleStepForm(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  function handleNewGroup() {
    setSelectedGroupEditorId('');
    setGroupProfiles([]);
    setLibraryTargetProfileId('');
    setGroupLifecycles([]);
    setLifecycleSteps([]);
    setLifecycleRuntimeIssues([]);
    setSelectedLifecycleId('');
    setLifecycleForm(null);
    setLifecycleStepForm(null);
    setGroupForm({
      id: '',
      name: '',
      kind: 'custom',
      description: '',
      defaultModelId: activeModelId,
    });
    setAgentForm(null);
    setSoulEditor(null);
  }

  async function handleSaveGroup(event: FormEvent) {
    event.preventDefault();
    setSavingGroup(true);
    setError('');
    try {
      const updated = groupForm.id
        ? await backend.updateAgentGroup(groupForm)
        : await backend.createAgentGroup({
            name: groupForm.name,
            kind: groupForm.kind,
            description: groupForm.description,
            defaultModelId: groupForm.defaultModelId || activeModelId,
          });
      setAgentGroups(updated);
      const saved =
        updated.find((group) => group.id === groupForm.id) ??
        updated.find((group) => group.name === groupForm.name) ??
        updated[0];
      if (saved) {
        setSelectedGroupEditorId(saved.id);
        await loadGroupDetails(saved.id, updated);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingGroup(false);
    }
  }

  async function handleCreateGroupFromTemplate(templateId: string) {
    setSavingGroup(true);
    setError('');
    try {
      const before = new Set(agentGroups.map((group) => group.id));
      const updated = await backend.createAgentGroupFromTemplate({
        templateId,
        defaultModelId: activeModelId,
      });
      setAgentGroups(updated);
      const created = updated.find((group) => !before.has(group.id)) ?? updated[0];
      if (created) {
        setSelectedGroupEditorId(created.id);
        await loadGroupDetails(created.id, updated);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingGroup(false);
    }
  }

  async function handleArchiveGroup(groupId: string) {
    setError('');
    try {
      const updated = await backend.archiveAgentGroup(groupId);
      setAgentGroups(updated);
      const nextGroupId = updated[0]?.id || '';
      setSelectedGroupEditorId(nextGroupId);
      await loadGroupDetails(nextGroupId, updated);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleBindProjectGroup(groupId: string) {
    if (!selectedProjectId) {
      return;
    }
    const group = agentGroups.find((item) => item.id === groupId);
    setError('');
    try {
      const state = await backend.bindProjectAgentGroup(selectedProjectId, groupId, group?.defaultLifecycleId ?? '');
      applyProjectState(state);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  function handleNewAgent() {
    setAgentForm({
      id: '',
      groupId: selectedGroupEditorId,
      name: '',
      roleKey: 'custom',
      description: '',
      avatarPath: '',
      soulPath: '',
      modelId: groupForm.defaultModelId || activeModelId,
      toolProfileId: '',
      capabilities: [],
      allowedTools: [],
      readPaths: [],
      writePaths: [],
      handoffRules: [],
      temperature: 0.1,
      contextBudget: 8000,
      enabled: true,
      sortOrder: groupProfiles.length,
      createdAt: '',
      updatedAt: '',
    });
  }

  async function handleSaveAgent(event: FormEvent) {
    event.preventDefault();
    if (!agentForm) {
      return;
    }
    setSavingAgent(true);
    setError('');
    try {
      const updated = await backend.saveAgentProfile(agentForm);
      setGroupProfiles(updated);
      setLibraryTargetProfileId((previous) => previous || updated[0]?.id || '');
      setAgentForm(null);
      const groups = await backend.listAgentGroups();
      setAgentGroups(groups);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingAgent(false);
    }
  }

  async function handleToggleAgent(profile: AgentProfile) {
    setError('');
    try {
      const updated = await backend.setAgentProfileEnabled(profile.id, !profile.enabled);
      setGroupProfiles(updated);
      setLibraryTargetProfileId((previous) => previous || updated[0]?.id || '');
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleDuplicateAgent(profile: AgentProfile) {
    setError('');
    try {
      const updated = await backend.duplicateAgentProfile(profile.id);
      setGroupProfiles(updated);
      setLibraryTargetProfileId(updated[updated.length - 1]?.id || updated[0]?.id || '');
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleAddLibraryAgent(libraryAgentId: string) {
    if (!selectedGroupEditorId) {
      return;
    }
    setAddingLibraryAgentId(libraryAgentId);
    setError('');
    try {
      const updated = await backend.addAgentFromLibrary(selectedGroupEditorId, libraryAgentId, groupForm.defaultModelId || activeModelId);
      setGroupProfiles(updated);
      setLibraryTargetProfileId(updated[updated.length - 1]?.id || updated[0]?.id || '');
      const groups = await backend.listAgentGroups();
      setAgentGroups(groups);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setAddingLibraryAgentId('');
    }
  }

  async function handleReplaceSoulFromLibrary(libraryAgentId: string) {
    if (!libraryTargetProfileId) {
      return;
    }
    setAddingLibraryAgentId(libraryAgentId);
    setError('');
    try {
      const savedSoul = await backend.replaceAgentSoulFromLibrary(libraryTargetProfileId, libraryAgentId, false);
      setSoulEditor(savedSoul);
      const profiles = await backend.listAgentProfiles(selectedGroupEditorId);
      setGroupProfiles(profiles);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setAddingLibraryAgentId('');
    }
  }

  async function handleOpenSoul(profile: AgentProfile) {
    setError('');
    try {
      const soul = await backend.getAgentSoul(profile.id);
      setSoulEditor(soul);
      setGroupProfiles((previous) =>
        previous.map((item) => (item.id === profile.id ? { ...item, soulPath: soul.path } : item)),
      );
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleSaveSoul(event: FormEvent) {
    event.preventDefault();
    if (!soulEditor) {
      return;
    }
    setSavingSoul(true);
    setError('');
    try {
      const saved = await backend.saveAgentSoul(soulEditor.profileId, soulEditor.content);
      setSoulEditor(saved);
      setGroupProfiles((previous) =>
        previous.map((item) => (item.id === saved.profileId ? { ...item, soulPath: saved.path } : item)),
      );
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingSoul(false);
    }
  }

  async function handleSelectLifecycle(lifecycleId: string) {
    setSelectedLifecycleId(lifecycleId);
    setLifecycleForm(groupLifecycles.find((item) => item.id === lifecycleId) ?? null);
    setLifecycleStepForm(null);
    setError('');
    try {
      if (!lifecycleId) {
        setLifecycleSteps([]);
        setLifecycleRuntimeIssues([]);
        return;
      }
      const [steps, issues] = await Promise.all([
        backend.listLifecycleSteps(lifecycleId),
        backend.validateLifecycleRuntime(lifecycleId),
      ]);
      setLifecycleSteps(steps);
      setLifecycleRuntimeIssues(issues);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function handleSaveLifecycle(event: FormEvent) {
    event.preventDefault();
    if (!lifecycleForm) {
      return;
    }
    setSavingLifecycle(true);
    setError('');
    try {
      const updated = await backend.saveLifecycleDefinition(lifecycleForm);
      setGroupLifecycles(updated);
      const saved = updated.find((item) => item.id === lifecycleForm.id) ?? updated[0];
      if (saved) {
        setSelectedLifecycleId(saved.id);
        setLifecycleForm(saved);
        const [steps, issues] = await Promise.all([
          backend.listLifecycleSteps(saved.id),
          backend.validateLifecycleRuntime(saved.id),
        ]);
        setLifecycleSteps(steps);
        setLifecycleRuntimeIssues(issues);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingLifecycle(false);
    }
  }

  function handleNewLifecycleStep() {
    setLifecycleStepForm({
      id: '',
      lifecycleId: selectedLifecycleId,
      stepKey: 'custom_step',
      title: 'Новый шаг',
      agentProfileId: groupProfiles[0]?.id ?? '',
      mode: 'llm',
      required: true,
      canRetry: true,
      maxRetries: 1,
      onSuccessStepKey: '',
      onFailureStepKey: '',
      outputSchema: '',
      visibleToUser: true,
      sortOrder: lifecycleSteps.length,
    });
  }

  async function handleSaveLifecycleStep(event: FormEvent) {
    event.preventDefault();
    if (!lifecycleStepForm) {
      return;
    }
    setSavingLifecycleStep(true);
    setError('');
    try {
      const updated = await backend.saveLifecycleStep(lifecycleStepForm);
      setLifecycleSteps(updated);
      setLifecycleRuntimeIssues(await backend.validateLifecycleRuntime(lifecycleStepForm.lifecycleId));
      setLifecycleStepForm(null);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSavingLifecycleStep(false);
    }
  }

  async function handleDeleteLifecycleStep(step: LifecycleStep) {
    setError('');
    try {
      const updated = await backend.deleteLifecycleStep(step.id, step.lifecycleId);
      setLifecycleSteps(updated);
      setLifecycleRuntimeIssues(await backend.validateLifecycleRuntime(step.lifecycleId));
      if (lifecycleStepForm?.id === step.id) {
        setLifecycleStepForm(null);
      }
    } catch (err) {
      setError(errorMessage(err));
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

  async function handleRollbackChanges() {
    if (!selectedProjectId || !workflowRun?.id || rollingBackChanges) {
      return;
    }
    const appliedCount = visibleChanges.filter((change) => change.status === 'applied').length;
    if (appliedCount === 0) {
      return;
    }
    const confirmed = window.confirm(`Откатить примененные изменения этого workflow? Файлов: ${appliedCount}.`);
    if (!confirmed) {
      return;
    }

    setRollingBackChanges(true);
    setError('');
    try {
      const state = await backend.rollbackWorkflowChanges(selectedProjectId, workflowRun.id);
      applyChatState(state);
      setAgents(state.agents);
      if (state.error) {
        setError(state.error);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setRollingBackChanges(false);
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
          {ctfWorkspace && <CTFWorkspacePanel workspace={ctfWorkspace} />}

          {visibleMessages.length === 0 && !ctfWorkspace && (
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

        {(visibleChanges.length > 0 || visibleWebSources.length > 0 || planSteps.length > 0) && (
          <div className="dock-row" aria-label="Сводка выполнения">
            {visibleChanges.length > 0 && (
              <ChangeSummaryDock
                changes={visibleChanges}
                summary={changeSummary}
                pendingCount={pendingChanges.length}
                isOpen={changesDockOpen}
                expandedDiffIds={expandedDiffIds}
                applyingChanges={applyingChanges}
                rollingBackChanges={rollingBackChanges}
                onToggleOpen={() => setChangesDockOpen((value) => !value)}
                onToggleDiff={toggleDiff}
                onApplyChanges={handleApplyChanges}
                onRollbackChanges={handleRollbackChanges}
              />
            )}
            {visibleWebSources.length > 0 && (
              <WebSourcesDock
                sources={visibleWebSources}
                isOpen={sourcesDockOpen}
                onToggleOpen={() => setSourcesDockOpen((value) => !value)}
              />
            )}
            {planSteps.length > 0 && <StepDock plan={workflowPlan} steps={planSteps} />}
          </div>
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
              <button
                className={settingsTab === 'groups' ? 'active' : ''}
                type="button"
                onClick={() => setSettingsTab('groups')}
              >
                Группы
              </button>
              <button
                className={settingsTab === 'web' ? 'active' : ''}
                type="button"
                onClick={() => setSettingsTab('web')}
              >
                Интернет
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
                      <ProjectGroupSelect
                        id="new-project-group"
                        label="Группа"
                        groups={agentGroups}
                        value={newProjectGroupId || defaultProjectGroupId(agentGroups)}
                        onChange={setNewProjectGroupId}
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
                      <ProjectGroupSelect
                        id="existing-project-group"
                        label="Группа"
                        groups={agentGroups}
                        value={existingProjectGroupId || defaultProjectGroupId(agentGroups)}
                        onChange={setExistingProjectGroupId}
                      />
                      <button className="primary-button" type="submit">
                        Добавить
                      </button>
                    </form>
                  )}
                </section>
              </div>
            )}

            {settingsTab === 'groups' && (
              <div className="settings-content settings-groups">
                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Группа проекта</h3>
                      <p className="muted">
                        {currentProject
                          ? `${currentProject.name}: ${currentAgentGroup?.name ?? 'Dev Squad'}`
                          : 'Выбери проект, чтобы назначить ему группу.'}
                      </p>
                    </div>
                  </div>
                  <div className="group-choice-list">
                    {agentGroups.map((group) => (
                      <button
                        key={group.id}
                        className={`group-choice ${group.id === groupBinding?.groupId ? 'active' : ''}`}
                        type="button"
                        disabled={!selectedProjectId}
                        onClick={() => void handleBindProjectGroup(group.id)}
                      >
                        <span>
                          <strong>{group.name}</strong>
                          <small>{groupKindLabel(group.kind)} · {group.agentCount} агентов</small>
                        </span>
                        <span className="group-kind-pill">{group.id === groupBinding?.groupId ? 'активна' : groupKindLabel(group.kind)}</span>
                      </button>
                    ))}
                  </div>
                </section>

                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Библиотека агентов</h3>
                      <p className="muted">Готовые локальные агенты: добавь в группу или замени `soul.md` выбранному агенту.</p>
                    </div>
                    <select
                      className="input compact-select"
                      value={libraryTargetProfileId}
                      disabled={groupProfiles.length === 0}
                      onChange={(event) => setLibraryTargetProfileId(event.target.value)}
                    >
                      {groupProfiles.map((profile) => (
                        <option key={profile.id} value={profile.id}>
                          {profile.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="agent-library-list">
                    {agentLibrary.map((item) => (
                      <article key={item.id} className="agent-library-card">
                        <div>
                          <div className="agent-library-heading">
                            <strong>{item.name}</strong>
                            <span>{item.category}</span>
                          </div>
                          <span>{item.roleKey}</span>
                          <p>{item.description}</p>
                          <div className="agent-capability-list">
                            {(item.tags || []).slice(0, 3).map((tag) => (
                              <span key={tag} className="agent-capability-chip muted-chip">
                                {tag}
                              </span>
                            ))}
                          </div>
                        </div>
                        <div className="group-agent-actions">
                          <button
                            className="small-button secondary"
                            type="button"
                            disabled={!selectedGroupEditorId || addingLibraryAgentId === item.id}
                            onClick={() => void handleAddLibraryAgent(item.id)}
                          >
                            Добавить
                          </button>
                          <button
                            className="small-button secondary"
                            type="button"
                            disabled={!libraryTargetProfileId || addingLibraryAgentId === item.id}
                            onClick={() => void handleReplaceSoulFromLibrary(item.id)}
                          >
                            soul.md
                          </button>
                        </div>
                      </article>
                    ))}
                  </div>
                </section>

                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Шаблоны команд</h3>
                      <p className="muted">Быстрый старт: создай группу из шаблона и донастрой агентов, soul.md и lifecycle.</p>
                    </div>
                  </div>
                  <div className="group-template-list">
                    {agentGroupTemplates.map((template) => (
                      <article key={template.id} className="group-template-card">
                        <div>
                          <strong>{template.name}</strong>
                          <span>{groupKindLabel(template.kind)} · {template.agentCount} агентов · {template.stepCount} шагов</span>
                          <p>{template.description}</p>
                        </div>
                        <button
                          className="secondary-button"
                          type="button"
                          disabled={savingGroup}
                          onClick={() => void handleCreateGroupFromTemplate(template.id)}
                        >
                          Создать
                        </button>
                      </article>
                    ))}
                  </div>
                </section>

                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Команды агентов</h3>
                      <p className="muted">Состав группы, модель по умолчанию, soul.md и lifecycle.</p>
                    </div>
                    <button className="icon-button" type="button" title="Новая группа" onClick={handleNewGroup}>
                      +
                    </button>
                  </div>

                  <div className="group-editor-layout">
                    <div className="group-list">
                      {agentGroups.map((group) => (
                        <button
                          key={group.id}
                          className={`group-list-item ${group.id === selectedGroupEditorId ? 'active' : ''}`}
                          type="button"
                          onClick={() => setSelectedGroupEditorId(group.id)}
                        >
                          <strong>{group.name}</strong>
                          <span>{groupKindLabel(group.kind)} · {group.agentCount} агентов</span>
                        </button>
                      ))}
                    </div>

                    <form className="group-form" onSubmit={handleSaveGroup}>
                      <label className="field-label" htmlFor="group-name">
                        Название
                      </label>
                      <input
                        id="group-name"
                        className="input"
                        value={groupForm.name}
                        onChange={(event) => setGroupForm({ ...groupForm, name: event.target.value })}
                        placeholder="Например, My Dev Team"
                      />

                      <label className="field-label" htmlFor="group-kind">
                        Тип
                      </label>
                      <select
                        id="group-kind"
                        className="input"
                        value={groupForm.kind}
                        onChange={(event) => setGroupForm({ ...groupForm, kind: event.target.value })}
                      >
                        <option value="dev">Dev</option>
                        <option value="ctf">CTF</option>
                        <option value="research">Research</option>
                        <option value="security">Security</option>
                        <option value="custom">Custom</option>
                      </select>

                      <label className="field-label" htmlFor="group-model">
                        Модель по умолчанию
                      </label>
                      <select
                        id="group-model"
                        className="input"
                        value={groupForm.defaultModelId}
                        onChange={(event) => setGroupForm({ ...groupForm, defaultModelId: event.target.value })}
                      >
                        {models.map((model) => (
                          <option key={model.id} value={model.id}>
                            {model.name}
                          </option>
                        ))}
                      </select>

                      <label className="field-label" htmlFor="group-description">
                        Описание
                      </label>
                      <textarea
                        id="group-description"
                        className="input textarea"
                        value={groupForm.description}
                        onChange={(event) => setGroupForm({ ...groupForm, description: event.target.value })}
                        placeholder="Для чего эта команда и какие задачи она решает"
                      />

                      <div className="model-form-actions">
                        <button className="primary-button" type="submit" disabled={savingGroup || !groupForm.name.trim()}>
                          {savingGroup ? 'Сохраняю...' : groupForm.id ? 'Сохранить группу' : 'Создать группу'}
                        </button>
                        <button
                          className="action-button secondary"
                          type="button"
                          disabled={!groupForm.id || groupForm.id === 'group_dev_squad' || groupForm.id === 'group_ctf_cell'}
                          onClick={() => void handleArchiveGroup(groupForm.id)}
                        >
                          Архивировать
                        </button>
                      </div>
                    </form>
                  </div>
                </section>

                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Агенты группы</h3>
                      <p className="muted">Состав команды, роли, модели и `soul.md` каждого агента.</p>
                    </div>
                    <button className="icon-button" type="button" title="Добавить агента" disabled={!selectedGroupEditorId} onClick={handleNewAgent}>
                      +
                    </button>
                  </div>

                  <div className="group-agent-list">
                    {groupProfiles.map((profile) => (
                      <article key={profile.id} className={`group-agent-card ${profile.enabled ? '' : 'disabled'}`}>
                        <div>
                          <strong>{profile.name}</strong>
                          <span>{profile.roleKey}</span>
                          {profile.description && <p>{profile.description}</p>}
                          <div className="agent-capability-list">
                            {(profile.capabilities || []).slice(0, 3).map((capability) => (
                              <span key={capability} className="agent-capability-chip">
                                {capability}
                              </span>
                            ))}
                            {(profile.capabilities || []).length > 3 && (
                              <span className="agent-capability-chip muted-chip">+{profile.capabilities.length - 3}</span>
                            )}
                          </div>
                          <div className="agent-access-meta">
                            <span>tools {(profile.allowedTools || []).length}</span>
                            <span>read {(profile.readPaths || []).length}</span>
                            <span>write {(profile.writePaths || []).length}</span>
                            <span>handoff {(profile.handoffRules || []).length}</span>
                          </div>
                        </div>
                        <div className="group-agent-actions">
                          <button className="small-button secondary" type="button" onClick={() => setAgentForm(profile)}>
                            Править
                          </button>
                          <button className="small-button secondary" type="button" onClick={() => void handleOpenSoul(profile)}>
                            soul.md
                          </button>
                          <button className="small-button secondary" type="button" onClick={() => void handleDuplicateAgent(profile)}>
                            Копировать
                          </button>
                          <button className="small-button" type="button" onClick={() => void handleToggleAgent(profile)}>
                            {profile.enabled ? 'Отключить' : 'Включить'}
                          </button>
                        </div>
                      </article>
                    ))}
                    {groupProfiles.length === 0 && <p className="panel-note">Выбери группу или создай первого агента.</p>}
                  </div>

                  {agentForm && (
                    <form className="agent-profile-form" onSubmit={handleSaveAgent}>
                      <div className="settings-grid-two">
                        <label className="field-label" htmlFor="agent-name">
                          Имя
                        </label>
                        <input
                          id="agent-name"
                          className="input"
                          value={agentForm.name}
                          onChange={(event) => setAgentForm({ ...agentForm, name: event.target.value })}
                        />

                        <label className="field-label" htmlFor="agent-role">
                          Роль
                        </label>
                        <input
                          id="agent-role"
                          className="input"
                          value={agentForm.roleKey}
                          onChange={(event) => setAgentForm({ ...agentForm, roleKey: event.target.value })}
                        />

                        <label className="field-label" htmlFor="agent-model">
                          Модель
                        </label>
                        <select
                          id="agent-model"
                          className="input"
                          value={agentForm.modelId}
                          onChange={(event) => setAgentForm({ ...agentForm, modelId: event.target.value })}
                        >
                          {models.map((model) => (
                            <option key={model.id} value={model.id}>
                              {model.name}
                            </option>
                          ))}
                        </select>

                        <label className="field-label" htmlFor="agent-context">
                          Context budget
                        </label>
                        <input
                          id="agent-context"
                          className="input"
                          type="number"
                          min={1000}
                          value={agentForm.contextBudget}
                          onChange={(event) => setAgentForm({ ...agentForm, contextBudget: Number(event.target.value) })}
                        />
                      </div>

                      <label className="field-label" htmlFor="agent-description">
                        Описание
                      </label>
                      <textarea
                        id="agent-description"
                        className="input textarea"
                        value={agentForm.description}
                        onChange={(event) => setAgentForm({ ...agentForm, description: event.target.value })}
                      />

                      <div className="agent-capabilities-editor">
                        <label className="field-label" htmlFor="agent-capabilities">
                          Что умеет
                        </label>
                        <textarea
                          id="agent-capabilities"
                          className="input textarea compact-textarea"
                          value={listToLines(agentForm.capabilities || [])}
                          placeholder="Один capability на строку"
                          onChange={(event) => setAgentForm({ ...agentForm, capabilities: linesToList(event.target.value) })}
                        />

                        <label className="field-label" htmlFor="agent-tools">
                          Разрешенные инструменты
                        </label>
                        <textarea
                          id="agent-tools"
                          className="input textarea compact-textarea"
                          value={listToLines(agentForm.allowedTools || [])}
                          placeholder="Команды, tool profiles или встроенные инструменты"
                          onChange={(event) => setAgentForm({ ...agentForm, allowedTools: linesToList(event.target.value) })}
                        />

                        <div className="settings-grid-two">
                          <div>
                            <label className="field-label" htmlFor="agent-read-paths">
                              Может читать
                            </label>
                            <textarea
                              id="agent-read-paths"
                              className="input textarea compact-textarea"
                              value={listToLines(agentForm.readPaths || [])}
                              placeholder="README*, docs/**, internal/**"
                              onChange={(event) => setAgentForm({ ...agentForm, readPaths: linesToList(event.target.value) })}
                            />
                          </div>
                          <div>
                            <label className="field-label" htmlFor="agent-write-paths">
                              Может писать
                            </label>
                            <textarea
                              id="agent-write-paths"
                              className="input textarea compact-textarea"
                              value={listToLines(agentForm.writePaths || [])}
                              placeholder="docs/**, solve/**, ./**/*.go"
                              onChange={(event) => setAgentForm({ ...agentForm, writePaths: linesToList(event.target.value) })}
                            />
                          </div>
                        </div>

                        <label className="field-label" htmlFor="agent-handoff">
                          Когда передавать дальше
                        </label>
                        <textarea
                          id="agent-handoff"
                          className="input textarea compact-textarea"
                          value={listToLines(agentForm.handoffRules || [])}
                          placeholder="Один handoff rule на строку"
                          onChange={(event) => setAgentForm({ ...agentForm, handoffRules: linesToList(event.target.value) })}
                        />
                      </div>

                      <label className="checkbox-row">
                        <input
                          type="checkbox"
                          checked={agentForm.enabled}
                          onChange={(event) => setAgentForm({ ...agentForm, enabled: event.target.checked })}
                        />
                        <span>Агент включен</span>
                      </label>

                      <div className="model-form-actions">
                        <button className="primary-button" type="submit" disabled={savingAgent || !agentForm.name.trim()}>
                          {savingAgent ? 'Сохраняю...' : 'Сохранить агента'}
                        </button>
                        <button className="action-button secondary" type="button" onClick={() => setAgentForm(null)}>
                          Отмена
                        </button>
                      </div>
                    </form>
                  )}

                  {soulEditor && (
                    <form className="soul-editor-form" onSubmit={handleSaveSoul}>
                      <div className="settings-section-heading">
                        <div>
                          <h3>soul.md</h3>
                          <p className="muted">{soulEditor.path}</p>
                        </div>
                        <button className="icon-button" type="button" title="Закрыть soul.md" onClick={() => setSoulEditor(null)}>
                          ×
                        </button>
                      </div>
                      {soulEditor.warnings.length > 0 && (
                        <div className="soul-warning-list">
                          {soulEditor.warnings.map((warning) => (
                            <p key={warning}>{warning}</p>
                          ))}
                        </div>
                      )}
                      <textarea
                        className="input textarea soul-textarea"
                        value={soulEditor.content}
                        onChange={(event) => setSoulEditor({ ...soulEditor, content: event.target.value })}
                      />
                      <div className="model-form-actions">
                        <button className="primary-button" type="submit" disabled={savingSoul || !soulEditor.content.trim()}>
                          {savingSoul ? 'Сохраняю...' : 'Сохранить soul.md'}
                        </button>
                        <button className="action-button secondary" type="button" onClick={() => setSoulEditor(null)}>
                          Закрыть
                        </button>
                      </div>
                    </form>
                  )}
                </section>

                <section className="settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Lifecycle</h3>
                      <p className="muted">Шаги, возвраты, видимость и retry-политика группы.</p>
                    </div>
                    <button className="icon-button" type="button" title="Добавить шаг" disabled={!selectedLifecycleId} onClick={handleNewLifecycleStep}>
                      +
                    </button>
                  </div>
                  {groupLifecycles.length > 0 && (
                    <div className="lifecycle-editor-shell">
                      <label className="field-label" htmlFor="lifecycle-select">
                        Активный сценарий
                      </label>
                      <select
                        id="lifecycle-select"
                        className="input"
                        value={selectedLifecycleId}
                        onChange={(event) => void handleSelectLifecycle(event.target.value)}
                      >
                        {groupLifecycles.map((item) => (
                          <option key={item.id} value={item.id}>
                            {item.name}
                          </option>
                        ))}
                      </select>

                      {lifecycleForm && (
                        <form className="lifecycle-form" onSubmit={handleSaveLifecycle}>
                          <div className="settings-grid-two">
                            <label className="field-label" htmlFor="lifecycle-name">
                              Название
                            </label>
                            <input
                              id="lifecycle-name"
                              className="input"
                              value={lifecycleForm.name}
                              onChange={(event) => setLifecycleForm({ ...lifecycleForm, name: event.target.value })}
                            />

                            <label className="field-label" htmlFor="lifecycle-kind">
                              Тип
                            </label>
                            <input
                              id="lifecycle-kind"
                              className="input"
                              value={lifecycleForm.kind}
                              onChange={(event) => setLifecycleForm({ ...lifecycleForm, kind: event.target.value })}
                            />

                            <label className="field-label" htmlFor="lifecycle-max-total">
                              Max steps
                            </label>
                            <input
                              id="lifecycle-max-total"
                              className="input"
                              type="number"
                              min={1}
                              value={lifecycleForm.maxTotalIterations}
                              onChange={(event) => setLifecycleForm({ ...lifecycleForm, maxTotalIterations: Number(event.target.value) })}
                            />

                            <label className="field-label" htmlFor="lifecycle-max-repair">
                              Repair retries
                            </label>
                            <input
                              id="lifecycle-max-repair"
                              className="input"
                              type="number"
                              min={0}
                              value={lifecycleForm.maxRepairIterations}
                              onChange={(event) => setLifecycleForm({ ...lifecycleForm, maxRepairIterations: Number(event.target.value) })}
                            />

                            <label className="field-label" htmlFor="lifecycle-same-error">
                              Same error limit
                            </label>
                            <input
                              id="lifecycle-same-error"
                              className="input"
                              type="number"
                              min={1}
                              value={lifecycleForm.sameErrorLimit}
                              onChange={(event) => setLifecycleForm({ ...lifecycleForm, sameErrorLimit: Number(event.target.value) })}
                            />
                          </div>
                          <label className="field-label" htmlFor="lifecycle-description">
                            Описание
                          </label>
                          <textarea
                            id="lifecycle-description"
                            className="input textarea compact"
                            value={lifecycleForm.description}
                            onChange={(event) => setLifecycleForm({ ...lifecycleForm, description: event.target.value })}
                          />
                          <div className="model-form-actions">
                            <button className="primary-button" type="submit" disabled={savingLifecycle || !lifecycleForm.name.trim()}>
                              {savingLifecycle ? 'Сохраняю...' : 'Сохранить lifecycle'}
                            </button>
                          </div>
                        </form>
                      )}
                    </div>
                  )}

                  <LifecycleVisualEditor
                    steps={lifecycleSteps}
                    profiles={groupProfiles}
                    issues={lifecycleRuntimeIssues}
                    selectedStepId={lifecycleStepForm?.id ?? ''}
                    onEdit={(step) => setLifecycleStepForm(step)}
                    onDelete={(step) => void handleDeleteLifecycleStep(step)}
                  />

                  {lifecycleStepForm && (
                    <form className="lifecycle-step-form" onSubmit={handleSaveLifecycleStep}>
                      <div className="settings-section-heading">
                        <div>
                          <h3>{lifecycleStepForm.id ? 'Редактировать шаг' : 'Новый шаг'}</h3>
                          <p className="muted">Ключ шага используется executor для переходов и статуса.</p>
                        </div>
                        <button className="icon-button" type="button" title="Закрыть редактор шага" onClick={() => setLifecycleStepForm(null)}>
                          ×
                        </button>
                      </div>
                      <div className="settings-grid-two">
                        <label className="field-label" htmlFor="lifecycle-step-title">
                          Название
                        </label>
                        <input
                          id="lifecycle-step-title"
                          className="input"
                          value={lifecycleStepForm.title}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, title: event.target.value })}
                        />

                        <label className="field-label" htmlFor="lifecycle-step-key">
                          Step key
                        </label>
                        <input
                          id="lifecycle-step-key"
                          className="input"
                          value={lifecycleStepForm.stepKey}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, stepKey: event.target.value })}
                        />

                        <label className="field-label" htmlFor="lifecycle-step-agent">
                          Агент
                        </label>
                        <select
                          id="lifecycle-step-agent"
                          className="input"
                          value={lifecycleStepForm.agentProfileId}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, agentProfileId: event.target.value })}
                        >
                          <option value="">Не назначен</option>
                          {groupProfiles.map((profile) => (
                            <option key={profile.id} value={profile.id}>
                              {profile.name} · {profile.roleKey}
                            </option>
                          ))}
                        </select>

                        <label className="field-label" htmlFor="lifecycle-step-mode">
                          Режим
                        </label>
                        <select
                          id="lifecycle-step-mode"
                          className="input"
                          value={lifecycleStepForm.mode}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, mode: event.target.value })}
                        >
                          {lifecycleModes.map((mode) => (
                            <option key={mode} value={mode}>
                              {mode}
                            </option>
                          ))}
                        </select>

                        <label className="field-label" htmlFor="lifecycle-step-order">
                          Порядок
                        </label>
                        <input
                          id="lifecycle-step-order"
                          className="input"
                          type="number"
                          min={0}
                          value={lifecycleStepForm.sortOrder}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, sortOrder: Number(event.target.value) })}
                        />

                        <label className="field-label" htmlFor="lifecycle-step-retries">
                          Retries
                        </label>
                        <input
                          id="lifecycle-step-retries"
                          className="input"
                          type="number"
                          min={0}
                          value={lifecycleStepForm.maxRetries}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, maxRetries: Number(event.target.value) })}
                        />

                        <label className="field-label" htmlFor="lifecycle-step-success">
                          On success
                        </label>
                        <input
                          id="lifecycle-step-success"
                          className="input"
                          value={lifecycleStepForm.onSuccessStepKey}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, onSuccessStepKey: event.target.value })}
                          placeholder="пусто = следующий"
                        />

                        <label className="field-label" htmlFor="lifecycle-step-failure">
                          On failure
                        </label>
                        <input
                          id="lifecycle-step-failure"
                          className="input"
                          value={lifecycleStepForm.onFailureStepKey}
                          onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, onFailureStepKey: event.target.value })}
                          placeholder="например developer_plan"
                        />
                      </div>
                      <div className="lifecycle-toggles">
                        <label className="checkbox-row">
                          <input
                            type="checkbox"
                            checked={lifecycleStepForm.required}
                            onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, required: event.target.checked })}
                          />
                          <span>Обязательный</span>
                        </label>
                        <label className="checkbox-row">
                          <input
                            type="checkbox"
                            checked={lifecycleStepForm.canRetry}
                            onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, canRetry: event.target.checked })}
                          />
                          <span>Можно повторять</span>
                        </label>
                        <label className="checkbox-row">
                          <input
                            type="checkbox"
                            checked={lifecycleStepForm.visibleToUser}
                            onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, visibleToUser: event.target.checked })}
                          />
                          <span>Показывать в ходе работы</span>
                        </label>
                      </div>
                      <label className="field-label" htmlFor="lifecycle-step-schema">
                        Output schema
                      </label>
                      <textarea
                        id="lifecycle-step-schema"
                        className="input textarea compact"
                        value={lifecycleStepForm.outputSchema}
                        onChange={(event) => setLifecycleStepForm({ ...lifecycleStepForm, outputSchema: event.target.value })}
                      />
                      <div className="model-form-actions">
                        <button className="primary-button" type="submit" disabled={savingLifecycleStep || !lifecycleStepForm.title.trim() || !lifecycleStepForm.stepKey.trim()}>
                          {savingLifecycleStep ? 'Сохраняю...' : 'Сохранить шаг'}
                        </button>
                        <button className="action-button secondary" type="button" onClick={() => setLifecycleStepForm(null)}>
                          Отмена
                        </button>
                      </div>
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

            {settingsTab === 'web' && (
              <form className="settings-content" onSubmit={handleSaveWebSettings}>
                <section className="settings-section web-settings-section">
                  <div className="settings-section-heading">
                    <div>
                      <h3>Web research</h3>
                      <p className="muted">Люмен использует сеть только для запросов, где нужен актуальный поиск или источники.</p>
                    </div>
                  </div>

                  <label className="checkbox-row web-toggle">
                    <input
                      type="checkbox"
                      checked={webSettings.enabled}
                      onChange={(event) => setWebSettings({ ...webSettings, enabled: event.target.checked })}
                    />
                    <span>Разрешить поиск в интернете</span>
                  </label>

                  <div className="settings-grid-two">
                    <label className="field-label" htmlFor="web-max-results">
                      Результатов на запрос
                    </label>
                    <input
                      id="web-max-results"
                      className="input"
                      type="number"
                      min={1}
                      max={10}
                      value={webSettings.maxResults}
                      onChange={(event) => setWebSettings({ ...webSettings, maxResults: Number(event.target.value) })}
                    />

                    <label className="field-label" htmlFor="web-max-pages">
                      Страниц на workflow
                    </label>
                    <input
                      id="web-max-pages"
                      className="input"
                      type="number"
                      min={1}
                      max={20}
                      value={webSettings.maxPagesPerWorkflow}
                      onChange={(event) => setWebSettings({ ...webSettings, maxPagesPerWorkflow: Number(event.target.value) })}
                    />

                    <label className="field-label" htmlFor="web-timeout">
                      Таймаут, сек
                    </label>
                    <input
                      id="web-timeout"
                      className="input"
                      type="number"
                      min={2}
                      max={30}
                      value={webSettings.timeoutSeconds}
                      onChange={(event) => setWebSettings({ ...webSettings, timeoutSeconds: Number(event.target.value) })}
                    />
                  </div>

                  <div className="field-heading-row">
                    <label className="field-label" htmlFor="web-allowed">
                      Ограничить домены
                    </label>
                    {webSettings.allowedDomains.length > 0 ? (
                      <button
                        className="inline-action-button"
                        type="button"
                        onClick={() => setWebSettings({ ...webSettings, allowedDomains: [] })}
                      >
                        Разрешить все
                      </button>
                    ) : (
                      <span className="field-state-pill">все публичные сайты</span>
                    )}
                  </div>
                  <textarea
                    id="web-allowed"
                    className="input textarea"
                    placeholder="Пусто = разрешены все публичные сайты"
                    value={webSettings.allowedDomains.join('\n')}
                    onChange={(event) => setWebSettings({ ...webSettings, allowedDomains: splitDomainList(event.target.value) })}
                  />
                  <p className="field-hint">
                    Заполняй только если нужно сузить поиск до конкретных доменов.
                  </p>

                  <label className="field-label" htmlFor="web-blocked">
                    Заблокировать домены
                  </label>
                  <textarea
                    id="web-blocked"
                    className="input textarea"
                    placeholder="example.com"
                    value={webSettings.blockedDomains.join('\n')}
                    onChange={(event) => setWebSettings({ ...webSettings, blockedDomains: splitDomainList(event.target.value) })}
                  />

                  <button className="primary-button" type="submit" disabled={savingWebSettings}>
                    {savingWebSettings ? 'Сохраняю...' : 'Сохранить интернет'}
                  </button>
                </section>
              </form>
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
  rollingBackChanges,
  onToggleOpen,
  onToggleDiff,
  onApplyChanges,
  onRollbackChanges,
}: {
  changes: DisplayChange[];
  summary: ChangeSummary;
  pendingCount: number;
  isOpen: boolean;
  expandedDiffIds: string[];
  applyingChanges: boolean;
  rollingBackChanges: boolean;
  onToggleOpen: () => void;
  onToggleDiff: (id: string) => void;
  onApplyChanges: () => void;
  onRollbackChanges: () => void;
}) {
  const appliedCount = changes.filter((change) => change.status === 'applied').length;
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
          {(pendingCount > 0 || appliedCount > 0) && (
            <div className="changes-dock-actions">
              {appliedCount > 0 && (
                <button
                  className="rollback-changes-button"
                  type="button"
                  disabled={rollingBackChanges}
                  onClick={onRollbackChanges}
                >
                  {rollingBackChanges ? 'Откатываю...' : `Откатить ${appliedCount}`}
                </button>
              )}
              {pendingCount > 0 && (
                <button className="apply-changes-button dock-apply" type="button" disabled={applyingChanges} onClick={onApplyChanges}>
                  {applyingChanges ? 'Применяю...' : `Применить ${pendingCount}`}
                </button>
              )}
            </div>
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

function CTFWorkspacePanel({ workspace }: { workspace: CTFWorkspace }) {
  const [copiedPath, setCopiedPath] = useState('');
  const sections = ctfWorkspaceSections(workspace).filter((item) => item.section.content || item.section.path);

  async function handleCopyPath(path: string) {
    if (!path) {
      return;
    }
    await copyToClipboard(path);
    setCopiedPath(path);
    window.setTimeout(() => setCopiedPath(''), 1200);
  }

  return (
    <section className="ctf-workspace-panel" aria-label="CTF workspace">
      <div className="ctf-workspace-header">
        <div>
          <p className="eyebrow">CTF workspace</p>
          <h2>{workspace.title || 'CTF задача'}</h2>
          <div className="ctf-workspace-meta">
            <span className="ctf-category">{workspace.category || 'web'}</span>
            {workspace.scopeStatus && <span className={`ctf-scope ${ctfScopeKind(workspace.scopeStatus)}`}>{ctfScopeLabel(workspace.scopeStatus)}</span>}
            {workspace.root && <code>{workspace.root}</code>}
          </div>
        </div>
        <div className="ctf-workspace-paths">
          {workspace.evidenceDir && <span>evidence: <code>{workspace.evidenceDir}</code></span>}
          {workspace.evidenceIndex && <span>index: <code>{workspace.evidenceIndex}</code></span>}
          {workspace.solveDir && <span>solve: <code>{workspace.solveDir}</code></span>}
          {workspace.writeupPath && <span>writeup: <code>{workspace.writeupPath}</code></span>}
        </div>
      </div>

      <div className="ctf-workspace-grid">
        {sections.map((item) => (
          <article key={item.key} className={`ctf-section-card ${item.kind}`}>
            <div className="ctf-section-heading">
              <div>
                <span>{item.label}</span>
                <strong>{item.section.title || item.label}</strong>
              </div>
              <span className={`workflow-chip ${item.section.status || 'queued'}`}>
                {ctfStatusLabel(item.section.status)}
              </span>
            </div>
            {item.section.agentId && <p className="ctf-section-agent">{agentNameById(item.section.agentId)}</p>}
            {item.section.content ? <MarkdownContent content={item.section.content} /> : <p className="muted">Пока нет данных.</p>}
            {item.section.path && (
              <button className="ctf-copy-path" type="button" onClick={() => void handleCopyPath(item.section.path)}>
                {copiedPath === item.section.path ? 'Путь скопирован' : item.section.path}
              </button>
            )}
          </article>
        ))}
      </div>

      {workspace.files.length > 0 && (
        <div className="ctf-files-panel">
          <div className="ctf-files-heading">
            <strong>Файлы workspace</strong>
            <span>{workspace.files.length}</span>
          </div>
          <div className="ctf-file-list">
            {workspace.files.map((file) => (
              <button key={`${file.kind}:${file.relativePath}`} type="button" onClick={() => void handleCopyPath(file.relativePath)}>
                <span>{file.title || file.relativePath}</span>
                <code>{file.relativePath}</code>
                <small>{copiedPath === file.relativePath ? 'скопировано' : ctfFileKindLabel(file)}</small>
              </button>
            ))}
          </div>
        </div>
      )}
    </section>
  );
}

function WebSourcesDock({
  sources,
  isOpen,
  onToggleOpen,
}: {
  sources: WebSource[];
  isOpen: boolean;
  onToggleOpen: () => void;
}) {
  const [copiedSourceId, setCopiedSourceId] = useState('');

  async function handleCopySource(source: WebSource) {
    const value = source.url.trim();
    if (!value) {
      return;
    }
    await copyToClipboard(value);
    setCopiedSourceId(source.id || source.url);
    window.setTimeout(() => setCopiedSourceId(''), 1200);
  }

  return (
    <section className="sources-dock" aria-label="Источники исследования">
      {isOpen && (
        <div className="sources-dock-popover">
          <div className="sources-dock-header">
            <strong>{sourcesTitle(sources.length)}</strong>
            <span>web sources</span>
          </div>
          <div className="sources-dock-list">
            {sources.map((source, index) => {
              const sourceId = source.id || source.url || String(index);
              const title = source.title || hostFromUrl(source.url) || `Источник ${index + 1}`;
              const excerpt = source.contentExcerpt || source.snippet;
              const date = formatSourceDate(source);
              return (
                <article key={sourceId} className="sources-dock-item">
                  <div className="sources-dock-item-main">
                    <a href={source.url} onClick={(event) => openExternalLink(event, source.url)}>
                      {title}
                    </a>
                    <div className="sources-dock-meta">
                      <span>{hostFromUrl(source.url)}</span>
                      {source.sourceType && <span>{sourceTypeLabel(source.sourceType)}</span>}
                      {date && <span>{date}</span>}
                      <span className={`source-trust ${sourceTrustKind(source.trustLevel)}`}>
                        {sourceTrustLabel(source.trustLevel)}
                      </span>
                    </div>
                    {excerpt && <p>{shortPreview(excerpt, 180)}</p>}
                  </div>
                  <div className="sources-dock-actions">
                    <button type="button" onClick={() => openExternalUrl(source.url)}>
                      Открыть
                    </button>
                    <button type="button" onClick={() => void handleCopySource(source)}>
                      {copiedSourceId === sourceId ? 'Скопировано' : 'Копировать'}
                    </button>
                  </div>
                </article>
              );
            })}
          </div>
        </div>
      )}
      <button className={`sources-dock-pill ${isOpen ? 'active' : ''}`} type="button" onClick={onToggleOpen}>
        <span>{sourcesTitle(sources.length)}</span>
      </button>
    </section>
  );
}

function StepDock({ plan, steps }: { plan?: WorkflowPlan | null; steps: WorkflowPlanStep[] }) {
  const orderedSteps = [...steps].sort((left, right) => left.sortOrder - right.sortOrder);
  const total = orderedSteps.length;
  if (total === 0) {
    return null;
  }
  const activeIndex = orderedSteps.findIndex((step) => step.id === plan?.currentStepId || step.status === 'running');
  const doneCount = orderedSteps.filter((step) => step.status === 'done' || step.status === 'skipped').length;
  const displayIndex = activeIndex >= 0 ? activeIndex + 1 : Math.max(1, Math.min(doneCount || 1, total));
  const activeStep = orderedSteps[activeIndex >= 0 ? activeIndex : Math.min(displayIndex - 1, total - 1)];
  const planStatus = plan?.status ?? activeStep?.status ?? 'queued';

  return (
    <section className="step-dock" aria-label="План выполнения">
      <div className="step-dock-popover" role="tooltip">
        <div className="step-dock-header">
          <strong>{plan?.title || 'План выполнения'}</strong>
          <span>{displayIndex}/{total}</span>
        </div>
        <div className="step-dock-list">
          {orderedSteps.map((step, index) => (
            <div key={step.id || `${step.stepKey}-${index}`} className={`step-dock-item ${step.status} ${step.id === activeStep?.id ? 'active' : ''}`}>
              <span className={`step-dock-icon ${step.status}`}>{stepIcon(step.status, index + 1)}</span>
              <div>
                <div className="step-dock-title">
                  <strong>{step.title}</strong>
                  <span>{agentNameById(step.agentId || agentForStep(step.stepKey))}</span>
                </div>
                {step.description && <p>{step.description}</p>}
                {step.error && <p className="workflow-step-error">{step.error}</p>}
              </div>
            </div>
          ))}
        </div>
      </div>
      <button className={`step-dock-pill ${planStatus}`} type="button">
        <span className={`step-dock-dot ${activeStep?.status || planStatus}`} aria-hidden="true" />
        <span>Шаг {displayIndex} / {total}</span>
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

function LifecycleVisualEditor({
  steps,
  profiles,
  issues,
  selectedStepId,
  onEdit,
  onDelete,
}: {
  steps: LifecycleStep[];
  profiles: AgentProfile[];
  issues: LifecycleRuntimeIssue[];
  selectedStepId: string;
  onEdit: (step: LifecycleStep) => void;
  onDelete: (step: LifecycleStep) => void;
}) {
  const orderedSteps = sortedLifecycleSteps(steps);
  const issueCountByStep = lifecycleIssueCountByStep(issues);

  if (orderedSteps.length === 0) {
    return <p className="panel-note">В lifecycle пока нет шагов.</p>;
  }

  return (
    <div className="lifecycle-visual-editor">
      <div className="lifecycle-visual-header">
        <div>
          <strong>{orderedSteps.length} шагов</strong>
          <span>{issues.length > 0 ? `есть замечания: ${issues.length}` : 'runtime валиден'}</span>
        </div>
        <span className={`lifecycle-runtime-pill ${issues.length > 0 ? 'warning' : 'ok'}`}>
          {issues.length > 0 ? 'проверить связи' : 'готово'}
        </span>
      </div>
      <div className="lifecycle-flow-track" role="list" aria-label="Визуальный lifecycle">
        {orderedSteps.map((step, index) => {
          const profile = profiles.find((item) => item.id === step.agentProfileId);
          const runtime = lifecycleRuntimeConfig(step);
          const links = lifecycleLinksForStep(step, index, orderedSteps, runtime);
          const stepIssues = issueCountByStep.get(step.stepKey) ?? 0;
          return (
            <article
              key={step.id || step.stepKey}
              className={`lifecycle-node ${step.required ? '' : 'optional'} ${selectedStepId === step.id ? 'selected' : ''} ${stepIssues > 0 ? 'has-issues' : ''}`}
              role="listitem"
              tabIndex={0}
              onClick={() => onEdit(step)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  onEdit(step);
                }
              }}
            >
              <div className="lifecycle-node-main">
                <span className="lifecycle-node-index">{index + 1}</span>
                <div className="lifecycle-node-copy">
                  <div className="lifecycle-node-title">
                    <strong>{step.title}</strong>
                    <span>{step.mode}</span>
                  </div>
                  <p>{profile?.name ?? 'агент не назначен'} · {profile?.roleKey || 'role не задан'}</p>
                  <code>{step.stepKey}</code>
                </div>
              </div>
              <div className="lifecycle-node-badges">
                <span className={step.required ? 'required' : 'optional'}>{step.required ? 'required' : 'optional'}</span>
                <span>{step.canRetry ? `retry ${step.maxRetries}` : 'no retry'}</span>
                {!step.visibleToUser && <span>hidden</span>}
                {runtime.humanGate && <span>gate</span>}
                {stepIssues > 0 && <span className="warning">{stepIssues} issues</span>}
              </div>
              {links.length > 0 && (
                <div className="lifecycle-node-links">
                  {links.map((link, linkIndex) => (
                    <span key={`${link.kind}-${link.to}-${linkIndex}`} className={`lifecycle-edge ${link.kind}`}>
                      <span>{link.label}</span>
                      <strong>{link.to}</strong>
                    </span>
                  ))}
                </div>
              )}
              <div className="lifecycle-node-actions">
                <button className="small-button secondary" type="button" onClick={(event) => { event.stopPropagation(); onEdit(step); }}>
                  Править
                </button>
                <button className="small-button danger" type="button" onClick={(event) => { event.stopPropagation(); onDelete(step); }}>
                  Удалить
                </button>
              </div>
            </article>
          );
        })}
      </div>
      {issues.length > 0 && (
        <div className="lifecycle-runtime-issues">
          {issues.map((issue, index) => (
            <p key={`${issue.stepKey}-${issue.field}-${index}`}>
              <strong>{issue.stepKey || 'lifecycle'}</strong>
              <span>{issue.field}: {issue.message}</span>
            </p>
          ))}
        </div>
      )}
    </div>
  );
}

function ProjectGroupSelect({
  id,
  label,
  groups,
  value,
  onChange,
}: {
  id: string;
  label: string;
  groups: AgentGroup[];
  value: string;
  onChange: (value: string) => void;
}) {
  const selectedGroup = groups.find((group) => group.id === value);
  return (
    <label className="project-group-select" htmlFor={id}>
      <span>{label}</span>
      <select
        id={id}
        className="input"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={groups.length === 0}
      >
        {groups.map((group) => (
          <option key={group.id} value={group.id}>
            {group.name}
          </option>
        ))}
      </select>
      <small>
        {selectedGroup
          ? `${groupKindLabel(selectedGroup.kind)} · ${selectedGroup.agentCount} агентов`
          : 'Dev Squad будет выбран по умолчанию'}
      </small>
    </label>
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
  if (stepKey === 'web_research') {
    return 'researcher';
  }
  if (stepKey === 'source_review') {
    return 'source_reviewer';
  }
  if (stepKey === 'research_synthesis') {
    return 'analyst';
  }
  if (stepKey === 'research_notes') {
    return 'researcher';
  }
  if (stepKey === 'scope_check' || stepKey === 'artifact_collection' || stepKey === 'triage') {
    return 'ctf_scout';
  }
  if (stepKey === 'category_solver') {
    return 'ctf_web';
  }
  if (stepKey === 'validation') {
    return 'ctf_validator';
  }
  if (stepKey === 'intake' || stepKey === 'hypothesis_board' || stepKey === 'writeup') {
    return 'manager';
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
  if (agentId === 'researcher') {
    return 'Исследователь';
  }
  if (agentId === 'source_reviewer') {
    return 'Проверяющая источники';
  }
  if (agentId === 'analyst') {
    return 'Аналитик';
  }
  if (agentId === 'ctf_scout') {
    return 'Разведчик';
  }
  if (agentId === 'ctf_web') {
    return 'Web Exploiter';
  }
  if (agentId === 'ctf_lfi') {
    return 'LFI Hunter';
  }
  if (agentId === 'ctf_rce') {
    return 'RCE Analyst';
  }
  if (agentId === 'ctf_sqli') {
    return 'SQLi Solver';
  }
  if (agentId === 'ctf_pwn') {
    return 'Pwner';
  }
  if (agentId === 'ctf_crypto') {
    return 'Криптограф';
  }
  if (agentId === 'ctf_reverse') {
    return 'Реверсер';
  }
  if (agentId === 'ctf_forensics') {
    return 'Форензик';
  }
  if (agentId === 'ctf_validator') {
    return 'Валидатор';
  }
  return 'Люмен';
}

function avatarForAgent(agentId: string): string {
  if (agentId === 'security' || agentId.startsWith('ctf_')) {
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
  if (agentId === 'researcher') {
    return {
      title: 'ищет источники',
      why: 'Чтобы актуальные вопросы опирались на публичные источники, а не на догадки модели.',
      responsibility: 'Поисковые запросы, сбор страниц, первичные excerpts, source notes и сохранение research notes.',
    };
  }
  if (agentId === 'source_reviewer') {
    return {
      title: 'проверяет источники',
      why: 'Чтобы в ответ не попадали устаревшие, слабые или противоречивые ссылки.',
      responsibility: 'Свежесть, trust level, прямые ссылки, противоречия и недостающие источники.',
    };
  }
  if (agentId === 'analyst') {
    return {
      title: 'собирает аналитику',
      why: 'Чтобы сравнить источники и отделить подтвержденные факты от выводов.',
      responsibility: 'Синтез, сравнение, ограничения, краткий ответ и цитируемые выводы.',
    };
  }
  if (agentId === 'ctf_scout') {
    return {
      title: 'собирает вводные CTF',
      why: 'Чтобы challenge не решался вслепую и не выходил за scope.',
      responsibility: 'Категория, scope, артефакты, evidence и первые гипотезы.',
    };
  }
  if (agentId === 'ctf_web' || agentId === 'ctf_lfi' || agentId === 'ctf_rce' || agentId === 'ctf_sqli') {
    return {
      title: 'решает web-категорию',
      why: 'Чтобы web/LFI/RCE/SQLi challenge разбирались отдельным профильным агентом.',
      responsibility: 'Гипотезы, безопасные проверки в рамках CTF/lab scope, payload notes и путь к flag.',
    };
  }
  if (agentId === 'ctf_pwn') {
    return {
      title: 'разбирает binary exploitation',
      why: 'Чтобы pwn задачи не смешивались с обычной web-разработкой.',
      responsibility: 'Локальный анализ бинарей, memory safety гипотезы, solver notes и воспроизводимость.',
    };
  }
  if (agentId === 'ctf_crypto') {
    return {
      title: 'решает crypto challenge',
      why: 'Чтобы криптографические задачи шли через математику и solver-скрипты.',
      responsibility: 'Шифры, ключи, oracle-гипотезы, proof notes и аккуратный writeup.',
    };
  }
  if (agentId === 'ctf_reverse') {
    return {
      title: 'разбирает reverse engineering',
      why: 'Чтобы реверс шёл через локальный анализ артефактов, а не через общий dev-пайплайн.',
      responsibility: 'Строки, CFG, псевдокод, форматы файлов, solver notes и evidence.',
    };
  }
  if (agentId === 'ctf_forensics') {
    return {
      title: 'ищет evidence в артефактах',
      why: 'Чтобы forensics задачи разбирались через файлы, метаданные, дампы и сетевые следы.',
      responsibility: 'PCAP, images, EXIF, memory/file carving, timeline и writeup.',
    };
  }
  if (agentId === 'ctf_validator') {
    return {
      title: 'проверяет CTF-решение',
      why: 'Чтобы writeup был воспроизводимым и не содержал выдуманного flag.',
      responsibility: 'Flag/result, evidence, scope, повторяемость шагов и пробелы.',
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
  if (run?.currentStep === 'web_research' || steps.some((step) => step.stepKey === 'web_research')) {
    return researchWorkflowStepOrder;
  }
  if (ctfWorkflowStepOrder.includes(run?.currentStep || '') || steps.some((step) => ctfWorkflowStepOrder.includes(step.stepKey))) {
    return ctfWorkflowStepOrder;
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
  const parts = normalizeInlineMarkdown(text).split(/(`[^`]+`)/g);
  parts.forEach((part, index) => {
    if (part.startsWith('`') && part.endsWith('`') && part.length > 1) {
      nodes.push(<code key={index}>{part.slice(1, -1)}</code>);
      return;
    }
    nodes.push(...renderTextLinks(part, `text-${index}`));
  });
  return nodes;
}

function normalizeInlineMarkdown(text: string): string {
  return text
    .replace(/\\([()[\]])/g, '$1')
    .replace(/\[([^\]]+)\]\(\[(https?:\/\/[^\]\s]+)\]\((https?:\/\/[^)\s]+)\)\)/g, '[$1]($3)');
}

function renderTextLinks(text: string, keyPrefix: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const linkPattern = /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)|(https?:\/\/[^\s<]+)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = linkPattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push(...renderBoldMarkdown(text.slice(lastIndex, match.index), `${keyPrefix}-plain-${nodes.length}`));
    }

    const label = match[1] || match[3] || '';
    const rawUrl = match[2] || match[3] || '';
    const { url, suffix } = splitLinkSuffix(rawUrl);
    nodes.push(
      <a key={`${keyPrefix}-link-${match.index}`} href={url} onClick={(event) => openExternalLink(event, url)}>
        {label}
      </a>,
    );
    if (suffix) {
      nodes.push(suffix);
    }
    lastIndex = linkPattern.lastIndex;
  }

  if (lastIndex < text.length) {
    nodes.push(...renderBoldMarkdown(text.slice(lastIndex), `${keyPrefix}-plain-${nodes.length}`));
  }

  return nodes;
}

function splitLinkSuffix(rawUrl: string): { url: string; suffix: string } {
  const match = rawUrl.match(/^(.+?)([.,;:!?]+)?$/);
  return {
    url: (match?.[1] ?? rawUrl).replace(/\\&/g, '&'),
    suffix: match?.[2] ?? '',
  };
}

function openExternalLink(event: MouseEvent<HTMLAnchorElement>, url: string) {
  event.preventDefault();
  openExternalUrl(url);
}

function openExternalUrl(url: string) {
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return;
    }
    BrowserOpenURL(parsed.toString());
  } catch {
    return;
  }
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

function shortPreview(value: string, limit = 140): string {
  const compact = value.replace(/\s+/g, ' ').trim();
  if (compact.length <= limit) {
    return compact;
  }
  return `${compact.slice(0, limit)}...`;
}

function hostFromUrl(value: string): string {
  try {
    return new URL(value).hostname.replace(/^www\./, '');
  } catch {
    return value;
  }
}

function ctfWorkspaceSections(workspace: CTFWorkspace): Array<{
  key: string;
  label: string;
  kind: string;
  section: CTFWorkspaceSection;
}> {
  return [
    { key: 'challenge', label: 'Задача', kind: 'challenge', section: workspace.challenge },
    { key: 'scope', label: 'Scope', kind: 'scope', section: workspace.scope },
    { key: 'artifacts', label: 'Артефакты', kind: 'artifacts', section: workspace.artifacts },
    { key: 'hypotheses', label: 'Гипотезы', kind: 'hypotheses', section: workspace.hypotheses },
    { key: 'attempts', label: 'Попытки', kind: 'attempts', section: workspace.attempts },
    { key: 'evidence', label: 'Evidence', kind: 'evidence', section: workspace.evidence },
    { key: 'solver', label: 'Solver scripts', kind: 'solver', section: workspace.solver },
    { key: 'writeup', label: 'Writeup', kind: 'writeup', section: workspace.writeup },
  ];
}

function ctfStatusLabel(value: string): string {
  switch (value) {
    case 'done':
    case 'ctf_challenge':
      return 'готово';
    case 'running':
      return 'в работе';
    case 'failed':
      return 'ошибка';
    case 'blocked':
      return 'блокер';
    case 'skipped':
      return 'пропущено';
    default:
      return 'ожидает';
  }
}

function ctfScopeKind(value: string): string {
  switch (value) {
    case 'needs_scope':
      return 'blocked';
    case 'ctf_or_lab_scope':
    case 'local_artifact_scope':
      return 'ok';
    default:
      return 'reviewed';
  }
}

function ctfScopeLabel(value: string): string {
  switch (value) {
    case 'needs_scope':
      return 'нужен scope';
    case 'ctf_or_lab_scope':
      return 'CTF/lab scope';
    case 'local_artifact_scope':
      return 'локальные артефакты';
    case 'reviewed':
      return 'scope проверен';
    default:
      return value;
  }
}

function ctfFileKindLabel(file: CTFWorkspaceFile): string {
  switch (file.kind) {
    case 'ctf_challenge':
      return 'challenge';
    case 'ctf_scope':
      return 'scope';
    case 'ctf_notes':
      return 'notes';
    case 'ctf_writeup':
      return 'writeup';
    case 'artifact':
      return 'artifact';
    case 'evidence':
      return 'evidence';
    case 'solver':
      return 'solver';
    default:
      return file.kind || 'file';
  }
}

function compactWebSources(items: WebSource[]): WebSource[] {
  const byUrl = new Map<string, WebSource>();
  for (const item of items) {
    const key = normalizeSourceUrl(item.url);
    if (!key) {
      continue;
    }
    const previous = byUrl.get(key);
    if (!previous || sourceScore(item) >= sourceScore(previous)) {
      byUrl.set(key, item);
    }
  }
  return Array.from(byUrl.values()).sort((left, right) => sourceTime(right) - sourceTime(left));
}

function normalizeSourceUrl(value: string): string {
  try {
    const parsed = new URL(value);
    parsed.hash = '';
    return parsed.toString().replace(/\/$/, '').toLowerCase();
  } catch {
    return value.trim().toLowerCase();
  }
}

function sourceScore(source: WebSource): number {
  let score = sourceTime(source);
  if (source.contentExcerpt) {
    score += 3;
  }
  if (source.snippet) {
    score += 2;
  }
  if (source.title && source.title !== source.url) {
    score += 1;
  }
  return score;
}

function sourceTime(source: WebSource): number {
  const raw = source.fetchedAt || source.createdAt;
  const value = new Date(raw).getTime();
  return Number.isNaN(value) ? 0 : value;
}

function sourcesTitle(count: number): string {
  if (count % 10 === 1 && count % 100 !== 11) {
    return `Источник ${count}`;
  }
  if ([2, 3, 4].includes(count % 10) && ![12, 13, 14].includes(count % 100)) {
    return `Источника ${count}`;
  }
  return `Источников ${count}`;
}

function sourceTrustKind(value: string): string {
  switch (value) {
    case 'high':
    case 'medium':
    case 'low':
      return value;
    case 'normal':
      return 'medium';
    default:
      return 'unknown';
  }
}

function sourceTrustLabel(value: string): string {
  switch (value) {
    case 'high':
      return 'высокое доверие';
    case 'medium':
    case 'normal':
      return 'среднее доверие';
    case 'low':
      return 'низкое доверие';
    default:
      return 'доверие не оценено';
  }
}

function sourceTypeLabel(value: string): string {
  switch (value) {
    case 'weather':
      return 'погода';
    case 'currency':
      return 'курс валют';
    case 'web':
      return 'страница';
    default:
      return value;
  }
}

function formatSourceDate(source: WebSource): string {
  const date = formatDateTime(source.fetchedAt || source.createdAt);
  return date ? `получено ${date}` : '';
}

function stepIcon(status: string, index: number): ReactNode {
  if (status === 'done') {
    return '✓';
  }
  if (status === 'running') {
    return '';
  }
  if (status === 'failed' || status === 'blocked') {
    return '!';
  }
  if (status === 'skipped') {
    return '-';
  }
  return index;
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

function groupKindLabel(kind: string): string {
  switch (kind) {
    case 'dev':
      return 'разработка';
    case 'ctf':
      return 'CTF';
    case 'research':
      return 'исследования';
    case 'security':
      return 'ИБ';
    default:
      return 'кастомная';
  }
}

type LifecycleRuntimeConfig = {
  returnTo?: string;
  returnToStepKey?: string;
  return_to?: string;
  join?: string;
  joinStepKey?: string;
  parallel?: string[];
  parallelSteps?: string[];
  branches?: Array<{
    next?: string;
    nextStepKey?: string;
    default?: boolean;
  }>;
  humanGate?: {
    reason?: string;
    requiredInputs?: string[];
  };
};

type LifecycleLink = {
  kind: 'success' | 'failure' | 'return' | 'branch' | 'parallel' | 'join';
  label: string;
  to: string;
};

function sortedLifecycleSteps(steps: LifecycleStep[]): LifecycleStep[] {
  return [...steps].sort((a, b) => {
    if (a.sortOrder === b.sortOrder) {
      return a.stepKey.localeCompare(b.stepKey);
    }
    return a.sortOrder - b.sortOrder;
  });
}

function lifecycleIssueCountByStep(issues: LifecycleRuntimeIssue[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const issue of issues) {
    const key = issue.stepKey || 'lifecycle';
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return counts;
}

function lifecycleRuntimeConfig(step: LifecycleStep): LifecycleRuntimeConfig {
  const raw = step.outputSchema.trim();
  if (!raw.startsWith('{')) {
    return {};
  }
  try {
    const parsed = JSON.parse(raw) as LifecycleRuntimeConfig;
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function lifecycleLinksForStep(
  step: LifecycleStep,
  index: number,
  orderedSteps: LifecycleStep[],
  runtime: LifecycleRuntimeConfig,
): LifecycleLink[] {
  const nextStep = orderedSteps[index + 1];
  const links: LifecycleLink[] = [];
  const successTarget = firstNonEmptyString(step.onSuccessStepKey, nextStep?.stepKey ?? '');
  if (successTarget) {
    links.push({ kind: 'success', label: step.onSuccessStepKey ? 'success' : 'next', to: successTarget });
  }
  const returnTarget = firstNonEmptyString(runtime.returnToStepKey ?? '', runtime.returnTo ?? '', runtime.return_to ?? '');
  const failureTarget = firstNonEmptyString(step.onFailureStepKey, returnTarget);
  if (failureTarget) {
    links.push({ kind: 'failure', label: 'failure', to: failureTarget });
  }
  if (returnTarget && returnTarget !== failureTarget) {
    links.push({ kind: 'return', label: 'return', to: returnTarget });
  }
  for (const target of normalizedLifecycleTargets([...(runtime.parallelSteps ?? []), ...(runtime.parallel ?? [])])) {
    links.push({ kind: 'parallel', label: 'parallel', to: target });
  }
  const joinTarget = firstNonEmptyString(runtime.joinStepKey ?? '', runtime.join ?? '');
  if (joinTarget) {
    links.push({ kind: 'join', label: 'join', to: joinTarget });
  }
  for (const branch of runtime.branches ?? []) {
    const target = firstNonEmptyString(branch.nextStepKey ?? '', branch.next ?? '');
    if (target) {
      links.push({ kind: 'branch', label: branch.default ? 'default' : 'branch', to: target });
    }
  }
  return dedupeLifecycleLinks(links);
}

function normalizedLifecycleTargets(targets: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const target of targets) {
    const value = target.trim();
    if (!value || seen.has(value)) {
      continue;
    }
    seen.add(value);
    out.push(value);
  }
  return out;
}

function dedupeLifecycleLinks(links: LifecycleLink[]): LifecycleLink[] {
  const seen = new Set<string>();
  return links.filter((link) => {
    const key = `${link.kind}:${link.label}:${link.to}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function defaultProjectGroupId(groups: AgentGroup[]): string {
  return groups.find((group) => group.id === 'group_dev_squad')?.id ?? groups[0]?.id ?? '';
}

function lifecycleForGroup(groups: AgentGroup[], groupId: string): string {
  return groups.find((group) => group.id === groupId)?.defaultLifecycleId ?? '';
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

function splitDomainList(value: string): string[] {
  return value
    .split(/[\n,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
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
