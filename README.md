# opencode-session

`opencode-session` is a tiny local reverse proxy for the OpenCode Go gateway.
It forwards requests unchanged and adds the required `x-opencode-session`
header when the client does not send one.

This is a dedicated replacement for ad-hoc mitmproxy scripts in Codex++ and
Codex workflows. It does not handle API keys, does not modify request bodies,
and does not rewrite model or tool fields.

Chinese documentation: [README.zh-CN.md](README.zh-CN.md).

This project is intentionally not a Codex plugin. Codex plugins bundle
skills, MCP servers, hooks, and app connectors, but they do not run in the
provider request path and cannot rewrite model request headers.

## Why

OpenCode Go requires a stable per-conversation `x-opencode-session` header for
routing and prompt-cache affinity. Codex already carries a conversation ID in
headers such as `thread-id` and `session-id`, but Codex++ custom provider
headers are static. This proxy bridges that gap without changing Codex++.

## Quick Start

Download a release binary or build from source:

```sh
go build -trimpath -o opencode-session .
```

After the repository is published, Go users can also install it with:

```sh
go install github.com/lh/opencode-session@latest
```

Run it:

```sh
./opencode-session
```

Default values:

```text
listen:   127.0.0.1:18777
upstream: https://opencode.ai/zen/go/v1
log:      info
```

Then set the Codex++ provider base URL to:

```toml
base_url = "http://127.0.0.1:18777"
```

`http://127.0.0.1:18777/v1` also works, which makes it easy to copy an
existing OpenAI-style provider configuration.

For a Codex++ chat-completions conversion profile, point `upstreamBaseUrl` to
the same local address. Keep the real OpenCode API key in Codex++ exactly as
before. The proxy passes `Authorization` through untouched.

## CLI

```text
--listen string       local listen address (default "127.0.0.1:18777")
--upstream string     OpenCode Go upstream base URL (default "https://opencode.ai/zen/go/v1")
--log-level string    debug, info, warn, error (default "info")
--max-body int        maximum request body bytes to inspect (default 16777216)
--proxy string        optional outbound proxy; empty means direct
--version             print version and exit
```

Environment variables:

```text
OPENCODE_SESSION_LISTEN
OPENCODE_SESSION_UPSTREAM
OPENCODE_SESSION_LOG_LEVEL
OPENCODE_SESSION_PROXY
```

## Upstream Proxy Behavior

The upstream connection is **always direct by default**. The proxy
deliberately ignores `HTTP_PROXY` / `HTTPS_PROXY` and the Windows system
proxy settings.

This matters when a local proxy tool such as Clash runs in global mode.
OpenCode Go rejects requests that exit through a foreign IP with a region
error, so inheriting the system proxy breaks the provider. Keeping the
upstream leg direct avoids that conflict, while your browser and other tools
keep using the system proxy.

If you really need an outbound proxy, pass it explicitly:

```sh
opencode-session --proxy http://127.0.0.1:7890
```

A TUN-style proxy still captures traffic at the network layer. In that case
add a direct rule for `opencode.ai` inside the proxy tool instead.

## Session Resolution

The proxy keeps an existing `x-opencode-session` header if present. Otherwise
it resolves the session ID in this order:

1. `thread-id` request header.
2. `session-id` request header.
3. JSON body fields: `thread_id`, `session_id`, `conversation_id`.
4. `client_metadata["x-codex-turn-metadata"]` in the JSON body.
5. `x-codex-turn-metadata` request header.
6. Generated `session-<uuid>` fallback.

The fallback keeps the request working, but it cannot provide prompt-cache
affinity across turns. If you see generated-session warnings on a chat path,
make the upstream client send a conversation ID in a header or body field.

## Streaming

Responses are streamed through with immediate flushing, including SSE used by
Responses API and chat-completions streaming.

## Security

The default listen address is loopback only. The proxy does not store or log
Authorization headers or request bodies. Do not bind it to a public interface
unless you understand the risk.

## Troubleshooting

If OpenCode still returns `MissingSessionID`, verify that the provider base URL
points to `http://127.0.0.1:18777` and that the proxy process is running. The
proxy logs `session_source` for each request; `generated` means the request did
not carry a usable conversation ID.

## Development

```sh
go test ./...
go vet ./...
```

On Windows, you can run the same checks with:

```powershell
.\scripts\test.ps1
```

## Publishing

Create an empty repository on GitHub first (no README, no .gitignore, no
license), then run one command from the project root:

```powershell
.\scripts\publish.ps1 -RepoUrl https://github.com/yourname/opencode-session.git
```

The script runs the checks, rewrites the Go module path to match your
repository, commits, pushes `main`, and pushes a `v0.1.0` tag that triggers
the release workflow. Use `-SkipTag` to push without tagging, or
`-Version v0.2.0` to pick a different tag.

The module path is `github.com/lh/opencode-session`. If you publish under a
different GitHub owner, update `go.mod` and the imports under `internal/`.

See `examples/` for Windows and systemd launch examples.

## License

MIT
