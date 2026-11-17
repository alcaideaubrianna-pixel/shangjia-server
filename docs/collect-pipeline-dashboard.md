# 采集消息到资料入库：观测口径与报表

本文面向“消息接收 → 原始事件 → Delivery → 发布侧采集事件 → 匹配/配对/媒体准备 → 资料事务提交”，不把 Telegram 发布成功纳入采集成功终点。

**状态：依据 2026-11-17 工作区实现及本轮确定的观测契约编写，尚未部署验收。本文没有执行 SQL、PromQL、测试、编译或生产查询，不代表指标已导出或 SLS 索引已配置。**面板上线前核对实际接收端名称、索引和样本，不得把缺失数据显示为零耗时。

相关入口：[Collector 观测说明](collector-observability-hooks.md)、[SLS 查询前提](publish-queue-sls.md)、[生产排障入口](production-debugging.md)。若旧文档与本轮新增字段不同，以最终源码和真实样本核对，不直接拼接两版数据。

## 成功、尝试、等待的定义

- **资料入库成功**：资料及关联数据的业务事务已成功返回，得到有效 profileId。事务内 Insert 返回 ID 尚不能证明提交成功；事务失败、回滚均不算成功。
- **持久化成功的准确条件**：`profile_persist/persisted` 表示 `commitCollectMaterial` 成功返回且 profileID>0。该结果包含复用，当前不能区分新增资料；面板只能展示“资料持久化成功尝试”，不能写“新增资料数”。
- **nil 返回不是成功证据**：handler 可以因禁用、去重、等待或无需处理返回 nil；delivery completed、event ready、delivery_submit queued、dispatch_handoff unconfirmed 均不等于 profile 入库。
- **attempt 不是唯一消息数**：入口、查询、恢复、匹配、事务、入队的计数都以各自调用边界计尝试。相同事件可能重试，同一事件可匹配多规则，多个消息也可合为一份资料。
- **等待与失败分开**：配对、验证、媒体、锁、配额、延迟重试等返回等待/可重试结果，不直接计为最终失败。重试耗尽/明确终止只有最终业务状态证据才能确认。
- **进程内耗时、队列等待、端到端耗时分开**：不能把嵌套父子阶段耗时求和；一次处理结束也不能代表长期等待结束。

不以“入口计数减入库计数”推断丢失：来源禁用、系统消息、规则不匹配、多消息归组、多规则分支、已有资料复用和重试都会使两个计数不同。需要唯一数量时，在限定作用域和时间窗的 SLS 中按已确认业务主键去重；日志不完整时只能称“观测到的唯一数量”。

## 当前实现与新增埋点位置

以下插件文件相对 `server/addons/`；函数名用于最终审查与接线核对。

| 范围 | 文件与符号 | 可证明的边界 |
|---|---|---|
| 共享上下文（新增） | `server/internal/library/collectobserve/context.go`：`Metadata`、`With`、`From` | 值拷贝观测身份，无 I/O；不能用于授权或调度 |
| Collector 入口 | `telegram_collector/logic/sys/collector.go`：`IngestBotUpdate`、`IngestAccountMessage`、`persistEvent` | 接收/持久化/入队本次调用结果，不是资料新增 |
| Collector 阶段 | `telegram_collector/logic/sys/collector_observability.go`：`startCollectorStage`、`recordCollectorStage`、`collectorObservationContext` | 强类型阶段结果、耗时、来源模式和共享上下文身份 |
| Event 消费 | `telegram_collector/logic/sys/event_queue.go`：`processEvent`、`collectorDeliveryFromEvent` | claim/read/parse/handoff；原始事件与发布侧事件不是同一主键 |
| Delivery 交接 | `telegram_collector/logic/sys/event_handoff.go`：`handoffCollectorEvent` | direct 同进程；queued 保留历史预算；交接完成不等于入库 |
| Delivery 消费 | `telegram_collector/logic/sys/delivery_queue.go`：`processDelivery`、`handleDeliveryTask` | handler 与交付完成状态；guard nil 仍可能跳过 |
| Publish adapter | `youban_publish/logic/sys/telegram_collector_adapter.go`：`HandleCollectorDelivery`、`logCollectorDeliverySkipped`、`collectorDeliveryMessage` | 来源适配、明确 skip、提交采集事件 |
| 采集事件入库 | `youban_publish/logic/sys/collect_message.go`：`ingestAndProcessCollectMessage`、`ingestCollectMessage` | 原始内容写入发布侧 collect_event；不是 profile 入库 |
| 处理队列 | `youban_publish/logic/sys/tg_queue_worker.go`：`handleCollectProcessTask` | 消费、slot/锁、处理与重试调度；新增观测应覆盖早退 |
| 实时窗口 | `youban_publish/logic/sys/collect_material_window.go`：`processTargetCollectEventWindow`、`processCollectMessageWindow` | 窗口查询、分类、配对、等待；batch 与 SQL 是不同边界 |
| 来源授权与规则 | `youban_publish/logic/sys/collect_event.go`：`processCollectEvent`、`dispatchCollectEventByRule`、`collectBusinessEventContext` | 来源/频道授权、规则匹配、复用分支与媒体就绪；每条规则独立观测 |
| 资料事务 | `youban_publish/logic/sys/collect_material_commit.go`：`prepareCollectMaterialSnapshot`、`commitCollectPreparedProfile` | 媒体准备与资料提交必须分开，后者事务成功是入库证据 |
| Publish 新阶段 | `youban_publish/logic/sys/collect_stage_observability.go`：`startCollectPipelineStage`、`logCollectPipelineObservation` | 执行期间真实父 span、指标和按结果/耗时筛选的 `collect_stage_completed` |
| 旧阶段与指标 | `youban_publish/logic/sys/collect_pipeline_observability.go`：`observeCollectPipeline`、`recordCollectPipelineMetrics` | 已有 submit/process/dispatch/admission/queue 边界；不要与新 hook 对同一阶段重复计数 |
| 窗口日志 | `youban_publish/logic/sys/collect_window_observability.go`：`observeCollectWindowQuery`、`logCollectBatchReuse` | SQL 往返与每次 batch 退出结果；不新增 Redis 统计计数器 |

Publish 阶段如下：

| stage | 边界 |
|---|---|
| event_persist | 发布侧 collect_event 持久化，非 profile 入库 |
| source_attempt | 本次来源处理尝试；源禁用/事件不存在为 ignored，已处理且无需修复为 duplicate |
| authorization | 来源与实际频道授权 |
| rule_preflight | 规则前置条件检查 |
| rule_match | 规则匹配与分支判断 |
| material_pair | 展示与验证资料配对 |
| media_ready | 媒体就绪判断 |
| media_cache | 媒体缓存处理 |
| media_quota_wait | 媒体配额等待步骤 |
| content_extract | 资料内容抽取 |
| profile_persist | commitCollectMaterial 调用；成功且 profileID>0 才为 persisted |
| window_primary / window_retry | primary/retry 窗口查询 |
| delivery_submit | 旧交付提交边界 |
| process | 旧事件处理汇总 |
| dispatch_handoff | 旧资料分发调用边界 |
| publish_admission | 旧发布 Job 准备回执 |
| publish_queue_enqueue | 旧发布队列提交回执 |

`source_attempt` 的结果明确区分：**源禁用或事件不存在为 ignored，事件已处理且无需修复为 duplicate**；这些路径不能因 nil 返回记为 completed，更不算资料成功。Publish 不新增 skipped 结果，不能沿用 Collector 的 skipped 枚举解释 Publish 指标。source_attempt、规则、事务等均是各自边界的非唯一业务尝试计数，不是唯一事件或资料数。

### SLS 输出策略与覆盖偏差

小于 100ms 的普通成功子步骤只记录指标/trace，不额外输出 SLS 阶段日志；pending、failure 及 persisted 等关键结果保留全量结果日志。这里的“全量”指应用侧符合策略的每次调用均记录，不保证日志运输、索引或查询结果没有缺失。

因此普通成功子步骤的 SLS 样本带有耗时阈值选择偏差，**不能用 SLS 统计所有快成功子阶段的 P95、平均值或完整 attempt 总量**。阶段总体分位数从 OTel Histogram 读取；Trace 明细受采样影响。关键结果日志可用于按频道下钻，但仍不能转换为唯一新增资料数。

## 维度契约

### 指标仅使用低基数标签

| 维度 | 预期值与解释 |
|---|---|
| stage | 白名单固定阶段；不能使用任意函数名或动态字符串 |
| result | 固定枚举；失败、等待、跳过、成功分开 |
| source_type | bot/account/unknown；两侧一致 |
| collection_mode | realtime/history/unknown；描述来源模式，不能由积压时长重新定义 |
| queue_class | urgent/realtime/history/mixed/unknown；是调度 lane，不等于原始 collection_mode |
| sample | valid/missing/future，仅时间样本可用性指标使用 |

Publish pipeline 标签为 `stage,result,source_type,collection_mode`。Collector pipeline 操作/耗时标签为 `queue_class,source_type,collection_mode,stage,result`；候选数和等待指标按各自边界使用相关子集及 sample。`collection_kind` 不保留为重复标签，使用 source_type 统一表达 bot/account 类型。不能把仅存在于 span/log 的维度直接写进 PromQL。

**禁止将 tenant/account/Bot/source/channel/rule/event/delivery/profile/message/trace ID 放入 Histogram 标签。**尤其不能用完整队列名、错误正文、动态等待消息代替固定标签。每增加一个 ID 维度都会与阶段、结果、buckets 相乘，不能因当前频道少就假设长期安全。

### 频道、Bot 与账户通过 span/SLS 下钻

建议面板筛选和日志字段以最终实现映射以下含义：

| 身份 | 解释与限制 |
|---|---|
| tenantId/sourceId | 业务作用域；相同频道可属于多个独立租户、来源和规则 |
| sourceChatId/canonicalChatId | 原始与规范化 Telegram Chat ID；canonical 可合并短/长号，但不可跨租户/来源合并业务身份 |
| channelId | 内部频道主键，不是 Telegram Chat ID；未加载不补查 |
| botId/tgAccountId | Bot 主键与 Telegram 运行账号，不能混用；Bot token/key 不进入面板 |
| accountId | 各入口现有语义不同：队列 scope、原始事件存储账户、解析后的业务 owner 必须明确区分 |
| eventAccountId | 窗口日志中为事件存储归属；Bot 为 0 可以合法，不表示采集失败 |
| upstreamEventId/deliveryId/collectEventId | Collector event、Delivery、Publish collect_event 是三套主键，不能当同一 eventId 联表 |
| ruleId/dispatchId/profileId | 规则分支、已接受交接和资料身份；唯一资料统计不得按消息数替代 |
| traceId/traceID | 日志字段大小写按实际解析；异步可变化，不作为业务幂等键 |

当前 `collectobserve.Metadata` 只有一个 AccountID，不能凭它同时得出 storage/queue/owner 三种值。字段未拆分时，报表标注本事件来源和语义，不自行推导未知身份。0 与 missing/unknown 需要区别，不能一律隐藏或填成真实 owner。

仅从 ctx、payload 和业务已加载记录补充身份。共享上下文为空时如实 unknown；跨事件/跨规则使用派生上下文，不能让上一条事件的 rule/profile 身份串入下一条。

## 时间与跨队列限制

### ReceivedAt 不能直接当端到端起点

历史 `ReceivedAt` 可能是 Telegram 消息日期；发布侧 event 的 received_at 也不能默认解释为本机采集入口时间。用“现在减 ReceivedAt”会把历史消息年龄误算成采集耗时。

共享观测 `Metadata.ReceivedAt` 的目标语义是**可信本地 ingress 时刻**，只可由本次真实接收入口提供，不从历史 Telegram 日期或无时区 DB 时间恢复。即使字段名相同，也不能强转为同一个时间口径。

- 没有可信 ingress 起点：端到端耗时报 missing，不填 0。
- 没有可信 producer enqueue epoch：队列等待报 missing，不从消费者开始、DB CreatedAt 或 Telegram 日期伪造。
- 未来时间：报 future，不当成 0 秒。
- realtime/history 分开展示；history 不能因 TG 消息很旧就触发“采集延迟数天”告警。
- durationMs 为整数兼容字段；durationUs 提供独立精细耗时，二者表示同一时段，不能相加。

### 本轮不改变队列幂等

`collect_process_queue.go::collectProcessTaskBody` 的 JSON 参与 Asynq.Unique 的 payload 哈希。不得为观测添加易变 trace、enqueue 时间；不得改变 TaskID、queue、Unique TTL 或业务调度键来传 telemetry。

Collector 的 `collectorTaskPayload/parseCollectorTaskID` 是数字协议，不能直接换 JSON 破坏旧消费者。Asynq 的 EnqueueContext 不会传输任意 context.Value；消费者的 task ID/retry count/queue 元数据也不是 producer carrier。

现有 direct 同步交接可以保留父子 span；跨 Asynq 消费未建立 carrier 时，新建本次 attempt span，通过业务 ID/SLS 关联。不得通过固定 TraceID、猜测 span ID 或强行复用上次上下文制造完整 Trace。本轮边界为 **noSQL/noRedis/noQueueSchema**：不为观测新增 SQL/Redis 查询或写入，不新增 sidecar、carrier 持久化，不改变队列 payload/schema、Unique 哈希或调度协议。复用业务已有读取结果，不改变原有业务 I/O。

### 嵌套耗时

父 event_dispatch 可能包含 delivery_consume，后者包含 handler；batch 可包含 primary/retry 查询和配对。**不能将父 batch duration 与两条 query.duration 求和。**窗口 query timer 是 ORM 往返，包含连接等待、网络、解码，不是 PG 纯执行时间。

部分旧 Publish span 是操作返回后补写，不是内部 DB span 的真实父级；新增 startCollectPipelineStage 才在执行期间建立上下文。全链路图需区分两类实现，不能仅以时间范围重叠推断父子关系。

## 已有指标与建议面板

下列 OTel instrument 名来自静态源码；Prometheus 转换后的名称、标签及 buckets 要在实际接收端确认。

| instrument | 用途 | 禁止误读 |
|---|---|---|
| telegram_collector_pipeline_operations | Collector 每阶段 attempt 数 | 不是唯一消息/新增事件数 |
| telegram_collector_pipeline_duration_seconds | Collector 单阶段执行耗时 | 不包含完整队列驻留 |
| telegram_collector_recovery_candidates | 每次恢复查询返回的候选行 | 不是积压总量；同一条可多轮出现 |
| telegram_collector_pipeline_delivery_queue_wait_seconds | 有可信 producer epoch 的 Delivery 等待样本 | 无样本不是等待为零 |
| telegram_collector_delivery_wait_samples | valid/missing/future 分布 | 只覆盖执行到观测点的调用 |
| telegram_collector_pipeline_event_queue_wait_seconds（新增） | 有效 Event 等待样本 | numeric task 当前无法提供可信 enqueue 起点 |
| telegram_collector_event_wait_samples（新增） | Event 时间样本质量 | 不等于全部入队事件数 |
| xiaohuiji.collect.pipeline.operations | Publish 阶段 attempt 数 | 事务 persisted 与调用 nil/queued 不同 |
| xiaohuiji.collect.pipeline.duration_seconds | Publish 各阶段执行耗时 | admission 按 Job 计数，无须为每个 ID重复记录批次耗时 |

旧 ingest/delivery 指标与 pipeline 指标不要相加。Trace 采样率不是 metric 采样率；SLS 与 Trace 缺失也可能是采集、采样或索引问题，不直接推断业务未执行。

建议六组面板：

1. **观测质量**：unknown source/mode 比例、时间 missing/future 次数、角色/版本是否覆盖。先看数据可靠性。
2. **入口与交付**：Collector 按 bot/account、realtime/history 展示 accepted/skipped/error attempt；Publish source_attempt 的源禁用/事件不存在为 ignored，已处理无需修复为 duplicate。direct 不强制出现 delivery_enqueue。
3. **匹配和等待**：授权/规则匹配/配对/媒体阶段的成功、跳过、等待、失败与 P50/P95/P99。等待原因明细从 SLS 看，不把自由文本加为 metric label。
4. **资料入库**：profile 事务成功 attempt、失败 attempt、阶段 P95；persisted 包含复用，不能区分新增。标题不能写成“唯一资料入库数”。
5. **窗口成本**：primary/retry 查询量、错误率、成功空查询率、ORM P95、batch hit/miss/not_evaluated 和未进入 run 的 miss。
6. **频道排查表**：按 tenant/account/source/canonicalChatId/Bot/运行账号从 SLS 小窗口聚合最后阶段、等待原因、失败 attempt、观测到的持久化成功；点击进入 event/rule/profile 日志链路，不做全量 DB 报表扫描。

不能用 submit 或 ingress attempt 作分母、profile persisted 作分子生成“消息采集成功率”：两者基数不同。可展示同一 profile 持久化阶段的成功尝试比例，但名称必须包含“尝试”，等待与复用也需明确口径。

## 查询模板（未执行、尚未部署验收）

### OTel/Prometheus

以下使用 stage/result/source_type/collection_mode 标签，metric 名按常见转换展示；若接收端保留点号或使用不同后缀，应调整。

```promql
sum by (stage, result, source_type, collection_mode) (
  rate(xiaohuiji_collect_pipeline_operations_total[5m])
)
```

```promql
histogram_quantile(0.95,
  sum by (le, stage, result, source_type, collection_mode) (
    rate(xiaohuiji_collect_pipeline_duration_seconds_bucket[5m])
  )
)
```

资料持久化成功尝试限定 profile_persist/persisted，包含复用，不表示新增数量：

```promql
sum by (source_type, collection_mode) (
  rate(xiaohuiji_collect_pipeline_operations_total{
    stage="profile_persist",result="persisted"
  }[5m])
)
```

不要用所有 completed 代替资料成功，也不要把所有 stage 的 Counter 累加成总消息吞吐。source_attempt 的源禁用/事件不存在结果为 ignored，已处理无需修复结果为 duplicate，分别展示。

Collector 按来源模式分别观察：

```promql
sum by (stage, source_type, collection_mode, result) (
  rate(telegram_collector_pipeline_operations_total[5m])
)
```

等待 Histogram 没有有效样本时显示“无可靠样本”，另展示 wait_samples 的 sample 分布；不得把空向量填 0 后宣称队列健康。

### SLS 频道与已记录阶段样本聚合

执行前遵循已有 SLS 手册核对真实日志绑定、时间窗、原文及索引。g.Map 可能仍包在 content/message 中，不能假定是顶层字段；需要字段统计配置，未解析时先全文检索。以下仅是顶层字段提取完成后的模板。**同一 stage 的普通快成功子步骤没有逐条 SLS 日志，因此本模板只统计已记录结果，不计算完整阶段平均耗时或 P95；完整阶段分位数使用 OTel Histogram。**

```sql
event:collect_stage_completed
| SELECT tenantId, accountId, sourceId, canonicalChatId,
    sourceType, collectionMode, stage, result,
    count(*) AS observed_attempts
  FROM log
  GROUP BY tenantId, accountId, sourceId, canonicalChatId,
    sourceType, collectionMode, stage, result
  ORDER BY observed_attempts DESC
  LIMIT 100
```

Publish helper 已输出 canonicalChatId，模板用它合并短/长号，sourceChatId 保留原始排障值。channel/Bot/account 的细分通过字段聚合实现，不转为 OTel 高基数标签。observed_attempts 仅是已记录的日志样本数；快成功子步骤被省略时不能用它作为总体 attempt 分母。

窗口查询口径：

```sql
event:collect_window_query_completed
| SELECT queryKind,
    sum(try_cast(queryCount AS bigint)) AS query_attempts,
    sum(try_cast(queryErrorCount AS bigint)) AS query_errors,
    100.0*sum(try_cast(emptyQueryCount AS bigint))
      /nullif(sum(try_cast(queryCount AS bigint))
        -sum(try_cast(queryErrorCount AS bigint)),0) AS successful_empty_pct
  FROM log
  GROUP BY queryKind
  LIMIT 10
```

emptyQueryCount 仅成功且 rows=0 时为 1；错误不算空。batch 命中率分母只用 hit+miss，not_evaluated 不纳入；miss 不保证 SQL 已执行。rowsLoaded/windowTruncated 仅 batchRunStarted=true 时有效，hit 的 0/false 不计为空窗口。

SLS LIMIT 仅限制返回行数，不保证查询成本有界；先选最近 5～15 分钟及明确作用域。结果 Incomplete、日志采样或索引延迟时需标注统计不完整，不能拿来声称资料丢失或完成全量验收。

## 最终接线核对

- Publish 使用本文 stage 与四个低基数标签；profile_persist/persisted 必须符合 commitCollectMaterial 成功且 profileID>0，不能解释为新增。source_attempt 的源禁用/事件不存在为 ignored，已处理无需修复为 duplicate，不新增 skipped。
- Collector 使用 source_type/collection_mode，不重复添加 collection_kind；保持 Bot/运行账号语义、numeric 队列兼容和 missing 样本。
- 同一操作只记录一次；新真实 span 与旧 observe wrapper 不重复增加相同 attempt。
- 并发规则和跨事件处理不串上下文；观测不改变业务归属、租约、去重或队列路由。
- 现有业务读取的数据直接绑定 ctx；不为指标新增 DB/Redis 查询、扫描、写计数器或后台循环。
- 超时、取消、panic、禁用、去重、等待、事务回滚均需准确分类；未完成验证的场景列出限制，不称已通过。
- 快成功子步骤只指标/trace；pending/failure/persisted 等关键结果记录日志。SLS 样本分位数不能替代完整 OTel 分位数。
- 受限验证和部署之后，再根据真实遥测样本启用面板。本文尚未部署验收，也不是测试通过记录。
