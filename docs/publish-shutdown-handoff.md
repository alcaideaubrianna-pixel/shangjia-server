# 发布实例停机与任务交接

适用角色：publish 的统一频道队列。消费 Attempt 超时任务的 background/publish
应同步更新。沿用现有 Job、Attempt、Outbox、Redis claim/renew/settle 和超时清理，
没有新增队列、后台扫描器或数据库字段。

## 停机顺序

1. 关闭执行准入，再取消 runtime 轮询；新取得但未准入的 claim 按 token 重新入队。
2. 已准入任务保留执行 Context，在排空窗口中继续发送、领取续租和发送租约。
   显式父 deadline、Job 总预算、资料取消和领取丢失保护仍有效。
3. 窗口耗尽后以明确的停机交接原因取消。成功回执仍使用独立有界 Context 保存。
4. 未完成执行再次核对 Redis token 后，在 Job 的 `last_dispatch_error` 保存
   `runtime_shutdown_handoff:<confirmation-deadline-ms>`。旧交接截止时间不延期，
   不覆盖协议号已接手标记；原 claim 通过既有 settle/requeue 交给新实例。
5. 新实例在已有 Job 对账租约中读取回执事实。明确成功只修复投影；
   结果未知禁止立即换 Bot/协议号或重发。
6. 交接确认期限到达后，复用已有超时隔离和消息清理流程。
   先清理已知展示消息，旧 Attempt 仍不确定或响应审计未投影时，
   既有清理门禁禁止自动激活替代任务。迟到回执继续按旧任务隔离规则处理。

`idle` 是可恢复的对账检查点，不是对账所有权。对账入口现在允许在持有
每 Job token 租约时接续 idle，避免不确定结果的第二轮检查变成空操作。

## 配置

| ENV | 配置项 | 默认值 | 含义 |
|---|---|---|---|
| `YOUBAN_PUBLISH_TG_SHUTDOWN_TIMEOUT` | `youbanPublish.telegram.shutdownTimeout` | `30s` | 应用停机总预算 |
| `YOUBAN_PUBLISH_TG_SHUTDOWN_DRAIN_TIMEOUT` | `youbanPublish.telegram.shutdownDrainTimeout` | `20s` | 已领取发送的排空窗口 |
| `YOUBAN_PUBLISH_TG_FINALIZE_TIMEOUT` | `youbanPublish.telegram.finalizeTimeout` | `10s` | 总预算中预留的回执与收尾时间 |
| `YOUBAN_PUBLISH_TG_WEBHOOK_WAIT` | `youbanPublish.telegram.webhookWait` | `15s` | 停机交接后的确认窗口 |

实际排空时间取请求值与“剩余总预算减收尾预留”的较小值，不叠加延长总预算。
平台终止宽限需覆盖应用总预算及其他插件退出耗时；增大应用配置不会延长平台的强杀时间。
进程被直接强杀时，仍由现有 Redis claim 到期恢复及持久 Attempt 兜底；
无法承诺强杀前一定写成停机标记。

## SLS 链路

按 `jobId/channelId/claimId/instance` 关联：

- `telegram_runtime_drain_started`：停止领取，记录 `activeExecutions/drainBudgetMs`。
- `shutdown_claim_requeued`：停机边界取得的 claim 已按 token 退回。
- `telegram_runtime_handoff`：排空窗口耗尽，进入取消和持久化交接。
- `shutdown_handoff_persisted`：记录 `confirmationDeadline/nextStep`。
- `telegram_finalize` 的 `shutdown.handoff`：记录凭证检查或落库失败。
- `shutdown_handoff_expired`：已进入旧任务隔离与已知消息清理。
- `telegram_runtime_drain_finished`：排空完成或总预算耗尽。

交接标记持久化失败时，不伪报交接成功；保留现有凭证恢复与 Job 总超时兜底。
确认未知结果仍需 Webhook/协议号实时事实；Redis 锁过期不代表 Telegram 未发送。

## 验证边界

新增用例覆盖停止领取而不取消执行、租约丢失取消、强制交接原因、父 deadline、
总预算预留、交接标记解析，以及真实隔离 Redis/PostgreSQL 下的旧 token 拒绝、
交接幂等和凭证保留。PostgreSQL 用例复用显式本机隔离入口，
需要 `YOUBAN_MISSING_READY_LOCAL_PG=1`，不读取生产 DSN。

本次遵照用户要求，未编译、未运行测试、未部署。上线验收需在隔离环境模拟
展示已确认、验证发送中触发停机，以及迟到回执和清理恢复；静态检查不等于此场景通过。
