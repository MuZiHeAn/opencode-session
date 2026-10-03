package session

import (
	"net/http"
	"strings"
	"testing"
)

func TestResolvePrefersExistingHeader(t *testing.T) {
	header := http.Header{}
	header.Set("x-opencode-session", "existing")
	header.Set("thread-id", "thread")

	got := NewResolver().Resolve(http.MethodPost, header, []byte(`{"session_id":"body"}`))
	if got.ID != "existing" || got.Source != SourceExistingHeader {
		t.Fatalf("got %#v, want existing header", got)
	}
}

func TestResolveFromThreadIDHeader(t *testing.T) {
	header := http.Header{}
	header.Set("thread-id", "thread-123")

	got := NewResolver().Resolve(http.MethodPost, header, nil)
	if got.ID != "thread-123" || got.Source != SourceThreadIDHeader {
		t.Fatalf("got %#v, want thread-id header", got)
	}
}

func TestResolveFromSessionIDHeader(t *testing.T) {
	header := http.Header{}
	header.Set("session-id", "session-123")

	got := NewResolver().Resolve(http.MethodPost, header, nil)
	if got.ID != "session-123" || got.Source != SourceSessionIDHeader {
		t.Fatalf("got %#v, want session-id header", got)
	}
}

func TestResolveHeaderLookupIsCaseInsensitive(t *testing.T) {
	header := http.Header{}
	header["Thread-Id"] = []string{"thread-lowercase-key"}

	got := NewResolver().Resolve(http.MethodPost, header, nil)
	if got.ID != "thread-lowercase-key" || got.Source != SourceThreadIDHeader {
		t.Fatalf("got %#v, want case-insensitive header", got)
	}
}

func TestResolveFromBodyFields(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantID string
		source string
	}{
		{"thread_id", `{"thread_id":"thread-body"}`, "thread-body", SourceBodyThreadID},
		{"session_id", `{"session_id":"session-body"}`, "session-body", SourceBodySessionID},
		{"conversation_id", `{"conversation_id":"conversation-body"}`, "conversation-body", SourceBodyConversationID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewResolver().Resolve(http.MethodPost, http.Header{}, []byte(tt.body))
			if got.ID != tt.wantID || got.Source != tt.source {
				t.Fatalf("got %#v, want %s", got, tt.source)
			}
		})
	}
}

func TestResolveFromClientMetadata(t *testing.T) {
	body := []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"thread_id\":\"nested-thread\",\"session_id\":\"nested-session\"}"}}`)
	got := NewResolver().Resolve(http.MethodPost, http.Header{}, body)
	if got.ID != "nested-thread" || got.Source != SourceBodyClientMetadata {
		t.Fatalf("got %#v, want client_metadata value", got)
	}
}

func TestResolveFromTurnMetadataHeader(t *testing.T) {
	header := http.Header{}
	header.Set("x-codex-turn-metadata", `{"thread_id":"metadata-thread","session_id":"metadata-session"}`)

	got := NewResolver().Resolve(http.MethodPost, header, nil)
	if got.ID != "metadata-thread" || got.Source != SourceTurnMetadataHeader {
		t.Fatalf("got %#v, want x-codex-turn-metadata", got)
	}
}

func TestResolveMalformedJSONFallsBack(t *testing.T) {
	resolver := Resolver{GenerateID: func() string { return "session-test" }}
	got := resolver.Resolve(http.MethodPost, http.Header{}, []byte(`{"thread_id":`))
	if got.ID != "session-test" || !got.Generated {
		t.Fatalf("got %#v, want generated fallback", got)
	}
}

func TestResolveNonPOSTDoesNotGenerate(t *testing.T) {
	got := NewResolver().Resolve(http.MethodGet, http.Header{}, nil)
	if got.ID != "" || got.Generated {
		t.Fatalf("got %#v, want empty resolution", got)
	}
}

func TestGeneratedIDShape(t *testing.T) {
	got := NewResolver().Resolve(http.MethodPost, http.Header{}, nil)
	if !got.Generated || !strings.HasPrefix(got.ID, "session-") {
		t.Fatalf("got %#v, want generated session ID", got)
	}
}
