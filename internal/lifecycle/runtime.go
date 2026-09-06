package lifecycle

import (
	"encoding/json"
	"fmt"
	"strings"

	"zavod_ai/internal/agentgroups"
	zw "zavod_ai/internal/workflow"
)

const (
	ModeLLM       = "llm"
	ModeTool      = "tool"
	ModeChecks    = "checks"
	ModeReview    = "review"
	ModeArtifact  = "artifact"
	ModeFinal     = "final"
	ModeHumanGate = "human_gate"
	ModeBranch    = "branch"
	ModeParallel  = "parallel"
	ModeJoin      = "join"

	ActionRun         = "run"
	ActionRunParallel = "run_parallel"
	ActionRetry       = "retry"
	ActionJump        = "jump"
	ActionWaitHuman   = "wait_human"
	ActionComplete    = "complete"
	ActionBlocked     = "blocked"
	ActionSkip        = "skip"
)

type RuntimeState struct {
	InFlight        map[string]int
	ExecutionCounts map[string]int
	TransitionCount int
	RepairCount     int
	LastFailure     string
	LastFailureStep string
	CurrentStepKey  string
	Results         map[string]StepResult
	Attempts        map[string]int
	Variables       map[string]string
	HumanGates      map[string]bool
}

type StepResult struct {
	Terminal bool
	StepKey  string
	Status   string
	Output   string
	Error    string
}

type RuntimeDecision struct {
	Action         string
	Step           agentgroups.LifecycleStep
	Steps          []agentgroups.LifecycleStep
	NextStepKey    string
	Reason         string
	RequiredInputs []string
}

type StepRuntimeConfig struct {
	Condition       Condition        `json:"condition,omitempty"`
	Conditions      []Condition      `json:"conditions,omitempty"`
	Branches        []BranchRule     `json:"branches,omitempty"`
	ParallelSteps   []string         `json:"parallelSteps,omitempty"`
	Parallel        []string         `json:"parallel,omitempty"`
	ParallelWait    string           `json:"parallelWait,omitempty"`
	JoinStepKey     string           `json:"joinStepKey,omitempty"`
	Join            string           `json:"join,omitempty"`
	HumanGate       HumanGateConfig  `json:"humanGate,omitempty"`
	CompletionRules []CompletionRule `json:"completion,omitempty"`
	ReturnToStepKey string           `json:"returnToStepKey,omitempty"`
	ReturnTo        string           `json:"returnTo,omitempty"`
	ReturnToSnake   string           `json:"return_to,omitempty"`
	Critical        bool             `json:"critical,omitempty"`
}

type Condition struct {
	Field    string   `json:"field,omitempty"`
	StepKey  string   `json:"stepKey,omitempty"`
	Operator string   `json:"operator,omitempty"`
	Value    string   `json:"value,omitempty"`
	Values   []string `json:"values,omitempty"`
	Negate   bool     `json:"negate,omitempty"`
}

type BranchRule struct {
	When        Condition `json:"when,omitempty"`
	NextStepKey string    `json:"nextStepKey,omitempty"`
	Next        string    `json:"next,omitempty"`
	Default     bool      `json:"default,omitempty"`
}

type HumanGateConfig struct {
	Reason         string   `json:"reason,omitempty"`
	RequiredInputs []string `json:"requiredInputs,omitempty"`
}

type CompletionRule struct {
	When        Condition `json:"when,omitempty"`
	Status      string    `json:"status,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	NextStepKey string    `json:"nextStepKey,omitempty"`
	Next        string    `json:"next,omitempty"`
}

type ValidationIssue struct {
	StepKey string
	Field   string
	Message string
}

func ParseStepRuntimeConfig(raw string) (StepRuntimeConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return StepRuntimeConfig{}, nil
	}
	if !strings.HasPrefix(raw, "{") {
		return StepRuntimeConfig{}, nil
	}
	var cfg StepRuntimeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return StepRuntimeConfig{}, fmt.Errorf("invalid lifecycle runtime config: %w", err)
	}
	cfg.ParallelWait = strings.ToLower(strings.TrimSpace(cfg.ParallelWait))
	return cfg, nil
}

func (e Executor) NextAction(state RuntimeState) RuntimeDecision {
	state = normalizeState(state)
	current, ok := e.currentStep(state.CurrentStepKey)
	if !ok {
		return RuntimeDecision{Action: ActionComplete, Reason: "lifecycle has no remaining steps"}
	}
	cfg, err := ParseStepRuntimeConfig(current.OutputSchema)
	if result := state.Results[current.StepKey]; result.Terminal {
		return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: result.Error}
	}
	if err != nil {
		return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: err.Error()}
	}
	if !conditionsMatch(state, current.StepKey, cfg) {
		next, found := e.Next(current.StepKey, true)
		if !found {
			return RuntimeDecision{Action: ActionComplete, Step: current, Reason: "step condition is false and no next step exists"}
		}
		return RuntimeDecision{Action: ActionSkip, Step: current, NextStepKey: next.StepKey, Reason: "step condition is false"}
	}

	switch normalizeMode(current.Mode) {
	case ModeHumanGate:
		if !state.HumanGates[current.StepKey] {
			return RuntimeDecision{
				Action:         ActionWaitHuman,
				Step:           current,
				Reason:         firstNonEmpty(cfg.HumanGate.Reason, "human gate requires user confirmation"),
				RequiredInputs: append([]string(nil), cfg.HumanGate.RequiredInputs...),
			}
		}
		return e.successorDecision(current, "human gate approved")
	case ModeBranch:
		return e.branchDecision(state, current, cfg)
	case ModeParallel:
		return e.parallelDecision(state, current, cfg)
	case ModeJoin:
		if !e.parallelComplete(state, cfg.ParallelSteps, "all") {
			return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: "join is waiting for parallel steps"}
		}
	}

	result, hasResult := state.Results[current.StepKey]
	if !hasResult || result.Status == "" || result.Status == zw.StepStatusQueued || result.Status == zw.StepStatusRunning {
		return RuntimeDecision{Action: ActionRun, Step: current, Reason: "step is ready"}
	}
	if terminal := e.completionDecision(state, current, cfg); terminal.Action != "" {
		return terminal
	}
	if isFailureStatus(result.Status) {
		if e.canRetryFromState(current, state) {
			return RuntimeDecision{Action: ActionRetry, Step: current, Reason: "step failed and retry budget remains"}
		}
		target := firstNonEmpty(returnTarget(cfg), current.OnFailureStepKey)
		if target != "" {
			return RuntimeDecision{Action: ActionJump, Step: current, NextStepKey: target, Reason: "step failed and has return target"}
		}
		if current.Required || cfg.Critical {
			return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: "required step failed without retry or return target"}
		}
		return e.successorDecision(current, "optional step failed")
	}
	if result.Status == zw.StatusWaitingUser || result.Status == zw.StatusBlocked {
		return RuntimeDecision{Action: ActionWaitHuman, Step: current, Reason: firstNonEmpty(result.Error, "step is waiting for user input")}
	}
	return e.successorDecision(current, "step completed")
}

func (e Executor) ValidateRuntime() []ValidationIssue {
	var issues []ValidationIssue
	seen := map[string]bool{}
	for _, step := range e.steps {
		if step.StepKey == "" {
			issues = append(issues, ValidationIssue{Field: "step_key", Message: "step key is required"})
			continue
		}
		if seen[step.StepKey] {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: "step_key", Message: "duplicate step key"})
		}
		seen[step.StepKey] = true
		mode := normalizeMode(step.Mode)
		if !isKnownMode(mode) {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: "mode", Message: "unknown lifecycle mode"})
		}
		cfg, err := ParseStepRuntimeConfig(step.OutputSchema)
		if err != nil {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: "output_schema", Message: err.Error()})
			continue
		}
		issues = append(issues, e.validateStepRefs(step, cfg)...)
	}
	return issues
}

func (e Executor) currentStep(currentStepKey string) (agentgroups.LifecycleStep, bool) {
	currentStepKey = strings.TrimSpace(currentStepKey)
	if currentStepKey != "" {
		if step, ok := e.Step(currentStepKey); ok {
			return step, true
		}
	}
	for _, step := range e.steps {
		return step, true
	}
	return agentgroups.LifecycleStep{}, false
}

func (e Executor) branchDecision(state RuntimeState, current agentgroups.LifecycleStep, cfg StepRuntimeConfig) RuntimeDecision {
	for _, branch := range cfg.Branches {
		target := branchTarget(branch)
		if target == "" {
			continue
		}
		if branch.Default || evaluateCondition(state, current.StepKey, branch.When) {
			if isCompletionTarget(target) {
				return RuntimeDecision{Action: ActionComplete, Step: current, Reason: "branch completed lifecycle"}
			}
			return RuntimeDecision{Action: ActionJump, Step: current, NextStepKey: target, Reason: "branch condition matched"}
		}
	}
	return e.successorDecision(current, "branch has no matching rule")
}

func (e Executor) parallelDecision(state RuntimeState, current agentgroups.LifecycleStep, cfg StepRuntimeConfig) RuntimeDecision {
	keys := parallelTargets(cfg)
	if len(keys) == 0 {
		return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: "parallel step has no parallelSteps config"}
	}
	var runnable []agentgroups.LifecycleStep
	for _, key := range keys {
		step, ok := e.Step(key)
		if !ok {
			return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: "parallel target not found: " + key}
		}
		result := state.Results[key]
		if result.Status == "" || result.Status == zw.StepStatusQueued || result.Status == zw.StepStatusRunning {
			runnable = append(runnable, step)
		}
	}
	if len(runnable) > 0 {
		return RuntimeDecision{Action: ActionRunParallel, Step: current, Steps: runnable, Reason: "parallel steps are ready"}
	}
	wait := firstNonEmpty(cfg.ParallelWait, "all")
	if e.parallelComplete(state, keys, wait) {
		target := firstNonEmpty(joinTarget(cfg), current.OnSuccessStepKey)
		if target != "" {
			return RuntimeDecision{Action: ActionJump, Step: current, NextStepKey: target, Reason: "parallel group completed"}
		}
		return e.successorDecision(current, "parallel group completed")
	}
	return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: "parallel group failed or did not satisfy completion policy"}
}

func (e Executor) parallelComplete(state RuntimeState, keys []string, wait string) bool {
	keys = normalizedKeys(keys)
	if len(keys) == 0 {
		return true
	}
	wait = firstNonEmpty(strings.ToLower(strings.TrimSpace(wait)), "all")
	done := 0
	for _, key := range keys {
		status := state.Results[key].Status
		if status == zw.StepStatusDone || status == zw.StepStatusSkipped {
			done++
		}
	}
	if wait == "any" {
		return done > 0
	}
	return done == len(keys)
}

func (e Executor) completionDecision(state RuntimeState, current agentgroups.LifecycleStep, cfg StepRuntimeConfig) RuntimeDecision {
	for _, rule := range cfg.CompletionRules {
		if !evaluateCondition(state, current.StepKey, rule.When) {
			continue
		}
		status := firstNonEmpty(rule.Status, zw.StatusDone)
		reason := firstNonEmpty(rule.Reason, "completion rule matched")
		switch status {
		case zw.StatusDone, "complete", "completed":
			return RuntimeDecision{Action: ActionComplete, Step: current, Reason: reason}
		case zw.StatusBlocked, zw.StepStatusFailed:
			return RuntimeDecision{Action: ActionBlocked, Step: current, Reason: reason}
		case zw.StatusWaitingUser:
			return RuntimeDecision{Action: ActionWaitHuman, Step: current, Reason: reason}
		default:
			if target := completionTarget(rule); target != "" {
				return RuntimeDecision{Action: ActionJump, Step: current, NextStepKey: target, Reason: reason}
			}
		}
	}
	if normalizeMode(current.Mode) == ModeFinal {
		if result := state.Results[current.StepKey]; result.Status == zw.StepStatusDone {
			return RuntimeDecision{Action: ActionComplete, Step: current, Reason: "final step completed"}
		}
	}
	return RuntimeDecision{}
}

func (e Executor) successorDecision(current agentgroups.LifecycleStep, reason string) RuntimeDecision {
	next, ok := e.Next(current.StepKey, true)
	if !ok {
		return RuntimeDecision{Action: ActionComplete, Step: current, Reason: reason}
	}
	return RuntimeDecision{Action: ActionJump, Step: current, NextStepKey: next.StepKey, Reason: reason}
}

func (e Executor) canRetryFromState(step agentgroups.LifecycleStep, state RuntimeState) bool {
	if !step.CanRetry {
		return false
	}
	limit := step.MaxRetries
	if limit <= 0 {
		return false
	}
	return state.Attempts[step.StepKey] < limit
}

func (e Executor) validateStepRefs(step agentgroups.LifecycleStep, cfg StepRuntimeConfig) []ValidationIssue {
	var issues []ValidationIssue
	for field, key := range map[string]string{
		"on_success_step_key": step.OnSuccessStepKey,
		"on_failure_step_key": step.OnFailureStepKey,
		"returnToStepKey":     returnTarget(cfg),
		"joinStepKey":         joinTarget(cfg),
	} {
		key = strings.TrimSpace(key)
		if key == "" || isCompletionTarget(key) {
			continue
		}
		if _, ok := e.Step(key); !ok {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: field, Message: "target step not found: " + key})
		}
	}
	for _, branch := range cfg.Branches {
		key := branchTarget(branch)
		if key == "" || isCompletionTarget(key) {
			continue
		}
		if _, ok := e.Step(key); !ok {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: "branches.nextStepKey", Message: "target step not found: " + key})
		}
	}
	for _, key := range parallelTargets(cfg) {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := e.Step(key); !ok {
			issues = append(issues, ValidationIssue{StepKey: step.StepKey, Field: "parallelSteps", Message: "parallel target not found: " + key})
		}
	}
	return issues
}

func conditionsMatch(state RuntimeState, currentStepKey string, cfg StepRuntimeConfig) bool {
	if !evaluateCondition(state, currentStepKey, cfg.Condition) {
		return false
	}
	for _, condition := range cfg.Conditions {
		if !evaluateCondition(state, currentStepKey, condition) {
			return false
		}
	}
	return true
}

func evaluateCondition(state RuntimeState, currentStepKey string, condition Condition) bool {
	field := strings.TrimSpace(condition.Field)
	operator := strings.ToLower(strings.TrimSpace(condition.Operator))
	value := condition.Value
	if field == "" && operator == "" && value == "" && len(condition.Values) == 0 && condition.StepKey == "" {
		return true
	}
	if operator == "" {
		operator = "exists"
	}
	actual := conditionValue(state, currentStepKey, condition)
	var matched bool
	switch operator {
	case "always", "true":
		matched = true
	case "equals", "eq", "status_is":
		matched = actual == value
	case "not_equals", "neq", "status_not":
		matched = actual != value
	case "contains":
		matched = strings.Contains(strings.ToLower(actual), strings.ToLower(value))
	case "not_contains":
		matched = !strings.Contains(strings.ToLower(actual), strings.ToLower(value))
	case "exists":
		matched = strings.TrimSpace(actual) != ""
	case "missing":
		matched = strings.TrimSpace(actual) == ""
	case "in":
		for _, item := range condition.Values {
			if actual == item {
				matched = true
				break
			}
		}
	default:
		matched = false
	}
	if condition.Negate {
		return !matched
	}
	return matched
}

func conditionValue(state RuntimeState, currentStepKey string, condition Condition) string {
	field := strings.TrimSpace(condition.Field)
	stepKey := firstNonEmpty(condition.StepKey, currentStepKey)
	result := state.Results[stepKey]
	switch {
	case field == "status":
		return result.Status
	case field == "output":
		return result.Output
	case field == "error":
		return result.Error
	case strings.HasPrefix(field, "var:"):
		return state.Variables[strings.TrimPrefix(field, "var:")]
	default:
		if value, ok := state.Variables[field]; ok {
			return value
		}
		return result.Output
	}
}

func normalizeState(state RuntimeState) RuntimeState {
	if state.Results == nil {
		state.Results = map[string]StepResult{}
	}
	if state.Attempts == nil {
		state.Attempts = map[string]int{}
	}
	if state.Variables == nil {
		state.Variables = map[string]string{}
	}
	if state.HumanGates == nil {
		state.HumanGates = map[string]bool{}
	}
	return state
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ModeLLM
	}
	return mode
}

func isKnownMode(mode string) bool {
	switch normalizeMode(mode) {
	case ModeLLM, ModeTool, ModeChecks, ModeReview, ModeArtifact, ModeFinal, ModeHumanGate, ModeBranch, ModeParallel, ModeJoin:
		return true
	default:
		return false
	}
}

func isFailureStatus(status string) bool {
	switch status {
	case zw.StepStatusFailed, zw.StatusBlocked, "needs_work":
		return true
	default:
		return false
	}
}

func isCompletionTarget(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "done", "complete", "completed", "end", "finish":
		return true
	default:
		return false
	}
}

func normalizedKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func parallelTargets(cfg StepRuntimeConfig) []string {
	return normalizedKeys(append(append([]string{}, cfg.ParallelSteps...), cfg.Parallel...))
}

func branchTarget(rule BranchRule) string {
	return firstNonEmpty(rule.NextStepKey, rule.Next)
}

func completionTarget(rule CompletionRule) string {
	return firstNonEmpty(rule.NextStepKey, rule.Next)
}

func joinTarget(cfg StepRuntimeConfig) string {
	return firstNonEmpty(cfg.JoinStepKey, cfg.Join)
}

func returnTarget(cfg StepRuntimeConfig) string {
	return firstNonEmpty(cfg.ReturnToStepKey, cfg.ReturnTo, cfg.ReturnToSnake)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
