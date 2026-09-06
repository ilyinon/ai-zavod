package webresearch

import (
	"encoding/xml"
	"testing"
)

func TestParsePlanExtractsJSONFromMarkdown(t *testing.T) {
	plan := ParsePlan("```json\n{\"summary\":\"docs\",\"queries\":[{\"query\":\"Go 1.25 release notes\",\"reason\":\"runtime\"}]}\n```", "fallback")
	if plan.Summary != "docs" {
		t.Fatalf("summary = %q", plan.Summary)
	}
	if len(plan.Queries) != 1 || plan.Queries[0].Query != "Go 1.25 release notes" {
		t.Fatalf("queries = %#v", plan.Queries)
	}
}

func TestParsePlanFallsBackToUserText(t *testing.T) {
	plan := ParsePlan("not json", "найди актуальную документацию Wails")
	if len(plan.Queries) != 1 {
		t.Fatalf("queries = %#v", plan.Queries)
	}
	if plan.Queries[0].Query != "найди актуальную документацию Wails" {
		t.Fatalf("query = %q", plan.Queries[0].Query)
	}
}

func TestExtractURLsTrimsTrailingPunctuation(t *testing.T) {
	urls := ExtractURLs("посмотри https://example.com/docs, и http://openai.com/api).")
	if len(urls) != 2 {
		t.Fatalf("urls = %#v", urls)
	}
	if urls[0] != "https://example.com/docs" || urls[1] != "http://openai.com/api" {
		t.Fatalf("urls = %#v", urls)
	}
}

func TestNormalizeSettingsSetsLimits(t *testing.T) {
	settings := NormalizeSettings(Settings{
		Enabled:             true,
		MaxResults:          200,
		MaxPagesPerWorkflow: -1,
		TimeoutSeconds:      0,
		BlockedDomains:      []string{"https://Example.com/"},
	})
	if settings.MaxResults != DefaultMaxResults {
		t.Fatalf("max results = %d", settings.MaxResults)
	}
	if settings.MaxPagesPerWorkflow != DefaultMaxPagesPerWorkflow {
		t.Fatalf("max pages = %d", settings.MaxPagesPerWorkflow)
	}
	if settings.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Fatalf("timeout = %d", settings.TimeoutSeconds)
	}
	if len(settings.BlockedDomains) != 1 || settings.BlockedDomains[0] != "example.com" {
		t.Fatalf("blocked domains = %#v", settings.BlockedDomains)
	}
}

func TestDefaultSettingsAllowsAnyPublicDomain(t *testing.T) {
	settings := DefaultSettings()
	if len(settings.AllowedDomains) != 0 {
		t.Fatalf("default allowlist should be empty, got %#v", settings.AllowedDomains)
	}
	if err := validateURL("https://example.com/docs", settings); err != nil {
		t.Fatalf("default settings should allow public domains: %v", err)
	}
}

func TestWeatherLocationRussianLocative(t *testing.T) {
	location := weatherLocation("загугли какая погода в Минске?")
	if location != "Минск" {
		t.Fatalf("location = %q", location)
	}
}

func TestWeatherLocationNizhnyNovgorod(t *testing.T) {
	for _, query := range []string{"Какая погода в нижнем Новгороде?", "погода в Нижнем Новгороде сейчас", "погода в Нижний Новгород"} {
		if got := weatherLocation(query); got != "Нижний Новгород" {
			t.Fatalf("%q: location=%q", query, got)
		}
	}
}

func TestWeatherLocationEnglish(t *testing.T) {
	location := weatherLocation("weather in Minsk today")
	if location != "Minsk" {
		t.Fatalf("location = %q", location)
	}
}

func TestResearchTextKeepsOriginalUserLocation(t *testing.T) {
	plan := ParsePlan(`{"summary":"Исследование погоды","queries":[{"query":"текущая погода","reason":"weather"}]}`, "загугли какая погода в Минске")
	location := weatherLocation(researchText(plan))
	if location != "Минск" {
		t.Fatalf("location = %q", location)
	}
}

func TestWeatherLocationDoesNotCaptureAcrossPlanLines(t *testing.T) {
	text := "Исследование погоды в Минске\nтекущая погода\nзагугли какая погода в Минске"
	location := weatherLocation(text)
	if location != "Минск" {
		t.Fatalf("location = %q", location)
	}
}

func TestWeatherLocationTrimsPromptTail(t *testing.T) {
	location := weatherLocation("погода в Минске для предоставления пользователю")
	if location != "Минск" {
		t.Fatalf("location = %q", location)
	}
}

func TestCurrencyPairDollarRubles(t *testing.T) {
	base, target := currencyPair("загугли сколько стоит доллар в рублях")
	if base != "USD" || target != "RUB" {
		t.Fatalf("pair = %s/%s", base, target)
	}
}

func TestBingRSSSources(t *testing.T) {
	raw := `<?xml version="1.0" encoding="utf-8" ?>
<rss version="2.0">
  <channel>
    <item>
      <title>Модельный ряд HAVAL и цены 2025-2026</title>
      <link>https://avilon-haval.ru/models/</link>
      <description>Актуальные цены на модельный ряд Хавал.</description>
    </item>
    <item>
      <title>Local</title>
      <link>http://127.0.0.1/private</link>
      <description>must be filtered</description>
    </item>
  </channel>
</rss>`
	var payload bingRSS
	if err := xml.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("xml parse failed: %v", err)
	}
	sources := bingRSSSources("haval f7 2 поколение 2026 цена", payload.Channel.Items, 5, DefaultSettings())
	if len(sources) != 1 {
		t.Fatalf("sources = %#v", sources)
	}
	if sources[0].URL != "https://avilon-haval.ru/models/" {
		t.Fatalf("url = %q", sources[0].URL)
	}
	if sources[0].Title != "Модельный ряд HAVAL и цены 2025-2026" {
		t.Fatalf("title = %q", sources[0].Title)
	}
}

func TestProviderURLBypassesUserAllowlist(t *testing.T) {
	settings := Settings{AllowedDomains: []string{"example.com"}}
	if err := validateProviderURL("https://api.open-meteo.com/v1/forecast", settings); err != nil {
		t.Fatalf("provider url rejected: %v", err)
	}
	if err := validateProviderURL("https://www.cbr-xml-daily.ru/daily_json.js", settings); err != nil {
		t.Fatalf("currency provider url rejected: %v", err)
	}
	if err := validateURL("https://api.open-meteo.com/v1/forecast", NormalizeSettings(settings)); err == nil {
		t.Fatalf("regular URL validation should still honor user allowlist")
	}
}
