# Youban Ops MCP

受控的采集与发布运维 MCP 服务。它通过 AI 运维 HTTP API 调用现有业务服务，不直接连接 PostgreSQL 或 Redis。

## 运行

```bash
export YOUBAN_AI_OPS_BASE_URL=https://api.example.com
export YOUBAN_AI_OPS_TOKEN=replace-me
go run ./cmd/youban-ops-mcp
```

MCP 客户端使用 stdio 启动上述命令即可。

## 工具

- `collect_diagnose_message`：按 Telegram 消息链接或 Chat ID/消息 ID 查询完整链路，只读。
- `resolve_profile_by_message`：按消息链接或 Chat ID/消息 ID 返回资料编号、是否采集、采集来源、规则和事件状态，只读。
- `collect_preview_repair`：预演修复范围，生成15分钟有效的确认令牌，不修改业务数据。
- `collect_apply_repair`：使用计划 ID 和确认令牌执行一次修复。

支持的修复动作：

- `recover_verifications`：重新匹配此前未找到上文的验证视频。
- `requeue_events`：使用正常采集链路重新处理展示事件。
- `queue_verification`：为展示已发送但缺验证消息的资料补投验证阶段。

所有写操作均经过预演、候选数量限制、一次性领取和数据库审计。禁止通过此 MCP 暴露任意 SQL、Redis 命令或无范围队列重放。
