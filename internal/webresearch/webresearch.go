package webresearch

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultMaxResults          = 5
	DefaultMaxPagesPerWorkflow = 8
	DefaultTimeoutSeconds      = 8
)

var providerDomains = []string{
	"api.open-meteo.com",
	"cbr-xml-daily.ru",
	"geocoding-api.open-meteo.com",
	"www.cbr-xml-daily.ru",
	"www.bing.com",
}

type Settings struct {
	Enabled             bool     `json:"enabled"`
	MaxResults          int      `json:"maxResults"`
	MaxPagesPerWorkflow int      `json:"maxPagesPerWorkflow"`
	TimeoutSeconds      int      `json:"timeoutSeconds"`
	AllowedDomains      []string `json:"allowedDomains"`
	BlockedDomains      []string `json:"blockedDomains"`
}

type Query struct {
	Query  string `json:"query"`
	Reason string `json:"reason"`
}

type Plan struct {
	Summary      string  `json:"summary"`
	Queries      []Query `json:"queries"`
	OriginalText string  `json:"-"`
}

type Source struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	TaskID         string `json:"taskId"`
	WorkflowRunID  string `json:"workflowRunId"`
	AgentID        string `json:"agentId"`
	Query          string `json:"query"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	Snippet        string `json:"snippet"`
	ContentExcerpt string `json:"contentExcerpt"`
	SourceType     string `json:"sourceType"`
	TrustLevel     string `json:"trustLevel"`
	FetchedAt      string `json:"fetchedAt"`
	CreatedAt      string `json:"createdAt"`
}

type Client struct {
	httpClient *http.Client
	userAgent  string
}

func DefaultSettings() Settings {
	return NormalizeSettings(Settings{Enabled: true})
}

func NormalizeSettings(settings Settings) Settings {
	if settings.MaxResults <= 0 || settings.MaxResults > 10 {
		settings.MaxResults = DefaultMaxResults
	}
	if settings.MaxPagesPerWorkflow <= 0 || settings.MaxPagesPerWorkflow > 20 {
		settings.MaxPagesPerWorkflow = DefaultMaxPagesPerWorkflow
	}
	if settings.TimeoutSeconds <= 0 || settings.TimeoutSeconds > 30 {
		settings.TimeoutSeconds = DefaultTimeoutSeconds
	}
	settings.AllowedDomains = normalizeDomains(settings.AllowedDomains)
	settings.BlockedDomains = normalizeDomains(settings.BlockedDomains)
	return settings
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = DefaultTimeoutSeconds * time.Second
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  "ZavodAI/0.6 web-research (+local desktop app)",
	}
}

func PlanFromText(text string) Plan {
	query := strings.TrimSpace(text)
	if query == "" {
		return Plan{}
	}
	return Plan{
		Summary:      "Поиск актуальной информации по запросу пользователя",
		OriginalText: query,
		Queries: []Query{
			{Query: query, Reason: "исходная формулировка пользователя"},
		},
	}
}

func ParsePlan(raw string, fallbackText string) Plan {
	raw = strings.TrimSpace(stripCodeFence(raw))
	if raw == "" {
		return PlanFromText(fallbackText)
	}
	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err == nil {
		return normalizePlan(plan, fallbackText)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(raw[start:end+1]), &plan); err == nil {
			return normalizePlan(plan, fallbackText)
		}
	}
	return PlanFromText(fallbackText)
}

func (c *Client) Research(ctx context.Context, plan Plan, settings Settings) ([]Source, error) {
	settings = NormalizeSettings(settings)
	if !settings.Enabled {
		return nil, errors.New("web research выключен в настройках")
	}
	var sources []Source
	seen := map[string]bool{}
	searchText := researchText(plan)
	var weatherErr error
	var currencyErr error
	var searchErr error

	for _, directURL := range ExtractURLs(searchText) {
		if len(sources) >= settings.MaxPagesPerWorkflow {
			break
		}
		source, err := c.Fetch(ctx, directURL, settings)
		if err != nil {
			continue
		}
		source.Query = "direct-url"
		if !seen[source.URL] {
			seen[source.URL] = true
			sources = append(sources, source)
		}
	}

	if isWeatherQuery(searchText) && len(sources) < settings.MaxPagesPerWorkflow {
		if source, err := c.weatherSource(ctx, searchText, settings); err == nil && !seen[source.URL] {
			seen[source.URL] = true
			sources = append(sources, source)
		} else if err != nil {
			weatherErr = err
		}
	}

	if isCurrencyQuery(searchText) && len(sources) < settings.MaxPagesPerWorkflow {
		if source, err := c.currencySource(ctx, searchText, settings); err == nil && !seen[source.URL] {
			seen[source.URL] = true
			sources = append(sources, source)
		} else if err != nil {
			currencyErr = err
		}
	}

	for _, item := range plan.Queries {
		query := strings.TrimSpace(item.Query)
		if query == "" {
			continue
		}
		results, err := c.searchDuckDuckGo(ctx, query, settings.MaxResults, settings)
		if err != nil {
			searchErr = err
		}
		if len(results) == 0 {
			bingResults, err := c.searchBingRSS(ctx, query, settings.MaxResults, settings)
			if err != nil {
				searchErr = err
			} else {
				results = bingResults
			}
		}
		for _, source := range results {
			if len(sources) >= settings.MaxPagesPerWorkflow {
				break
			}
			if seen[source.URL] {
				continue
			}
			seen[source.URL] = true
			sources = append(sources, source)
		}
	}

	if len(sources) == 0 {
		if isWeatherQuery(searchText) {
			if weatherErr != nil {
				return nil, fmt.Errorf("источники не найдены: weather fallback не сработал: %w", weatherErr)
			}
			return nil, errors.New("источники не найдены: weather fallback и web search не вернули данных")
		}
		if isCurrencyQuery(searchText) {
			if currencyErr != nil {
				return nil, fmt.Errorf("источники не найдены: currency fallback не сработал: %w", currencyErr)
			}
			return nil, errors.New("источники не найдены: currency fallback и web search не вернули данных")
		}
		if searchErr != nil {
			return nil, fmt.Errorf("источники не найдены: search providers не вернули подходящих публичных страниц: %w", searchErr)
		}
		return nil, errors.New("источники не найдены: search provider не вернул подходящих публичных страниц")
	}
	return sources, nil
}

func (c *Client) Fetch(ctx context.Context, rawURL string, settings Settings) (Source, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return Source{}, err
	}
	if err := validateURL(normalized, settings); err != nil {
		return Source{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return Source{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Source{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return Source{}, fmt.Errorf("страница вернула HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return Source{}, err
	}
	title, excerpt := extractPageText(string(body))
	return Source{
		Title:          firstNonEmpty(title, normalized),
		URL:            normalized,
		Snippet:        "",
		ContentExcerpt: excerpt,
		SourceType:     "web",
		TrustLevel:     trustLevel(normalized),
		FetchedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func ExtractURLs(text string) []string {
	matches := urlPattern.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		cleaned := strings.TrimRight(match, ".,);]}>\"'")
		if cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return uniqueStrings(out)
}

func FormatSourcesForPrompt(sources []Source) string {
	var builder strings.Builder
	for index, source := range sources {
		builder.WriteString(fmt.Sprintf("[%d] %s\nURL: %s\n", index+1, source.Title, source.URL))
		if source.Snippet != "" {
			builder.WriteString("Кратко: ")
			builder.WriteString(shorten(source.Snippet, 500))
			builder.WriteString("\n")
		}
		if source.ContentExcerpt != "" {
			builder.WriteString("Фрагмент: ")
			builder.WriteString(shorten(source.ContentExcerpt, 1200))
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func (c *Client) searchDuckDuckGo(ctx context.Context, query string, limit int, settings Settings) ([]Source, error) {
	endpoint := "https://api.duckduckgo.com/"
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("no_html", "1")
	params.Set("skip_disambig", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("поиск вернул HTTP %d", resp.StatusCode)
	}
	var payload ddgResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&payload); err != nil {
		return nil, err
	}
	results := payload.toSources(query, limit)
	for index := range results {
		if err := validateURL(results[index].URL, settings); err != nil {
			results[index].URL = ""
			continue
		}
		fetched, err := c.Fetch(ctx, results[index].URL, settings)
		if err == nil {
			results[index].Title = firstNonEmpty(fetched.Title, results[index].Title)
			results[index].ContentExcerpt = fetched.ContentExcerpt
			results[index].TrustLevel = fetched.TrustLevel
			results[index].FetchedAt = fetched.FetchedAt
		}
	}
	filtered := results[:0]
	for _, item := range results {
		if item.URL != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (c *Client) searchBingRSS(ctx context.Context, query string, limit int, settings Settings) ([]Source, error) {
	endpoint := "https://www.bing.com/search"
	params := url.Values{}
	params.Set("format", "rss")
	params.Set("q", query)
	params.Set("setlang", "ru-RU")
	params.Set("cc", "RU")
	requestURL := endpoint + "?" + params.Encode()
	if err := validateProviderURL(requestURL, settings); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("bing rss вернул HTTP %d", resp.StatusCode)
	}
	var payload bingRSS
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&payload); err != nil {
		return nil, err
	}
	results := bingRSSSources(query, payload.Channel.Items, limit, settings)
	for index := range results {
		link := results[index].URL
		if fetched, err := c.Fetch(ctx, link, settings); err == nil {
			results[index].Title = firstNonEmpty(fetched.Title, results[index].Title)
			results[index].ContentExcerpt = firstNonEmpty(fetched.ContentExcerpt, results[index].ContentExcerpt)
			results[index].TrustLevel = fetched.TrustLevel
			results[index].FetchedAt = fetched.FetchedAt
		}
	}
	return results, nil
}

func bingRSSSources(query string, items []bingRSSItem, limit int, settings Settings) []Source {
	results := make([]Source, 0, minInt(limit, len(items)))
	for _, item := range items {
		if len(results) >= limit {
			break
		}
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		if err := validateURL(link, settings); err != nil {
			continue
		}
		results = append(results, Source{
			Query:          query,
			Title:          normalizeText(item.Title),
			URL:            link,
			Snippet:        normalizeText(item.Description),
			ContentExcerpt: normalizeText(item.Description),
			SourceType:     "web",
			TrustLevel:     trustLevel(link),
			FetchedAt:      time.Now().UTC().Format(time.RFC3339),
		})
	}
	return results
}

func (c *Client) weatherSource(ctx context.Context, text string, settings Settings) (Source, error) {
	location := weatherLocation(text)
	if location == "" {
		return Source{}, errors.New("город для погоды не найден")
	}
	place, err := c.geocode(ctx, location, settings)
	if err != nil {
		return Source{}, err
	}
	forecast, requestURL, err := c.currentWeather(ctx, place, settings)
	if err != nil {
		return Source{}, err
	}
	description := weatherCodeText(forecast.Current.WeatherCode)
	title := fmt.Sprintf("Погода: %s, %s", place.Name, place.Country)
	excerpt := fmt.Sprintf(
		"%s, %s (%.4f, %.4f). Данные на %s, местное время.\n\nТемпература: **%.1f %s**, ощущается как **%.1f %s**.\n\n- Влажность: %d %s\n- Осадки: %.1f %s\n- Ветер: %.1f %s, направление %.0f°\n- Условия: %s.",
		place.Name,
		place.Country,
		place.Latitude,
		place.Longitude,
		forecast.Current.Time,
		forecast.Current.Temperature2M,
		forecast.CurrentUnits.Temperature2M,
		forecast.Current.ApparentTemperature,
		forecast.CurrentUnits.ApparentTemperature,
		forecast.Current.RelativeHumidity2M,
		forecast.CurrentUnits.RelativeHumidity2M,
		forecast.Current.Precipitation,
		forecast.CurrentUnits.Precipitation,
		forecast.Current.WindSpeed10M,
		forecast.CurrentUnits.WindSpeed10M,
		forecast.Current.WindDirection10M,
		description,
	)
	return Source{
		Query:          "weather:" + location,
		Title:          title,
		URL:            requestURL,
		Snippet:        fmt.Sprintf("%.1f %s, %s", forecast.Current.Temperature2M, forecast.CurrentUnits.Temperature2M, description),
		ContentExcerpt: excerpt,
		SourceType:     "weather",
		TrustLevel:     "high",
		FetchedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) geocode(ctx context.Context, location string, settings Settings) (geoPlace, error) {
	params := url.Values{}
	params.Set("name", location)
	params.Set("count", "1")
	params.Set("language", "ru")
	params.Set("format", "json")
	requestURL := "https://geocoding-api.open-meteo.com/v1/search?" + params.Encode()
	if err := validateProviderURL(requestURL, settings); err != nil {
		return geoPlace{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return geoPlace{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return geoPlace{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return geoPlace{}, fmt.Errorf("geocoding вернул HTTP %d", resp.StatusCode)
	}
	var payload geoResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256*1024)).Decode(&payload); err != nil {
		return geoPlace{}, err
	}
	if len(payload.Results) == 0 {
		return geoPlace{}, fmt.Errorf("город %q не найден в geocoding", location)
	}
	return payload.Results[0], nil
}

func (c *Client) currentWeather(ctx context.Context, place geoPlace, settings Settings) (weatherResponse, string, error) {
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.6f", place.Latitude))
	params.Set("longitude", fmt.Sprintf("%.6f", place.Longitude))
	params.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m,wind_direction_10m")
	params.Set("timezone", "auto")
	requestURL := "https://api.open-meteo.com/v1/forecast?" + params.Encode()
	if err := validateProviderURL(requestURL, settings); err != nil {
		return weatherResponse{}, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return weatherResponse{}, "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return weatherResponse{}, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return weatherResponse{}, "", fmt.Errorf("forecast вернул HTTP %d", resp.StatusCode)
	}
	var payload weatherResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256*1024)).Decode(&payload); err != nil {
		return weatherResponse{}, "", err
	}
	if payload.Current.Time == "" {
		return weatherResponse{}, "", errors.New("forecast не вернул current weather")
	}
	return payload, requestURL, nil
}

func (c *Client) currencySource(ctx context.Context, text string, settings Settings) (Source, error) {
	base, target := currencyPair(text)
	if base == "" || target == "" {
		return Source{}, errors.New("валютная пара не распознана")
	}
	rate, err := c.cbrRate(ctx, base, target, settings)
	if err != nil {
		return Source{}, err
	}
	direction := ""
	if rate.Previous > 0 {
		diff := rate.Value - rate.Previous
		if diff > 0 {
			direction = fmt.Sprintf(" Рост к предыдущему значению: +%.4f.", diff)
		} else if diff < 0 {
			direction = fmt.Sprintf(" Снижение к предыдущему значению: %.4f.", diff)
		}
	}
	title := fmt.Sprintf("Курс %s/%s", base, target)
	excerpt := fmt.Sprintf(
		"Источник: CBR XML Daily. Дата курса: %s. 1 %s = %.4f %s. Предыдущее значение: %.4f %s.%s",
		rate.Date,
		base,
		rate.Value,
		target,
		rate.Previous,
		target,
		direction,
	)
	return Source{
		Query:          "currency:" + base + "/" + target,
		Title:          title,
		URL:            rate.SourceURL,
		Snippet:        fmt.Sprintf("1 %s = %.4f %s", base, rate.Value, target),
		ContentExcerpt: excerpt,
		SourceType:     "currency",
		TrustLevel:     "high",
		FetchedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Client) cbrRate(ctx context.Context, base string, target string, settings Settings) (currencyRate, error) {
	if target != "RUB" && base != "RUB" {
		return currencyRate{}, fmt.Errorf("поддерживаются валютные пары с RUB, получено %s/%s", base, target)
	}
	if base == target {
		return currencyRate{Base: base, Target: target, Value: 1, Previous: 1, SourceURL: "https://www.cbr-xml-daily.ru/daily_json.js"}, nil
	}
	requestURL := "https://www.cbr-xml-daily.ru/daily_json.js"
	if err := validateProviderURL(requestURL, settings); err != nil {
		return currencyRate{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return currencyRate{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return currencyRate{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return currencyRate{}, fmt.Errorf("currency provider вернул HTTP %d", resp.StatusCode)
	}
	var payload cbrDailyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&payload); err != nil {
		return currencyRate{}, err
	}
	if base == "RUB" {
		item, ok := payload.Valute[target]
		if !ok || item.Value <= 0 || item.Nominal <= 0 {
			return currencyRate{}, fmt.Errorf("курс %s не найден", target)
		}
		return currencyRate{
			Base:      base,
			Target:    target,
			Value:     float64(item.Nominal) / item.Value,
			Previous:  float64(item.Nominal) / item.Previous,
			Date:      payload.Date,
			SourceURL: requestURL,
		}, nil
	}
	item, ok := payload.Valute[base]
	if !ok || item.Value <= 0 || item.Nominal <= 0 {
		return currencyRate{}, fmt.Errorf("курс %s не найден", base)
	}
	return currencyRate{
		Base:      base,
		Target:    target,
		Value:     item.Value / float64(item.Nominal),
		Previous:  item.Previous / float64(item.Nominal),
		Date:      payload.Date,
		SourceURL: requestURL,
	}, nil
}

type ddgResponse struct {
	Heading       string            `json:"Heading"`
	AbstractText  string            `json:"AbstractText"`
	AbstractURL   string            `json:"AbstractURL"`
	RelatedTopics []json.RawMessage `json:"RelatedTopics"`
}

type geoResponse struct {
	Results []geoPlace `json:"results"`
}

type geoPlace struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	Timezone  string  `json:"timezone"`
}

type weatherResponse struct {
	Current      weatherCurrent `json:"current"`
	CurrentUnits weatherUnits   `json:"current_units"`
}

type weatherCurrent struct {
	Time                string  `json:"time"`
	Temperature2M       float64 `json:"temperature_2m"`
	RelativeHumidity2M  int     `json:"relative_humidity_2m"`
	ApparentTemperature float64 `json:"apparent_temperature"`
	Precipitation       float64 `json:"precipitation"`
	WeatherCode         int     `json:"weather_code"`
	WindSpeed10M        float64 `json:"wind_speed_10m"`
	WindDirection10M    float64 `json:"wind_direction_10m"`
}

type weatherUnits struct {
	Temperature2M       string `json:"temperature_2m"`
	RelativeHumidity2M  string `json:"relative_humidity_2m"`
	ApparentTemperature string `json:"apparent_temperature"`
	Precipitation       string `json:"precipitation"`
	WeatherCode         string `json:"weather_code"`
	WindSpeed10M        string `json:"wind_speed_10m"`
	WindDirection10M    string `json:"wind_direction_10m"`
}

type cbrDailyResponse struct {
	Date   string                 `json:"Date"`
	Valute map[string]cbrCurrency `json:"Valute"`
}

type cbrCurrency struct {
	CharCode string  `json:"CharCode"`
	Nominal  int     `json:"Nominal"`
	Name     string  `json:"Name"`
	Value    float64 `json:"Value"`
	Previous float64 `json:"Previous"`
}

type currencyRate struct {
	Base      string
	Target    string
	Value     float64
	Previous  float64
	Date      string
	SourceURL string
}

type bingRSS struct {
	Channel bingRSSChannel `xml:"channel"`
}

type bingRSSChannel struct {
	Items []bingRSSItem `xml:"item"`
}

type bingRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
}

type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Topics   []ddgTopic `json:"Topics"`
}

func (r ddgResponse) toSources(query string, limit int) []Source {
	var sources []Source
	if r.AbstractURL != "" {
		sources = append(sources, Source{
			Query:      query,
			Title:      firstNonEmpty(r.Heading, r.AbstractURL),
			URL:        r.AbstractURL,
			Snippet:    r.AbstractText,
			SourceType: "web",
			TrustLevel: trustLevel(r.AbstractURL),
			FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	}
	for _, raw := range r.RelatedTopics {
		var topic ddgTopic
		if err := json.Unmarshal(raw, &topic); err != nil {
			continue
		}
		sources = append(sources, flattenTopic(query, topic)...)
		if len(sources) >= limit {
			break
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return len(sources[i].ContentExcerpt)+len(sources[i].Snippet) > len(sources[j].ContentExcerpt)+len(sources[j].Snippet)
	})
	if len(sources) > limit {
		sources = sources[:limit]
	}
	return sources
}

func flattenTopic(query string, topic ddgTopic) []Source {
	var sources []Source
	if topic.FirstURL != "" {
		sources = append(sources, Source{
			Query:      query,
			Title:      firstSentence(topic.Text),
			URL:        topic.FirstURL,
			Snippet:    topic.Text,
			SourceType: "web",
			TrustLevel: trustLevel(topic.FirstURL),
			FetchedAt:  time.Now().UTC().Format(time.RFC3339),
		})
	}
	for _, nested := range topic.Topics {
		sources = append(sources, flattenTopic(query, nested)...)
	}
	return sources
}

func normalizePlan(plan Plan, fallbackText string) Plan {
	plan.Summary = strings.TrimSpace(plan.Summary)
	plan.OriginalText = strings.TrimSpace(fallbackText)
	queries := make([]Query, 0, len(plan.Queries))
	seen := map[string]bool{}
	for _, item := range plan.Queries {
		query := strings.TrimSpace(item.Query)
		if query == "" || seen[strings.ToLower(query)] {
			continue
		}
		seen[strings.ToLower(query)] = true
		queries = append(queries, Query{Query: query, Reason: strings.TrimSpace(item.Reason)})
		if len(queries) >= 3 {
			break
		}
	}
	if len(queries) == 0 {
		return PlanFromText(fallbackText)
	}
	plan.Queries = queries
	return plan
}

func isWeatherQuery(text string) bool {
	text = strings.ToLower(text)
	return strings.Contains(text, "погод") ||
		strings.Contains(text, "температур") ||
		strings.Contains(text, "weather") ||
		strings.Contains(text, "forecast")
}

func weatherLocation(text string) string {
	candidates := []string{}
	for _, pattern := range weatherLocationPatterns {
		matches := pattern.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			if len(match) > 1 {
				candidates = append(candidates, match[1])
			}
		}
	}
	for _, candidate := range candidates {
		location := cleanWeatherLocation(candidate)
		if location != "" {
			return normalizeWeatherLocation(location)
		}
	}
	return ""
}

func cleanWeatherLocation(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " ?!.,:;\"'`()[]{}")
	stopWords := []string{
		" сегодня", " сейчас", " завтра", " на сегодня", " на завтра",
		" для ", " чтобы ", " пользовател", " актуальн", " источник",
		" today", " now", " tomorrow", " for ", " to ",
	}
	lower := strings.ToLower(value)
	for _, stop := range stopWords {
		if index := strings.Index(lower, stop); index >= 0 {
			value = strings.TrimSpace(value[:index])
			lower = strings.ToLower(value)
		}
	}
	fields := strings.Fields(value)
	if len(fields) > 3 {
		fields = fields[:3]
	}
	value = strings.Join(fields, " ")
	return value
}

func normalizeWeatherLocation(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	aliases := map[string]string{
		"нижнем новгороде": "Нижний Новгород",
		"нижний новгород":  "Нижний Новгород",
		"минске":           "Минск",
		"минск":            "Минск",
		"москве":           "Москва",
		"москва":           "Москва",
		"питере":           "Санкт-Петербург",
		"санкт-петербурге": "Санкт-Петербург",
	}
	if alias, ok := aliases[lower]; ok {
		return alias
	}
	return value
}

func isCurrencyQuery(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "курс") ||
		strings.Contains(lower, "валют") ||
		strings.Contains(lower, "стоит доллар") ||
		strings.Contains(lower, "доллар в руб") ||
		strings.Contains(lower, "usd") ||
		strings.Contains(lower, "exchange rate")
}

func currencyPair(text string) (string, string) {
	tokens := currencyTokens(text)
	if len(tokens) == 0 {
		return "", ""
	}
	base := ""
	target := ""
	for _, token := range tokens {
		code := currencyCode(token)
		if code == "" {
			continue
		}
		if base == "" {
			base = code
			continue
		}
		if code != base {
			target = code
			break
		}
	}
	if base != "" && target == "" && base != "RUB" && mentionsRuble(text) {
		target = "RUB"
	}
	if base == "" && mentionsDollar(text) && mentionsRuble(text) {
		base, target = "USD", "RUB"
	}
	if base == "" || target == "" {
		return "", ""
	}
	return base, target
}

func currencyTokens(text string) []string {
	rawTokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(rawTokens))
	for _, token := range rawTokens {
		if currencyCode(token) != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func currencyCode(token string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	aliases := map[string]string{
		"usd": "USD", "доллар": "USD", "доллара": "USD", "долларов": "USD", "бакс": "USD", "бакса": "USD", "баксов": "USD",
		"eur": "EUR", "евро": "EUR",
		"cny": "CNY", "юань": "CNY", "юаня": "CNY", "юаней": "CNY",
		"rub": "RUB", "руб": "RUB", "рубль": "RUB", "рубля": "RUB", "рублей": "RUB", "рублях": "RUB",
	}
	return aliases[token]
}

func mentionsDollar(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "доллар") || strings.Contains(lower, "usd") || strings.Contains(lower, "бакс")
}

func mentionsRuble(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "руб") || strings.Contains(lower, "rub")
}

func weatherCodeText(code int) string {
	switch code {
	case 0:
		return "ясно"
	case 1, 2, 3:
		return "переменная облачность"
	case 45, 48:
		return "туман"
	case 51, 53, 55:
		return "морось"
	case 56, 57:
		return "ледяная морось"
	case 61, 63, 65:
		return "дождь"
	case 66, 67:
		return "ледяной дождь"
	case 71, 73, 75:
		return "снег"
	case 77:
		return "снежные зерна"
	case 80, 81, 82:
		return "ливневый дождь"
	case 85, 86:
		return "снежные ливни"
	case 95:
		return "гроза"
	case 96, 99:
		return "гроза с градом"
	default:
		return fmt.Sprintf("код погоды WMO %d", code)
	}
}

func normalizeURL(rawURL string) (string, error) {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return "", errors.New("пустой URL")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("поддерживаются только http/https URL")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("URL без host")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validateURL(rawURL string, settings Settings) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return errors.New("URL без host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("локальные адреса не используются для web research")
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return fmt.Errorf("приватные IP не используются для web research")
	}
	if len(settings.AllowedDomains) > 0 && !domainMatchesAny(host, settings.AllowedDomains) {
		return fmt.Errorf("domain %s не входит в allowlist", host)
	}
	if domainMatchesAny(host, settings.BlockedDomains) {
		return fmt.Errorf("domain %s заблокирован", host)
	}
	return nil
}

func validateProviderURL(rawURL string, settings Settings) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	host := strings.ToLower(parsed.Hostname())
	if !domainMatchesAny(host, providerDomains) {
		return validateURL(rawURL, settings)
	}
	settings.AllowedDomains = nil
	return validateURL(rawURL, settings)
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified())
}

func extractPageText(body string) (string, string) {
	title := ""
	if match := titlePattern.FindStringSubmatch(body); len(match) > 1 {
		title = normalizeText(match[1])
	}
	cleaned := scriptPattern.ReplaceAllString(body, " ")
	cleaned = stylePattern.ReplaceAllString(cleaned, " ")
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	cleaned = normalizeText(html.UnescapeString(cleaned))
	return title, shorten(cleaned, 2200)
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(html.UnescapeString(text)), " ")
}

func stripCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			return strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return text
}

func planText(plan Plan) string {
	parts := []string{plan.Summary}
	for _, query := range plan.Queries {
		parts = append(parts, query.Query)
	}
	return strings.Join(parts, "\n")
}

func researchText(plan Plan) string {
	parts := []string{planText(plan)}
	if strings.TrimSpace(plan.OriginalText) != "" {
		parts = append(parts, plan.OriginalText)
	}
	return strings.Join(parts, "\n")
}

func firstSentence(text string) string {
	text = normalizeText(text)
	for _, separator := range []string{" - ", ". "} {
		if index := strings.Index(text, separator); index > 0 {
			return text[:index]
		}
	}
	return shorten(text, 90)
}

func trustLevel(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "unknown"
	}
	host := strings.ToLower(parsed.Hostname())
	if strings.HasSuffix(host, ".gov") || strings.HasSuffix(host, ".edu") {
		return "high"
	}
	if strings.Contains(host, "docs.") || strings.Contains(host, "developer.") || strings.Contains(host, "github.com") {
		return "medium"
	}
	return "normal"
}

func normalizeDomains(domains []string) []string {
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.ToLower(domain))
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimPrefix(domain, ".")
		domain = strings.TrimSuffix(domain, "/")
		if domain != "" {
			out = append(out, domain)
		}
	}
	return uniqueStrings(out)
}

func domainMatchesAny(host string, domains []string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func shorten(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

var (
	urlPattern              = regexp.MustCompile(`https?://[^\s<>()"']+`)
	titlePattern            = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	scriptPattern           = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	stylePattern            = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	tagPattern              = regexp.MustCompile(`(?is)<[^>]+>`)
	weatherLocationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)погод[\pL]*\s+(?:в|во|на|для)\s+([\pL \-]+)`),
		regexp.MustCompile(`(?i)температур[\pL]*\s+(?:в|во|на|для)\s+([\pL \-]+)`),
		regexp.MustCompile(`(?i)(?:weather|forecast)\s+(?:in|for)\s+([a-zA-Z \-]+)`),
	}
)
