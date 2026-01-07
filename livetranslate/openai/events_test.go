package openai

import (
	"testing"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantType  string
		wantErr   bool
		checkFunc func(t *testing.T, e Event)
	}{
		{
			name: "TranscriptCompleted",
			json: `{
				"type": "conversation.item.input_audio_transcription.completed",
				"event_id": "evt_123",
				"item_id": "item_123",
				"transcript": "Hello world"
			}`,
			wantType: EventTranscriptionCompleted,
			checkFunc: func(t *testing.T, e Event) {
				te, ok := e.(TranscriptEvent)
				if !ok {
					t.Fatalf("got %T, want TranscriptEvent", e)
				}
				if te.Transcript != "Hello world" {
					t.Errorf("Transcript = %q, want %q", te.Transcript, "Hello world")
				}
				if te.ItemID != "item_123" {
					t.Errorf("ItemID = %q, want %q", te.ItemID, "item_123")
				}
			},
		},
		{
			name: "TranscriptionDelta",
			json: `{
				"type": "conversation.item.input_audio_transcription.delta",
				"event_id": "evt_124",
				"item_id": "item_123",
				"content_index": 0,
				"delta": "Hello"
			}`,
			wantType: EventTranscriptionDelta,
			checkFunc: func(t *testing.T, e Event) {
				de, ok := e.(TranscriptDeltaEvent)
				if !ok {
					t.Fatalf("got %T, want TranscriptDeltaEvent", e)
				}
				if de.Delta != "Hello" {
					t.Errorf("Delta = %q, want %q", de.Delta, "Hello")
				}
			},
		},

		{
			name: "Error",
			json: `{
				"type": "error",
				"event_id": "evt_err",
				"error": {
					"type": "invalid_request_error",
					"message": "Invalid API key"
				}
			}`,
			wantType: EventError,
			checkFunc: func(t *testing.T, e Event) {
				ee, ok := e.(ErrorEvent)
				if !ok {
					t.Fatalf("got %T, want ErrorEvent", e)
				}
				if ee.Error.Type != "invalid_request_error" {
					t.Errorf("Error.Type = %q, want %q", ee.Error.Type, "invalid_request_error")
				}
			},
		},
		{
			name: "UnknownType",
			json: `{
				"type": "unknown.event",
				"event_id": "evt_u"
			}`,
			wantType: "unknown.event",
			checkFunc: func(t *testing.T, e Event) {
				ue, ok := e.(UnknownEvent)
				if !ok {
					t.Fatalf("got %T, want UnknownEvent", e)
				}
				if ue.Type != "unknown.event" {
					t.Errorf("Type = %q, want %q", ue.Type, "unknown.event")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := ParseEvent([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if e.eventType() != tt.wantType {
				t.Errorf("eventType() = %q, want %q", e.eventType(), tt.wantType)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, e)
			}
		})
	}
}
