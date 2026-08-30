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
	    project: project.Project;
	    task?: chat.Task;
	    messages: chat.Message[];
	    workflowRun?: workflow.Run;
	    workflowSteps: workflow.Step[];
	    artifacts: artifacts.Artifact[];
	    blueprint?: blueprint.Blueprint;
	    clarification?: ClarificationDTO;
	    changes: changes.ProposedChange[];
	    testRuns: checks.TestRun[];
	    reviews: reviews.ReviewRun[];
	
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
	        this.artifacts = this.convertValues(source["artifacts"], artifacts.Artifact);
	        this.blueprint = this.convertValues(source["blueprint"], blueprint.Blueprint);
	        this.clarification = this.convertValues(source["clarification"], ClarificationDTO);
	        this.changes = this.convertValues(source["changes"], changes.ProposedChange);
	        this.testRuns = this.convertValues(source["testRuns"], checks.TestRun);
	        this.reviews = this.convertValues(source["reviews"], reviews.ReviewRun);
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
	    artifacts: artifacts.Artifact[];
	    blueprint?: blueprint.Blueprint;
	    clarification?: ClarificationDTO;
	    changes: changes.ProposedChange[];
	    testRuns: checks.TestRun[];
	    reviews: reviews.ReviewRun[];
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
	        this.artifacts = this.convertValues(source["artifacts"], artifacts.Artifact);
	        this.blueprint = this.convertValues(source["blueprint"], blueprint.Blueprint);
	        this.clarification = this.convertValues(source["clarification"], ClarificationDTO);
	        this.changes = this.convertValues(source["changes"], changes.ProposedChange);
	        this.testRuns = this.convertValues(source["testRuns"], checks.TestRun);
	        this.reviews = this.convertValues(source["reviews"], reviews.ReviewRun);
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

export namespace reviews {
	
	export class Finding {
	    severity: string;
	    file_path: string;
	    message: string;
	    suggestion: string;
	
	    static createFrom(source: any = {}) {
	        return new Finding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
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

