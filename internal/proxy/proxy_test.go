package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/MuZiHeAn/opencode-session/internal/session"
)

func TestProxyInjectsSessionAndPreservesRequest(t *testing.T) {
	type captured struct {
		path          string
		authorization string
		contentType   string
		session       string
		body          string
	}
	gotCh := make(chan captured, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		gotCh <- captured{
			path:          req.URL.Path,
			authorization: req.Header.Get("Authorization"),
			contentType:   req.Header.Get("Content-Type"),
			session:       req.Header.Get("x-opencode-session"),
			body:          string(body),
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	body := `{"model":"deepseek-v4-pro","input":"hello"}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("thread-id", "thread-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := <-gotCh

	if got.path != "/zen/go/v1/responses" {
		t.Fatalf("path = %q", got.path)
	}
	if got.authorization != "Bearer test-key" {
		t.Fatalf("authorization = %q", got.authorization)
	}
	if got.contentType != "application/json" {
		t.Fatalf("content-type = %q", got.contentType)
	}
	if got.session != "thread-123" {
		t.Fatalf("session = %q", got.session)
	}
	if got.body != body {
		t.Fatalf("body changed: got %q want %q", got.body, body)
	}
}

func TestProxyDoesNotOverwriteExistingSessionHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("x-opencode-session"); got != "explicit-session" {
			http.Error(w, "unexpected session", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/responses", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-opencode-session", "explicit-session")
	req.Header.Set("thread-id", "thread-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestProxyDoesNotInjectForNonPOST(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("x-opencode-session"); got != "" {
			http.Error(w, "unexpected session", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	resp, err := http.Get(server.URL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestProxyUsesBodySessionIDForChatCompletions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("x-opencode-session"); got != "chat-session" {
			http.Error(w, "unexpected session", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	body := `{"session_id":"chat-session","messages":[]}`
	resp, err := http.Post(server.URL+"/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestProxyAcceptsLocalV1Prefix(t *testing.T) {
	gotPathCh := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPathCh <- req.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"thread_id":"thread-v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	gotPath := <-gotPathCh
	if gotPath != "/zen/go/v1/responses" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestProxyStreamsSSEResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "response writer cannot flush", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: one\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: two\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", defaultMaxBodyBytes)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/responses", bytes.NewReader([]byte(`{"thread_id":"sse-thread"}`)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "data: one\n\ndata: two\n\n" {
		t.Fatalf("body = %q", got)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("content-type = %q", contentType)
	}
}

func TestProxyOversizedBodyUsesHeadersOnly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := req.Header.Get("x-opencode-session"); got != "header-session" {
			http.Error(w, "unexpected session", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(req.Body)
		if string(body) != `{"session_id":"body-session","padding":"1234567890"}` {
			http.Error(w, "unexpected body", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	server := newTestProxy(t, upstream.URL+"/zen/go/v1", 16)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/chat/completions", strings.NewReader(`{"session_id":"body-session","padding":"1234567890"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("thread-id", "header-session")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func newTestProxy(t *testing.T, upstream string, maxBodyBytes int64) *httptest.Server {
	t.Helper()
	parsed, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(Config{
		Upstream:     parsed,
		MaxBodyBytes: maxBodyBytes,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:     session.Resolver{GenerateID: func() string { return "session-generated" }},
	})
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(handler)
}
