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

	ToolGoDev        = "tool_go_dev"
	ToolPythonDev    = "tool_python_dev"
	ToolResearch     = "tool_research"
	ToolCTFWeb       = "tool_ctf_web"
	ToolCTFLFI       = "tool_ctf_lfi"
	ToolCTFRCE       = "tool_ctf_rce"
	ToolCTFSQLi      = "tool_ctf_sqli"
	ToolCTFPwn       = "tool_ctf_pwn"
	ToolCTFCrypto    = "tool_ctf_crypto"
	ToolCTFReverse   = "tool_ctf_reverse"
	ToolCTFForensics = "tool_ctf_forensics"
	ToolCTFValidator = "tool_ctf_validator"

	DecisionAuto    = "auto"
	DecisionConfirm = "confirm"
	DecisionDeny    = "deny"
)

type Evaluation struct {
	Context       string `json:"context"`
	ToolProfileID string `json:"toolProfileId,omitempty"`
	Command       string `json:"command"`
	Decision      string `json:"decision"`
	Rule          string `json:"rule"`
	Reason        string `json:"reason"`
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
	args, rejected, ok := prepareCommand(base)
	if !ok {
		return rejected
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

func EvaluateToolProfile(toolProfileID string, command string) Evaluation {
	toolProfileID = normalizeToolProfileID(toolProfileID)
	contextName := contextForToolProfile(toolProfileID)
	command = strings.TrimSpace(command)
	base := Evaluation{Context: contextName, ToolProfileID: toolProfileID, Command: command}
	args, rejected, ok := prepareCommand(base)
	if !ok {
		return rejected
	}

	switch toolProfileID {
	case ToolGoDev:
		return evaluateGoDevProfile(base, args)
	case ToolPythonDev:
		return evaluatePythonDevProfile(base, args)
	case ToolResearch:
		return evaluateResearch(base, args)
	case ToolCTFWeb:
		return evaluateCTFWebProfile(base, args)
	case ToolCTFLFI:
		return evaluateCTFLFIProfile(base, args)
	case ToolCTFRCE:
		return evaluateCTFRCEProfile(base, args)
	case ToolCTFSQLi:
		return evaluateCTFSQLiProfile(base, args)
	case ToolCTFPwn:
		return evaluateCTFPwnProfile(base, args)
	case ToolCTFCrypto:
		return evaluateCTFCryptoProfile(base, args)
	case ToolCTFReverse:
		return evaluateCTFReverseProfile(base, args)
	case ToolCTFForensics:
		return evaluateCTFForensicsProfile(base, args)
	case ToolCTFValidator:
		return evaluateCTFValidatorProfile(base, args)
	default:
		return deny(base, "unknown_tool_profile", "неизвестный tool profile")
	}
}

func prepareCommand(base Evaluation) ([]string, Evaluation, bool) {
	command := strings.TrimSpace(base.Command)
	if command == "" {
		return nil, deny(base, "empty", "команда пустая"), false
	}
	if token := shellOperator(command); token != "" {
		return nil, deny(base, "shell_operator", fmt.Sprintf("команда содержит shell-оператор %q", token)), false
	}
	args := strings.Fields(command)
	if len(args) == 0 {
		return nil, deny(base, "empty", "команда пустая"), false
	}
	if isAlwaysDenied(args) {
		return nil, deny(base, "forbidden_command", "команда запрещена политикой выполнения"), false
	}
	return args, Evaluation{}, true
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

func CTFToolProfiles() []Profile {
	return []Profile{
		{
			Context: ToolCTFWeb,
			Auto: []Rule{
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальные solver scripts"},
				{Command: "file | strings", Decision: DecisionAuto, Reason: "локальный анализ выданных артефактов"},
			},
			Confirm: []Rule{{Command: "curl | dig | whois", Decision: DecisionConfirm, Reason: "сетевые действия только при явном CTF/lab scope"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFLFI,
			Auto: []Rule{
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальный LFI solver"},
				{Command: "file | strings", Decision: DecisionAuto, Reason: "локальная работа с артефактами"},
			},
			Confirm: []Rule{{Command: "curl", Decision: DecisionConfirm, Reason: "LFI HTTP-проверки только в scope"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFRCE,
			Auto: []Rule{
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальный RCE helper"},
				{Command: "file | strings", Decision: DecisionAuto, Reason: "локальная работа с артефактами"},
			},
			Confirm: []Rule{{Command: "curl", Decision: DecisionConfirm, Reason: "активные проверки только в scope"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFSQLi,
			Auto: []Rule{
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальный SQLi solver"},
				{Command: "file | strings", Decision: DecisionAuto, Reason: "локальная работа с артефактами"},
			},
			Confirm: []Rule{{Command: "curl | sqlmap", Decision: DecisionConfirm, Reason: "SQLi tooling только с явным scope"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFPwn,
			Auto: []Rule{
				{Command: "file | strings | checksec | readelf | objdump | nm", Decision: DecisionAuto, Reason: "локальный static binary triage"},
				{Command: ".venv/bin/python <pwntools solver.py>", Decision: DecisionAuto, Reason: "pwntools solver внутри .venv"},
			},
			Confirm: []Rule{{Command: "gdb | ROPgadget | one_gadget", Decision: DecisionConfirm, Reason: "debug/exploit tooling требует подтверждения"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFCrypto,
			Auto: []Rule{
				{Command: ".venv/bin/python <solver.py>", Decision: DecisionAuto, Reason: "локальный crypto solver"},
				{Command: "file | strings", Decision: DecisionAuto, Reason: "локальная работа с артефактами"},
			},
			Confirm: []Rule{{Command: "sage", Decision: DecisionConfirm, Reason: "Sage запускается после подтверждения"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFReverse,
			Auto: []Rule{
				{Command: "file | strings | readelf | objdump | nm", Decision: DecisionAuto, Reason: "локальный static reverse triage"},
				{Command: ".venv/bin/python <helper.py>", Decision: DecisionAuto, Reason: "локальные helper scripts"},
			},
			Confirm: []Rule{{Command: "radare2 | r2 | ghidra", Decision: DecisionConfirm, Reason: "интерактивные reverse tools требуют подтверждения"}},
			Deny:    commonDenyRules(),
		},
		{
			Context: ToolCTFForensics,
			Auto: []Rule{
				{Command: "file | strings | exiftool | binwalk | xxd", Decision: DecisionAuto, Reason: "локальная форензика без extract"},
				{Command: ".venv/bin/python <helper.py>", Decision: DecisionAuto, Reason: "локальные forensic helpers"},
			},
			Confirm: []Rule{{Command: "binwalk -e | foremost | tshark", Decision: DecisionConfirm, Reason: "извлечение/тяжелые tools требуют подтверждения"}},
			Deny:    commonDenyRules(),
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

func evaluateGoDevProfile(base Evaluation, args []string) Evaluation {
	if args[0] == "gofmt" && len(args) >= 2 && allSafeRelativeArgs(args[1:]) {
		return auto(base, "go_dev_gofmt", "gofmt разрешен для project-local Go файлов")
	}
	return evaluateDev(base, args)
}

func evaluatePythonDevProfile(base Evaluation, args []string) Evaluation {
	if isAllowedPythonCheck(args) {
		return auto(base, "python_dev_venv", "Python-команда разрешена только внутри project virtualenv")
	}
	if isPythonInstallConfirm(args) {
		return confirm(base, "python_dev_install_confirm", "установка Python-зависимостей требует подтверждения")
	}
	return deny(base, "python_dev_not_allowed", "команда не входит в Python tool profile")
}

func evaluateCTFWebProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_web_local_auto", "локальный web solver/evidence разрешен автоматически")
	}
	if isNetworkConfirm(args) {
		return confirm(base, "ctf_web_scope_confirm", "HTTP/DNS действия требуют явного CTF/lab scope")
	}
	return deny(base, "ctf_web_not_allowed", "команда не входит в web CTF profile")
}

func evaluateCTFLFIProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_lfi_local_auto", "локальный LFI solver/evidence разрешен автоматически")
	}
	if isNetworkConfirm(args) {
		return confirm(base, "ctf_lfi_scope_confirm", "проверки LFI по сети требуют явного CTF/lab scope")
	}
	return deny(base, "ctf_lfi_not_allowed", "команда не входит в LFI CTF profile")
}

func evaluateCTFRCEProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_rce_local_auto", "локальный RCE solver/evidence разрешен автоматически")
	}
	if isNetworkConfirm(args) {
		return confirm(base, "ctf_rce_scope_confirm", "RCE/command-injection проверки требуют явного CTF/lab scope")
	}
	return deny(base, "ctf_rce_not_allowed", "команда не входит в RCE CTF profile")
}

func evaluateCTFSQLiProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_sqli_local_auto", "локальный SQLi solver/evidence разрешен автоматически")
	}
	if isNetworkConfirm(args) || isScopedTool(args, "sqlmap") {
		return confirm(base, "ctf_sqli_scope_confirm", "SQLi-инструменты требуют явного CTF/lab scope")
	}
	return deny(base, "ctf_sqli_not_allowed", "команда не входит в SQLi CTF profile")
}

func evaluateCTFPwnProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) {
		return auto(base, "ctf_pwn_pwntools_venv", "pwntools/solver scripts разрешены только через project virtualenv")
	}
	if isLocalArtifactAuto(args, "file", "strings", "checksec", "readelf", "objdump", "nm") {
		return auto(base, "ctf_pwn_local_auto", "локальный анализ бинарей разрешен автоматически")
	}
	if isScopedTool(args, "gdb", "ROPgadget", "one_gadget") {
		return confirm(base, "ctf_pwn_debug_confirm", "debug/exploit tooling требует подтверждения пользователя")
	}
	return deny(base, "ctf_pwn_not_allowed", "команда не входит в pwn CTF profile")
}

func evaluateCTFCryptoProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) {
		return auto(base, "ctf_crypto_solver_venv", "crypto solver scripts разрешены через project virtualenv")
	}
	if isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_crypto_local_auto", "локальный просмотр crypto-артефактов разрешен автоматически")
	}
	if isScopedTool(args, "sage") {
		return confirm(base, "ctf_crypto_sage_confirm", "Sage solver запускается только после подтверждения")
	}
	return deny(base, "ctf_crypto_not_allowed", "команда не входит в crypto CTF profile")
}

func evaluateCTFReverseProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings", "readelf", "objdump", "nm") {
		return auto(base, "ctf_reverse_local_auto", "локальный reverse-анализ разрешен автоматически")
	}
	if isScopedTool(args, "radare2", "r2", "ghidra") {
		return confirm(base, "ctf_reverse_tool_confirm", "интерактивные reverse-инструменты требуют подтверждения")
	}
	return deny(base, "ctf_reverse_not_allowed", "команда не входит в reverse CTF profile")
}

func evaluateCTFForensicsProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings", "exiftool", "binwalk", "xxd") {
		if args[0] == "binwalk" && hasAnyArg(args[1:], "-e", "--extract", "-Me", "-M") {
			return confirm(base, "ctf_forensics_extract_confirm", "извлечение binwalk пишет файлы и требует подтверждения")
		}
		return auto(base, "ctf_forensics_local_auto", "локальная форензика файлов разрешена автоматически")
	}
	if isScopedTool(args, "foremost", "tshark") {
		return confirm(base, "ctf_forensics_heavy_confirm", "тяжелые forensic-инструменты требуют подтверждения")
	}
	return deny(base, "ctf_forensics_not_allowed", "команда не входит в forensics CTF profile")
}

func evaluateCTFValidatorProfile(base Evaluation, args []string) Evaluation {
	if isCTFSolverPython(args) || isLocalArtifactAuto(args, "file", "strings") {
		return auto(base, "ctf_validator_local_auto", "локальная проверка solver/writeup разрешена автоматически")
	}
	return deny(base, "ctf_validator_not_allowed", "валидатор запускает только локальные проверки")
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

func isCTFSolverPython(args []string) bool {
	return isAllowedPythonCheck(args)
}

func isPythonInstallConfirm(args []string) bool {
	if len(args) >= 4 && isVenvPythonExecutable(args[0]) && args[1] == "-m" && args[2] == "pip" && args[3] == "install" {
		return true
	}
	return len(args) >= 2 && oneOf(args[0], "pip", "pip3")
}

func isNetworkConfirm(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return oneOf(args[0], "curl", "dig", "whois") && len(args) >= 2 && allSafeToolArgs(args[1:])
}

func isScopedTool(args []string, commands ...string) bool {
	if len(args) < 2 || !oneOf(args[0], commands...) {
		return false
	}
	return allSafeToolArgs(args[1:])
}

func isLocalArtifactAuto(args []string, commands ...string) bool {
	if len(args) < 2 || !oneOf(args[0], commands...) {
		return false
	}
	return allSafeLocalArtifactArgs(args[1:])
}

func allSafeLocalArtifactArgs(args []string) bool {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		if filepath.IsAbs(arg) || strings.Contains(arg, "..") || isLikelyURL(arg) {
			return false
		}
	}
	return true
}

func isLikelyURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func hasAnyArg(args []string, needles ...string) bool {
	for _, arg := range args {
		for _, needle := range needles {
			if arg == needle {
				return true
			}
		}
	}
	return false
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

func normalizeToolProfileID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func contextForToolProfile(toolProfileID string) string {
	switch toolProfileID {
	case ToolResearch:
		return ContextResearch
	case ToolCTFWeb, ToolCTFLFI, ToolCTFRCE, ToolCTFSQLi, ToolCTFPwn, ToolCTFCrypto, ToolCTFReverse, ToolCTFForensics, ToolCTFValidator:
		return ContextCTF
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
