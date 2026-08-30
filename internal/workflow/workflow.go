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

	StepManagerIntake       = "manager_intake"
	StepProductRequirements = "product_requirements"
	StepTaskBlueprint       = "task_blueprint"
	StepArchitectPlan       = "architect_plan"
	StepSecurityAnalysis    = "security_analysis"
	StepDeveloperPlan       = "developer_plan"
	StepTesterCommands      = "tester_commands"
	StepReview              = "review"
	StepManagerFinal        = "manager_final"
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
