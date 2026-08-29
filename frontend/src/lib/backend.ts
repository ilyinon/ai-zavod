export type AppPaths = {
  codeDir: string;
  projectsDir: string;
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

export type ProjectState = {
  project: Project;
  task?: Task;
  messages: Message[];
  workflowRun?: WorkflowRun;
  workflowSteps: WorkflowStep[];
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
};

type WailsApp = {
  Bootstrap(): Promise<BootstrapState>;
  ListProjects(query: string): Promise<Project[]>;
  CreateProject(input: { name: string }): Promise<Project>;
  AddExistingProject(input: { name: string; path: string }): Promise<Project>;
  SelectProject(projectId: string): Promise<ProjectState>;
  SendMessage(input: { projectId: string; content: string }): Promise<ChatState>;
  SaveModelConfig(input: ModelConfig): Promise<ModelConfig[]>;
  SetActiveModel(modelId: string): Promise<ModelConfig[]>;
  CheckModel(modelId: string): Promise<ModelConfig[]>;
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
  createProject: (name: string) => app().CreateProject({ name }),
  addExistingProject: (name: string, path: string) => app().AddExistingProject({ name, path }),
  selectProject: (projectId: string) => app().SelectProject(projectId),
  sendMessage: (projectId: string, content: string) => app().SendMessage({ projectId, content }),
  saveModelConfig: (model: ModelConfig) => app().SaveModelConfig(model),
  setActiveModel: (modelId: string) => app().SetActiveModel(modelId),
  checkModel: (modelId: string) => app().CheckModel(modelId),
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
