package router

import (
	"regexp"
	"strings"
)

var quotedData = regexp.MustCompile("(?s)```.*?```|`[^`]*`|«[^»]*»|“[^”]*”|\"[^\"]*\"")
var politeAction = regexp.MustCompile(`(?:^|\s)(?:можешь|можете|прошу)\s+(?:исправить|написать|добавить|создать|реализовать|удалить|изменить)`)

// Quoted instructions and pasted code are data, not the user's command.
func instructionText(text string) string { return quotedData.ReplaceAllString(text, " ") }

func explanationRequest(text string) bool {
	noEdits := containsAny(text, "файлы не меняй", "не меняй файлы", "ничего не создавай", "без изменения файлов", "без правок")
	if noEdits && containsAny(text, "объясни", "пример", "покажи", "напиши здесь", "как ") {
		return true
	}
	action := hasCodingVerb(text)
	if action && containsAny(text, "объясни и добавь", "и добавь", "и исправь", "и реализуй", "и создай") {
		return false
	}
	if containsAny(text, "покажи пример", "напиши здесь пример", "пример в чате") && !containsAny(text, "добавь", "исправь", "создай", "реализуй", "сохрани") {
		return true
	}
	if action {
		return false
	}
	return strings.HasPrefix(text, "как ") || strings.HasPrefix(text, "объясни") || strings.HasPrefix(text, "что означает") || strings.HasPrefix(text, "что такое") || containsAny(text, "как написать", "как реализовать", "что это значит")
}
