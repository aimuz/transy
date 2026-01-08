package app

import (
	"context"
	"testing"

	"go.aimuz.me/transy/internal/types"
	"go.aimuz.me/transy/llm"
)

// mockCompleter implements llm.Completer for testing.
type mockCompleter struct {
	response string
	usage    types.Usage
	err      error
}

func (m *mockCompleter) Complete(_ context.Context, _ []llm.Message) (string, types.Usage, error) {
	return m.response, m.usage, m.err
}

func TestBuildTranslateMessages(t *testing.T) {
	tests := []struct {
		name         string
		systemPrompt string
		req          types.TranslateRequest
		wantMsgCount int
		wantSystem   string
		wantContains string
	}{
		{
			name:         "basic translation",
			systemPrompt: "You are a translator.",
			req: types.TranslateRequest{
				Text:       "Hello",
				SourceLang: "en",
				TargetLang: "zh",
			},
			wantMsgCount: 2,
			wantSystem:   "You are a translator.",
			wantContains: "translate the following text from en to zh",
		},
		{
			name:         "translation with context",
			systemPrompt: "Translate accurately.",
			req: types.TranslateRequest{
				Text:       "world",
				SourceLang: "en",
				TargetLang: "zh",
				Context:    "Hello,",
			},
			wantMsgCount: 2,
			wantSystem:   "Translate accurately.",
			wantContains: "Context (previous sentences)",
		},
		{
			name:         "empty system prompt",
			systemPrompt: "",
			req: types.TranslateRequest{
				Text:       "Test",
				SourceLang: "auto",
				TargetLang: "en",
			},
			wantMsgCount: 2,
			wantSystem:   "",
			wantContains: "from auto to en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := buildTranslateMessages(tt.systemPrompt, tt.req)

			if len(msgs) != tt.wantMsgCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantMsgCount)
			}

			if msgs[0].Role != "system" {
				t.Errorf("first message role = %q, want %q", msgs[0].Role, "system")
			}

			if msgs[0].Content != tt.wantSystem {
				t.Errorf("system prompt = %q, want %q", msgs[0].Content, tt.wantSystem)
			}

			if msgs[1].Role != "user" {
				t.Errorf("second message role = %q, want %q", msgs[1].Role, "user")
			}

			if tt.wantContains != "" {
				if !contains(msgs[1].Content, tt.wantContains) {
					t.Errorf("user message does not contain %q, got %q", tt.wantContains, msgs[1].Content)
				}
			}
		})
	}
}

func TestTranslator_Translate(t *testing.T) {
	tests := []struct {
		name      string
		completer *mockCompleter
		profile   TranslateProfile
		req       types.TranslateRequest
		wantText  string
		wantErr   bool
	}{
		{
			name: "successful translation",
			completer: &mockCompleter{
				response: "你好",
				usage:    types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			profile:  TranslateProfile{Name: "test", Model: "gpt-4", SystemPrompt: "Translate."},
			req:      types.TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "zh"},
			wantText: "你好",
			wantErr:  false,
		},
		{
			name: "completer error",
			completer: &mockCompleter{
				err: context.DeadlineExceeded,
			},
			profile:  TranslateProfile{Name: "test", Model: "gpt-4", SystemPrompt: "Translate."},
			req:      types.TranslateRequest{Text: "Hello", SourceLang: "en", TargetLang: "zh"},
			wantText: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewTranslator(nil) // no cache for this test

			result, err := tr.Translate(context.Background(), tt.completer, tt.profile, tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("Translate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result.Text != tt.wantText {
				t.Errorf("Translate() text = %q, want %q", result.Text, tt.wantText)
			}
		})
	}
}

// contains is a simple helper to check if substr is in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
