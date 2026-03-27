# ctl_device

分布式 AI 任务调度中间层，连接 OpenClaw 与 Trae CN / Claude Code / Cursor。

## 快速开始

### 下载最新版

```bash
# Linux (amd64)
curl -L https://github.com/0xdevelop/ctl_device/releases/latest/download/ctl_device_linux_amd64 -o ctl_device
chmod +x ctl_device

# Linux (arm64)
curl -L https://github.com/0xdevelop/ctl_device/releases/latest/download/ctl_device_linux_arm64 -o ctl_device
chmod +x ctl_device

# macOS (amd64)
curl -L https://github.com/0xdevelop/ctl_device/releases/latest/download/ctl_device_darwin_amd64 -o ctl_device
chmod +x ctl_device

# macOS (arm64)
curl -L https://github.com/0xdevelop/ctl_device/releases/latest/download/ctl_device_darwin_arm64 -o ctl_device
chmod +x ctl_device

# Windows (amd64)
curl -L https://github.com/0xdevelop/ctl_device/releases/latest/download/ctl_device_windows_amd64.exe -o ctl_device.exe
```

### 启动 Full 模式（默认）

```bash
# 直接启动，默认 full 模式（JSON-RPC + MCP SSE + Dashboard 全部启动）
./ctl_device

# 带 token
./ctl_device --token your-secret

# 指定配置文件
./ctl_device --config /path/to/config.yaml

# 指定端口
./ctl_device --jsonrpc-port 3711 --mcp-port 3710 --dashboard-port 3712
```

### 启动 Client 模式

其他机器指向 Full 节点，自动以 Client 身份运行：

```bash
# 连接到 full 节点
./ctl_device --connect http://192.168.1.100:3711

# 带 token
./ctl_device --connect http://your-vps.com:3711 --token your-secret
```

Client 启动后会自动注册 agent、发送心跳。

### 环境变量

```bash
export CTL_DEVICE_TOKEN=your-secret-token
export CTL_DEVICE_SERVER=http://192.168.1.100:3711
export CTL_DEVICE_AGENT_ID=macbook-m4
```

## 统一配置文件

Full 和 Client 共用同一个配置文件（`conf/config.yaml`，首次启动自动生成）：

```yaml
# mode: full (默认) | client
# 设置 connect 后自动变 client
mode: full
connect: ""

server:
  bind: "0.0.0.0"
  jsonrpc_port: 3711
  mcp_port: 3710
  dashboard_port: 3712
  grpc_port: 3713
  token: ""
  state_dir: "~/.config/ctl_device"
  snapshot_interval_seconds: 30
  heartbeat_timeout_seconds: 45
  tls:
    enabled: false

client:
  agent_id: ""
  role: "executor"
  capabilities: []

notify:
  channel: "none"
  target: ""

projects: []
```

## CLI 命令

```bash
# Full 模式（默认，启动所有协议）
ctl_device                          # JSON-RPC :3711 + MCP SSE :3710 + gRPC :3713 + Dashboard :3712
ctl_device --token your-secret      # 带认证
ctl_device --grpc-port 4000         # 自定义 gRPC 端口

# Client 模式
ctl_device --connect <addr>         # 自动注册 agent + 发送心跳

# 工具命令
ctl_device mcp                      # MCP stdio 模式（供 IDE 配置）
ctl_device status                   # 查询项目/任务状态
ctl_device dispatch -p <project> -f <task.json>  # 下发任务
ctl_device logs -f                  # 实时日志（SSE）

# 向后兼容（已废弃，仍可用）
ctl_device server                   # → 等同于 ctl_device
ctl_device client mcp               # → 等同于 ctl_device mcp
```

## IDE 配置（MCP）

### Claude Code（.claude.json）

```json
{
  "mcpServers": {
    "ctl_device": {
      "command": "/path/to/ctl_device",
      "args": ["mcp", "--server", "http://your-server:3711", "--token", "xxx"]
    }
  }
}
```

### Trae CN / Cursor / JB / VSCode

```json
{
  "mcpServers": {
    "ctl_device": {
      "command": "ctl_device",
      "args": ["mcp", "--server", "http://localhost:3711"]
    }
  }
}
```

### OpenClaw 配置

```json
{
  "mcp": {
    "servers": {
      "ctl_device": {
        "command": "ctl_device",
        "args": ["mcp", "--server", "http://localhost:3711"]
      }
    }
  }
}
```

## gRPC 接入

gRPC server 默认监听 `:3713`，支持所有核心操作，适合高性能场景或自定义客户端。

### proto 文件

```
api/proto/ctl_device.proto
```

### 生成 Go client

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       api/proto/ctl_device.proto
```

### 认证

在 gRPC metadata 中传递 token：

```go
ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
    "authorization", "Bearer your-secret-token",
))
```

### 端口一览

| 协议 | 默认端口 | 说明 |
|------|----------|------|
| JSON-RPC HTTP | 3711 | 主要 RPC API |
| MCP SSE | 3710 | IDE / OpenClaw 接入 |
| Web Dashboard | 3712 | 可视化管理界面 |
| gRPC | 3713 | 高性能 RPC |

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    ctl_device (Full 模式)                        │
│                 （VPS / 局域网任意一台机器）                       │
│                                                                  │
│  ┌─────────────┐  ┌─────────────────┐  ┌────────────────────┐  │
│  │ MCP SSE     │  │ JSON-RPC HTTP   │  │ Web Dashboard      │  │
│  │ :3710/sse   │  │ :3711           │  │ :3712              │  │
│  └──────┬──────┘  └────────┬────────┘  └────────────────────┘  │
│         └──────────────────┘                                    │
│                     ▼                                           │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Core Engine                                             │  │
│  │  AgentManager / ProjectStore / TaskScheduler             │  │
│  │  EventBus / Notifier / RecoveryManager                   │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                  │
│  持久化层：~/.config/ctl_device/ (JSON files)                    │
└─────────────────────────────────────────────────────────────────┘
         ▲                              ▲
         │ MCP SSE / JSON-RPC           │ --connect / MCP stdio
┌────────────────────┐      ┌──────────────────────────────────┐
│  调度者             │      │  执行者（任意机器）               │
│  OpenClaw + MCP    │      │  ctl_device --connect <addr>     │
│                    │      │  或 IDE + ctl_device mcp          │
└────────────────────┘      └──────────────────────────────────┘
```

## 核心功能

- **分布式调度**：支持跨平台（Linux / Windows / macOS）、跨网络（局域网 / 公网）、多机并发
- **MCP 协议**：兼容 Model Context Protocol，无缝对接各种 AI IDE
- **JSON-RPC**：简单的 HTTP API，易于集成
- **容灾恢复**：断线重连、Server 重启恢复、Token 限制处理、超时重试
- **Token 认证**：可选的共享 token 认证，支持 TLS
- **Web Dashboard**：实时查看在线 Agent、项目状态、任务进度
- **事件流**：SSE 实时推送任务状态变更

## 开发

```bash
go build ./...
go test -race -timeout 5m ./...
go run ./cmd/ctl_device
```

## 发版

```bash
./git_tag.sh       # 自动递增版本号，生成 changelog，push + tag
./git_tag.sh custom  # 手动指定版本号
```

## 许可证

MIT License
