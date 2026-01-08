package app

import (
	"context"
	"fmt"
	"time"

	"go.aimuz.me/transy/cache"
	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/llm"
)

// Translator encapsulates translation logic with caching.
// Zero value is not useful; create via NewTranslator.
type Translator struct {
	cache *cache.Cache
}

// NewTranslator creates a Translator with optional caching.
// If cachePath is empty, caching is disabled.
func NewTranslator(c *cache.Cache) *Translator {
	return &Translator{cache: c}
}

// Translate performs translation using the given completer, with cache lookup.
func (t *Translator) Translate(ctx context.Context, completer llm.Completer, profile TranslateProfile, req types.TranslateRequest) (types.TranslateResult, error) {
	key := t.cacheKey(profile, req)

	// Check cache first
	if result, ok := t.getCached(key); ok {
		return result, nil
	}

	// Build messages
	msgs := buildTranslateMessages(profile.SystemPrompt, req)

	// Call LLM
	text, usage, err := completer.Complete(ctx, msgs)
	if err != nil {
		return types.TranslateResult{}, fmt.Errorf("translate: %w", err)
	}

	// Store in cache (best effort)
	t.setCache(key, text, usage)

	return types.TranslateResult{Text: text, Usage: usage}, nil
}

// TranslateProfile holds the minimal config needed for translation.
type TranslateProfile struct {
	Name         string
	Model        string
	SystemPrompt string
}

func buildTranslateMessages(systemPrompt string, req types.TranslateRequest) []llm.Message {
	content := fmt.Sprintf(
		"please translate the following text from %s to %s:\n\n%s",
		req.SourceLang, req.TargetLang, req.Text,
	)

	if req.Context != "" {
		content = fmt.Sprintf(
			"Context (previous sentences): %s\n\n%s",
			req.Context, content,
		)
	}

	return []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: content},
	}
}

func (t *Translator) cacheKey(p TranslateProfile, req types.TranslateRequest) string {
	return cache.GenerateKey(p.Name, p.Model, req.SourceLang, req.TargetLang, req.Text)
}

func (t *Translator) getCached(key string) (types.TranslateResult, bool) {
	if t.cache == nil {
		return types.TranslateResult{}, false
	}

	entry, found := t.cache.Get(key)
	if !found {
		return types.TranslateResult{}, false
	}

	return types.TranslateResult{
		Text: entry.Text,
		Usage: types.Usage{
			PromptTokens:     entry.Usage.PromptTokens,
			CompletionTokens: entry.Usage.CompletionTokens,
			TotalTokens:      entry.Usage.TotalTokens,
			CacheHit:         true,
		},
	}, true
}

func (t *Translator) setCache(key, text string, usage types.Usage) {
	if t.cache == nil {
		return
	}

	entry := &cache.Entry{
		Text: text,
		Usage: cache.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}

	// Ignore error - caching is best effort
	_ = t.cache.Set(key, entry, cache.DefaultTTL)
}
