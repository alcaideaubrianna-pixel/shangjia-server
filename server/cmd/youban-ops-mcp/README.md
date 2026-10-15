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
- `profile_preview_operation`：预演批量下架、重新上架或下架删除，返回候选资料和确认令牌。
- `profile_apply_operation`：使用确认令牌执行一次资料批量操作。
- `verification_preview_missing`：按时间和采集源预演“已有配对验证视频但频道未确认发送验证”的资料。
- `verification_apply_repair`：确认后恢复验证媒体，并将完整资料按原频道替换重发。
- `ops_plan_status`：查询批量操作执行状态、成功数和失败目标。
- `profile_media_preview`：只读检查资料媒体是否可恢复，并生成确认令牌。
- `profile_media_apply`：使用确认令牌修复明确指定资料的可恢复媒体。

支持的修复动作：

- `recover_verifications`：重新匹配此前未找到上文的验证视频。
- `requeue_events`：使用正常采集链路重新处理展示事件。
- `queue_verification`：兼容旧调用名；实际会清理旧发布并将完整资料按原频道替换重发，禁止单独补发验证视频。

所有写操作均经过预演、候选数量限制、一次性领取和数据库审计。禁止通过此 MCP 暴露任意 SQL、Redis 命令或无范围队列重放。

资料操作说明：`offline` 保留资料并清理线上消息；`republish` 仅允许媒体完整的资料重新上架；`delete` 会先进入统一下架清理链路，再删除指定账号下的资料。缺验证修复最多查询 48 小时范围，且只处理数据库中已经存在、能够确定归属的验证视频，不会猜测或跨资料绑定。历史验证恢复一律整份资料替换重发，不存在验证视频单独补发路径。
