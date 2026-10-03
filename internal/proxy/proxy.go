package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/MuZiHeAn/opencode-session/internal/session"
)

const defaultMaxBodyBytes = int64(16 << 20)

type Config struct {
	Upstream     *url.URL
	MaxBodyBytes int64
	Logger       *slog.Logger
	Resolver     session.Resolver
	Proxy        *url.URL
}

type Proxy struct {
	handler      http.Handler
	logger       *slog.Logger
	resolver     session.Resolver
	maxBodyBytes int64
}

func New(config Config) (*Proxy, error) {
	if config.Upstream == nil {
		return nil, errors.New("upstream URL is required")
	}
	if config.Upstream.Scheme != "http" && config.Upstream.Scheme != "https" {
		return nil, errors.New("upstream URL must use http or https")
	}
	if config.Upstream.Host == "" {
		return nil, errors.New("upstream URL must include a host")
	}
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = defaultMaxBodyBytes
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Resolver.GenerateID == nil {
		config.Resolver = session.NewResolver()
	}

	target := *config.Upstream
	reverseProxy := httputil.NewSingleHostReverseProxy(&target)
	reverseProxy.Transport = newTransport(config.Proxy)
	originalDirector := reverseProxy.Director
	reverseProxy.Director = func(req *http.Request) {
		req.URL.Path = normalizeRequestPath(target.Path, req.URL.Path)
		req.URL.RawPath = ""
		originalDirector(req)
		req.Host = target.Host
	}
	reverseProxy.ModifyResponse = func(resp *http.Response) error {
		info, ok := requestInfoFrom(resp.Request)
		if !ok {
			return nil
		}
		config.Logger.Info("request completed",
			"method", resp.Request.Method,
			"path", resp.Request.URL.Path,
			"status", resp.StatusCode,
			"duration_ms", time.Since(info.startedAt).Milliseconds(),
			"session_id", info.sessionID,
			"session_source", info.sessionSource,
			"generated_session", info.generated,
		)
		return nil
	}
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		config.Logger.Error("upstream request failed",
			"method", req.Method,
			"path", req.URL.Path,
			"error", err,
		)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
	}
	reverseProxy.FlushInterval = -1

	return &Proxy{
		handler:      reverseProxy,
		logger:       config.Logger,
		resolver:     config.Resolver,
		maxBodyBytes: config.MaxBodyBytes,
	}, nil
}

func newTransport(proxyURL *url.URL) http.RoundTripper {
	// Codex++ and Clash both rewrite system proxy settings. The upstream call must
	// stay direct by default, otherwise OpenCode Go rejects the request as an
	// unsupported region.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return transport
}

func normalizeRequestPath(targetPath, requestPath string) string {
	targetPath = strings.TrimSuffix(targetPath, "/")
	if targetPath == "" {
		return requestPath
	}
	if strings.HasPrefix(requestPath, targetPath+"/") {
		return strings.TrimPrefix(requestPath, targetPath)
	}
	if strings.HasSuffix(targetPath, "/v1") && strings.HasPrefix(requestPath, "/v1/") {
		return strings.TrimPrefix(requestPath, "/v1")
	}
	return requestPath
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resolution := session.Resolution{}
	if strings.EqualFold(req.Method, http.MethodPost) {
		body, tooLarge, err := readBody(req, p.maxBodyBytes)
		if err != nil {
			p.logger.Error("failed to read request body", "error", err)
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if tooLarge {
			p.logger.Warn("request body exceeded max-body; body inspection skipped",
				"path", req.URL.Path,
				"max_body_bytes", p.maxBodyBytes,
			)
			resolution = p.resolver.Resolve(req.Method, req.Header, nil)
		} else {
			resolution = p.resolver.Resolve(req.Method, req.Header, body)
		}
		if resolution.ID != "" {
			req.Header.Set("x-opencode-session", resolution.ID)
		}
		if resolution.Generated {
			p.logger.Warn("generated fallback session ID",
				"method", req.Method,
				"path", req.URL.Path,
				"session_id", resolution.ID,
			)
		}
	}

	info := requestInfo{
		startedAt:     time.Now(),
		sessionID:     resolution.ID,
		sessionSource: resolution.Source,
		generated:     resolution.Generated,
	}
	req = req.WithContext(context.WithValue(req.Context(), requestInfoKey{}, info))
	p.handler.ServeHTTP(w, req)
}

func readBody(req *http.Request, maxBodyBytes int64) ([]byte, bool, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, false, nil
	}

	originalBody := req.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, maxBodyBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > maxBodyBytes {
		req.Body = &multiReadCloser{
			Reader: io.MultiReader(bytes.NewReader(body), originalBody),
			Closer: originalBody,
		}
		return nil, true, nil
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	return body, false, nil
}

type multiReadCloser struct {
	io.Reader
	io.Closer
}

type requestInfoKey struct{}

type requestInfo struct {
	startedAt     time.Time
	sessionID     string
	sessionSource string
	generated     bool
}

func requestInfoFrom(req *http.Request) (requestInfo, bool) {
	if req == nil {
		return requestInfo{}, false
	}
	info, ok := req.Context().Value(requestInfoKey{}).(requestInfo)
	return info, ok
}
