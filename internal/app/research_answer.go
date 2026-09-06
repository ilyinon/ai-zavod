package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"zavod_ai/internal/llm"
	"zavod_ai/internal/webresearch"
)

func generateResearchAnswer(ctx context.Context, provider llm.Provider, req llm.Request, sources []webresearch.Source) string {
	return generateResearchAnswerWithVerification(ctx, provider, req, sources, false)
}

func generateResearchAnswerWithVerification(ctx context.Context, provider llm.Provider, req llm.Request, sources []webresearch.Source, verify bool) string {
	if verify {
		if len(sources) == 0 {
			return unverifiedProductAnswer
		}
		req.Messages = append(append([]llm.Message(nil), req.Messages...), llm.Message{Role: "system", Content: verifiedResearchFormat})
	}
	// Synthesis has its own budget; time spent searching must not consume it.
	answerCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	response, err := provider.Generate(answerCtx, req)
	reason := ""
	switch {
	case err != nil:
		var failure *llm.ProviderError
		switch {
		case errors.As(err, &failure):
			reason = failure.Kind
		case errors.Is(err, context.DeadlineExceeded):
			reason = "timeout"
		case errors.Is(err, context.Canceled):
			reason = "cancelled"
		default:
			reason = "provider_error"
		}
	case response == nil || strings.TrimSpace(response.Content) == "":
		reason = "empty_response"
	case !verify && looksLikeRawJSONAnswer(response.Content):
		reason = "unexpected_json"
	case looksLikeRepetitionLoop(response.Content):
		reason = "repetition"
	case len(response.Content) > managerMaxAnswerBytes:
		reason = "response_too_long"
	default:
		if verify {
			return renderVerifiedResearch(response.Content, sources)
		}
		return strings.TrimSpace(response.Content)
	}
	// Do not log provider bodies, credentials, prompts, or source contents.
	slog.WarnContext(ctx, "research answer fallback", "reason", reason, "source_count", len(sources))
	if verify {
		return unverifiedProductAnswer
	}
	return webResearchFallbackAnswer(sources)
}
