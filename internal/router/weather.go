package router

func hasWeatherLookup(text string) bool {
	// Weather data needs retrieval; explaining a weather API does not.
	if containsAny(text, "api", "sdk", "код", "скрипт", "программ", "как получить", "как узнать", "как работает", "как формируется", "что такое", "что означает") {
		return false
	}
	return containsAny(text, "какая погода", "какую погоду", "погода в ", "погода во ", "погода на ", "погода сегодня", "погода сейчас", "прогноз погоды", "температура воздуха в ", "weather in ", "weather today", "weather forecast")
}
