export type AppPaths = {
  codeDir: string;
  projectsDir: string;
  agentsDir: string;
  dbPath: string;
};

export type Project = {
  id: string;
  name: string;
  path: string;
  createdAt: string;
  lastOpenedAt: string;
};

export type Task = {
  id: string;
  projectId: string;
  title: string;
  status: string;
  createdAt: string;
  updatedAt: string;
};

export type Message = {
  id: string;
  taskId: string;
  role: 'user' | 'agent';
  agentId: string;
  content: string;
  createdAt: string;
};

export type AgentStatus = {
  id: string;
  role: string;
  name: string;
  status: string;
  activity: string;
  modelId: string;
  updatedAt: string;
};

export type AgentGroup = {
  id: string;
  name: string;
  slug: string;
  kind: string;
  description: string;
  defaultModelId: string;
  defaultLifecycleId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
  agentCount: number;
};

export type AgentGroupTemplate = {
  id: string;
  name: string;
  kind: string;
  description: string;
  agentCount: number;
  stepCount: number;
};

export type AgentLibraryItem = {
  id: string;
  name: string;
  roleKey: string;
  category: string;
  description: string;
  toolProfileId: string;
  capabilities: string[];
  allowedTools: string[];
  readPaths: string[];
  writePaths: string[];
  handoffRules: string[];
  tags: string[];
};

export type AgentProfile = {
  id: string;
  groupId: string;
  name: string;
  roleKey: string;
  description: string;
  avatarPath: string;
  soulPath: string;
  modelId: string;
  toolProfileId: string;
  capabilities: string[];
  allowedTools: string[];
  readPaths: string[];
  writePaths: string[];
  handoffRules: string[];
  temperature: number;
  contextBudget: number;
  enabled: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
};

export type AgentSoul = {
  profileId: string;
  path: string;
  content: string;
  warnings: string[];
};

export type LifecycleDefinition = {
  id: string;
  groupId: string;
  name: string;
  kind: string;
  description: string;
  maxTotalIterations: number;
  maxRepairIterations: number;
  sameErrorLimit: number;
  status: string;
  createdAt: string;
  updatedAt: string;
};

export type LifecycleStep = {
  id: string;
  lifecycleId: string;
  stepKey: string;
  title: string;
  agentProfileId: string;
  mode: string;
  required: boolean;
  canRetry: boolean;
  maxRetries: number;
  onSuccessStepKey: string;
  onFailureStepKey: string;
  outputSchema: string;
  visibleToUser: boolean;
  sortOrder: number;
};

export type ProjectGroupBinding = {
  id: string;
  projectId: string;
  groupId: string;
  lifecycleId: string;
  isDefault: boolean;
  createdAt: string;
  updatedAt: string;
};

export type WorkflowRun = {
  id: string;
  taskId: string;
  status: string;
  currentStep: string;
  startedAt: string;
  finishedAt: string;
  error: string;
};

export type WorkflowStep = {
  id: string;
  workflowRunId: string;
  stepKey: string;
  agentId: string;
  status: string;
  input: string;
  output: string;
  startedAt: string;
  finishedAt: string;
  error: string;
};

export type WorkflowPlan = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  title: string;
  status: string;
  currentStepId: string;
  createdAt: string;
  updatedAt: string;
};

export type WorkflowPlanStep = {
  id: string;
  planId: string;
  stepKey: string;
  title: string;
  description: string;
  agentId: string;
  status: string;
  startedAt: string;
  finishedAt: string;
  error: string;
  sortOrder: number;
};

export type Artifact = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  agentId: string;
  kind: string;
  title: string;
  path: string;
  relativePath: string;
  createdAt: string;
};

export type ClarificationQuestion = {
  id: string;
  text: string;
};

export type PendingClarification = {
  workflowRunId: string;
  summary: string;
  goal: string;
  questions: ClarificationQuestion[];
};

export type ClarificationAnswer = {
  questionId: string;
  question: string;
  answer: string;
};

export type BlueprintExpectedFile = {
  path: string;
  action: string;
  purpose: string;
};

export type BlueprintDependencyPolicy = {
  policy: string;
  items: string[];
};

export type BlueprintTestCommand = {
  command: string;
  working_dir: string;
  reason: string;
};

export type TaskBlueprint = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  stack: string;
  runtime: string;
  projectType: string;
  scaffoldRequired: boolean;
  entrypoints: string[];
  expectedFiles: BlueprintExpectedFile[];
  forbiddenFiles: string[];
  dependencies: BlueprintDependencyPolicy;
  testCommands: BlueprintTestCommand[];
  openQuestions: string[];
  confidence: string;
  rawJson: string;
  createdAt: string;
};

export type ProposedChange = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  agentId: string;
  filePath: string;
  action: string;
  content: string;
  reason: string;
  status: string;
  error: string;
  backupPath: string;
  beforeContent: string;
  afterContent: string;
  diffText: string;
  createdAt: string;
  appliedAt: string;
};

export type TestRun = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  command: string;
  workingDir: string;
  reason: string;
  status: string;
  exitCode: number;
  stdout: string;
  stderr: string;
  error: string;
  startedAt: string;
  finishedAt: string;
  createdAt: string;
};

export type ReviewFinding = {
  severity: string;
  file_path: string;
  message: string;
  suggestion: string;
};

export type ReviewRun = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  status: string;
  summary: string;
  findings: ReviewFinding[];
  requiredChanges: string[];
  recommendedNextStep: string;
  returnTo: string;
  iteration: number;
  blockingReason: string;
  error: string;
  startedAt: string;
  finishedAt: string;
  createdAt: string;
};

export type WebSource = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  agentId: string;
  query: string;
  title: string;
  url: string;
  snippet: string;
  contentExcerpt: string;
  sourceType: string;
  trustLevel: string;
  fetchedAt: string;
  createdAt: string;
};

export type WebSettings = {
  enabled: boolean;
  maxResults: number;
  maxPagesPerWorkflow: number;
  timeoutSeconds: number;
  allowedDomains: string[];
  blockedDomains: string[];
};

export type ModelConfig = {
  id: string;
  name: string;
  provider: string;
  baseUrl: string;
  apiKeyRef: string;
  modelName: string;
  isActive: boolean;
  status: string;
  lastCheckedAt: string;
  lastError: string;
  latencyMs: number;
  createdAt: string;
  updatedAt: string;
};

export type AgentMessageDelta = {
  taskId: string;
  agentId: string;
  delta: string;
  done: boolean;
  error?: string;
};

export type TaskSpecAnswer = {
  questionId: string;
  question: string;
  answer: string;
};

export type TaskSpec = {
  id: string;
  projectId: string;
  taskId: string;
  workflowRunId: string;
  userRequest: string;
  summary: string;
  goal: string;
  requirements: string[];
  acceptanceCriteria: string[];
  decisions: string[];
  openQuestions: string[];
  acceptedAnswers: TaskSpecAnswer[];
  status: string;
  source: string;
  createdAt: string;
  updatedAt: string;
};

export type ProjectMemory = {
  id: string;
  projectId: string;
  architecture: string;
  stack: string;
  runtime: string;
  projectType: string;
  buildCommands: string[];
  testCommands: string[];
  styleGuide: string[];
  decisions: string[];
  environment: string[];
  updatedFromTaskId: string;
  createdAt: string;
  updatedAt: string;
};

export type ProjectState = {
  project: Project;
  task?: Task;
  messages: Message[];
  workflowRun?: WorkflowRun;
  workflowSteps: WorkflowStep[];
  workflowPlan?: WorkflowPlan;
  planSteps: WorkflowPlanStep[];
  artifacts: Artifact[];
  blueprint?: TaskBlueprint;
  clarification?: PendingClarification;
  taskSpec?: TaskSpec;
  projectMemory?: ProjectMemory;
  changes: ProposedChange[];
  testRuns: TestRun[];
  reviews: ReviewRun[];
  webSources: WebSource[];
  agentGroup?: AgentGroup;
  groupBinding?: ProjectGroupBinding;
};

export type ChatState = ProjectState & {
  agents: AgentStatus[];
  error?: string;
};

export type BootstrapState = {
  paths: AppPaths;
  projects: Project[];
  selectedProjectId: string;
  chat: ProjectState;
  agents: AgentStatus[];
  models: ModelConfig[];
  activeModelId: string;
  webSettings: WebSettings;
  agentGroups: AgentGroup[];
  agentGroupTemplates: AgentGroupTemplate[];
  agentLibrary: AgentLibraryItem[];
};

type WailsApp = {
  Bootstrap(): Promise<BootstrapState>;
  ListProjects(query: string): Promise<Project[]>;
  CreateProject(input: { name: string; groupId: string; lifecycleId: string }): Promise<Project>;
  AddExistingProject(input: { name: string; path: string; groupId: string; lifecycleId: string }): Promise<Project>;
  UpdateProject(input: { projectId: string; name: string; path: string }): Promise<Project>;
  DeleteProject(input: { projectId: string }): Promise<BootstrapState>;
  SelectProject(projectId: string): Promise<ProjectState>;
  SendMessage(input: { projectId: string; content: string }): Promise<ChatState>;
  SubmitClarification(input: { projectId: string; workflowRunId: string; answers: ClarificationAnswer[] }): Promise<ChatState>;
  ApplyWorkflowChanges(input: { projectId: string; workflowRunId: string }): Promise<ChatState>;
  RollbackWorkflowChanges(input: { projectId: string; workflowRunId: string }): Promise<ChatState>;
  RunTestCommand(input: { projectId: string; testRunId: string }): Promise<ChatState>;
  RunReview(input: { projectId: string; workflowRunId: string }): Promise<ChatState>;
  SaveModelConfig(input: ModelConfig): Promise<ModelConfig[]>;
  SetActiveModel(modelId: string): Promise<ModelConfig[]>;
  CheckModel(modelId: string): Promise<ModelConfig[]>;
  SaveWebSettings(input: WebSettings): Promise<WebSettings>;
  ListAgentGroups(): Promise<AgentGroup[]>;
  ListAgentGroupTemplates(): Promise<AgentGroupTemplate[]>;
  ListAgentLibrary(): Promise<AgentLibraryItem[]>;
  CreateAgentGroup(input: { name: string; kind: string; description: string; defaultModelId: string }): Promise<AgentGroup[]>;
  CreateAgentGroupFromTemplate(input: {
    templateId: string;
    name: string;
    defaultModelId: string;
    selectForProjectId: string;
  }): Promise<AgentGroup[]>;
  UpdateAgentGroup(input: { id: string; name: string; kind: string; description: string; defaultModelId: string }): Promise<AgentGroup[]>;
  ArchiveAgentGroup(input: { groupId: string }): Promise<AgentGroup[]>;
  ListAgentProfiles(groupId: string): Promise<AgentProfile[]>;
  SaveAgentProfile(input: AgentProfile): Promise<AgentProfile[]>;
  AddAgentFromLibrary(input: { groupId: string; libraryAgentId: string; modelId: string }): Promise<AgentProfile[]>;
  DuplicateAgentProfile(input: { profileId: string }): Promise<AgentProfile[]>;
  ReplaceAgentSoulFromLibrary(input: {
    profileId: string;
    libraryAgentId: string;
    replaceContract: boolean;
  }): Promise<AgentSoul>;
  SetAgentProfileEnabled(input: { profileId: string; enabled: boolean }): Promise<AgentProfile[]>;
  GetAgentSoul(profileId: string): Promise<AgentSoul>;
  SaveAgentSoul(input: { profileId: string; content: string }): Promise<AgentSoul>;
  ListLifecycleDefinitions(groupId: string): Promise<LifecycleDefinition[]>;
  ListLifecycleSteps(lifecycleId: string): Promise<LifecycleStep[]>;
  SaveLifecycleDefinition(input: {
    id: string;
    groupId: string;
    name: string;
    kind: string;
    description: string;
    maxTotalIterations: number;
    maxRepairIterations: number;
    sameErrorLimit: number;
    status: string;
  }): Promise<LifecycleDefinition[]>;
  SaveLifecycleStep(input: LifecycleStep): Promise<LifecycleStep[]>;
  DeleteLifecycleStep(input: { stepId: string; lifecycleId: string }): Promise<LifecycleStep[]>;
  BindProjectAgentGroup(input: { projectId: string; groupId: string; lifecycleId: string }): Promise<ProjectState>;
};

declare global {
  interface Window {
    go?: {
      main?: {
        App?: WailsApp;
      };
    };
    runtime?: {
      EventsOn?: (name: string, callback: (data: unknown) => void) => (() => void) | void;
    };
  }
}

function app(): WailsApp {
  const bridge = window.go?.main?.App;
  if (!bridge) {
    throw new Error('Wails bridge недоступен. Запусти приложение через wails dev или собранный .app.');
  }
  return bridge;
}

export const backend = {
  bootstrap: () => app().Bootstrap(),
  listProjects: (query: string) => app().ListProjects(query),
  createProject: (name: string, groupId = '', lifecycleId = '') => app().CreateProject({ name, groupId, lifecycleId }),
  addExistingProject: (name: string, path: string, groupId = '', lifecycleId = '') =>
    app().AddExistingProject({ name, path, groupId, lifecycleId }),
  updateProject: (projectId: string, name: string, path: string) => app().UpdateProject({ projectId, name, path }),
  deleteProject: (projectId: string) => app().DeleteProject({ projectId }),
  selectProject: (projectId: string) => app().SelectProject(projectId),
  sendMessage: (projectId: string, content: string) => app().SendMessage({ projectId, content }),
  submitClarification: (projectId: string, workflowRunId: string, answers: ClarificationAnswer[]) =>
    app().SubmitClarification({ projectId, workflowRunId, answers }),
  applyWorkflowChanges: (projectId: string, workflowRunId: string) =>
    app().ApplyWorkflowChanges({ projectId, workflowRunId }),
  rollbackWorkflowChanges: (projectId: string, workflowRunId: string) =>
    app().RollbackWorkflowChanges({ projectId, workflowRunId }),
  runTestCommand: (projectId: string, testRunId: string) => app().RunTestCommand({ projectId, testRunId }),
  runReview: (projectId: string, workflowRunId: string) => app().RunReview({ projectId, workflowRunId }),
  saveModelConfig: (model: ModelConfig) => app().SaveModelConfig(model),
  setActiveModel: (modelId: string) => app().SetActiveModel(modelId),
  checkModel: (modelId: string) => app().CheckModel(modelId),
  saveWebSettings: (settings: WebSettings) => app().SaveWebSettings(settings),
  listAgentGroups: () => app().ListAgentGroups(),
  listAgentGroupTemplates: () => app().ListAgentGroupTemplates(),
  listAgentLibrary: () => app().ListAgentLibrary(),
  createAgentGroup: (input: { name: string; kind: string; description: string; defaultModelId: string }) =>
    app().CreateAgentGroup(input),
  createAgentGroupFromTemplate: (input: {
    templateId: string;
    name?: string;
    defaultModelId?: string;
    selectForProjectId?: string;
  }) =>
    app().CreateAgentGroupFromTemplate({
      templateId: input.templateId,
      name: input.name ?? '',
      defaultModelId: input.defaultModelId ?? '',
      selectForProjectId: input.selectForProjectId ?? '',
    }),
  updateAgentGroup: (input: { id: string; name: string; kind: string; description: string; defaultModelId: string }) =>
    app().UpdateAgentGroup(input),
  archiveAgentGroup: (groupId: string) => app().ArchiveAgentGroup({ groupId }),
  listAgentProfiles: (groupId: string) => app().ListAgentProfiles(groupId),
  saveAgentProfile: (profile: AgentProfile) => app().SaveAgentProfile(profile),
  addAgentFromLibrary: (groupId: string, libraryAgentId: string, modelId = '') =>
    app().AddAgentFromLibrary({ groupId, libraryAgentId, modelId }),
  duplicateAgentProfile: (profileId: string) => app().DuplicateAgentProfile({ profileId }),
  replaceAgentSoulFromLibrary: (profileId: string, libraryAgentId: string, replaceContract = true) =>
    app().ReplaceAgentSoulFromLibrary({ profileId, libraryAgentId, replaceContract }),
  setAgentProfileEnabled: (profileId: string, enabled: boolean) =>
    app().SetAgentProfileEnabled({ profileId, enabled }),
  getAgentSoul: (profileId: string) => app().GetAgentSoul(profileId),
  saveAgentSoul: (profileId: string, content: string) => app().SaveAgentSoul({ profileId, content }),
  listLifecycleDefinitions: (groupId: string) => app().ListLifecycleDefinitions(groupId),
  listLifecycleSteps: (lifecycleId: string) => app().ListLifecycleSteps(lifecycleId),
  saveLifecycleDefinition: (input: {
    id: string;
    groupId: string;
    name: string;
    kind: string;
    description: string;
    maxTotalIterations: number;
    maxRepairIterations: number;
    sameErrorLimit: number;
    status: string;
  }) => app().SaveLifecycleDefinition(input),
  saveLifecycleStep: (input: LifecycleStep) => app().SaveLifecycleStep(input),
  deleteLifecycleStep: (stepId: string, lifecycleId: string) => app().DeleteLifecycleStep({ stepId, lifecycleId }),
  bindProjectAgentGroup: (projectId: string, groupId: string, lifecycleId: string) =>
    app().BindProjectAgentGroup({ projectId, groupId, lifecycleId }),
  onAgentStatusChanged: (callback: (status: AgentStatus) => void) =>
    subscribe('agent_status_changed', (data) => callback(data as AgentStatus)),
  onChatStateChanged: (callback: (state: ChatState) => void) =>
    subscribe('chat_state_changed', (data) => callback(data as ChatState)),
  onAgentMessageDelta: (callback: (delta: AgentMessageDelta) => void) =>
    subscribe('agent_message_delta', (data) => callback(data as AgentMessageDelta)),
  onWorkflowRunChanged: (callback: (run: WorkflowRun) => void) =>
    subscribe('workflow_run_changed', (data) => callback((data as { run: WorkflowRun }).run)),
  onWorkflowStepChanged: (callback: (step: WorkflowStep) => void) =>
    subscribe('workflow_step_changed', (data) => callback((data as { step: WorkflowStep }).step)),
  onModelsChanged: (callback: (models: ModelConfig[]) => void) =>
    subscribe('models_changed', (data) => callback(data as ModelConfig[])),
};

function subscribe(name: string, callback: (data: unknown) => void): () => void {
  const unsubscribe = window.runtime?.EventsOn?.(name, callback);
  if (typeof unsubscribe === 'function') {
    return unsubscribe;
  }
  return () => {};
}
