package executionpolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	ContextDev      = "dev"
	ContextCTF      = "ctf"
	ContextResearch = "research"

	DecisionAuto    = "auto"
	DecisionConfirm = "confirm"
	DecisionDeny    = "deny"
)

type Evaluation struct {
	Context  string `json:"context"`
	Command  string `json:"command"`
	Decision string `json:"decision"`
	Rule     string `json:"rule"`
	Reason   string `json:"reason"`
}

type Rule struct {
	Command  string `json:"command"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type Profile struct {
	Context string `json:"context"`
	Auto    []Rule `json:"auto"`
	Confirm []Rule `json:"confirm"`
	Deny    []Rule `json:"deny"`
}

func Evaluate(contextName string, command string) Evaluation {
	contextName = normalizeContext(contextName)
	command = strings.TrimSpace(command)
	base := Evaluation{Context: contextName, Command: command}
	if command == "" {
		return deny(base, "empty", "команда пустая")
	}
	if token := shellOperator(command); token != "" {
		return deny(base, "shell_operator", fmt.Sprintf("команда содержит shell-оператор %q", token))
	}
	args := strings.Fields(command)
	if len(args) == 0 {
		return deny(base, "empty", "команда пустая")
	}
	if isAlwaysDenied(args) {
		return deny(base, "forbidden_command", "команда запрещена политикой выполнения")
	}

	switch contextName {
	case ContextDev:
		return evaluateDev(base, args)
	case ContextCTF:
		return evaluateCTF(base, args)
	case ContextResearch:
		return evaluateResearch(base, args)
	default:
		return deny(base, "unknown_context", "неизвестный профиль выполнения")
	}
}

func Profiles() []Profile {
	return []Profile{
		{
			Context: ContextDev,
			Auto: []Rule{
				{Command: "go test ./... | go test ./internal/...", Decision: DecisionAuto, Reason: "детерминированные Go-проверки внутри проекта"},
				{Command: "go vet ./...", Decision: DecisionAuto, Reason: "статическая проверка Go без записи файлов"},
				{Command: "npm test | npm run test/build/lint", Decision: DecisionAuto, Reason: "скрипты проверки frontend без установки зависимостей"},
				{Command: ".venv/bin/python <script.py>", Decision: DecisionAuto, Reason: "запуск Python-проверки только внутри project virtualenv"},
				{Command: ".venv/bin/python -m pytest|py_compile", Decision: DecisionAuto, Reason: "Python-тесты и синтаксическая проверка внутри project virtualenv"},
			},
			Confirm: []Rule{
				{Command: "go mod tidy | go get", Decision: DecisionConfirm, Reason: "меняет зависимости или go.sum"},
				{Command: "npm install | npm ci | pip install", Decision: DecisionConfirm, Reason: "устанавливает зависимости и использует сеть"},
				{Command: "make build | make test | wails build", Decision: DecisionConfirm, Reason: "может запускать произвольные команды из локальных файлов"},
				{Command: "go run . | npm run dev | .venv/bin/python app.py", Decision: DecisionConfirm, Reason: "запускает приложение, а не только проверку"},
			},
			Deny: commonDenyRules(),
		},
		{
			Context: ContextCTF,
			Auto: []Rule{
				{Command: "file | strings | objdump", Decision: DecisionAuto, Reason: "локальный анализ challenge-артефактов"},
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальный solver внутри project virtualenv"},
				{Command: ".venv/bin/python -m py_compile <solver.py>", Decision: DecisionAuto, Reason: "проверка solver без сетевых действий"},
			},
			Confirm: []Rule{
				{Command: "curl | dig | whois", Decision: DecisionConfirm, Reason: "сетевые действия разрешены только при явном CTF/scope"},
				{Command: "sqlmap | gdb | radare2 | binwalk | exiftool", Decision: DecisionConfirm, Reason: "активные или тяжёлые CTF-инструменты требуют подтверждения/scope"},
			},
			Deny: commonDenyRules(),
		},
		{
			Context: ContextResearch,
			Auto: []Rule{
				{Command: "web provider", Decision: DecisionAuto, Reason: "поиск выполняется встроенным web research provider, не shell-командой"},
			},
			Confirm: nil,
			Deny:    append(commonDenyRules(), Rule{Command: "любая shell-команда", Decision: DecisionDeny, Reason: "research не запускает локальные команды"}),
		},
	}
}

func AllowsAuto(contextName string, command string) bool {
	return Evaluate(contextName, command).Decision == DecisionAuto
}

func RequiresConfirmation(contextName string, command string) bool {
	return Evaluate(contextName, command).Decision == DecisionConfirm
}

func evaluateDev(base Evaluation, args []string) Evaluation {
	if isDevAuto(args) {
		return auto(base, "dev_auto_check", "команда разрешена для автоматической dev-проверки")
	}
	if isDevConfirm(args) {
		return confirm(base, "dev_confirm", "команда требует подтверждения пользователя")
	}
	return deny(base, "dev_not_allowed", "команда не входит в dev auto-allowlist")
}

func evaluateCTF(base Evaluation, args []string) Evaluation {
	if isCTFAuto(args) {
		return auto(base, "ctf_local_auto", "локальная CTF-команда разрешена автоматически")
	}
	if isCTFConfirm(args) {
		return confirm(base, "ctf_scope_confirm", "команда требует явного CTF scope и подтверждения")
	}
	return deny(base, "ctf_not_allowed", "команда не входит в CTF allowlist")
}

func evaluateResearch(base Evaluation, args []string) Evaluation {
	return deny(base, "research_no_shell", "research-профиль не запускает shell-команды")
}

func isDevAuto(args []string) bool {
	switch args[0] {
	case "go":
		if len(args) != 3 || (args[1] != "test" && args[1] != "vet") {
			return false
		}
		return args[2] == "./..." || isSafeGoPackage(args[2])
	case "npm":
		if len(args) == 2 && args[1] == "test" {
			return true
		}
		return len(args) == 3 && args[1] == "run" && oneOf(args[2], "test", "build", "lint")
	default:
		return isAllowedPythonCheck(args)
	}
}

func isDevConfirm(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "go":
		return len(args) >= 2 && oneOf(args[1], "mod", "get", "run", "build", "install", "generate")
	case "npm":
		return len(args) >= 2 && oneOf(args[1], "install", "ci", "run", "exec")
	case "pip", "pip3":
		return true
	case "make", "wails":
		return true
	default:
		return isPythonExecutable(args[0]) && len(args) >= 2 && isSafePythonScript(args[1])
	}
}

func isCTFAuto(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if oneOf(args[0], "file", "strings", "objdump") {
		return len(args) >= 2 && allSafeRelativeArgs(args[1:])
	}
	return isAllowedPythonCheck(args)
}

func isCTFConfirm(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "curl", "dig", "whois":
		return len(args) >= 2
	case "sqlmap", "gdb", "radare2", "r2", "binwalk", "exiftool":
		return len(args) >= 2 && allSafeToolArgs(args[1:])
	default:
		return false
	}
}

func isAllowedPythonCheck(args []string) bool {
	if len(args) < 2 || !isVenvPythonExecutable(args[0]) {
		return false
	}
	if len(args) == 2 {
		return isSafePythonScript(args[1])
	}
	if len(args) == 3 && args[1] == "-m" && args[2] == "pytest" {
		return true
	}
	if len(args) == 4 && args[1] == "-m" && args[2] == "py_compile" {
		return isSafePythonScript(args[3])
	}
	return false
}

func isAlwaysDenied(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if oneOf(args[0], "rm", "mv", "cp", "dd", "chmod", "chown", "sudo", "su", "bash", "sh", "zsh", "fish", "osascript", "open") {
		return true
	}
	if oneOf(args[0], "docker", "kubectl", "helm", "terraform", "ansible") {
		return true
	}
	if oneOf(args[0], "nc", "netcat", "nmap", "masscan", "hydra", "john", "hashcat", "msfconsole") {
		return true
	}
	return false
}

func shellOperator(command string) string {
	for _, token := range []string{"&&", "||", ";", "|", ">", "<", "$(", "`", "\n", "\r"} {
		if strings.Contains(command, token) {
			return token
		}
	}
	return ""
}

func isPythonExecutable(value string) bool {
	switch strings.TrimSpace(filepath.ToSlash(value)) {
	case "python", "python3", ".venv/bin/python", ".venv/bin/python3":
		return true
	default:
		return false
	}
}

func isVenvPythonExecutable(value string) bool {
	switch strings.TrimSpace(filepath.ToSlash(value)) {
	case ".venv/bin/python", ".venv/bin/python3":
		return true
	default:
		return false
	}
}

func isSafePythonScript(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "..") {
		return false
	}
	if strings.HasPrefix(value, "-") || !strings.HasSuffix(value, ".py") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '/' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeGoPackage(value string) bool {
	if !strings.HasPrefix(value, "./") || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '/' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func allSafeRelativeArgs(args []string) bool {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		if filepath.IsAbs(arg) || strings.Contains(arg, "..") {
			return false
		}
	}
	return true
}

func allSafeToolArgs(args []string) bool {
	for _, arg := range args {
		if strings.ContainsAny(arg, "\n\r`") {
			return false
		}
	}
	return true
}

func commonDenyRules() []Rule {
	return []Rule{
		{Command: "rm/mv/cp/dd/chmod/chown", Decision: DecisionDeny, Reason: "разрушительные или массовые операции с файлами"},
		{Command: "sudo/su/bash/sh/zsh", Decision: DecisionDeny, Reason: "обход runner-политики через shell или повышение прав"},
		{Command: "docker/kubectl/helm/terraform", Decision: DecisionDeny, Reason: "инфраструктурные команды вне managed runner"},
		{Command: "nmap/masscan/hydra/hashcat/msfconsole", Decision: DecisionDeny, Reason: "активные security-инструменты не запускаются автоматически"},
	}
}

func normalizeContext(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ContextCTF:
		return ContextCTF
	case ContextResearch:
		return ContextResearch
	default:
		return ContextDev
	}
}

func auto(base Evaluation, rule string, reason string) Evaluation {
	base.Decision = DecisionAuto
	base.Rule = rule
	base.Reason = reason
	return base
}

func confirm(base Evaluation, rule string, reason string) Evaluation {
	base.Decision = DecisionConfirm
	base.Rule = rule
	base.Reason = reason
	return base
}

func deny(base Evaluation, rule string, reason string) Evaluation {
	base.Decision = DecisionDeny
	base.Rule = rule
	base.Reason = reason
	return base
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
