package realtime

import "encoding/json"

// ClientEvent represents a message sent to or received from the API.
type ClientEvent struct {
	EventID string         `json:"event_id,omitempty"`
	Type    string         `json:"type"`
	Error   *RealtimeError `json:"error,omitempty"`
	// Store all other fields dynamically
	Extra map[string]interface{} `json:"-"`
}

// RealtimeError represents an error from the Realtime API
type RealtimeError struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling to capture all fields.
func (e *ClientEvent) UnmarshalJSON(data []byte) error {
	// First unmarshal into a map to get all fields
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// Then unmarshal type-specific fields
	type Alias ClientEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Store extra fields
	e.Extra = make(map[string]interface{})
	for k, v := range rawMap {
		if k != "event_id" && k != "type" && k != "error" {
			e.Extra[k] = v
		}
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Event Builders
// ─────────────────────────────────────────────────────────────────────────────

// EventSessionUpdate creates a session.update event.
func EventSessionUpdate(session map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":    "session.update",
		"session": session,
	}
}

// EventInputAudioBufferAppend creates an input_audio_buffer.append event.
func EventInputAudioBufferAppend(audioBase64 string) map[string]interface{} {
	return map[string]interface{}{
		"type":  "input_audio_buffer.append",
		"audio": audioBase64,
	}
}

// EventInputAudioBufferCommit creates an input_audio_buffer.commit event.
func EventInputAudioBufferCommit() map[string]interface{} {
	return map[string]interface{}{
		"type": "input_audio_buffer.commit",
	}
}

// EventResponseCreate creates a response.create event.
func EventResponseCreate(response map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"type":     "response.create",
		"response": response,
	}
}
