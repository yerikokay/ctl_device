# ctl_device - 架构设计

> 版本: v0.1 | 日期: 2026-03-24

## 项目定位

`ctl_device` 是一个**分布式 AI 任务调度中间层**，让 OpenClaw（规划者）和各种 AI 编码工具（Trae CN / Claude Code / Cursor / JB / VSCode）通过统一协议协作完成项目开发。

支持跨平台（Linux / Windows / macOS）、跨网络（局域网 / 公网）、多机并发。

---

## 核心概念

| 概念 | 说明 |
|------|------|
| **Server** | 中心调度节点，管理所有项目、任务、agent 状态 |
| **Scheduler** | 调度者角色（通常是 OpenClaw），负责下发任务、验证结果、推进进度 |
| **Executor** | 执行者角色（AI IDE），负责接单、编码、提交报告 |
| **Project** | 一个被管理的代码仓库，有独立任务队列和状态 |
| **Task** | 一个原子工作单元，有明确的描述和验收标准 |
| **Bridge File** | `bridge_trae.md` + `.bridge-state.json`，兼容旧协议的文件层 |

---

## 系统架构

```
┌─────────────────────────────────────────────────────────────────┐
│                 ctl_device (Full 模式)                           │
│              （VPS / 局域网任意一台机器）                         │
│                                                                  │
│  ┌──────────┐ ┌───────────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ MCP SSE  │ │ JSON-RPC HTTP │ │  gRPC    │ │  Dashboard  │  │
│  │ :3710    │ │ :3711         │ │ :3713    │ │  :3712      │  │
│  └────┬─────┘ └──────┬────────┘ └────┬─────┘ └─────────────┘  │
│       └──────────────┴───────────────┘                         │
│                              ▼                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Core Engine                                             │  │
│  │  ┌────────────┐ ┌────────────┐ ┌──────────────────────┐  │  │
│  │  │AgentManager│ │ProjectStore│ │TaskScheduler         │  │  │
│  │  │在线检测    │ │任务状态    │ │分配/路由/恢复        │  │  │
│  │  │心跳/重连   │ │文件持久化  │ │超时/重试/幂等        │  │  │
│  │  └────────────┘ └────────────┘ └──────────────────────┘  │  │
│  │  ┌────────────┐ ┌────────────┐ ┌──────────────────────┐  │  │
│  │  │EventBus    │ │Notifier    │ │RecoveryManager       │  │  │
│  │  │状态变更推送│ │微信/webhook│ │断线恢复/任务续接     │  │  │
│  │  └────────────┘ └────────────┘ └──────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                  │
│  持久化层: ~/.config/ctl_device/ (JSON files)                    │
│  锁文件:   ~/.config/ctl_device/ctl_device.lock (PID)           │
└─────────────────────────────────────────────────────────────────┘
         ▲                              ▲
         │ MCP SSE / JSON-RPC / gRPC    │ --connect / MCP stdio
┌────────────────────┐      ┌──────────────────────────────────┐
│  调度者             │      │  执行者（任意机器）               │
│  OpenClaw + MCP    │      │  ctl_device --connect <addr>     │
│  任何有 API 的工具  │      │  或 IDE + ctl_device mcp         │
└────────────────────┘      └──────────────────────────────────┘
```

---

## 协议设计

### MCP Tools（统一接口，调度者和执行者都用）

#### 执行者工具
```
task_get        拉取当前任务
task_status     更新执行状态
task_complete   提交完成报告（含 commit hash）
task_block      报告阻塞
```

#### 调度者工具
```
project_register  注册新项目
project_list      列出所有项目及状态
task_dispatch     下发任务
task_advance      验证完成，推进下一任务
agent_list        列出在线 executor
subscribe         订阅项目事件（SSE）
```

### JSON-RPC 方法（对应 MCP tools）
```
bridge.task.get / status / complete / block
bridge.project.register / list
bridge.task.dispatch / advance
bridge.agent.register / list / heartbeat
bridge.event.subscribe
```

### gRPC Services（:3713）
```
AgentService:   Register / Heartbeat / ListAgents
ProjectService: Register / List
TaskService:    Get / UpdateStatus / Complete / Block / Dispatch / Advance
EventService:   Subscribe（server-streaming，实时事件流）
```
认证：gRPC metadata `authorization: Bearer <token>`

### CLI 子命令
```bash
ctl_device                     # 默认启动 full 模式（所有协议全开）
ctl_device --connect <addr>    # 以 client 身份加入，自动注册 agent + 心跳
ctl_device mcp                 # MCP stdio 模式（供 IDE 配置）
ctl_device status              # 查询状态
ctl_device dispatch            # 下发任务
ctl_device logs                # 实时日志

# 向后兼容（已废弃）
ctl_device server              # → 等同于 ctl_device
ctl_device client mcp          # → 等同于 ctl_device mcp
```

---

## 多机网络模型

### 局域网
- Server 监听 `0.0.0.0`
- Client 填局域网 IP：`http://192.168.x.x:3711`
- Token 认证（共享 secret）

### 公网
- Server 前置 nginx 做 TLS 终止（或 server 内置 TLS）
- Client 填域名：`https://your-vps.com:3711`
- Token 认证 + TLS

### 配置（client 侧）
```yaml
# ~/.config/ctl_device/client.yaml
server: http://192.168.1.100:3711
token: your-secret-token
agent_id: macbook-m4          # 唯一标识
role: executor                 # scheduler / executor / both
capabilities: [go, python]
```

---

## 容灾与恢复设计

### 场景1：执行者断网/断电
- Server 检测心跳超时（30s），标记 executor 离线
- 任务状态保持 `executing`，不重置
- executor 重新上线后：
  1. 发送 `agent.register`（带 `resume: true`）
  2. Server 检查该 agent 持有的任务，推送恢复指令
  3. executor 调用 `task_get` 拿回任务继续

### 场景2：Server 重启（断电/更新）
- 所有状态持久化到 `~/.config/ctl_device/state.json`
- 重启后自动加载，任务状态完整恢复
- Clients 重连时 Server 推送当前状态快照

### 场景3：调度者（OpenClaw）token 达到日限制
- Server 维护完整任务队列，不依赖 OpenClaw 在线
- 任务处于 `queued` 状态，executor 可继续执行已分配任务
- OpenClaw 恢复后：
  1. 订阅事件流，接收期间所有状态变更
  2. 调用 `project.list` 获取当前进度
  3. 继续下发后续任务

### 场景4：执行者 token 达到限制
- 任务标记为 `executor_limit`（特殊阻塞状态）
- Server 发送通知
- 恢复后 executor 调用 `task_get` 重新拿任务，幂等

### 场景5：任务执行超时
- 默认超时 120 分钟（可配置）
- 超时后：通知 → 等待 30 分钟 → 自动重置为 `pending` → 重新分配

### 场景6：Git push 失败（网络抖动）
- executor 报告时标记 `commit_pending`
- Server 记录 commit hash
- 恢复后 executor 重试 push，成功后更新为 `completed`

---

## 状态机

```
pending ──→ executing ──→ completed ──→ archived
   ▲             │
   │             ├──→ blocked         （人工介入）
   │             ├──→ executor_limit  （token 限制，自动恢复）
   │             └──→ timeout         （超时，自动重置）
   │
   └─── (重置/重试)
```

---

## 数据结构

### Project
```json
{
  "name": "scrypt-wallet",
  "dir": "/home/ubuntu/workspace/scrypt-wallet",
  "tech": "go",
  "test_cmd": "go test ./...",
  "executor": "macbook-m4",
  "timeout_minutes": 120,
  "notify_channel": "openclaw-weixin"
}
```

### Task
```json
{
  "id": "scrypt-wallet:03",
  "project": "scrypt-wallet",
  "num": "03",
  "name": "wallet-core",
  "description": "...",
  "acceptance_criteria": ["所有测试通过", "覆盖率≥80%"],
  "context_files": ["plan/architecture.md", "plan/tasks/03.md"],
  "status": "executing",
  "assigned_to": "macbook-m4",
  "started_at": "2026-03-24T12:00:00+08:00",
  "updated_at": "2026-03-24T12:10:00+08:00",
  "commit": "",
  "report": ""
}
```

### Agent
```json
{
  "id": "macbook-m4",
  "role": "executor",
  "capabilities": ["go", "python"],
  "last_heartbeat": "2026-03-24T12:10:00+08:00",
  "online": true,
  "current_task": "scrypt-wallet:03"
}
```

---

## 版本路线图

| 版本 | 功能 | 目标 |
|------|------|------|
| v0.1 | Server + JSON-RPC + 项目/任务管理 + CLI | 单机可用 |
| v0.2 | MCP Server（SSE + stdio）| IDE 接入 |
| v0.3 | Agent 注册 + 心跳 + 多机路由 | 多机可用 |
| v0.4 | 容灾恢复（断线/token限制/超时）| 生产可用 |
| v0.5 | Token 认证 + TLS | 公网安全 |
| v0.6 | Web Dashboard | 可视化管理 |
| v0.7 | OpenClaw MCP config + 通知集成 | 全自动化 |
