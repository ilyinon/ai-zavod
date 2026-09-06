export namespace agentgroups {
	
	export class Group {
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
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.slug = source["slug"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.defaultModelId = source["defaultModelId"];
	        this.defaultLifecycleId = source["defaultLifecycleId"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.agentCount = source["agentCount"];
	    }
	}
	export class LifecycleDefinition {
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
	
	    static createFrom(source: any = {}) {
	        return new LifecycleDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.groupId = source["groupId"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.maxTotalIterations = source["maxTotalIterations"];
	        this.maxRepairIterations = source["maxRepairIterations"];
	        this.sameErrorLimit = source["sameErrorLimit"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class LifecycleStep {
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
	
	    static createFrom(source: any = {}) {
	        return new LifecycleStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.lifecycleId = source["lifecycleId"];
	        this.stepKey = source["stepKey"];
	        this.title = source["title"];
	        this.agentProfileId = source["agentProfileId"];
	        this.mode = source["mode"];
	        this.required = source["required"];
	        this.canRetry = source["canRetry"];
	        this.maxRetries = source["maxRetries"];
	        this.onSuccessStepKey = source["onSuccessStepKey"];
	        this.onFailureStepKey = source["onFailureStepKey"];
	        this.outputSchema = source["outputSchema"];
	        this.visibleToUser = source["visibleToUser"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class Profile {
	    id: string;
	    groupId: string;
	    name: string;
	    roleKey: string;
	    description: string;
	    avatarPath: string;
	    soulPath: string;
	    modelId: string;
	    toolProfileId: string;
	    defaultSkills: string[];
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
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.groupId = source["groupId"];
	        this.name = source["name"];
	        this.roleKey = source["roleKey"];
	        this.description = source["description"];
	        this.avatarPath = source["avatarPath"];
	        this.soulPath = source["soulPath"];
	        this.modelId = source["modelId"];
	        this.toolProfileId = source["toolProfileId"];
	        this.defaultSkills = source["defaultSkills"];
	        this.capabilities = source["capabilities"];
	        this.allowedTools = source["allowedTools"];
	        this.readPaths = source["readPaths"];
	        this.writePaths = source["writePaths"];
	        this.handoffRules = source["handoffRules"];
	        this.temperature = source["temperature"];
	        this.contextBudget = source["contextBudget"];
	        this.enabled = source["enabled"];
	        this.sortOrder = source["sortOrder"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ProjectBinding {
	    id: string;
	    projectId: string;
	    groupId: string;
	    lifecycleId: string;
	    isDefault: boolean;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectBinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.groupId = source["groupId"];
	        this.lifecycleId = source["lifecycleId"];
	        this.isDefault = source["isDefault"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace agents {
	
	export class Status {
	    id: string;
	    role: string;
	    name: string;
	    status: string;
	    activity: string;
	    modelId: string;
	    toolId: string;
	    soulPath: string;
	    stepKey: string;
	    startedAt: string;
	    elapsedMs: number;
	    inputTokens: number;
	    outputTokens: number;
	    totalTokens: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.activity = source["activity"];
	        this.modelId = source["modelId"];
	        this.toolId = source["toolId"];
	        this.soulPath = source["soulPath"];
	        this.stepKey = source["stepKey"];
	        this.startedAt = source["startedAt"];
	        this.elapsedMs = source["elapsedMs"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.totalTokens = source["totalTokens"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace app {
	
	export class AddAgentFromLibraryInput {
	    groupId: string;
	    libraryAgentId: string;
	    modelId: string;
	
	    static createFrom(source: any = {}) {
	        return new AddAgentFromLibraryInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	        this.libraryAgentId = source["libraryAgentId"];
	        this.modelId = source["modelId"];
	    }
	}
	export class AddExistingProjectInput {
	    name: string;
	    path: string;
	    groupId: string;
	    lifecycleId: string;
	
	    static createFrom(source: any = {}) {
	        return new AddExistingProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.groupId = source["groupId"];
	        this.lifecycleId = source["lifecycleId"];
	    }
	}
	export class AgentGroupTemplateDTO {
	    id: string;
	    name: string;
	    kind: string;
	    description: string;
	    agentCount: number;
	    stepCount: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentGroupTemplateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.agentCount = source["agentCount"];
	        this.stepCount = source["stepCount"];
	    }
	}
	export class AgentLibraryItemDTO {
	    id: string;
	    name: string;
	    roleKey: string;
	    category: string;
	    description: string;
	    toolProfileId: string;
	    defaultSkills: string[];
	    capabilities: string[];
	    allowedTools: string[];
	    readPaths: string[];
	    writePaths: string[];
	    handoffRules: string[];
	    tags: string[];
	
	    static createFrom(source: any = {}) {
	        return new AgentLibraryItemDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.roleKey = source["roleKey"];
	        this.category = source["category"];
	        this.description = source["description"];
	        this.toolProfileId = source["toolProfileId"];
	        this.defaultSkills = source["defaultSkills"];
	        this.capabilities = source["capabilities"];
	        this.allowedTools = source["allowedTools"];
	        this.readPaths = source["readPaths"];
	        this.writePaths = source["writePaths"];
	        this.handoffRules = source["handoffRules"];
	        this.tags = source["tags"];
	    }
	}
	export class AgentSoulDTO {
	    profileId: string;
	    path: string;
	    content: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new AgentSoulDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.path = source["path"];
	        this.content = source["content"];
	        this.warnings = source["warnings"];
	    }
	}
	export class ApplyWorkflowChangesInput {
	    projectId: string;
	    workflowRunId: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyWorkflowChangesInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.workflowRunId = source["workflowRunId"];
	    }
	}
	export class ArchiveAgentGroupInput {
	    groupId: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveAgentGroupInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	    }
	}
	export class BindProjectAgentGroupInput {
	    projectId: string;
	    groupId: string;
	    lifecycleId: string;
	
	    static createFrom(source: any = {}) {
	        return new BindProjectAgentGroupInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.groupId = source["groupId"];
	        this.lifecycleId = source["lifecycleId"];
	    }
	}
	export class CTFWorkspaceFile {
	    kind: string;
	    title: string;
	    relativePath: string;
	
	    static createFrom(source: any = {}) {
	        return new CTFWorkspaceFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.relativePath = source["relativePath"];
	    }
	}
	export class CTFWorkspaceSection {
	    title: string;
	    status: string;
	    content: string;
	    path: string;
	    agentId: string;
	
	    static createFrom(source: any = {}) {
	        return new CTFWorkspaceSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.status = source["status"];
	        this.content = source["content"];
	        this.path = source["path"];
	        this.agentId = source["agentId"];
	    }
	}
	export class CTFWorkspaceDTO {
	    title: string;
	    category: string;
	    scopeStatus: string;
	    root: string;
	    artifactsDir: string;
	    evidenceDir: string;
	    evidenceIndex: string;
	    evidenceEvents: string;
	    solveDir: string;
	    writeupPath: string;
	    challenge: CTFWorkspaceSection;
	    scope: CTFWorkspaceSection;
	    artifacts: CTFWorkspaceSection;
	    hypotheses: CTFWorkspaceSection;
	    attempts: CTFWorkspaceSection;
	    evidence: CTFWorkspaceSection;
	    solver: CTFWorkspaceSection;
	    writeup: CTFWorkspaceSection;
	    files: CTFWorkspaceFile[];
	
	    static createFrom(source: any = {}) {
	        return new CTFWorkspaceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.category = source["category"];
	        this.scopeStatus = source["scopeStatus"];
	        this.root = source["root"];
	        this.artifactsDir = source["artifactsDir"];
	        this.evidenceDir = source["evidenceDir"];
	        this.evidenceIndex = source["evidenceIndex"];
	        this.evidenceEvents = source["evidenceEvents"];
	        this.solveDir = source["solveDir"];
	        this.writeupPath = source["writeupPath"];
	        this.challenge = this.convertValues(source["challenge"], CTFWorkspaceSection);
	        this.scope = this.convertValues(source["scope"], CTFWorkspaceSection);
	        this.artifacts = this.convertValues(source["artifacts"], CTFWorkspaceSection);
	        this.hypotheses = this.convertValues(source["hypotheses"], CTFWorkspaceSection);
	        this.attempts = this.convertValues(source["attempts"], CTFWorkspaceSection);
	        this.evidence = this.convertValues(source["evidence"], CTFWorkspaceSection);
	        this.solver = this.convertValues(source["solver"], CTFWorkspaceSection);
	        this.writeup = this.convertValues(source["writeup"], CTFWorkspaceSection);
	        this.files = this.convertValues(source["files"], CTFWorkspaceFile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ClarificationQuestion {
	    id: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new ClarificationQuestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	    }
	}
	export class ClarificationDTO {
	    workflowRunId: string;
	    summary: string;
	    goal: string;
	    questions: ClarificationQuestion[];
	
	    static createFrom(source: any = {}) {
	        return new ClarificationDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workflowRunId = source["workflowRunId"];
	        this.summary = source["summary"];
	        this.goal = source["goal"];
	        this.questions = this.convertValues(source["questions"], ClarificationQuestion);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectState {
	    projectionRevision: string;
	    requestState?: chat.RequestState;
	    workflowFailure?: store.WorkflowFailure;
	    toolInvocations: toolruntime.Invocation[];
	    project: project.Project;
	    task?: chat.Task;
	    messages: chat.Message[];
	    workflowRun?: workflow.Run;
	    workflowSteps: workflow.Step[];
	    workflowPlan?: workflow.Plan;
	    planSteps: workflow.PlanStep[];
	    artifacts: artifacts.Artifact[];
	    blueprint?: blueprint.Blueprint;
	    clarification?: ClarificationDTO;
	    taskSpec?: taskspec.Spec;
	    projectMemory?: projectmemory.Memory;
	    changes: changes.ProposedChange[];
	    testRuns: checks.TestRun[];
	    reviews: reviews.ReviewRun[];
	    webSources: webresearch.Source[];
	    ctfWorkspace?: CTFWorkspaceDTO;
	    agentGroup?: agentgroups.Group;
	    groupBinding?: agentgroups.ProjectBinding;
	
	    static createFrom(source: any = {}) {
	        return new ProjectState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectionRevision = source["projectionRevision"];
	        this.requestState = this.convertValues(source["requestState"], chat.RequestState);
	        this.workflowFailure = this.convertValues(source["workflowFailure"], store.WorkflowFailure);
	        this.toolInvocations = this.convertValues(source["toolInvocations"], toolruntime.Invocation);
	        this.project = this.convertValues(source["project"], project.Project);
	        this.task = this.convertValues(source["task"], chat.Task);
	        this.messages = this.convertValues(source["messages"], chat.Message);
	        this.workflowRun = this.convertValues(source["workflowRun"], workflow.Run);
	        this.workflowSteps = this.convertValues(source["workflowSteps"], workflow.Step);
	        this.workflowPlan = this.convertValues(source["workflowPlan"], workflow.Plan);
	        this.planSteps = this.convertValues(source["planSteps"], workflow.PlanStep);
	        this.artifacts = this.convertValues(source["artifacts"], artifacts.Artifact);
	        this.blueprint = this.convertValues(source["blueprint"], blueprint.Blueprint);
	        this.clarification = this.convertValues(source["clarification"], ClarificationDTO);
	        this.taskSpec = this.convertValues(source["taskSpec"], taskspec.Spec);
	        this.projectMemory = this.convertValues(source["projectMemory"], projectmemory.Memory);
	        this.changes = this.convertValues(source["changes"], changes.ProposedChange);
	        this.testRuns = this.convertValues(source["testRuns"], checks.TestRun);
	        this.reviews = this.convertValues(source["reviews"], reviews.ReviewRun);
	        this.webSources = this.convertValues(source["webSources"], webresearch.Source);
	        this.ctfWorkspace = this.convertValues(source["ctfWorkspace"], CTFWorkspaceDTO);
	        this.agentGroup = this.convertValues(source["agentGroup"], agentgroups.Group);
	        this.groupBinding = this.convertValues(source["groupBinding"], agentgroups.ProjectBinding);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BootstrapState {
	    chats: chat.Task[];
	    paths: config.Paths;
	    projects: project.Project[];
	    selectedProjectId: string;
	    chat: ProjectState;
	    agents: agents.Status[];
	    models: llm.ModelConfig[];
	    activeModelId: string;
	    webSettings: webresearch.Settings;
	    agentGroups: agentgroups.Group[];
	    agentGroupTemplates: AgentGroupTemplateDTO[];
	    agentLibrary: AgentLibraryItemDTO[];
	
	    static createFrom(source: any = {}) {
	        return new BootstrapState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chats = this.convertValues(source["chats"], chat.Task);
	        this.paths = this.convertValues(source["paths"], config.Paths);
	        this.projects = this.convertValues(source["projects"], project.Project);
	        this.selectedProjectId = source["selectedProjectId"];
	        this.chat = this.convertValues(source["chat"], ProjectState);
	        this.agents = this.convertValues(source["agents"], agents.Status);
	        this.models = this.convertValues(source["models"], llm.ModelConfig);
	        this.activeModelId = source["activeModelId"];
	        this.webSettings = this.convertValues(source["webSettings"], webresearch.Settings);
	        this.agentGroups = this.convertValues(source["agentGroups"], agentgroups.Group);
	        this.agentGroupTemplates = this.convertValues(source["agentGroupTemplates"], AgentGroupTemplateDTO);
	        this.agentLibrary = this.convertValues(source["agentLibrary"], AgentLibraryItemDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class ChatState {
	    projectionRevision: string;
	    requestState?: chat.RequestState;
	    workflowFailure?: store.WorkflowFailure;
	    toolInvocations: toolruntime.Invocation[];
	    project: project.Project;
	    task?: chat.Task;
	    messages: chat.Message[];
	    workflowRun?: workflow.Run;
	    workflowSteps: workflow.Step[];
	    workflowPlan?: workflow.Plan;
	    planSteps: workflow.PlanStep[];
	    artifacts: artifacts.Artifact[];
	    blueprint?: blueprint.Blueprint;
	    clarification?: ClarificationDTO;
	    taskSpec?: taskspec.Spec;
	    projectMemory?: projectmemory.Memory;
	    changes: changes.ProposedChange[];
	    testRuns: checks.TestRun[];
	    reviews: reviews.ReviewRun[];
	    webSources: webresearch.Source[];
	    ctfWorkspace?: CTFWorkspaceDTO;
	    agentGroup?: agentgroups.Group;
	    groupBinding?: agentgroups.ProjectBinding;
	    agents: agents.Status[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectionRevision = source["projectionRevision"];
	        this.requestState = this.convertValues(source["requestState"], chat.RequestState);
	        this.workflowFailure = this.convertValues(source["workflowFailure"], store.WorkflowFailure);
	        this.toolInvocations = this.convertValues(source["toolInvocations"], toolruntime.Invocation);
	        this.project = this.convertValues(source["project"], project.Project);
	        this.task = this.convertValues(source["task"], chat.Task);
	        this.messages = this.convertValues(source["messages"], chat.Message);
	        this.workflowRun = this.convertValues(source["workflowRun"], workflow.Run);
	        this.workflowSteps = this.convertValues(source["workflowSteps"], workflow.Step);
	        this.workflowPlan = this.convertValues(source["workflowPlan"], workflow.Plan);
	        this.planSteps = this.convertValues(source["planSteps"], workflow.PlanStep);
	        this.artifacts = this.convertValues(source["artifacts"], artifacts.Artifact);
	        this.blueprint = this.convertValues(source["blueprint"], blueprint.Blueprint);
	        this.clarification = this.convertValues(source["clarification"], ClarificationDTO);
	        this.taskSpec = this.convertValues(source["taskSpec"], taskspec.Spec);
	        this.projectMemory = this.convertValues(source["projectMemory"], projectmemory.Memory);
	        this.changes = this.convertValues(source["changes"], changes.ProposedChange);
	        this.testRuns = this.convertValues(source["testRuns"], checks.TestRun);
	        this.reviews = this.convertValues(source["reviews"], reviews.ReviewRun);
	        this.webSources = this.convertValues(source["webSources"], webresearch.Source);
	        this.ctfWorkspace = this.convertValues(source["ctfWorkspace"], CTFWorkspaceDTO);
	        this.agentGroup = this.convertValues(source["agentGroup"], agentgroups.Group);
	        this.groupBinding = this.convertValues(source["groupBinding"], agentgroups.ProjectBinding);
	        this.agents = this.convertValues(source["agents"], agents.Status);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ClarificationAnswer {
	    questionId: string;
	    question: string;
	    answer: string;
	
	    static createFrom(source: any = {}) {
	        return new ClarificationAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.questionId = source["questionId"];
	        this.question = source["question"];
	        this.answer = source["answer"];
	    }
	}
	
	
	export class CreateAgentGroupFromTemplateInput {
	    templateId: string;
	    name: string;
	    defaultModelId: string;
	    selectForProjectId: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateAgentGroupFromTemplateInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.templateId = source["templateId"];
	        this.name = source["name"];
	        this.defaultModelId = source["defaultModelId"];
	        this.selectForProjectId = source["selectForProjectId"];
	    }
	}
	export class CreateAgentGroupInput {
	    name: string;
	    kind: string;
	    description: string;
	    defaultModelId: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateAgentGroupInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.defaultModelId = source["defaultModelId"];
	    }
	}
	export class CreateChatInput {
	    projectId: string;

	    static createFrom(source: any = {}) {
	        return new CreateChatInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	    }
	}
	export class CreateProjectInput {
	    name: string;
	    groupId: string;
	    lifecycleId: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.groupId = source["groupId"];
	        this.lifecycleId = source["lifecycleId"];
	    }
	}
	export class DeleteLifecycleStepInput {
	    stepId: string;
	    lifecycleId: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteLifecycleStepInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stepId = source["stepId"];
	        this.lifecycleId = source["lifecycleId"];
	    }
	}
	export class DeleteProjectInput {
	    projectId: string;
	
	    static createFrom(source: any = {}) {
	        return new DeleteProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	    }
	}
	export class DuplicateAgentProfileInput {
	    profileId: string;
	
	    static createFrom(source: any = {}) {
	        return new DuplicateAgentProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	    }
	}
	
	export class ReplaceAgentSoulFromLibraryInput {
	    profileId: string;
	    libraryAgentId: string;
	    replaceContract: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ReplaceAgentSoulFromLibraryInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.libraryAgentId = source["libraryAgentId"];
	        this.replaceContract = source["replaceContract"];
	    }
	}
	export class RollbackWorkflowChangesInput {
	    projectId: string;
	    workflowRunId: string;
	
	    static createFrom(source: any = {}) {
	        return new RollbackWorkflowChangesInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.workflowRunId = source["workflowRunId"];
	    }
	}
	export class RunReviewInput {
	    projectId: string;
	    workflowRunId: string;
	
	    static createFrom(source: any = {}) {
	        return new RunReviewInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.workflowRunId = source["workflowRunId"];
	    }
	}
	export class RunTestCommandInput {
	    projectId: string;
	    testRunId: string;
	
	    static createFrom(source: any = {}) {
	        return new RunTestCommandInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.testRunId = source["testRunId"];
	    }
	}
	export class SaveAgentProfileInput {
	    id: string;
	    groupId: string;
	    name: string;
	    roleKey: string;
	    description: string;
	    avatarPath: string;
	    soulPath: string;
	    modelId: string;
	    toolProfileId: string;
	    defaultSkills: string[];
	    capabilities: string[];
	    allowedTools: string[];
	    readPaths: string[];
	    writePaths: string[];
	    handoffRules: string[];
	    temperature: number;
	    contextBudget: number;
	    enabled: boolean;
	    sortOrder: number;
	
	    static createFrom(source: any = {}) {
	        return new SaveAgentProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.groupId = source["groupId"];
	        this.name = source["name"];
	        this.roleKey = source["roleKey"];
	        this.description = source["description"];
	        this.avatarPath = source["avatarPath"];
	        this.soulPath = source["soulPath"];
	        this.modelId = source["modelId"];
	        this.toolProfileId = source["toolProfileId"];
	        this.defaultSkills = source["defaultSkills"];
	        this.capabilities = source["capabilities"];
	        this.allowedTools = source["allowedTools"];
	        this.readPaths = source["readPaths"];
	        this.writePaths = source["writePaths"];
	        this.handoffRules = source["handoffRules"];
	        this.temperature = source["temperature"];
	        this.contextBudget = source["contextBudget"];
	        this.enabled = source["enabled"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class SaveAgentSoulInput {
	    profileId: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveAgentSoulInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.content = source["content"];
	    }
	}
	export class SaveLifecycleDefinitionInput {
	    id: string;
	    groupId: string;
	    name: string;
	    kind: string;
	    description: string;
	    maxTotalIterations: number;
	    maxRepairIterations: number;
	    sameErrorLimit: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveLifecycleDefinitionInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.groupId = source["groupId"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.maxTotalIterations = source["maxTotalIterations"];
	        this.maxRepairIterations = source["maxRepairIterations"];
	        this.sameErrorLimit = source["sameErrorLimit"];
	        this.status = source["status"];
	    }
	}
	export class SaveLifecycleStepInput {
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
	
	    static createFrom(source: any = {}) {
	        return new SaveLifecycleStepInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.lifecycleId = source["lifecycleId"];
	        this.stepKey = source["stepKey"];
	        this.title = source["title"];
	        this.agentProfileId = source["agentProfileId"];
	        this.mode = source["mode"];
	        this.required = source["required"];
	        this.canRetry = source["canRetry"];
	        this.maxRetries = source["maxRetries"];
	        this.onSuccessStepKey = source["onSuccessStepKey"];
	        this.onFailureStepKey = source["onFailureStepKey"];
	        this.outputSchema = source["outputSchema"];
	        this.visibleToUser = source["visibleToUser"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class SaveModelConfigInput {
	    id: string;
	    name: string;
	    provider: string;
	    baseUrl: string;
	    apiKeyRef: string;
	    modelName: string;
	    isActive: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SaveModelConfigInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKeyRef = source["apiKeyRef"];
	        this.modelName = source["modelName"];
	        this.isActive = source["isActive"];
	    }
	}
	export class SaveWebSettingsInput {
	    enabled: boolean;
	    maxResults: number;
	    maxPagesPerWorkflow: number;
	    timeoutSeconds: number;
	    allowedDomains: string[];
	    blockedDomains: string[];
	
	    static createFrom(source: any = {}) {
	        return new SaveWebSettingsInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.maxResults = source["maxResults"];
	        this.maxPagesPerWorkflow = source["maxPagesPerWorkflow"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.allowedDomains = source["allowedDomains"];
	        this.blockedDomains = source["blockedDomains"];
	    }
	}
	export class SendMessageInput {
	    resumeWorkflowRunId: string;
	    routingAnswerFor: string;
	    toolConsentModelId: string;
	    taskId: string;
	    projectId: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SendMessageInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.resumeWorkflowRunId = source["resumeWorkflowRunId"];
	        this.routingAnswerFor = source["routingAnswerFor"];
	        this.toolConsentModelId = source["toolConsentModelId"];
	        this.taskId = source["taskId"];
	        this.projectId = source["projectId"];
	        this.content = source["content"];
	    }
	}
	export class SetAgentProfileEnabledInput {
	    profileId: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SetAgentProfileEnabledInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.enabled = source["enabled"];
	    }
	}
	export class SubmitClarificationInput {
	    projectId: string;
	    workflowRunId: string;
	    answers: ClarificationAnswer[];
	
	    static createFrom(source: any = {}) {
	        return new SubmitClarificationInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.answers = this.convertValues(source["answers"], ClarificationAnswer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateAgentGroupInput {
	    id: string;
	    name: string;
	    kind: string;
	    description: string;
	    defaultModelId: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateAgentGroupInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.description = source["description"];
	        this.defaultModelId = source["defaultModelId"];
	    }
	}
	export class UpdateChatInput {
	    groupId: string;
	    modelId: string;
	    taskId: string;
	    title: string;
	    projectId: string;
	    pinned: boolean;
	    archived: boolean;

	    static createFrom(source: any = {}) {
	        return new UpdateChatInput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	        this.modelId = source["modelId"];
	        this.taskId = source["taskId"];
	        this.title = source["title"];
	        this.projectId = source["projectId"];
	        this.pinned = source["pinned"];
	        this.archived = source["archived"];
	    }
	}
	export class UpdateProjectInput {
	    projectId: string;
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}

}

export namespace artifacts {
	
	export class Artifact {
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
	
	    static createFrom(source: any = {}) {
	        return new Artifact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.agentId = source["agentId"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.path = source["path"];
	        this.relativePath = source["relativePath"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

export namespace blueprint {
	
	export class TestCommand {
	    command: string;
	    working_dir: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new TestCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.command = source["command"];
	        this.working_dir = source["working_dir"];
	        this.reason = source["reason"];
	    }
	}
	export class DependencyPolicy {
	    policy: string;
	    items: string[];
	
	    static createFrom(source: any = {}) {
	        return new DependencyPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.policy = source["policy"];
	        this.items = source["items"];
	    }
	}
	export class ExpectedFile {
	    path: string;
	    action: string;
	    purpose: string;
	
	    static createFrom(source: any = {}) {
	        return new ExpectedFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.action = source["action"];
	        this.purpose = source["purpose"];
	    }
	}
	export class Blueprint {
	    id: string;
	    projectId: string;
	    taskId: string;
	    workflowRunId: string;
	    stack: string;
	    runtime: string;
	    projectType: string;
	    scaffoldRequired: boolean;
	    entrypoints: string[];
	    expectedFiles: ExpectedFile[];
	    forbiddenFiles: string[];
	    dependencies: DependencyPolicy;
	    testCommands: TestCommand[];
	    openQuestions: string[];
	    confidence: string;
	    rawJson: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Blueprint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.stack = source["stack"];
	        this.runtime = source["runtime"];
	        this.projectType = source["projectType"];
	        this.scaffoldRequired = source["scaffoldRequired"];
	        this.entrypoints = source["entrypoints"];
	        this.expectedFiles = this.convertValues(source["expectedFiles"], ExpectedFile);
	        this.forbiddenFiles = source["forbiddenFiles"];
	        this.dependencies = this.convertValues(source["dependencies"], DependencyPolicy);
	        this.testCommands = this.convertValues(source["testCommands"], TestCommand);
	        this.openQuestions = source["openQuestions"];
	        this.confidence = source["confidence"];
	        this.rawJson = source["rawJson"];
	        this.createdAt = source["createdAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace changes {
	
	export class ProposedChange {
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
	
	    static createFrom(source: any = {}) {
	        return new ProposedChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.agentId = source["agentId"];
	        this.filePath = source["filePath"];
	        this.action = source["action"];
	        this.content = source["content"];
	        this.reason = source["reason"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.backupPath = source["backupPath"];
	        this.beforeContent = source["beforeContent"];
	        this.afterContent = source["afterContent"];
	        this.diffText = source["diffText"];
	        this.createdAt = source["createdAt"];
	        this.appliedAt = source["appliedAt"];
	    }
	}

}

export namespace chat {
	
	export class Message {
	    id: string;
	    taskId: string;
	    role: string;
	    agentId: string;
	    content: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.role = source["role"];
	        this.agentId = source["agentId"];
	        this.content = source["content"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class RequestState {
	    id: string;
	    sequence: number;
	    mode: string;
	    original: string;
	    question?: string;
	    workflowRunId?: string;

	    static createFrom(source: any = {}) {
	        return new RequestState(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sequence = source["sequence"];
	        this.mode = source["mode"];
	        this.original = source["original"];
	        this.question = source["question"];
	        this.workflowRunId = source["workflowRunId"];
	    }
	}
	export class Task {
	    groupId: string;
	    modelId: string;
	    id: string;
	    projectId: string;
	    title: string;
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	    pinned: boolean;
	    pendingRequest?: string;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groupId = source["groupId"];
	        this.modelId = source["modelId"];
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.pinned = source["pinned"];
	        this.pendingRequest = source["pendingRequest"];
	    }
	}

}

export namespace checks {
	
	export class TestRun {
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
	
	    static createFrom(source: any = {}) {
	        return new TestRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.command = source["command"];
	        this.workingDir = source["workingDir"];
	        this.reason = source["reason"];
	        this.status = source["status"];
	        this.exitCode = source["exitCode"];
	        this.stdout = source["stdout"];
	        this.stderr = source["stderr"];
	        this.error = source["error"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

export namespace config {
	
	export class Paths {
	    codeDir: string;
	    projectsDir: string;
	    agentsDir: string;
	    dbPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Paths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codeDir = source["codeDir"];
	        this.projectsDir = source["projectsDir"];
	        this.agentsDir = source["agentsDir"];
	        this.dbPath = source["dbPath"];
	    }
	}

}

export namespace llm {
	
	export class ModelConfig {
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
	
	    static createFrom(source: any = {}) {
	        return new ModelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKeyRef = source["apiKeyRef"];
	        this.modelName = source["modelName"];
	        this.isActive = source["isActive"];
	        this.status = source["status"];
	        this.lastCheckedAt = source["lastCheckedAt"];
	        this.lastError = source["lastError"];
	        this.latencyMs = source["latencyMs"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ProviderError {
	    kind: string;
	    retryable: boolean;
	    modelId: string;
	    httpStatus?: number;
	    attempt: number;
	    maxAttempts: number;
	    elapsedMs: number;

	    static createFrom(source: any = {}) {
	        return new ProviderError(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.retryable = source["retryable"];
	        this.modelId = source["modelId"];
	        this.httpStatus = source["httpStatus"];
	        this.attempt = source["attempt"];
	        this.maxAttempts = source["maxAttempts"];
	        this.elapsedMs = source["elapsedMs"];
	    }
	}

}

export namespace project {
	
	export class Project {
	    id: string;
	    name: string;
	    path: string;
	    createdAt: string;
	    lastOpenedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.createdAt = source["createdAt"];
	        this.lastOpenedAt = source["lastOpenedAt"];
	    }
	}

}

export namespace projectmemory {
	
	export class Memory {
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
	
	    static createFrom(source: any = {}) {
	        return new Memory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.architecture = source["architecture"];
	        this.stack = source["stack"];
	        this.runtime = source["runtime"];
	        this.projectType = source["projectType"];
	        this.buildCommands = source["buildCommands"];
	        this.testCommands = source["testCommands"];
	        this.styleGuide = source["styleGuide"];
	        this.decisions = source["decisions"];
	        this.environment = source["environment"];
	        this.updatedFromTaskId = source["updatedFromTaskId"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace reviews {
	
	export class Finding {
	    category?: string;
	    severity: string;
	    file_path: string;
	    message: string;
	    suggestion: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.severity = source["severity"];
	        this.file_path = source["file_path"];
	        this.message = source["message"];
	        this.suggestion = source["suggestion"];
	    }
	}
	export class ReviewRun {
	    id: string;
	    projectId: string;
	    taskId: string;
	    workflowRunId: string;
	    status: string;
	    summary: string;
	    findings: Finding[];
	    requiredChanges: string[];
	    recommendedNextStep: string;
	    returnTo: string;
	    iteration: number;
	    blockingReason: string;
	    error: string;
	    startedAt: string;
	    finishedAt: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.status = source["status"];
	        this.summary = source["summary"];
	        this.findings = this.convertValues(source["findings"], Finding);
	        this.requiredChanges = source["requiredChanges"];
	        this.recommendedNextStep = source["recommendedNextStep"];
	        this.returnTo = source["returnTo"];
	        this.iteration = source["iteration"];
	        this.blockingReason = source["blockingReason"];
	        this.error = source["error"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.createdAt = source["createdAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace store {

	export class WorkflowFailure {
	    runId: string;
	    stepKey: string;
	    kind: string;
	    message: string;
	    provider?: llm.ProviderError;
	    canResume: boolean;
	    resumeFingerprint?: string;

	    static createFrom(source: any = {}) {
	        return new WorkflowFailure(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.runId = source["runId"];
	        this.stepKey = source["stepKey"];
	        this.kind = source["kind"];
	        this.message = source["message"];
	        this.provider = this.convertValues(source["provider"], llm.ProviderError);
	        this.canResume = source["canResume"];
	        this.resumeFingerprint = source["resumeFingerprint"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace taskspec {
	
	export class AcceptedAnswer {
	    questionId: string;
	    question: string;
	    answer: string;
	
	    static createFrom(source: any = {}) {
	        return new AcceptedAnswer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.questionId = source["questionId"];
	        this.question = source["question"];
	        this.answer = source["answer"];
	    }
	}
	export class Spec {
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
	    acceptedAnswers: AcceptedAnswer[];
	    status: string;
	    source: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Spec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.userRequest = source["userRequest"];
	        this.summary = source["summary"];
	        this.goal = source["goal"];
	        this.requirements = source["requirements"];
	        this.acceptanceCriteria = source["acceptanceCriteria"];
	        this.decisions = source["decisions"];
	        this.openQuestions = source["openQuestions"];
	        this.acceptedAnswers = this.convertValues(source["acceptedAnswers"], AcceptedAnswer);
	        this.status = source["status"];
	        this.source = source["source"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace toolruntime {

	export class Result {
	    status: string;
	    output?: string;
	    error?: string;
	    exitCode?: number;
	    truncated: boolean;

	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.exitCode = source["exitCode"];
	        this.truncated = source["truncated"];
	    }
	}
	export class Invocation {
	    projectId: string;
	    taskId: string;
	    agentId: string;
	    agentName: string;
	    modelId: string;
	    workflowRunId?: string;
	    workingDir: string;
	    toolProfileId: string;
	    id: string;
	    loopId: string;
	    callId: string;
	    tool: string;
	    arguments: string;
	    result: Result;
	    startedAt: string;
	    finishedAt?: string;

	    static createFrom(source: any = {}) {
	        return new Invocation(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.agentId = source["agentId"];
	        this.agentName = source["agentName"];
	        this.modelId = source["modelId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.workingDir = source["workingDir"];
	        this.toolProfileId = source["toolProfileId"];
	        this.id = source["id"];
	        this.loopId = source["loopId"];
	        this.callId = source["callId"];
	        this.tool = source["tool"];
	        this.arguments = source["arguments"];
	        this.result = this.convertValues(source["result"], Result);
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace webresearch {
	
	export class Settings {
	    enabled: boolean;
	    maxResults: number;
	    maxPagesPerWorkflow: number;
	    timeoutSeconds: number;
	    allowedDomains: string[];
	    blockedDomains: string[];
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.maxResults = source["maxResults"];
	        this.maxPagesPerWorkflow = source["maxPagesPerWorkflow"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.allowedDomains = source["allowedDomains"];
	        this.blockedDomains = source["blockedDomains"];
	    }
	}
	export class Source {
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
	
	    static createFrom(source: any = {}) {
	        return new Source(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.agentId = source["agentId"];
	        this.query = source["query"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.snippet = source["snippet"];
	        this.contentExcerpt = source["contentExcerpt"];
	        this.sourceType = source["sourceType"];
	        this.trustLevel = source["trustLevel"];
	        this.fetchedAt = source["fetchedAt"];
	        this.createdAt = source["createdAt"];
	    }
	}

}

export namespace workflow {
	
	export class Plan {
	    id: string;
	    projectId: string;
	    taskId: string;
	    workflowRunId: string;
	    title: string;
	    status: string;
	    currentStepId: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.taskId = source["taskId"];
	        this.workflowRunId = source["workflowRunId"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.currentStepId = source["currentStepId"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class PlanStep {
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
	
	    static createFrom(source: any = {}) {
	        return new PlanStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.planId = source["planId"];
	        this.stepKey = source["stepKey"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.agentId = source["agentId"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.error = source["error"];
	        this.sortOrder = source["sortOrder"];
	    }
	}
	export class Run {
	    id: string;
	    taskId: string;
	    status: string;
	    currentStep: string;
	    startedAt: string;
	    finishedAt: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.taskId = source["taskId"];
	        this.status = source["status"];
	        this.currentStep = source["currentStep"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.error = source["error"];
	    }
	}
	export class Step {
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
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workflowRunId = source["workflowRunId"];
	        this.stepKey = source["stepKey"];
	        this.agentId = source["agentId"];
	        this.status = source["status"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.error = source["error"];
	    }
	}

}
