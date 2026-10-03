# opencode-session

`opencode-session` 是一个很小的本地反向代理，专门给 OpenCode Go 网关补上必需的
`x-opencode-session` 请求头。它只做请求转发和 Header 注入，不改 body，不碰模型、
消息、工具字段，也不处理 API Key。

英文文档：[README.md](README.md)。

这个项目刻意不做成 Codex 插件。Codex 插件只能组合 Skill、MCP、Hook 和 App
连接器，它不在模型请求链路上，也无法改写供应商请求 Header。

## 为什么需要它

OpenCode Go 要求每个对话请求带稳定的 `x-opencode-session`，用于路由和 Prompt
缓存。Codex 已经会发送 `thread-id`、`session-id` 这类对话标识，但 Codex++ 的自定义
供应商 Header 是静态的，无法按对话动态变化。这个代理用来补上这一层。

## 快速开始

下载 Release 二进制，或本地构建：

```sh
go build -trimpath -o opencode-session .
```

仓库发布后，Go 用户也可以直接安装：

```sh
go install github.com/lh/opencode-session@latest
```

启动：

```sh
./opencode-session
```

默认配置：

```text
监听地址：127.0.0.1:18777
上游地址：https://opencode.ai/zen/go/v1
日志级别：info
```

然后把 Codex++ 供应商的 `base_url` 改成：

```toml
base_url = "http://127.0.0.1:18777"
```

`http://127.0.0.1:18777/v1` 也可以，方便直接沿用原来的 OpenAI 风格配置。

如果是 Codex++ 的 chat-completions 转换链路，把 `upstreamBaseUrl` 指向同一个本地
地址。真正的 OpenCode API Key 继续放在 Codex++ 供应商配置里，代理只透传
`Authorization`。

## 命令行参数

```text
--listen string       本地监听地址，默认 127.0.0.1:18777
--upstream string     OpenCode Go 上游地址，默认 https://opencode.ai/zen/go/v1
--log-level string    debug、info、warn、error，默认 info
--max-body int        最多检查多少字节的请求体，默认 16777216
--proxy string        可选的上游代理，留空表示直连
--version             输出版本
```

环境变量：

```text
OPENCODE_SESSION_LISTEN
OPENCODE_SESSION_UPSTREAM
OPENCODE_SESSION_LOG_LEVEL
OPENCODE_SESSION_PROXY
```

## 上游代理行为

上游连接默认**始终直连**，会刻意忽略 `HTTP_PROXY` / `HTTPS_PROXY` 和 Windows
系统代理设置。

这一点在 Clash 开全局模式时很关键。OpenCode Go 会对来自境外 IP 的请求返回区域
限制错误，所以继承系统代理会让供应商直接不可用。让上游这一跳直连就能避免冲突，
而浏览器和其他工具照常走系统代理。

确实需要走上游代理时，显式传入：

```sh
opencode-session --proxy http://127.0.0.1:7890
```

TUN 模式的代理仍会在网络层接管全部流量，这种情况下请在代理软件里为
`opencode.ai` 加一条直连规则。

## 会话 ID 解析顺序

如果请求已经带 `x-opencode-session`，代理不会覆盖。否则按以下顺序解析：

1. `thread-id` 请求头。
2. `session-id` 请求头。
3. JSON body 顶层字段：`thread_id`、`session_id`、`conversation_id`。
4. JSON body 中 `client_metadata["x-codex-turn-metadata"]`。
5. `x-codex-turn-metadata` 请求头。
6. 生成 `session-<uuid>` 兜底。

兜底只保证请求可用，不能保证跨轮 Prompt 缓存亲和性。如果 chat 链路频繁出现兜底
警告，需要让上游客户端在 Header 或 Body 中带上对话 ID。

## 流式响应

Responses API 和 chat-completions 的流式响应都会立即 flush 透传，包括 SSE。

## 安全说明

默认只监听本机回环地址。代理不会保存或打印 Authorization，也不会记录请求 body。
除非你明确知道风险，否则不要把监听地址暴露到公网网卡。

## 排错

如果 OpenCode 仍然返回 `MissingSessionID`，先确认供应商 `base_url` 指向
`http://127.0.0.1:18777`，并且代理进程正在运行。代理会为每个请求记录
`session_source`；如果出现 `generated`，说明请求里没有可用的对话 ID。

## 开发

```sh
go test ./...
go vet ./...
```

Windows 下也可以直接运行：

```powershell
.\scripts\test.ps1
```

## 发布到 GitHub

先在 GitHub 上建一个空仓库（不要勾选 README、.gitignore、License），然后在
项目根目录执行一条命令：

```powershell
.\scripts\publish.ps1 -RepoUrl https://github.com/你的用户名/opencode-session.git
```

脚本会依次跑测试、把 Go module 路径改成你的仓库地址、提交、推送 `main`，并推送
`v0.1.0` 标签触发 Release 构建。`-SkipTag` 可以只推送不发布，
`-Version v0.2.0` 可以指定其他版本号。

当前 module path 是 `github.com/lh/opencode-session`。如果发布到其他 GitHub
账号，需要同步修改 `go.mod` 和 `internal/` 下的 import 前缀。

Windows 与 systemd 启动示例见 `examples/`。

## 许可证

MIT
