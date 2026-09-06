package router

import "strings"

// NeedsCurrentSources separates product facts from stable conceptual explanations.
func NeedsCurrentSources(message string) bool {
	text := normalize(instructionText(message))
	if hasCodingVerb(text) || hasProjectAnalysisMarker(text) {
		return false
	}
	if containsAny(text, "модель данных", "модели данных", "модель памяти", "модели памяти", "линейн", "логистическ", "дерево решений", "деревья решений", "нейросеть от", "языковая модель от", "языковые модели от", "математическ") {
		return false
	}
	comparison := containsAny(text, "чем отлич", "сравни", "сравнение", "разниц", "лучше выбрать", "какая лучше", "which is better", "compare ", " vs ")
	fresh := containsAny(text, "сейчас", "актуальн", "последняя версия", "новая версия", "стоимость", "сколько стоит", "цен", "лимит", "контекстное окно")
	product := containsAny(text, "модел", "llm", "openai", "open ai", "gpt", "claude", "gemini", "qwen", "deepseek", "тариф", "подписк", "смартфон", "ноутбук", "iphone", "macbook", "видеокарт", "процессор")
	return product && (comparison || fresh || strings.HasPrefix(text, "расскажи о модели"))
}
