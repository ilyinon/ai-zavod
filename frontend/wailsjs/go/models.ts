export namespace agents {
	
	export class Status {
	    id: string;
	    role: string;
	    name: string;
	    status: string;
	    activity: string;
	    modelId: string;
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
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace app {
	
	export class AddExistingProjectInput {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new AddExistingProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class ProjectState {
	    project: project.Project;
	    task?: chat.Task;
	    messages: chat.Message[];
	    workflowRun?: workflow.Run;
	    workflowSteps: workflow.Step[];
	
	    static createFrom(source: any = {}) {
	        return new ProjectState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], project.Project);
	        this.task = this.convertValues(source["task"], chat.Task);
	        this.messages = this.convertValues(source["messages"], chat.Message);
	        this.workflowRun = this.convertValues(source["workflowRun"], workflow.Run);
	        this.workflowSteps = this.convertValues(source["workflowSteps"], workflow.Step);
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
	    paths: config.Paths;
	    projects: project.Project[];
	    selectedProjectId: string;
	    chat: ProjectState;
	    agents: agents.Status[];
	    models: llm.ModelConfig[];
	    activeModelId: string;
	
	    static createFrom(source: any = {}) {
	        return new BootstrapState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.paths = this.convertValues(source["paths"], config.Paths);
	        this.projects = this.convertValues(source["projects"], project.Project);
	        this.selectedProjectId = source["selectedProjectId"];
	        this.chat = this.convertValues(source["chat"], ProjectState);
	        this.agents = this.convertValues(source["agents"], agents.Status);
	        this.models = this.convertValues(source["models"], llm.ModelConfig);
	        this.activeModelId = source["activeModelId"];
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
	    project: project.Project;
	    task?: chat.Task;
	    messages: chat.Message[];
	    workflowRun?: workflow.Run;
	    workflowSteps: workflow.Step[];
	    agents: agents.Status[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = this.convertValues(source["project"], project.Project);
	        this.task = this.convertValues(source["task"], chat.Task);
	        this.messages = this.convertValues(source["messages"], chat.Message);
	        this.workflowRun = this.convertValues(source["workflowRun"], workflow.Run);
	        this.workflowSteps = this.convertValues(source["workflowSteps"], workflow.Step);
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
	export class CreateProjectInput {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
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
	export class SendMessageInput {
	    projectId: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SendMessageInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.content = source["content"];
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
	export class Task {
	    id: string;
	    projectId: string;
	    title: string;
	    status: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

export namespace config {
	
	export class Paths {
	    codeDir: string;
	    projectsDir: string;
	    dbPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Paths(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codeDir = source["codeDir"];
	        this.projectsDir = source["projectsDir"];
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

export namespace workflow {
	
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

