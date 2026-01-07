package openai

import "encoding/json"

// Event types from OpenAI Realtime API.
const (
	EventTranscriptionCompleted = "conversation.item.input_audio_transcription.completed"
	EventTranscriptionDelta     = "conversation.item.input_audio_transcription.delta"
	EventError                  = "error"

	// VAD Events
	EventSpeechStarted = "input_audio_buffer.speech_started"
	EventSpeechStopped = "input_audio_buffer.speech_stopped"
	EventItemAdded     = "conversation.item.added"
	EventItemDone      = "conversation.item.done"
)

// VADType specifies the type of voice activity detection.
type VADType string

const (
	VADTypeSemanticVAD VADType = "semantic_vad"
	VADTypeServerVAD   VADType = "server_vad"
)

// VADEagerness controls how aggressive the VAD is.
type VADEagerness string

const (
	VADEagernessLow    VADEagerness = "low"
	VADEagernessMedium VADEagerness = "medium"
	VADEagernessHigh   VADEagerness = "high"
	VADEagernessAuto   VADEagerness = "auto"
)

// TurnDetection configures voice activity detection.
type TurnDetection struct {
	Type              VADType      `json:"type"`
	Eagerness         VADEagerness `json:"eagerness,omitempty"`
	CreateResponse    bool         `json:"create_response,omitempty"`
	InterruptResponse bool         `json:"interrupt_response,omitempty"`
}

// SessionUpdate is a client event to update session configuration.
type SessionUpdate struct {
	Type    string `json:"type"`
	Session struct {
		TurnDetection *TurnDetection `json:"turn_detection,omitempty"`
	} `json:"session"`
}

// Event is a discriminated union for Realtime API events.
// Check the concrete type via type switch.
type Event interface {
	eventType() string
}

// SpeechStartedEvent is emitted when VAD detects speech.
type SpeechStartedEvent struct {
	EventID      string `json:"event_id"`
	AudioStartMs int    `json:"audio_start_ms"`
	ItemID       string `json:"item_id"`
}

func (SpeechStartedEvent) eventType() string { return EventSpeechStarted }

// SpeechStoppedEvent is emitted when VAD detects silence.
type SpeechStoppedEvent struct {
	EventID    string `json:"event_id"`
	AudioEndMs int    `json:"audio_end_ms"`
	ItemID     string `json:"item_id"`
}

func (SpeechStoppedEvent) eventType() string { return EventSpeechStopped }

// ItemAddedEvent is emitted when an item (like user speech) is committed.
type ItemAddedEvent struct {
	EventID        string `json:"event_id"`
	PreviousItemID string `json:"previous_item_id"`
	Item           struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Status  string `json:"status"`
		Role    string `json:"role"`
		Content []struct {
			Type       string `json:"type"`
			Transcript string `json:"transcript,omitempty"`
		} `json:"content"`
	} `json:"item"`
}

func (ItemAddedEvent) eventType() string { return EventItemAdded }

// ItemDoneEvent is emitted when an item is fully processed.
type ItemDoneEvent struct {
	EventID        string `json:"event_id"`
	PreviousItemID string `json:"previous_item_id"`
	Item           struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"`
		Role   string `json:"role"`
	} `json:"item"`
}

func (ItemDoneEvent) eventType() string { return EventItemDone }

// TranscriptEvent is emitted when transcription completes.
type TranscriptEvent struct {
	EventID    string `json:"event_id"`
	ItemID     string `json:"item_id"`
	Transcript string `json:"transcript"`
}

func (TranscriptEvent) eventType() string { return EventTranscriptionCompleted }

// TranscriptDeltaEvent is emitted for streaming transcription updates.
type TranscriptDeltaEvent struct {
	EventID    string `json:"event_id"`
	ItemID     string `json:"item_id"`
	ContentIdx int    `json:"content_index"`
	Delta      string `json:"delta"`
}

func (TranscriptDeltaEvent) eventType() string { return EventTranscriptionDelta }

// ErrorEvent is emitted when an API error occurs.
type ErrorEvent struct {
	EventID string `json:"event_id"`
	Error   struct {
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
		Message string `json:"message"`
		Param   string `json:"param,omitempty"`
	} `json:"error"`
}

func (ErrorEvent) eventType() string { return EventError }

// UnknownEvent holds events we don't recognize.
type UnknownEvent struct {
	EventID string `json:"event_id"`
	Type    string `json:"type"`
	Raw     json.RawMessage
}

func (e UnknownEvent) eventType() string { return e.Type }

// ParseEvent unmarshals JSON into the appropriate Event type.
func ParseEvent(data []byte) (Event, error) {
	// Parse type field first.
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}

	switch header.Type {
	case EventSpeechStarted:
		var e SpeechStartedEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventSpeechStopped:
		var e SpeechStoppedEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventItemAdded:
		var e ItemAddedEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventItemDone:
		var e ItemDoneEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventTranscriptionCompleted:
		var e TranscriptEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	case EventTranscriptionDelta:
		var e TranscriptDeltaEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil

	case EventError:
		var e ErrorEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		return e, nil
	default:
		return UnknownEvent{Type: header.Type, Raw: data}, nil
	}
}
