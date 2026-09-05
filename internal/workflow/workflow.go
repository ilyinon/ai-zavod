package workflow

const (
	StatusRunning     = "running"
	StatusWaitingUser = "waiting_user"
	StatusDone        = "done"
	StatusFailed      = "failed"
	StatusBlocked     = "blocked"

	StepStatusQueued  = "queued"
	StepStatusRunning = "running"
	StepStatusDone    = "done"
	StepStatusFailed  = "failed"
	StepStatusSkipped = "skipped"

	StepUserPlan             = "user_plan"
	StepManagerIntake        = "manager_intake"
	StepProductRequirements  = "product_requirements"
	StepTaskBlueprint        = "task_blueprint"
	StepArchitectPlan        = "architect_plan"
	StepSecurityAnalysis     = "security_analysis"
	StepWebResearch          = "web_research"
	StepResearchSourceReview = "source_review"
	StepResearchSynthesis    = "research_synthesis"
	StepResearchNotes        = "research_notes"
	StepDeveloperPlan        = "developer_plan"
	StepTesterCommands       = "tester_commands"
	StepReview               = "review"
	StepManagerFinal         = "manager_final"

	StepCTFIntake             = "intake"
	StepCTFScopeCheck         = "scope_check"
	StepCTFArtifactCollection = "artifact_collection"
	StepCTFTriage             = "triage"
	StepCTFHypothesisBoard    = "hypothesis_board"
	StepCTFCategorySolver     = "category_solver"
	StepCTFValidation         = "validation"
	StepCTFWriteup            = "writeup"
)

type Run struct {
	ID          string `json:"id"`
	TaskID      string `json:"taskId"`
	Status      string `json:"status"`
	CurrentStep string `json:"currentStep"`
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt"`
	Error       string `json:"error"`
}

type Step struct {
	ID            string `json:"id"`
	WorkflowRunID string `json:"workflowRunId"`
	StepKey       string `json:"stepKey"`
	AgentID       string `json:"agentId"`
	Status        string `json:"status"`
	Input         string `json:"input"`
	Output        string `json:"output"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	Error         string `json:"error"`
}

type Plan struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	TaskID        string `json:"taskId"`
	WorkflowRunID string `json:"workflowRunId"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	CurrentStepID string `json:"currentStepId"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type PlanStep struct {
	ID          string `json:"id"`
	PlanID      string `json:"planId"`
	StepKey     string `json:"stepKey"`
	Title       string `json:"title"`
	Description string `json:"description"`
	AgentID     string `json:"agentId"`
	Status      string `json:"status"`
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt"`
	Error       string `json:"error"`
	SortOrder   int    `json:"sortOrder"`
}
