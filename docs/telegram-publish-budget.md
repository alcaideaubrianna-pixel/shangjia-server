# Telegram 发布预算、协议号交接与日志

本说明对应本次发布链路重构。代码修改不代表已构建或部署；上线后需要按同一频道、Job 和 Attempt 核对真实发送。

## 时间策略

所有配置使用 Go duration，例如 `30m`、`10m`、`15s`。统一入口为 `server/utility/telegrambudget`，启动时通过 `envconfig.Apply` 将 ENV 覆盖到配置。

| ENV | 默认值 | 作用 |
|---|---:|---|
| `YOUBAN_PUBLISH_TG_SEND_BUDGET` | `30m` | Job 持久化总预算，允许 `2m–30m` |
| `YOUBAN_PUBLISH_TG_DISPLAY_TIMEOUT` | `10m` | Bot 展示阶段 |
| `YOUBAN_PUBLISH_TG_VERIFY_TIMEOUT` | `10m` | Bot 验证阶段 |
| `YOUBAN_PUBLISH_TG_BOT_REQUEST_TIMEOUT` | `10m` | Bot 单次媒体请求 |
| `YOUBAN_PUBLISH_TG_ACCOUNT_TIMEOUT` | `10m` | 协议号单次发送执行 |
| `YOUBAN_PUBLISH_TG_ACCOUNT_GATE_TIMEOUT` | `30s` | 协议号媒体降级等待账号执行槽 |
| `YOUBAN_PUBLISH_TG_FINALIZE_TIMEOUT` | `10s` | 本地状态与恢复意图收尾 |
| `YOUBAN_PUBLISH_TG_RESPONSE_TIMEOUT` | `5s` | API 返回事实保存 |
| `YOUBAN_PUBLISH_TG_SHUTDOWN_TIMEOUT` | `30s` | 退出时等待已登记的收尾工作 |
| `YOUBAN_PUBLISH_TG_ENQUEUE_TIMEOUT` | `5s` | Redis 入队调用 |
| `YOUBAN_PUBLISH_TG_PREVIEW_TIMEOUT` | `30s` | 视频预览生成 |
| `YOUBAN_PUBLISH_TG_POSTER_TIMEOUT` | `5s` | 视频封面下载 |
| `YOUBAN_PUBLISH_TG_SANITIZE_TIMEOUT` | `10m` | 视频规范化与元数据清理 |
| `YOUBAN_PUBLISH_TG_CLEANUP_TIMEOUT` | `15s` | 单次清理执行 |
| `YOUBAN_PUBLISH_TG_LEASE_OPERATION_TIMEOUT` | `3s` | 续租、释放、领取确认操作 |
| `YOUBAN_PUBLISH_TG_WEBHOOK_WAIT` | `15s` | 未知响应等待消息事实的窗口 |

无效配置会产生 `telegram_budget_invalid`，并使用默认值。合法范围见统一策略定义。配置更新需在相关角色启动时生效；非空 `send_deadline_at` 不因配置变化、429 重试、切 Bot 或协议号交接而重新计时。

Job 的 `send_deadline_at` 是发送主链路唯一的业务总截止时间。阶段、HTTP 请求和协议号执行只能使用剩余预算。频道 Worker 和 Stream 不再各自重新创建一个发送总预算；租约丢失仍会取消发送。Asynq 执行上限由 Job 预算加收尾余量推导。

Account 媒体降级任务的领取租约需覆盖等待执行槽、实际执行和本地收尾，不能继续使用旧的 5 分钟执行值推导。其他类型 AccountTask 的执行和领取策略保持各自原有规则。

收尾可以脱离已经取消的发送 Context，但必须有独立的截止时间；嵌套收尾继承已有截止时间，不能不断延长。收尾主要保存事实、状态和恢复意图，批量删除由持久化清理链路恢复执行。

## 协议号交接

```text
Bot 明确失败
  → 持久化 Account 阶段归属与任务
  → Account 核对 Job、阶段、任务身份和租约后接管
  → 保存发送响应与阶段确认
  → 展示完成且需要验证：
       短事务交还可发送状态，并写入验证续发 Outbox
  → 正常发布队列继续验证
```

普通 Bot 消费者不能抢占已经交给 Account 的阶段。旧任务回调不能把新操作、已完成阶段或已废弃 Job 改成失败。恢复器补齐相同交接的持久化任务，不重新启动首次交接。

Bot 请求超时可能已经在 Telegram 产生消息。此时先保存部分消息及 Attempt，等待 Webhook／协议号监听聚合的消息事实；不能仅凭本地消息数量为零就直接换协议号重发。明确 API 成功则直接保存响应事实和阶段，无需等 Webhook。

展示响应的消息 ID 在验证发送前同步保存。`file_id` 为后续复用投影，失败不会把已成功的远端发送改判失败。

## SLS 排查

精确匹配 `targetChatId + messageId`。全文检索频道 `-1001178095860` 时使用完整数字 token `"1001178095860"`；单独 `"1178095860"` 可能漏掉日志。不同频道的 message ID 可以重复。

优先从一个 `jobId` 搜索完整链路，再按 `attemptId`、`accountTaskId` 追踪交接。关键事件：

| 事件 | 说明 |
|---|---|
| `telegram_bot_send` | Bot 返回结果，区分 `purpose=display/verify`、错误与 `message_ids` |
| `telegram_protocol_send` | 协议号真实发送的开始和结果，查看 `node`、`verdict`、消息 ID、耗时与错误 |
| `telegram_handoff_requested` | Account 交接任务已持久化 |
| `telegram_handoff_claimed` | Account 已通过身份、状态和租约检查 |
| `telegram_handoff_rejected` | 交接或接管被拒绝，检查 `reason` |
| `telegram_handoff_waiting` | 普通发布／入队入口发现 Account 已拥有该阶段，跳过普通调度 |
| `telegram_handoff_completed` | 当前交接完成；检查原因是否为验证 Outbox 已提交或终态失败 |
| `telegram_response_pending` | 未知响应证据已保存，等待事实确认 |
| `telegram_finalize` | 独立收尾进入、超时或退出等待结果 |
| `telegram_file_id_cache_failed` | 远端发送已成功，复用缓存未完成 |
| `telegram_budget_invalid` | 时间配置无效，使用默认值 |

同时查看 `jobId`、`profileId`、`channelId`、`targetChatId`、`operationNo`、`attemptId`、`purpose`、`transport`、任务 ID、`reason`、状态迁移结果及截止时间字段。`telegram_publish_stage` 的 `effectiveDeadline`、剩余时间与 `cancelCause` 用于区分 Job 到期、阶段超时、父 Context 取消和租约丢失。

“已提交交接”不能替代“已接管”；“验证 Outbox 已提交”也不能替代“验证 API 成功”。必须确认最后的验证消息 ID 和本地完成状态。

## 部署和验收

发送行为涉及 `publish`、`account`；Bot Gateway 客户端和统一 ENV 入口由同一镜像提供，需要同时更新承载相关调用的角色。恢复、Outbox、Webhook 处理的角色以当前 SAE 启动配置为准。禁止仅更新 Publish 后假定 Account 消费条件也已更新。

本次不需要自动执行 DDL。2026-11-17 已在隔离的提交候选目录通过生产构建（`GOMAXPROCS=4 go build -p 2 ./main.go`），并通过本地 PostgreSQL 的交接身份隔离、验证阶段 Outbox 原子提交及普通 Job 对账失败收尾三项集成测试。单元测试使用 `-gcflags=hotgo/addons/youban_publish/logic/sys=-l` 降低大型测试包编译开销；生产构建使用默认优化。

首轮相关单元测试通过 568 项，跳过 32 项，另有 3 项已有测试失败：恢复查询的两个旧 SQL 断言，以及依赖真实 Redis 分布式锁的对账索引测试。已核对这三个测试及其直接被测函数与本次修改前 HEAD 一致；这些失败仍需单独处理，不能视为全量测试通过。

验收至少包括：Bot 429 交接、Account 展示完成后续发验证、Account 终态失败、重复恢复通知、未知响应及迟到成功、Job 到期与 API 返回同时发生、租约丢失、发送成功后退出，以及无验证媒体的正常完成。
