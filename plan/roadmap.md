# ctl_device 版本路线图

## v0.1.0 - 单机可用 ✅
任务: 01, 02, 03, 04, 05
- Go 项目骨架
- 协议类型定义
- 状态持久化（断电恢复）
- Agent 心跳管理
- JSON-RPC Server + CLI

## v0.2.0 - IDE 接入 ✅
任务: 06
- MCP Server（stdio + SSE）
- Claude Code / Cursor / JB / VSCode 可用
- OpenClaw MCP config 可用

## v0.3.0 - 生产可用 ✅
任务: 07, 08
- 完整容灾恢复（所有场景）
- Token 认证 + TLS
- 公网安全部署

## v0.4.0 - 可视化 ✅
任务: 09, 10
- Web Dashboard
- 跨平台 CI/CD
- 完整集成测试

## v0.5.0 - 架构简化 + gRPC ✅（当前 v0.0.6）
任务: 11, 12
- **启动架构简化**
  - 默认 full 模式，直接 `./ctl_device` 启动
  - `--connect <addr>` 自动变 client（agent 注册 + 心跳）
  - 统一配置文件（mode/connect/server/client/notify 合一）
  - 平级子命令（mcp/status/dispatch/logs）
- **gRPC Server**（:3713）
  - AgentService / ProjectService / TaskService / EventService
  - Token auth interceptor
  - server-streaming 事件流
- **Full 模式唯一性锁**（PID lock file，stale 自动清理）

## v0.6.0 - 待规划
- gRPC client SDK
- 多 full 节点主备（HA）
- 更丰富的 Dashboard 操作（任务手动干预等）
