# 频道不推送：SLS 只读排查

本手册用于区分没有入队、等待到期、领取/租约异常、恢复未推进以及 Telegram 结果未知。日志事件按本轮源码整理；代码落盘、镜像部署、日志采集、字段解析和索引生效是不同事实，不能互相代替。本文未查询生产或修改 SLS 配置。

先阅读 [线上调试入口](production-debugging.md)、[发布架构约束](publish-architecture.md) 和会话提供的 `alibabacloud-sls-query` 技能（本机位置 `/Users/ley/.codex/skills/alibabacloud-sls-query/SKILL.md`）。遵循技能的凭据、user-agent、索引前置检查、分页和完整性约定；不打印密钥，不用改账号/改索引绕过权限错误。

## 先用五分钟样本找停点

1. 记录业务 `tenantId/accountId/channelId`，区分 Telegram `chat_id` 与业务频道编号；相同 chat 可能有多个业务频道配置。已知则加 `jobId/profileId/operationNo`。从当前 SAE 角色的实际日志绑定确认 Project/Logstore，不把历史示例当实时配置。
2. 固定最近 5～15 分钟窗口，注明时区，API 使用 Unix 秒，起始包含、结束不包含。先查 Publish，再按 job/operation 查创建方 Web、Collector、Scheduler、Background；不要一开始扫全租户全天。
3. 查 `channel_queue_dequeue/requeue/ack` 和 `execution_finished`；加入本轮新事件定位领取、续期、确认和恢复。先确认镜像包含这些事件，旧实例没有新事件不能证明没有执行。
4. 返回最后一个确认节点、对应时间和关键 ID，再给出待证实的下一步。单页查不到、某事件不记录空轮询、日志尚未入库，都不足以认定任务不存在。

## 日志载荷与关联键

统一 helper 输出 JSON 文本，固定 envelope 为 `log_type=telegram_queue`、`schemaVersion=1`、`event`、`traceId`、`instance`、`pid`。仅复制白名单标量字段，例如 channelId/jobId/profileId/operationNo、executionId、claimId、queue、stage、reason、action、result、durationMs、leaseMs、availableAtMs、retryAtMs 和恢复计数/游标。错误经过脱敏，包括错误文本中的 lease token；同时保留错误原因，便于识别 NOAUTH（认证失败）、NOPERM（ACL 权限不足）等问题。不要额外索取原始租约 token、完整任务载荷、Bot Token 或媒体内容。

**g.Map、JSON 文本和 SLS 字段提取不是同一件事。**旧队列事件仍可能是普通日志加 g.Map；新 helper 的 JSON 也可能包在带时间/级别前缀的 `content` 或 `message` 中。字段未提取时不能直接使用 `event:`、`channelId:` 或 SQL 列名。本文不假定采集解析、字段索引或统计已经上线。

- 优先按 `jobId + channelId` 连链路；领取阶段尚未加载 job，traceId 允许为空，使用 `claimId/channelId/jobId` 及窄时间窗。claimId 是诊断摘要，不是授权 token。
- 同一执行用 executionId/traceId 关联；跨重试、进程重启可能变化。恢复循环 started/finished 共用 executionId、traceId。
- instance/pid 区分滚动部署和重启，单独 pid 不全局唯一。不能因为两台实例记录同一 job 就认定重复发送，要核对不同 claim、发送 attempt 和 Telegram 成功结果。
- `execution_finished` 属于 `telegram_publish_summary`，不要在 `log_type=telegram_queue` 过滤器里找它。业务 ACK 也不等于 Telegram 发送成功。

## 事件对照

| 事件 | 证据和边界 |
|---|---|
| channel_queue_dequeue | Worker 已加载/取出任务；结合 jobId、queue、priority、score、reason，不能等同发送成功 |
| channel_queue_requeue | 业务决定再次等待；看 reason/error、stage、重试时间及后续 claim，不只统计条数 |
| channel_queue_ack | 业务决定收口；核对终态/原因及实际 settle，不能当作发送回执 |
| execution_finished | 发布执行汇总；核对发送阶段、attempt、结果和错误。执行结束不必然所有展示/验证消息成功 |
| channel_queue_enqueue_failed | 入队调用失败；缺少它不证明已成功入队，也不证明没有入队 |
| claim_success / claim_error | 领取成功或异常；成功含 claimId、jobId、queue、score、leaseMs、durationMs。空队列/未到期/已有 processing 可能没有 claim 日志 |
| renew_lost / renew_error | 前者明确令牌不匹配或已过期，后者仅证明续期调用异常。成功续期不逐次打印，不能以无日志判定未续期 |
| settle_success / settle_rejected / settle_error | action=ack/requeue；rejected 表示旧令牌/过期确认被拒绝，是 fencing 生效，不表示任务已被新实例完成；error 为调用异常 |
| expired_recovery_success / expired_recovery_error | 单频道过期 processing 恢复成功/异常；无过期项时不逐次打印，成功恢复也不代表发送完成 |
| rehydrate_summary | 有界恢复轮次，查看 stage/reason/result、scanned/recovered/failed/pending、cursor/nextCursor、dueCount/activeRestored。游标不可跨不同扫描阶段直接比较；无错误、无恢复且未丢失游标租约的空扫描不打印汇总 |
| rehydrate_bad_key | 指定 key 恢复异常；看 channelId/queue/stage，不把一个坏 key 推断为全部频道停摆；整轮异常看 rehydrate_summary 的 result/error |
| 普通 Warning 文本（无 event） | runtime 使用“频道活跃集合读取失败”“频道队列消费失败”“频道活跃集合清理失败”“频道队列低频恢复失败”；用全文搜索，不假设存在对应结构化事件 |
| channel_recovery_cycle_started / channel_recovery_cycle_finished | 恢复循环边界，按 executionId/traceId 配对；finished 看 durationMs/cancelled/nextStep（阶段下标），进程退出也可能没有 finished |
| channel_recovery_lock_error | 恢复锁调用异常；不等于正常锁 busy，也不等于证明另一个实例健康运行 |
| channel_recovery_step_started / channel_recovery_step_finished | 按 step 对比开始/结束、durationMs 与错误；恢复阶段可能跨轮续调 |
| channel_recovery_budget_exhausted | 本轮预算结束，nextStep 为续调阶段名称（区别于 cycle 事件的下标）；要验证后续轮次继续，不能解释为全部任务失败 |
| reconcile_lease_acquire | result=acquired/busy/error；busy 表示另一执行持有对账租约，不能直接重发 |
| reconcile_execution_finished | 默认 result=ok；业务错误或执行上下文取消记录 result=error。取消看 cancelled=true/reason=execution_context_cancelled，并记录脱敏 context.Cause；可能是续租失败、deadline 或父级取消，不能单独证明租约已丢失。函数仍返回原业务 err，日志不改重试策略 |

result 值按事件解释：claim/settle 恢复成功常为 `success`，通用 helper 默认为 `ok`，不统一假设只存在一种成功枚举。统计 ack/requeue 与 settle 时不要把同一次处理重复计数。

## SLS 查询：先核实索引，再选语法

以下仅为只读模板，未执行。使用已授权的 CLI profile/地域与当前 SAE 日志绑定。执行前通过技能规定的本地方式核实凭据；不要输出配置文件。生成会话 user-agent，一次会话沿用同一值。

```bash
SLS_SESSION_ID=$(openssl rand -hex 16)
SLS_UA="AlibabaCloud-Agent-Skills/alibabacloud-sls-query/$SLS_SESSION_ID"
SLS_PROJECT='<当前SlsProject>'
SLS_LOGSTORE='<当前SlsLogStore>'
SLS_TO=$(date +%s)
SLS_FROM=$((SLS_TO - 900))
aliyun sls get-index \
  --project "$SLS_PROJECT" --logstore "$SLS_LOGSTORE" \
  --user-agent "$SLS_UA"
```

检查 `line` 是否启用全文索引、`keys` 是否包含实际字段以及字段类型。SQL 统计还需对应字段 `doc_value=true`。没有可用索引时停止索引查询并报告前提缺失，不擅自创建索引。已有只读实例日志兜底见主调试文档。

### 全文搜索兜底（仅当 line 已启用）

当 JSON 未解析/字段未建索引，用原始日志中稳定词和业务 ID 搜索。例如业务 channelId=231，ID 仅做候选过滤，返回后人工核对原始 JSON 中的键和值，避免数字命中无关字段。

```text
"231" and ("channel_queue_dequeue" or "channel_queue_requeue" or "channel_queue_ack" or "execution_finished")
```

```text
"231" and ("claim_success" or "claim_error" or "renew_lost" or "renew_error" or "settle_rejected" or "settle_error")
```

恢复循环/rehydrate 汇总可能不带 channelId，须另用同一窄窗口搜索，不附加频道过滤：

```text
"telegram_queue" and ("rehydrate_summary" or "rehydrate_bad_key" or "channel_recovery_cycle_started" or "channel_recovery_cycle_finished" or "channel_recovery_step_finished" or "channel_recovery_budget_exhausted" or "channel_recovery_lock_error")
```

```bash
aliyun sls get-logs-v2 \
  --project "$SLS_PROJECT" --logstore "$SLS_LOGSTORE" \
  --from "$SLS_FROM" --to "$SLS_TO" \
  --query '"231" and ("channel_queue_dequeue" or "channel_queue_requeue" or "channel_queue_ack" or "execution_finished")' \
  --line 100 --offset 0 --reverse true --user-agent "$SLS_UA"
```

按 offset=0、100、200 有界翻页；若满页且尚未覆盖目标事件，缩短时间窗或继续有限翻页，不能把第一页当全量。响应 `meta.progress=Incomplete` 时重试同一请求直到 Complete，并记录本次覆盖窗口。仍持续不完整则报告限制，不声称缺失。全文标点分词按实际索引配置验证，不依赖精确匹配 JSON 空格/冒号。

### 已有字段提取与索引时

只有原始样本确认顶层字段存在且 `keys` 已建立相应索引时，才使用：

```text
log_type:telegram_queue and channelId:231
```

```text
channelId:231 and (event:channel_queue_dequeue or event:channel_queue_requeue or event:channel_queue_ack or event:execution_finished)
```

字段名/大小写必须与实际解析一致。旧事件没有 log_type 时不要加该过滤。channelId 若为 text，用其精确文本匹配，不做数值范围推断。

若 event/result/channelId 均可统计，可做小范围分类；没有统计配置不要照抄：

```sql
log_type:telegram_queue and channelId:231
| SELECT event, result, count(*) AS n
  GROUP BY event, result ORDER BY n DESC LIMIT 30
```

若 JSON 只存在 content/message 中，先确认是否为完整合法 JSON（有日志前缀时不能直接当 JSON 解析），再由授权的采集解析或查询 JSON 提取处理。本文不部署解析配置；不以 SQL scan/SPL 全量扫描替代缺失前提。

## 根据最后节点判断下一步

| 现象 | 当前能确认什么 | 下一项只读证据 |
|---|---|---|
| 无 dequeue/claim | 尚未观察到消费，不能断言没有任务 | 确认部署/日志窗口与完整性；查创建方同 operation 的创建/入队证据、显式授权/取消结果，再查有界任务状态；不要自动恢复 disabled 关系 |
| requeue 持续 | 任务被消费但暂缓 | 同 job 的 reason/error、availableAtMs/retryAtMs、媒体或限流原因；未来到期不等于卡死，score 含编码，不能直接当 Unix 时间 |
| claim_success 后长期无执行结果 | 有持久领取，但后续停点未知 | 同 claimId/jobId 的 renew、settle、instance/pid、执行汇总及恢复日志；没有 renew_success 属正常设计 |
| renew_lost / settle_rejected | 旧领取已失去确认资格 | 新 claim/expired_recovery/settle 证据；不要手工删除 processing 或越过 fencing |
| 恢复反复超预算/坏 key | 恢复有局部失败或续调 | nextStep 后续轮是否推进、rehydrate stage/游标/失败 key；全局汇总没有频道字段也要查 |
| reconcile busy 或发送网络异常 | 可能存在未知发送结果 | 按 job/attempt、展示/验证阶段查 Telegram API 成功 message_id、Redis 消息投影/webhook 与对账结论；未知不等于失败，不直接重发 |
| ack / execution_finished | 某次队列或业务执行结束 | 区分 sent/failed/superseded 等实际结果及原因，核对展示和应有验证资料；不可仅凭 ack 宣称发布成功 |

最终交接格式：业务频道/chat、时间窗与时区、角色/镜像已知范围、原始事件时间及 job/claim/execution、最后确认节点、下一步待证据、索引/分页限制。把“未观察到”与“已证明未执行”分开。本流程不包含补发、清队列、改权限或修改生产状态。

## 普通任务入队成功后消失：旧索引误删

2026-11-17 在频道 1674 确认：采集端和发布端均可记录 `channel_direct_enqueue` 成功，
Outbox 随后成为 `delivered`，但普通优先级任务仍停在 `pending/idle`，没有发送 Attempt。
原因是旧 `tgChannelPendingIndex` 与普通 ready 队列共用 key，Outbox 收口清理旧索引时
删除了刚入队的成员；历史队列使用独立 key，因此历史任务成功不能证明普通任务正常。

频道队列模式下，旧索引的 `add/remove/head` 必须全部停用：`add` 还会覆盖 ready 的到期
分数，`head` 也不能充当新队列的领取凭证。新队列继续由 enqueue/claim/settle 管理；
关闭频道队列时保留旧索引行为。该修复不需要 SQL 迁移或新增恢复扫描器。

部署应覆盖可能执行 Outbox 投递或旧索引访问的角色（publish、collector、web、
background、scheduler），滚动期间旧实例仍可能误删。已消失的历史任务需先检查当前
operation、权限、Attempt、消息与协议号归属，再通过已有指定 Job 恢复入口有界补偿。
不要仅重新激活 Outbox 后就宣称恢复：旧版本会在下一次 Outbox 收口时再次删除成员。
