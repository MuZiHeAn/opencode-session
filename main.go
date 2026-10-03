package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/MuZiHeAn/opencode-session/internal/proxy"
)

var version = "dev"

const (
	defaultListen   = "127.0.0.1:18777"
	defaultUpstream = "https://opencode.ai/zen/go/v1"
	defaultMaxBody  = int64(16 << 20)
	defaultLogLevel = "info"
)

func main() {
	config, err := parseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	logger, err := newLogger(config.logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	upstream, err := url.Parse(config.upstream)
	if err != nil {
		logger.Error("invalid upstream URL", "error", err)
		os.Exit(2)
	}
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		logger.Error("upstream URL must use http or https", "scheme", upstream.Scheme)
		os.Exit(2)
	}
	if upstream.Host == "" {
		logger.Error("upstream URL must include a host")
		os.Exit(2)
	}

	var outboundProxy *url.URL
	if strings.TrimSpace(config.proxy) != "" {
		outboundProxy, err = url.Parse(config.proxy)
		if err != nil {
			logger.Error("invalid proxy URL", "error", err)
			os.Exit(2)
		}
		if outboundProxy.Scheme != "http" && outboundProxy.Scheme != "https" {
			logger.Error("proxy URL must use http or https", "scheme", outboundProxy.Scheme)
			os.Exit(2)
		}
	}

	handler, err := proxy.New(proxy.Config{
		Upstream:     upstream,
		MaxBodyBytes: config.maxBody,
		Logger:       logger,
		Proxy:        outboundProxy,
	})
	if err != nil {
		logger.Error("failed to create proxy", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              config.listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("opencode-session started",
			"version", version,
			"listen", config.listen,
			"upstream", upstream.String(),
			"max_body_bytes", config.maxBody,
		)
		errCh <- server.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logger.Info("shutting down", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}
}

type config struct {
	listen   string
	upstream string
	logLevel string
	maxBody  int64
	proxy    string
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		listen:   envOrDefault("OPENCODE_SESSION_LISTEN", defaultListen),
		upstream: envOrDefault("OPENCODE_SESSION_UPSTREAM", defaultUpstream),
		logLevel: envOrDefault("OPENCODE_SESSION_LOG_LEVEL", defaultLogLevel),
		maxBody:  defaultMaxBody,
		proxy:    strings.TrimSpace(os.Getenv("OPENCODE_SESSION_PROXY")),
	}

	fs := flag.NewFlagSet("opencode-session", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: opencode-session [options]\n\n")
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.listen, "listen", cfg.listen, "local listen address")
	fs.StringVar(&cfg.upstream, "upstream", cfg.upstream, "OpenCode Go upstream base URL")
	fs.StringVar(&cfg.logLevel, "log-level", cfg.logLevel, "log level: debug, info, warn, error")
	fs.Int64Var(&cfg.maxBody, "max-body", cfg.maxBody, "maximum request body bytes to inspect")
	fs.StringVar(&cfg.proxy, "proxy", cfg.proxy, "optional outbound proxy; empty means direct")
	showVersion := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}
	if strings.TrimSpace(cfg.listen) == "" {
		return config{}, errors.New("--listen cannot be empty")
	}
	if strings.TrimSpace(cfg.upstream) == "" {
		return config{}, errors.New("--upstream cannot be empty")
	}
	if cfg.maxBody <= 0 {
		return config{}, errors.New("--max-body must be greater than zero")
	}
	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func newLogger(level string) (*slog.Logger, error) {
	var slogLevel slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid --log-level %q", level)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})), nil
}
