package router

import "strings"

type Intent string

const (
	IntentDirectAnswer        Intent = "direct_answer"
	IntentProjectAnalysis     Intent = "project_analysis"
	IntentCodingTask          Intent = "coding_task"
	IntentClarificationAnswer Intent = "clarification_answer"
	IntentWorkflowControl     Intent = "workflow_control"
	IntentPentestTask         Intent = "pentest_task"
	IntentResearchTask        Intent = "research_task"
	IntentGeneralChat         Intent = "general_chat"
)

type Decision struct {
	Intent              Intent `json:"intent"`
	Confidence          string `json:"confidence"`
	Reason              string `json:"reason"`
	NeedsProjectContext bool   `json:"needs_project_context"`
	NeedsWorkflow       bool   `json:"needs_workflow"`
	NeedsClarification  bool   `json:"needs_clarification"`
	Source              string `json:"source,omitempty"`
}

type Context struct {
	HasActiveClarification bool
}

func Route(message string, context Context) Decision {
	text := normalize(message)
	if text == "" {
		return Decision{Intent: IntentGeneralChat, Confidence: "high", Reason: "пустое сообщение", Source: "rules"}
	}
	if strings.HasPrefix(text, "ответы на уточнения:") {
		return Decision{
			Intent:              IntentClarificationAnswer,
			Confidence:          "high",
			Reason:              "ответ из native clarification UI",
			NeedsProjectContext: true,
			NeedsWorkflow:       true,
			Source:              "rules",
		}
	}
	if context.HasActiveClarification && !hasCodingVerb(text) && !hasDirectQuestionMarker(text) {
		return Decision{
			Intent:              IntentClarificationAnswer,
			Confidence:          "medium",
			Reason:              "есть активное уточнение, сообщение похоже на ответ",
			NeedsProjectContext: true,
			NeedsWorkflow:       true,
			Source:              "rules",
		}
	}
	if hasWorkflowControlMarker(text) {
		return Decision{
			Intent:              IntentWorkflowControl,
			Confidence:          "high",
			Reason:              "пользователь управляет текущим workflow",
			NeedsProjectContext: true,
			Source:              "rules",
		}
	}
	if hasPentestMarker(text) {
		return Decision{
			Intent:              IntentPentestTask,
			Confidence:          "high",
			Reason:              "запрос относится к безопасности или пентесту",
			NeedsProjectContext: true,
			NeedsWorkflow:       true,
			Source:              "rules",
		}
	}
	if hasResearchMarker(text) && !hasCodingVerb(text) {
		return Decision{
			Intent:              IntentResearchTask,
			Confidence:          "high",
			Reason:              "пользователь просит найти или проверить информацию в интернете",
			NeedsProjectContext: likelyNeedsProjectContext(text),
			NeedsWorkflow:       true,
			Source:              "rules",
		}
	}
	if hasCodingVerb(text) {
		return Decision{
			Intent:              IntentCodingTask,
			Confidence:          "high",
			Reason:              "пользователь просит изменить или создать результат в проекте",
			NeedsProjectContext: true,
			NeedsWorkflow:       true,
			Source:              "rules",
		}
	}
	if hasProjectAnalysisMarker(text) {
		return Decision{
			Intent:              IntentProjectAnalysis,
			Confidence:          "high",
			Reason:              "пользователь просит разобраться в проекте без правок",
			NeedsProjectContext: true,
			Source:              "rules",
		}
	}
	if hasDirectQuestionMarker(text) {
		return Decision{
			Intent:              IntentDirectAnswer,
			Confidence:          "high",
			Reason:              "пользователь задает вопрос или просит объяснить существующую информацию",
			NeedsProjectContext: likelyNeedsProjectContext(text),
			Source:              "rules",
		}
	}
	if hasGeneralChatMarker(text) {
		return Decision{
			Intent:     IntentGeneralChat,
			Confidence: "high",
			Reason:     "общий вопрос без признаков проектной работы",
			Source:     "rules",
		}
	}
	return Decision{
		Intent:              IntentDirectAnswer,
		Confidence:          "low",
		Reason:              "не удалось уверенно классифицировать локальными правилами",
		NeedsProjectContext: true,
		Source:              "rules",
	}
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func hasCodingVerb(text string) bool {
	return containsAny(text,
		"реализуй", "исправь", "почини", "добавь", "измени", "создай", "удали", "перепиши",
		"собери", "внедри", "сгенерируй файл", "сделай чтобы", "сделай так", "поправь",
		"замени", "перенеси", "обнови код", "доработай", "переименуй", "отрефактори",
		"напиши", "написать", "сделай", "сгенерируй код", "сгенерируй скрипт",
		"сгенерируй программу", "создай скрипт", "создай программу",
	)
}

func hasWorkflowControlMarker(text string) bool {
	return containsAny(text,
		"останови", "продолжи", "перезапусти", "повтори проверку", "запусти проверку",
		"запусти ревью", "покажи diff", "покажи дифф", "вернись к разработчику",
	)
}

func hasPentestMarker(text string) bool {
	return containsAny(text,
		"пентест", "pentest", "penetration test", "уязвимост", "security audit",
		"аудит безопасности", "threat model", "sql injection", "xss", "cve",
	)
}

func hasProjectAnalysisMarker(text string) bool {
	return containsAny(text,
		"посмотри архитектуру", "найди где", "где созда", "где ломается", "оцени качество",
		"разберись в проекте", "проверь архитектуру", "почему кнопка", "почему не работает",
		"что в проекте", "как устроен проект",
	)
}

func hasResearchMarker(text string) bool {
	return containsAny(text,
		"найди в интернете", "поищи в интернете", "посмотри в интернете", "загугли",
		"web search", "internet search", "поиск в интернете", "актуальную информацию",
		"актуальные данные", "с источниками", "дай источники", "ссылки на источники",
		"проверь в интернете", "проверь актуально", "что сейчас известно",
	)
}

func hasDirectQuestionMarker(text string) bool {
	if strings.HasSuffix(text, "?") {
		return true
	}
	return containsAny(text,
		"что ", "почему", "зачем", "как работает", "как устро", "объясни", "опиши",
		"покажи", "какой статус", "что было в спеке", "что изменилось", "по какой спеке",
		"спеку по которой", "какой следующий шаг", "какой этап", "что дальше",
	)
}

func hasGeneralChatMarker(text string) bool {
	return containsAny(text,
		"что такое", "чем отличается", "как лучше назвать", "как правильно", "расскажи",
	)
}

func likelyNeedsProjectContext(text string) bool {
	return containsAny(text,
		"спек", "проект", "workflow", "воркфлоу", "пайп", "тест", "diff", "дифф",
		"файл", "архитектур", "blueprint", "логик", "измен",
	)
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
