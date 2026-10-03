package session

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	SourceExistingHeader     = "existing-header"
	SourceThreadIDHeader     = "thread-id-header"
	SourceSessionIDHeader    = "session-id-header"
	SourceBodyThreadID       = "body.thread_id"
	SourceBodySessionID      = "body.session_id"
	SourceBodyConversationID = "body.conversation_id"
	SourceBodyClientMetadata = "body.client_metadata"
	SourceTurnMetadataHeader = "x-codex-turn-metadata-header"
	SourceGenerated          = "generated"
)

type Resolver struct {
	GenerateID func() string
}

type Resolution struct {
	ID        string
	Source    string
	Generated bool
}

func NewResolver() Resolver {
	return Resolver{GenerateID: generateID}
}

func (r Resolver) Resolve(method string, header http.Header, body []byte) Resolution {
	if !strings.EqualFold(method, http.MethodPost) {
		return Resolution{}
	}

	if id := headerValue(header, "x-opencode-session"); id != "" {
		return Resolution{ID: id, Source: SourceExistingHeader}
	}
	if id := headerValue(header, "thread-id"); id != "" {
		return Resolution{ID: id, Source: SourceThreadIDHeader}
	}
	if id := headerValue(header, "session-id"); id != "" {
		return Resolution{ID: id, Source: SourceSessionIDHeader}
	}
	if id, source := fromJSONBody(body); id != "" {
		return Resolution{ID: id, Source: source}
	}
	if id, source := fromTurnMetadataHeader(header); id != "" {
		return Resolution{ID: id, Source: source}
	}

	generate := r.GenerateID
	if generate == nil {
		generate = generateID
	}
	return Resolution{ID: generate(), Source: SourceGenerated, Generated: true}
}

func headerValue(header http.Header, name string) string {
	if value := strings.TrimSpace(header.Get(name)); value != "" {
		return value
	}
	for key, values := range header {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func fromJSONBody(body []byte) (string, string) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return "", ""
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return "", ""
	}

	for _, field := range []string{"thread_id", "session_id", "conversation_id"} {
		if value, ok := rawString(object[field]); ok && value != "" {
			return value, "body." + field
		}
	}

	for _, field := range []string{"client_metadata", "metadata"} {
		if value, ok := nestedMetadata(object[field]); ok && value != "" {
			return value, SourceBodyClientMetadata
		}
	}

	return "", ""
}

func nestedMetadata(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", false
	}

	for _, field := range []string{"thread_id", "session_id", "conversation_id"} {
		if value, ok := rawString(object[field]); ok && value != "" {
			return value, true
		}
	}

	for _, field := range []string{"x-codex-turn-metadata", "x_codex_turn_metadata"} {
		raw := object[field]
		if len(raw) == 0 {
			continue
		}
		if id, ok := metadataJSONString(raw); ok {
			return id, true
		}
	}

	return "", false
}

func fromTurnMetadataHeader(header http.Header) (string, string) {
	value := headerValue(header, "x-codex-turn-metadata")
	if value == "" {
		return "", ""
	}
	if id, ok := metadataJSONString([]byte(value)); ok {
		return id, SourceTurnMetadataHeader
	}
	return "", ""
}

func metadataJSONString(data []byte) (string, bool) {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err == nil {
		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return "", false
		}
		return metadataJSONString([]byte(encoded))
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return "", false
	}
	for _, field := range []string{"thread_id", "session_id"} {
		if value, ok := rawString(object[field]); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func rawString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func generateID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "session-fallback"
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(data[:])
	return "session-" + encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}
