package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/projectmemory"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/taskspec"
)

func TestStoreSeedsAgentGroupsAndProjectBinding(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if err := s.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure default models: %v", err)
	}
	if err := s.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure default agent groups: %v", err)
	}

	groups, err := s.ListAgentGroups(ctx, false)
	if err != nil {
		t.Fatalf("list agent groups: %v", err)
	}
	if len(groups) < 2 {
		t.Fatalf("expected seeded groups, got %#v", groups)
	}

	var devGroup agentgroups.Group
	var ctfGroup agentgroups.Group
	var researchGroup agentgroups.Group
	var securityGroup agentgroups.Group
	for _, group := range groups {
		if group.ID == "group_dev_squad" {
			devGroup = group
		}
		if group.ID == "group_ctf_cell" {
			ctfGroup = group
		}
		if group.ID == "group_research_squad" {
			researchGroup = group
		}
		if group.ID == "group_security_audit" {
			securityGroup = group
		}
	}
	if devGroup.ID == "" || devGroup.DefaultLifecycleID != "lifecycle_dev_default" || devGroup.AgentCount < 6 {
		t.Fatalf("unexpected Dev Squad seed: %#v", devGroup)
	}
	if ctfGroup.ID == "" || ctfGroup.DefaultLifecycleID != "lifecycle_ctf_default" || ctfGroup.AgentCount < 11 {
		t.Fatalf("unexpected CTF Cell seed: %#v", ctfGroup)
	}
	if researchGroup.ID == "" || researchGroup.DefaultLifecycleID != "lifecycle_research_default" || researchGroup.AgentCount < 4 {
		t.Fatalf("unexpected Research Squad seed: %#v", researchGroup)
	}
	if securityGroup.ID == "" || securityGroup.DefaultLifecycleID != "lifecycle_security_default" || securityGroup.AgentCount < 5 {
		t.Fatalf("unexpected Security Audit seed: %#v", securityGroup)
	}

	profiles, err := s.ListAgentProfiles(ctx, devGroup.ID)
	if err != nil {
		t.Fatalf("list dev profiles: %v", err)
	}
	if len(profiles) < 6 {
		t.Fatalf("expected dev profiles, got %#v", profiles)
	}
	if len(profiles[0].Capabilities) == 0 || len(profiles[0].AllowedTools) == 0 || len(profiles[0].HandoffRules) == 0 {
		t.Fatalf("expected seeded dev profile capabilities, got %#v", profiles[0])
	}
	devSkills := map[string]bool{}
	for _, skill := range profiles[0].DefaultSkills {
		devSkills[skill] = true
	}
	if !devSkills["pony-tail"] {
		t.Fatalf("expected seeded profile to include pony-tail skill, got %#v", profiles[0].DefaultSkills)
	}
	ctfProfiles, err := s.ListAgentProfiles(ctx, ctfGroup.ID)
	if err != nil {
		t.Fatalf("list ctf profiles: %v", err)
	}
	ctfRoles := map[string]bool{}
	for _, profile := range ctfProfiles {
		ctfRoles[profile.RoleKey] = true
	}
	for _, role := range []string{"ctf_web", "ctf_lfi", "ctf_rce", "ctf_sqli", "ctf_pwn", "ctf_crypto", "ctf_reverse", "ctf_forensics"} {
		if !ctfRoles[role] {
			t.Fatalf("expected CTF role %s in seed, got %#v", role, ctfRoles)
		}
	}
	for _, profile := range ctfProfiles {
		if strings.HasPrefix(profile.RoleKey, "ctf_") && (len(profile.Capabilities) == 0 || len(profile.ReadPaths) == 0 || len(profile.WritePaths) == 0) {
			t.Fatalf("expected CTF profile capability contract, got %#v", profile)
		}
		if strings.HasPrefix(profile.RoleKey, "ctf_") && !containsString(profile.DefaultSkills, "ctf") {
			t.Fatalf("expected CTF profile to include ctf skill, got %#v", profile.DefaultSkills)
		}
	}
	for profileID, want := range map[string]string{
		"tool_ctf_pwn":       "pwntools",
		"tool_ctf_forensics": "binwalk",
		"tool_ctf_validator": "validator.py",
	} {
		var allowed string
		if err := s.db.QueryRowContext(ctx, `SELECT allowed_commands_json FROM tool_profiles WHERE id = ?`, profileID).Scan(&allowed); err != nil {
			t.Fatalf("query tool profile %s: %v", profileID, err)
		}
		if !strings.Contains(allowed, want) {
			t.Fatalf("expected tool profile %s to mention %q, got %s", profileID, want, allowed)
		}
	}
	researchProfiles, err := s.ListAgentProfiles(ctx, researchGroup.ID)
	if err != nil {
		t.Fatalf("list research profiles: %v", err)
	}
	researchRoles := map[string]bool{}
	for _, profile := range researchProfiles {
		researchRoles[profile.RoleKey] = true
	}
	for _, role := range []string{"manager", "researcher", "source_reviewer", "analyst"} {
		if !researchRoles[role] {
			t.Fatalf("expected Research role %s in seed, got %#v", role, researchRoles)
		}
	}
	securityProfiles, err := s.ListAgentProfiles(ctx, securityGroup.ID)
	if err != nil {
		t.Fatalf("list security profiles: %v", err)
	}
	if len(securityProfiles) < 5 || !containsString(securityProfiles[1].DefaultSkills, "security") {
		t.Fatalf("expected Security Audit profiles with security skill, got %#v", securityProfiles)
	}

	steps, err := s.ListLifecycleSteps(ctx, devGroup.DefaultLifecycleID)
	if err != nil {
		t.Fatalf("list dev lifecycle steps: %v", err)
	}
	if len(steps) != 8 || steps[0].StepKey != "manager_intake" || steps[len(steps)-1].StepKey != "manager_final" {
		t.Fatalf("unexpected dev lifecycle steps: %#v", steps)
	}
	researchSteps, err := s.ListLifecycleSteps(ctx, researchGroup.DefaultLifecycleID)
	if err != nil {
		t.Fatalf("list research lifecycle steps: %v", err)
	}
	if len(researchSteps) != 5 || researchSteps[0].StepKey != "web_research" || researchSteps[len(researchSteps)-1].StepKey != "manager_final" {
		t.Fatalf("unexpected research lifecycle steps: %#v", researchSteps)
	}

	project, err := s.CreateProject(ctx, "Проект с группой", filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	binding, err := s.ProjectGroupBinding(ctx, project.ID)
	if err != nil {
		t.Fatalf("project group binding: %v", err)
	}
	if binding.GroupID != devGroup.ID || binding.LifecycleID != devGroup.DefaultLifecycleID {
		t.Fatalf("expected project bound to Dev Squad, got %#v", binding)
	}
}

func TestStorePersistsAgentProfileCapabilities(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	group, err := s.CreateAgentGroup(ctx, agentgroups.Group{
		Name:           "Custom Team",
		Kind:           agentgroups.GroupKindCustom,
		DefaultModelID: "qwen-remote",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	saved, err := s.SaveAgentProfile(ctx, agentgroups.Profile{
		GroupID:       group.ID,
		Name:          "Специалист",
		RoleKey:       "security",
		DefaultSkills: []string{"pony-tail", "$security", "Security"},
		Capabilities:  []string{"scope review", "scope review", "risk notes"},
		AllowedTools:  []string{"web_search", "fetch_url"},
		ReadPaths:     []string{"docs/**", "README*"},
		WritePaths:    []string{"docs/**"},
		HandoffRules:  []string{"Передать ревьюеру после анализа"},
		ContextBudget: 9000,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("save profile: %v", err)
	}

	got, err := s.GetAgentProfile(ctx, saved.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if len(got.Capabilities) != 2 || got.Capabilities[0] != "scope review" || got.Capabilities[1] != "risk notes" {
		t.Fatalf("unexpected capabilities: %#v", got.Capabilities)
	}
	if strings.Join(got.DefaultSkills, ",") != "pony-tail,security" {
		t.Fatalf("unexpected default skills: %#v", got.DefaultSkills)
	}
	if strings.Join(got.AllowedTools, ",") != "web_search,fetch_url" {
		t.Fatalf("unexpected tools: %#v", got.AllowedTools)
	}
	if strings.Join(got.ReadPaths, ",") != "docs/**,README*" || strings.Join(got.WritePaths, ",") != "docs/**" {
		t.Fatalf("unexpected file access: read=%#v write=%#v", got.ReadPaths, got.WritePaths)
	}
	if len(got.HandoffRules) != 1 || !strings.Contains(got.HandoffRules[0], "ревьюеру") {
		t.Fatalf("unexpected handoff rules: %#v", got.HandoffRules)
	}
}

func TestStoreEditsLifecycleDefinitionAndSteps(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if err := s.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure default models: %v", err)
	}
	if err := s.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure default agent groups: %v", err)
	}

	updated, err := s.SaveLifecycleDefinition(ctx, agentgroups.LifecycleDefinition{
		ID:                  "lifecycle_dev_default",
		GroupID:             "group_dev_squad",
		Name:                "Dev Autopilot tuned",
		Kind:                agentgroups.GroupKindDev,
		Description:         "custom limits",
		MaxTotalIterations:  20,
		MaxRepairIterations: 4,
		SameErrorLimit:      3,
		Status:              agentgroups.StatusActive,
	})
	if err != nil {
		t.Fatalf("save lifecycle definition: %v", err)
	}
	if updated.MaxRepairIterations != 4 || updated.SameErrorLimit != 3 {
		t.Fatalf("unexpected lifecycle definition: %#v", updated)
	}

	step, err := s.SaveLifecycleStep(ctx, agentgroups.LifecycleStep{
		LifecycleID:      updated.ID,
		StepKey:          "docs_update",
		Title:            "Документация",
		AgentProfileID:   "agent_dev_docs",
		Mode:             "llm",
		Required:         false,
		CanRetry:         true,
		MaxRetries:       1,
		OnFailureStepKey: "developer_plan",
		VisibleToUser:    true,
	})
	if err != nil {
		t.Fatalf("save lifecycle step: %v", err)
	}
	if step.ID == "" || step.StepKey != "docs_update" || !step.CanRetry {
		t.Fatalf("unexpected lifecycle step: %#v", step)
	}

	steps, err := s.ListLifecycleSteps(ctx, updated.ID)
	if err != nil {
		t.Fatalf("list lifecycle steps: %v", err)
	}
	if len(steps) != 9 {
		t.Fatalf("expected added step, got %#v", steps)
	}

	if err := s.DeleteLifecycleStep(ctx, step.ID); err != nil {
		t.Fatalf("delete lifecycle step: %v", err)
	}
	steps, err = s.ListLifecycleSteps(ctx, updated.ID)
	if err != nil {
		t.Fatalf("list lifecycle steps after delete: %v", err)
	}
	if len(steps) != 8 {
		t.Fatalf("expected deleted step, got %#v", steps)
	}
}

func TestStoreUpsertsTaskSpec(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	project, err := s.CreateProject(ctx, "Spec project", filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := s.CreateTask(ctx, project.ID, "Сделать проверку сайта")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	saved, err := s.UpsertTaskSpec(ctx, taskspec.Spec{
		ProjectID:          project.ID,
		TaskID:             task.ID,
		WorkflowRunID:      "run_1",
		UserRequest:        "Сделать проверку сайта",
		Goal:               "Проверять доступность сайта",
		Requirements:       []string{"Принимать domain", "Принимать domain", "Принимать URL"},
		AcceptanceCriteria: []string{"`go test ./...` проходит успешно"},
		OpenQuestions:      []string{"Какая версия Go?"},
		Status:             taskspec.StatusWaitingClarification,
		Source:             "manager_intake",
	})
	if err != nil {
		t.Fatalf("upsert spec: %v", err)
	}
	if saved.ID == "" || len(saved.Requirements) != 2 {
		t.Fatalf("expected saved deduped spec, got %#v", saved)
	}

	updated, err := s.UpsertTaskSpec(ctx, taskspec.Spec{
		ID:                 saved.ID,
		ProjectID:          project.ID,
		TaskID:             task.ID,
		WorkflowRunID:      "run_2",
		UserRequest:        saved.UserRequest,
		Goal:               saved.Goal,
		Requirements:       []string{"Принимать domain", "Принимать URL"},
		AcceptanceCriteria: []string{"`go test ./...` проходит успешно"},
		Decisions:          []string{"Runtime: Go 1.25+"},
		OpenQuestions:      []string{},
		AcceptedAnswers: []taskspec.AcceptedAnswer{{
			QuestionID: "q1",
			Question:   "Какая версия Go?",
			Answer:     "Go 1.25+",
		}},
		Status: taskspec.StatusDone,
		Source: "clarification_answers",
	})
	if err != nil {
		t.Fatalf("update spec: %v", err)
	}
	if updated.ID != saved.ID || updated.WorkflowRunID != "run_2" || updated.Status != taskspec.StatusDone {
		t.Fatalf("unexpected updated spec: %#v", updated)
	}
	if len(updated.AcceptedAnswers) != 1 || updated.AcceptedAnswers[0].Answer != "Go 1.25+" {
		t.Fatalf("expected accepted answer, got %#v", updated.AcceptedAnswers)
	}

	byTask, err := s.LatestTaskSpecByTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("latest by task: %v", err)
	}
	byProject, err := s.LatestTaskSpecByProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("latest by project: %v", err)
	}
	if byTask.ID != saved.ID || byProject.ID != saved.ID {
		t.Fatalf("expected latest spec by task/project, got task=%#v project=%#v", byTask, byProject)
	}
}

func TestStoreUpsertsProjectMemory(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	project, err := s.CreateProject(ctx, "Memory project", filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := s.CreateTask(ctx, project.ID, "Сделать CLI")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	saved, err := s.UpsertProjectMemory(ctx, projectmemory.Memory{
		ProjectID:         project.ID,
		Stack:             "go",
		Runtime:           "Go 1.25+",
		BuildCommands:     []string{"make build", "make build"},
		TestCommands:      []string{"go test ./..."},
		StyleGuide:        []string{"gofmt"},
		Decisions:         []string{"Использовать стандартную библиотеку"},
		Environment:       []string{"Makefile available"},
		UpdatedFromTaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("upsert project memory: %v", err)
	}
	if saved.ID == "" || len(saved.BuildCommands) != 1 {
		t.Fatalf("expected saved deduped memory, got %#v", saved)
	}

	updated, err := s.UpsertProjectMemory(ctx, projectmemory.Memory{
		ID:                saved.ID,
		ProjectID:         project.ID,
		Stack:             "go, python",
		Runtime:           "Go 1.25+",
		BuildCommands:     []string{"make build", "make dmg"},
		TestCommands:      []string{"go test ./...", "python -m pytest"},
		StyleGuide:        []string{"gofmt", "Follow .editorconfig"},
		Decisions:         []string{"Использовать стандартную библиотеку", "Python tasks use .venv"},
		Environment:       []string{"Makefile available", "requirements.txt declares deps"},
		UpdatedFromTaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("update project memory: %v", err)
	}
	if updated.ID != saved.ID || updated.Stack != "go, python" {
		t.Fatalf("unexpected updated memory: %#v", updated)
	}
	if len(updated.TestCommands) != 2 || updated.TestCommands[1] != "python -m pytest" {
		t.Fatalf("expected merged test commands, got %#v", updated.TestCommands)
	}

	got, err := s.ProjectMemory(ctx, project.ID)
	if err != nil {
		t.Fatalf("project memory: %v", err)
	}
	if got.ID != saved.ID || got.Runtime != "Go 1.25+" {
		t.Fatalf("expected latest memory, got %#v", got)
	}
}

func TestStorePersistsV01Entities(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if err := s.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure default models: %v", err)
	}

	model, err := s.ActiveModelConfig(ctx)
	if err != nil {
		t.Fatalf("active model: %v", err)
	}
	if model.ID == "" {
		t.Fatal("expected active model")
	}

	project, err := s.CreateProject(ctx, "Тестовый проект", filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	task, err := s.CreateTask(ctx, project.ID, "Первая задача")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := s.AddMessage(ctx, task.ID, "user", "", "Сделай план"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	if _, err := s.AddMessage(ctx, task.ID, "agent", "manager", "План готов"); err != nil {
		t.Fatalf("add agent message: %v", err)
	}

	messages, err := s.ListMessages(ctx, task.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	runID, err := s.CreateAgentRun(ctx, task.ID, "manager")
	if err != nil {
		t.Fatalf("create agent run: %v", err)
	}
	if err := s.FinishAgentRun(ctx, runID, "done", ""); err != nil {
		t.Fatalf("finish agent run: %v", err)
	}

	workflowRun, err := s.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	step, err := s.CreateWorkflowStep(ctx, workflowRun.ID, "manager_intake", "manager", "input")
	if err != nil {
		t.Fatalf("create workflow step: %v", err)
	}
	if _, err := s.FinishWorkflowStep(ctx, step.ID, "done", "output", ""); err != nil {
		t.Fatalf("finish workflow step: %v", err)
	}
	if err := s.UpdateWorkflowRun(ctx, workflowRun.ID, "done", "manager_intake", ""); err != nil {
		t.Fatalf("finish workflow run: %v", err)
	}

	latestRun, err := s.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("latest workflow run: %v", err)
	}
	if latestRun == nil || latestRun.ID != workflowRun.ID {
		t.Fatalf("expected latest workflow run %q, got %#v", workflowRun.ID, latestRun)
	}
	steps, err := s.ListWorkflowSteps(ctx, workflowRun.ID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	if len(steps) != 1 || steps[0].Output != "output" {
		t.Fatalf("expected persisted workflow step output, got %#v", steps)
	}

	artifact, err := s.CreateArtifact(ctx, artifacts.Artifact{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		AgentID:       "manager",
		Kind:          "task_spec",
		Title:         "Спека задачи",
		Path:          filepath.Join(project.Path, "docs", "task-spec.md"),
		RelativePath:  filepath.Join("docs", "task-spec.md"),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if artifact.ID == "" {
		t.Fatal("expected artifact id")
	}

	artifactsList, err := s.ListArtifacts(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifactsList) != 1 || artifactsList[0].RelativePath != filepath.Join("docs", "task-spec.md") {
		t.Fatalf("expected persisted artifact, got %#v", artifactsList)
	}

	proposed, err := s.CreateProposedChange(ctx, changes.ProposedChange{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		AgentID:       "developer",
		FilePath:      "check_llm.py",
		Action:        changes.ActionCreate,
		Content:       "print('ok')\n",
		Reason:        "проверка LLM",
		Status:        changes.StatusPending,
	})
	if err != nil {
		t.Fatalf("create proposed change: %v", err)
	}
	if proposed.ID == "" || proposed.Status != changes.StatusPending {
		t.Fatalf("unexpected proposed change: %#v", proposed)
	}
	pendingChanges, err := s.ListPendingProposedChanges(ctx, workflowRun.ID)
	if err != nil {
		t.Fatalf("list pending proposed changes: %v", err)
	}
	if len(pendingChanges) != 1 || pendingChanges[0].FilePath != "check_llm.py" {
		t.Fatalf("expected pending change, got %#v", pendingChanges)
	}
	if err := s.MarkProposedChangeApplied(
		ctx,
		proposed.ID,
		filepath.Join(".zavod", "backups", proposed.ID, "check_llm.py"),
		"old",
		"new",
		"--- a/check_llm.py\n+++ b/check_llm.py\n-old\n+new",
	); err != nil {
		t.Fatalf("mark proposed change applied: %v", err)
	}
	allChanges, err := s.ListProposedChanges(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list proposed changes: %v", err)
	}
	if len(allChanges) != 1 ||
		allChanges[0].Status != changes.StatusApplied ||
		allChanges[0].AppliedAt == "" ||
		allChanges[0].BeforeContent != "old" ||
		allChanges[0].AfterContent != "new" ||
		!strings.Contains(allChanges[0].DiffText, "+new") {
		t.Fatalf("expected applied change, got %#v", allChanges)
	}

	testRun, err := s.CreateTestRun(ctx, checks.TestRun{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		Command:       "go test ./...",
		Reason:        "backend",
		Status:        checks.StatusPending,
	})
	if err != nil {
		t.Fatalf("create test run: %v", err)
	}
	if testRun.ID == "" || testRun.Status != checks.StatusPending {
		t.Fatalf("unexpected test run: %#v", testRun)
	}
	if err := s.MarkTestRunRunning(ctx, testRun.ID); err != nil {
		t.Fatalf("mark test run running: %v", err)
	}
	if err := s.FinishTestRun(ctx, testRun.ID, checks.RunResult{
		Status:   checks.StatusPassed,
		ExitCode: 0,
		Stdout:   "ok",
	}); err != nil {
		t.Fatalf("finish test run: %v", err)
	}
	testRuns, err := s.ListTestRuns(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list test runs: %v", err)
	}
	if len(testRuns) != 1 || testRuns[0].Status != checks.StatusPassed || testRuns[0].Stdout != "ok" {
		t.Fatalf("expected passed test run, got %#v", testRuns)
	}

	reviewRun, err := s.CreateReviewRun(ctx, reviews.ReviewRun{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		Status:        reviews.StatusRunning,
	})
	if err != nil {
		t.Fatalf("create review run: %v", err)
	}
	if err := s.FinishReviewRun(ctx, reviewRun.ID, reviews.ParsedReview{
		Status:  reviews.StatusNeedsWork,
		Summary: "Нужна доработка",
		Findings: []reviews.Finding{{
			Severity:   "major",
			FilePath:   "check_llm.py",
			Message:    "нет проверки статуса",
			Suggestion: "добавить обработку non-2xx",
		}},
		RequiredChanges:     []string{"добавить обработку non-2xx"},
		RecommendedNextStep: "Вернуть Разработчику",
	}, ""); err != nil {
		t.Fatalf("finish review run: %v", err)
	}
	reviewRuns, err := s.ListReviewRuns(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list review runs: %v", err)
	}
	if len(reviewRuns) != 1 ||
		reviewRuns[0].Status != reviews.StatusNeedsWork ||
		len(reviewRuns[0].Findings) != 1 ||
		reviewRuns[0].RequiredChanges[0] != "добавить обработку non-2xx" {
		t.Fatalf("expected persisted review run, got %#v", reviewRuns)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
