# Collector 运行观测与报表

入库端到端均值、匹配耗时、具体来源／资料 TOP10 的统计口径与查询见
[采集入库耗时与积压排查](collector-lifecycle-dashboard.md)。

Collector 观测覆盖接收、原始事件持久化、事件入队、事件消费与交接、交付消费、业务 handler 和恢复扫描。发布侧观测另行覆盖采集提交、事件处理、资料分发调用、TG Job 准备及发布队列投递。两者分别使用 `hotgo/addons/telegram_collector`、`hotgo/addons/youban_publish` scope，复用通用 OpenTelemetry 导出配置。观测本身不新增数据库/Redis 查询或后台扫描。

## 配置与运行角色

各个实际执行采集阶段的进程都需要启用通用 OTel：

```yaml
telemetry:
  switch: true
  endpoint: "127.0.0.1:4317"
  secure: false
  serviceName: "xiaohuiji"
  sampleRatio: 0.15
  metricsIntervalSeconds: 15
```

endpoint 是部署环境可达的 OTLP gRPC 接收地址；示例本机地址只适用于同机接收器。secure=false 使用明文连接，TLS 接收器使用 true。metricsIntervalSeconds 默认 15 秒，小于 5 秒会回退到 15 秒。sampleRatio 控制 Trace 采样，不是指标抽样；单条日志存在但未检索到 span，不足以判定业务未执行。

资源属性包含 `service.name`、`service.instance.id`、`service.version`、`deployment.environment.name`、`runtime.role`。存在 `RAILWAY_SERVICE_NAME` 时覆盖 telemetry.serviceName。接收端是否将资源属性转换为 Prometheus 标签取决于其配置。

| 执行阶段 | 运行位置 |
|---|---|
| BOT/协议号消息接收、原始持久化及 event 入队 | 实际调用 IngestBotUpdate/IngestAccountMessage 的入口进程 |
| event 消费与恢复 | `collector-worker`；兼容 `worker` |
| queued delivery 消费与恢复 | `push-worker`；兼容 `worker` |
| direct delivery 消费 | event 所在进程内联调用，不要求经过独立 delivery 队列 |
| 历史交付调度 | `background-worker` |

部署应用名称与 runtime.role 可能不同，以启动命令、资源属性及启动日志为准。event 启动日志为 `collector_worker_started`，delivery 为 `collect_delivery_worker_started`。采集开关关闭时不会启动相应消费循环。仅入口有指标、消费端无指标时，应分别核对消费角色是否启动及该角色是否启用 OTel。

## 实际阶段覆盖

| stage | 覆盖范围 |
|---|---|
| `ingest_bot` / `ingest_account` | 单次入口接收，到原始持久化、入队和 spool 收口返回；包含跳过和错误 |
| `event_persist` | 原始事件恢复 scope 注册、事件 upsert 及 ID 回读 |
| `event_enqueue` | event 的 Asynq 入队调用及结果分类 |
| `event_consume` | processEvent 的整体执行 |
| `event_claim` / `event_read` / `event_parse` | 领取事件、读取原始记录、解析 delivery |
| `event_dispatch` | handoffCollectorEvent 的交接过程；direct 模式可嵌套 delivery 执行 |
| `delivery_enqueue` | delivery 路由读取及 Asynq 入队 |
| `delivery_consume` | processDelivery 整体，包含 handler 和完成状态落库 |
| `delivery_claim` / `delivery_read` / `delivery_event_read` / `delivery_parse` | 领取交付、读交付记录、读原始事件、解析 delivery |
| `delivery_handler` | HandleCollectorDelivery 调用；不包含后续 completed 写入 |
| `delivery_complete` | delivery completed 状态的数据库写入 |
| `event_recovery` / `delivery_recovery` | 对应恢复轮次 |
| `event_recovery_scan` / `delivery_recovery_scan` | 恢复候选的有界查询页 |
| `event_recovery_enqueue` / `delivery_recovery_enqueue` | 恢复循环的单次重新投递 |

阶段存在父子嵌套，不能把所有 duration 相加当端到端耗时。特别是 direct 模式的 event_dispatch 可能包含 delivery_consume，delivery_consume 又包含 delivery_handler。direct 交付可以不产生 delivery_enqueue 数据，不能因此判定交付遗漏。event_dispatch ok 表示 Collector 已完成交接；event ready 之后重试责任属于 delivery。它不等于资料入库、各条规则分发或 Telegram 发送成功。

发布侧阶段使用另一组指标，不能与上表混为相同计数边界：

| stage | 实际覆盖与结果 |
|---|---|
| `delivery_submit` | submitCollectorDelivery 的持久化与处理队列提交调用；当前结果为 queued/failed |
| `process` | 成功读取发布侧 collect_event 后的处理终态观测；completed/pending/ignored/retryable/failed |
| `process_consume` | 一次处理队列消费，从恢复观测上下文到函数返回，包含本实例规则并发闸门等待；nil 结果记 unconfirmed，实际业务结果看内层阶段 |
| `dispatch_handoff` | 新资料非审核分支对 submitCollectProfileDispatch 的调用；nil 返回为 unconfirmed，错误为 failed |
| `publish_admission` | prepareProfilePublishJobs 返回的每个有效 TG Job ID；结果为 persisted，无耗时样本 |
| `publish_queue_enqueue` | 权威 Redis channel 队列及 Asynq 发布任务入队调用；queued/duplicate/conflict/unconfirmed/failed |

`dispatch_handoff=unconfirmed` 明确表示没有入队回执，可能仍在等待媒体或被跳过，不能换算成 queued、成功发布或失败。`delivery_submit=queued` 表示提交包装函数返回 nil；底层处理队列接口丢弃了 enqueued 布尔值，因此可能包含去重/调度保留，不证明创建了新队列任务。

`publish_queue_enqueue=queued` 才表示该次发布入队调用获得其运输层认可的回执：Asynq 返回任务信息且无错误，或权威 channel 入队函数返回成功。duplicate/conflict 不作为新入队回执；无错误但无回执为 unconfirmed。channel 内部 Redis 重试折算为一次最终尝试；独立 Asynq 调用及 fallback 各计一次。这里仍然不代表 Telegram 消息发送成功。

`publish_admission` 按返回 ID 数计数，幂等准备可以再次返回已有 Job ID。admission 与 queue 均在共享发布路径，覆盖手动、循环、批量等发布任务，并非仅采集来源。不能将它们与仅采集的 dispatch_handoff 计数相除，作为采集转化率。

## 结果与计数语义

| result | 含义 |
|---|---|
| `accepted` | 入口完成接收交付，或原始事件 upsert 得到有效 ID；不代表新建或最终发布成功 |
| `ok` | 当前阶段成功完成 |
| `skipped` | 开关关闭、relay 过滤、任务不满足领取条件等明确跳过 |
| `duplicate` | 入队检测到已有任务，并按该调用的业务语义接受去重结果 |
| `error` / `canceled` / `timeout` | 当前阶段失败 / 取消 / deadline 或网络超时 |
| `unknown` | 未知或未明确分类的观测值 |

Counter 统计每次尝试，不统计唯一消息、资料或任务。upsert 成功不能据此判断新增数量；修复已有原始事件的路径可能直接进入入队，不经过 event_persist。入口 spool 收口失败时，即使入队已经成功，入口仍是 error，应同时查看 event_enqueue。

入口级开关关闭或 relay guard 明确记 `ingest_*=skipped`。发布 adapter 的 global_disabled/source_unavailable/self_loop_publish_target/approval_blocked/channel_access_blocked 等 guard 使用 `logCollectorDeliverySkipped` 写显式跳过日志后返回 nil：此时 Collector 的 delivery_handler/delivery_consume 可以是 ok，但没有进入 delivery_submit。这里的 ok 只表示调用正常返回，不能算资料实际采集成功。编辑消息更新路径也可能绕过 delivery_submit。

恢复候选计数是现有查询每次返回的行数，同一任务可在多轮恢复中被重复计数，不代表 backlog 大小。恢复外层调用返回 nil 只能说明接受交接；应结合内层 enqueue 的 duplicate/error 判断是否真正创建了新任务。

## 标签、日志与关联字段

指标仅使用有界标签：

- `stage`：上述固定阶段；其他值归入 unknown。
- `result`：固定结果分类。
- `queue_class`：urgent/realtime/history/mixed/unknown，不含账户分片或真实队列名。
- `source_type`：bot/account/unknown。
- `collection_mode`：realtime/history/unknown，未知时不从队列优先级猜测采集模式。
- `error_type`：Collector 受控错误分类；发布业务阶段对应 `error_kind`。
- `sample`：仅等待样本计数使用 valid/missing/future。

tenant、account、source、chat、message、event、delivery ID 不作为这些指标的标签。阶段读取记录前，身份或 lane 尚不明确时使用 unknown；后续阶段及整体 span 可以补全。指标已经记录的数据点不会追溯改标签。direct delivery 的 lane 按已加载事件及解析后的 History/路由规则判断，不从父 event task 的 Asynq 队列名推断。

发布侧 `xiaohuiji.collect.pipeline.*` 指标使用白名单 stage/result/error_kind/wait_reason/source_type/collection_mode 标签；job/channel/profile ID、transport、提交原因只进 span，不扩展指标标签。旧 `telegram_collector_ingest_latency_seconds` 也已移除 source_id/account_id 标签；既有报表若使用这两个维度，需要改为按阶段聚合，具体来源通过日志和 span 查询。这不代表已完成仓库所有指标的基数审计。

局部 span 与慢/错误日志事件 `collector.stage.completed` 包含 `stage/result/durationMs/queue_class/source_type/traceID`，并带已获得的 `tenantId/accountId/sourceId/eventId/deliveryId/chatId/messageId`。span 属性对应 snake_case ID，零值身份表示当时未知；accountId 的含义需结合原始事件来源判断，不得将 Telegram 运行账户 ID 与业务账户 ID 混用。

阶段耗时达到 100ms 或失败时记录阶段日志；高频成功阶段以指标为主，原有业务终态日志继续提供生命周期证据。失败包括显式 error 结果的 panic 兜底，err 为 nil 也标记 span Error。观测日志不复制原始错误正文、消息内容或签名 URL。

业务日志可按 eventId/collectEventId 关联原始事件，按 deliveryId 关联交付；结合 tenantId/sourceId 防止串链。`eventCreatedAtRaw`、`deliveryCreatedAtRaw` 是原始 DB 墙钟时间诊断字段，不是可信 epoch。事件日志通过 `ageTimeBasis=db_wall_clock_unverified`、`eventAgeAvailable=false` 明确其时间限制，不提供基于 DB CreatedAt 推算的 eventAgeMs。

关联发布侧时需区分两套 event ID：span 的 `collect.upstream_event_id` 是原始 Collector event，`collect.event_id` 是发布侧 collect_event，`collect.delivery_id` 是交付 ID。它们不是可互换的同一主键。继续用 `collect.dispatch_id`、`profile.id`、`job.id` 关联后续步骤。发布侧 process 指标与终态日志的 outcome 名称不完全等价，例如未 processed 的 pending 不能仅凭日志默认 outcome=completed 判定已经结束，应核对 processed/eventStatus。

## Trace 与等待时间边界

当前任务 payload 保持纯数字 ID，由 ParseInt 解析，没有跨进程 trace carrier，也没有可信 enqueue epoch。进程内 direct 调用可形成父子 span；跨 Asynq 消费不能保证同一 Trace，需使用 event/delivery ID 与结构化日志关联。Trace 缺少跨队列父子关系不等于消息丢失。

当前 delivery 等待观测传入缺失的 enqueue 时间，只增加 `telegram_collector_delivery_wait_samples{sample="missing"}`，不向 queue-wait Histogram 写入 0 或伪造耗时。该计数出现在执行到等待观测点的调用中，不是所有入队任务总数。数据库 CreatedAt/UpdatedAt、Telegram 消息日期均不能代替本次可靠的 enqueue 时间。

`delivery_consume` 是进程内处理耗时，不是 Redis 等待。预留的 queue-wait Histogram 仅接受可信 producer epoch 到 consumer 接收时刻的时长；该语义包含定时延迟与重试退避，不是 Redis CPU 耗时。缺失或未来时间不产生 histogram 样本。当前没有有效样本时，等待 P95 应显示“无可靠样本”，不能显示“等待为零”。

有效 queue-wait 的跨进程时钟需要同步；当前 numeric 协议不提供这项数据。消费入口在 payload 解析或业务记录校验前提前失败时，应结合原有 Worker 错误日志排查，不能仅用后续 parse/handler 子阶段计数判断所有失败。

部分阶段只有计数/耗时，没有独立 span；发布侧 observeCollectPipeline 在操作返回后按起止时间补写 span，这些 span 不是操作内部 DB/Redis 子 span 的父级。Trace 可用于定位阶段与业务身份，不能据此宣称跨队列、跨全部 DB 操作的完整父子关系已经贯通。

## 覆盖限制

- `event_complete`、`delivery_prepare` 当前只有枚举定义，没有实际 hook；图表无数据不表示这些阶段耗时为零。event ready 写入包含在 handoff 内，delivery 前置路由/slot 不应被误读为 processDelivery 整体。
- 非法 numeric task ID 在 process span 之前返回；原始记录为空也可能在 parse 子阶段之前返回。整体失败与子阶段计数不能强行逐项相等。
- 发布侧 process 观测在事件锁获取和 event 读取之后建立，锁竞争跳过、加载前失败不在该阶段的计数和耗时中。
- dispatch_handoff 当前不覆盖所有审核、历史恢复/已有 dispatch 复用路径；它不是全部资料提交或实际入库数量计数。
- handler nil guard skip、编辑更新、upsert 去重、媒体 pending 会让相邻阶段计数不同；应按业务 ID 查日志，不用“入口减出口”直接推算丢失资料数。

## 指标与 buckets


| OTel 指标名 | 单位 | 标签 |
|---|---|---|
| `telegram_collector_pipeline_operations` | `{operation}` Counter | stage/result/error_type/queue_class/source_type/collection_mode |
| `telegram_collector_pipeline_duration_seconds` | `s` Histogram | 同上 |
| `telegram_collector_recovery_candidates` | `{candidate}` Counter | stage/queue_class/source_type/collection_mode |
| `telegram_collector_pipeline_delivery_queue_wait_seconds` | `s` Histogram | queue_class/source_type/collection_mode |
| `telegram_collector_delivery_wait_samples` | `{sample}` Counter | sample(valid/missing/future)/queue_class/source_type/collection_mode |
| `telegram_collector_pipeline_event_queue_wait_seconds` | `s` Histogram | queue_class/source_type/collection_mode |
| `telegram_collector_event_wait_samples` | `{sample}` Counter | sample(valid/missing/future)/queue_class/source_type/collection_mode |
| `xiaohuiji.collect.pipeline.operations` | 未显式设置单位，Counter | stage/result/error_kind/wait_reason/source_type/collection_mode |
| `xiaohuiji.collect.pipeline.duration_seconds` | `s` Histogram | 同上 |

阶段 seconds buckets：`0.001,0.005,0.01,0.025,0.05,0.1,0.25,0.5,1,2.5,5,10,30,60,120`。

等待 seconds buckets：`0.01,0.05,0.1,0.5,1,5,10,30,60,120,300,600,1800,3600,21600,86400`。更大值进入 +Inf，并未被丢弃。SDK View 可覆盖 instrument advisory buckets，部署报表前核对导出 `_bucket{le=...}`。

发布侧 `xiaohuiji.collect.pipeline.duration_seconds` 也使用上述阶段 seconds buckets。SDK View 可以覆盖边界，上线仍应核对实际导出。publish_admission 只增加 operations，不重复为批次内每个 Job 记录准备耗时，因此该 stage 没有对应 duration 样本。

Counter 是每次尝试，不是唯一 event/delivery 数。恢复候选是每次查询实际返回行数，同一记录可跨轮次重复出现，不等于当前 backlog。空页通过 scan 的 operations=ok 计数表示；candidate 的零增加不生成数据点。

## 报表 PromQL

以下基于标准 Prometheus 转换后的 `_total/_bucket/_sum/_count` 名称。上线先核对接收端实际名称（OTLP 转换策略可能不同）；不要对新旧指标相加，避免双重计数。

入口已接受、跳过和失败的每秒尝试数：

```promql
sum by (stage, source_type, result) (
  rate(telegram_collector_pipeline_operations_total{
    stage=~"ingest_bot|ingest_account",
    result=~"accepted|skipped|error|canceled|timeout"
  }[5m])
)
```

入口吞吐使用 accepted；不要把 skipped 或 event_persist 的 upsert 计数作为新增资料数。

发布侧各阶段每秒观测数（stage/result 口径独立）：

```promql
sum by (stage, result) (
  rate(xiaohuiji_collect_pipeline_operations_total[5m])
)
```

确认的发布入队尝试与分发调用未确认数，分别展示：

```promql
sum(rate(xiaohuiji_collect_pipeline_operations_total{
  stage="publish_queue_enqueue",result="queued"
}[5m]))
```

```promql
sum(rate(xiaohuiji_collect_pipeline_operations_total{
  stage="dispatch_handoff",result="unconfirmed"
}[5m]))
```

前者覆盖共享发布路径，后者只覆盖对应采集分发调用，不能相减或相除判断采集丢失率。delivery_submit 的 queued 也不能替代前者。unconfirmed 是缺少回执，不加入 failed 或 queued 的成功率统计。

发布侧处理分布和阶段 P95：

```promql
sum by (result) (
  rate(xiaohuiji_collect_pipeline_operations_total{stage="process"}[5m])
)
```

```promql
histogram_quantile(0.95, sum by (le, stage, result) (
  rate(xiaohuiji_collect_pipeline_duration_seconds_bucket[5m])
))
```

publish_admission 的 persisted 仅供观察返回的 Job ID 数量，不是唯一新增 Job 数，也不出现在 P95 中。

阶段每秒尝试数和错误率：

```promql
sum by (stage, result, queue_class) (
  rate(telegram_collector_pipeline_operations_total[5m])
)
sum by (stage) (
  rate(telegram_collector_pipeline_operations_total{result=~"error|canceled|timeout"}[5m])
) / clamp_min(sum by (stage) (
  rate(telegram_collector_pipeline_operations_total[5m])
), 0.000001)
```

上述错误率的分母是全部阶段尝试，包含 skipped/duplicate；它表示尝试失败比例，不是唯一资料发布失败率。

阶段 P95（保留 result 以观察失败耗时）和超过 100ms 比例：

```promql
histogram_quantile(0.95, sum by (le, stage, result, queue_class) (
  rate(telegram_collector_pipeline_duration_seconds_bucket[5m])
))
1 - sum by (stage) (
  rate(telegram_collector_pipeline_duration_seconds_bucket{le="0.1"}[5m])
) / clamp_min(sum by (stage) (
  rate(telegram_collector_pipeline_duration_seconds_count[5m])
), 0.000001)
```

恢复每轮平均候选数（两者 stage 一致，按 page 计算）：

```promql
sum by (stage, queue_class) (rate(telegram_collector_recovery_candidates_total[5m]))
/ clamp_min(sum by (stage, queue_class) (
  rate(telegram_collector_pipeline_operations_total{
    stage=~"event_recovery_scan|delivery_recovery_scan",result="ok"
  }[5m])
), 0.000001)
```

等待 P95 与缺失/时钟问题：

```promql
histogram_quantile(0.95, sum by (le, queue_class) (
  rate(telegram_collector_pipeline_delivery_queue_wait_seconds_bucket[5m])
))
sum by (sample, queue_class) (rate(telegram_collector_delivery_wait_samples_total[5m]))
```

阶段观察应配合已有终态日志：event_dispatch ok 仅说明 Collector 已交付，不代表 profile 入库、各条规则分发成功或 Telegram 发布成功。不要用它替代下游发布成功率。

## 2026-11-18 采集全流程指标契约

业务边界按以下顺序分析，不将相邻 Counter 相减当作丢失资料数：

```text
入口接收（Bot / 协议号，实时 / 历史）
  → 原始事件持久化、event 队列
  → 来源分发、delivery 队列、发布模块接纳
  → process 队列、授权、规则预检
  → 窗口读取、展示/验证配对、媒体处理、内容提取
  → 规则分支、资料创建或复用
  → TG Job 准备、发布队列回执
```

来源分发的对象是“消息 × 采集源”，规则分发的对象是“候选资料 × 规则”，
发布队列交付的对象是“资料 × 目标频道 Job”。一条消息可以产生多次交付；
多个消息可以组成一份资料；一份资料可以投递多个目标频道。Telegram 实际发送
由发布模块负责，不属于采集入库成功的定义。

已有 Collector 阶段次数、执行耗时、恢复候选和发布阶段指标继续使用。
阶段结束通过 once 保护；超时与取消单独归类；Metrics 不依赖 Span 采样。
嵌套阶段耗时不能相加；`waiting` 类结果表示本次执行进入等待，
不是已经准确测量了整个配对或媒体等待周期。

| OTel 指标名 | 统计对象、单位和确认节点 | 有界标签 |
|---|---|---|
| `xiaohuiji.collect.transport.stage.started` | Collector 阶段开始尝试数，`{attempt}`，包括重试 | stage/queue_class/source_type/collection_mode |
| `xiaohuiji.collect.transport.stage.active` | 本进程阶段开始减结束，`{attempt}` UpDownCounter；不是共享队列积压 | 同上，使用开始时标签，避免补全身份后减错序列 |
| `xiaohuiji.collect.query.executions` | 窗口 ORM 调用数，`{query}`，包括失败与重试 | query_name/result/source_type/collection_mode |
| `xiaohuiji.collect.query.duration_seconds` | 单次 ORM 往返耗时，`s`；包含连接等待及结果解析，不是 PG 服务端执行时间 | 同上 |
| `xiaohuiji.collect.query.returned_rows` | 成功查询实际返回行数 Histogram，`{row}`；成功空结果记 0，失败不伪装成空结果 | 同上 |
| `xiaohuiji.collect.batch_reuse` | 已评估的批次复用决定，`{decision}`；缓存错误、租约忙等未评估分支不进入分母 | reuse_result(hit/miss)/source_type/collection_mode |
| `xiaohuiji.collect.process.queue.wait_seconds` | 可信生产者提交到本次消费，`s`；含计划延迟及 Asynq 重试停留 | source_type/collection_mode |
| `xiaohuiji.collect.process.queue.wait_samples` | 排队时钟样本可用性，`{sample}`；缺失或未来时间不写耗时 0 | sample(valid/missing/future)/source_type/collection_mode |
| `xiaohuiji.collect.delivery.skipped` | 发布 adapter 明确策略跳过的交付尝试，`{delivery}`；不算采集成功 | reason/source_type/collection_mode |
| `xiaohuiji.collect.profiles.persisted` | 顶层事务成功返回后的持久化结果 Counter；created 为新建，reused 为复用，包括幂等重试复用 | action(created/reused)/source_type/collection_mode |
| `xiaohuiji.collect.lifecycle.duration_seconds` | 可信本地接收至顶层事务确认新建资料，`s`；复用不重复计算新建生命周期 | action/source_type/collection_mode |
| `xiaohuiji.collect.lifecycle.missing` | 无可信起点、时钟异常或外层事务未确认次数；不补造生命周期耗时 | action/reason/source_type/collection_mode |

新指标复用全局 MeterProvider 和现有 OTLP 配置，不新增 exporter、连接、后台扫描、
数据库 COUNT 或 Redis 查询。ID、频道名称、账号、Bot、完整错误明细用现有结构化
日志及 Span 关联；它们不作为新 Metrics 标签。

`delivery.skipped.reason` 仅允许 global_disabled/source_unavailable/
self_loop_publish_target/approval_blocked/channel_access_blocked/unknown。
这样即使上游 transport handler 正常返回，也能识别业务策略跳过，避免当成入库成功。

### 跨队列上下文与兼容限制

可安全修改载荷的任务透传 W3C traceparent/tracestate、本地接收时间和本次提交时间，
不传 Baggage、不改业务任务 ID、权限或路由。载体只用于观测，不能作为授权依据。
原始 Telegram 消息时间和未经核实的数据库墙钟时间不作为排队或端到端起点。

处理队列仅为固定 TaskID 或不使用载荷哈希去重的事件任务添加可选观测字段；
按载荷哈希执行 Asynq Unique 的任务保持原有载荷，避免时间戳/Trace 改变去重结果。
source wake 会处理一批事件，不携带某一条事件的接收时间。旧载荷、恢复任务或
未安全携带起点的路径会出现 missing/unknown，这是覆盖率限制，不是零延迟。

Collector 的数字载荷兼容旧消费者，但新 JSON envelope 需要新消费者解析。
因此 `YOUBAN_COLLECTOR_TELEMETRY_CARRIER_ENABLED` 默认关闭；
先部署所有 event/delivery 消费角色，再开启生产者侧开关。
未开启时仍有阶段指标，只缺少相应跨进程 Trace 和可信排队样本。
Unique delivery 始终保留旧数字载荷，不能通过开启开关绕过去重保护。
处理队列的观测字段为原 JSON 中的可选扩展，旧处理消费者可忽略，
与 Collector 数字协议的滚动升级限制不同。

资料持久化若运行在外层事务内，子事务返回 nil 不等于最终提交：
只记录 `transaction_unconfirmed`，不增加已提交资料数。legacy_sent 等复用分支
不会误算为 created。`profiles.persisted{action="reused"}` 是复用结果次数，
不是唯一复用资料数；created 也不能用来替代所有入口消息总数。

### 大盘计算

批次复用率 = hit / (hit + miss)，不将 not_evaluated 算成 miss。
查询平均返回行数 = returned_rows_sum / returned_rows_count，
与 executions 的失败率一起看，避免把失败误判为查询空跑。

资料新建吞吐看 `profiles.persisted{action="created"}`；
端到端耗时看 lifecycle Histogram，并同时展示 lifecycle.missing。
发布队列接纳看 `pipeline.operations{stage="publish_queue_enqueue",result="queued"}`，
其覆盖共享发布入口，不能和仅采集新建数直接相除计算转化率。

全局积压、最老任务年龄、完整配对等待周期没有可信快照时不新增伪 Gauge。
transport.active 是每实例正在执行的阶段数，且阶段存在嵌套；
可以按同一 stage 跨实例汇总，不能跨 stage 求和称为资料数。
共享队列快照若后续添加 Gauge，需要明确采样上限和实例归属，禁止多实例重复
上报同一份共享积压再直接 sum。

现存 `runAsynqObserveMetrics` 的全局数据库聚合和跨角色重复采样不在这组新指标中；
不能因为本轮新埋点没有额外查询，就认为所有旧监控查询已完成性能治理。
这类旧库存指标跨实例展示应先去重，不能直接当本轮 `transport.stage.active` 使用。

正常停机在 `serverWg` 排空后触发一次 `server.drained`，再关闭 OTel。
Trace 与 Metrics 各有独立 5 秒预算，不继承已取消的业务 ctx。
平台退出宽限时间需要覆盖业务排空和这两份预算；强杀仍不保证最终导出。

### 本轮本地验证

2026-11-18 使用 `GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p 1`，
发布大包另加 `-gcflags=hotgo/addons/youban_publish/logic/sys=-l` 串行验证：

- collectobserve：W3C 及纳秒时间往返、旧数字任务、非法 Trace、source wake 清理身份；
- Collector：并发结束至多一次、不采样仍记录、active 标签配对、错误分类、缺失/未来时钟，
  以及 miniredis 下固定 TaskID 和 Unique 去重兼容；
- Publish：阶段结果/等待原因、队列回执、新建/复用/外层事务未确认、查询失败与返回行数、
  复用命中率分母、处理队列载荷和策略跳过指标；
- global/cmd：已取消 ctx 下的独立 Shutdown 预算、根命令等待业务排空、无 Func 的帮助命令兼容。

以上为选定测试通过，不代表全仓测试或线上验收。未连接生产数据库/Redis，
未执行迁移、重放或部署。全工作区 architecture_guard 仍报告其他发布/删除等路径违规；
本轮采集观测相关路径没有报告违规，未修改或放宽 guard/baseline。

## 部署前置条件与回滚

本次采集交接与埋点涉及 `web`、`collector`、`publish`。这只是本次变更的角色范围，不覆盖同一工作区内其他并行改动的部署要求。

恢复扫描依赖以下源范围索引，不能只部署镜像：

- `server/addons/telegram_collector/resource/sql/raw_event_scoped_recovery.pgsql.sql`：received、retry、lease 三个索引。
- `server/addons/telegram_collector/resource/sql/delivery_scope_recovery.pgsql.sql`：pool recovery、lease 两个索引。

这些文件使用 `CREATE INDEX CONCURRENTLY`，需要在事务外逐条执行，不能加入实例启动迁移。上线前核对索引定义、`indisvalid` 和 `indisready`；只有同名索引不足以证明可用。历史积压所属的源还必须注册在恢复范围集合中，索引不会自动发现未注册的旧源；包括 account/source 为 0 的合法历史范围。补登记必须先核对精确范围。

实时交接默认使用 direct，历史交接仍走原有队列与并发预算。需要回滚交接方式时配置 `YOUBAN_TELEGRAM_COLLECTOR_HANDOFF_MODE=queued`，已持久化的 Delivery 继续由自身状态机和恢复器处理。

本地验证使用受限并发，生产索引与数据操作未执行。部署后应按同一 eventId/deliveryId 核对 ready、handler、completed 和发布入队回执，不能用一次 handler 返回代替完整验收。

## 本地验证记录（2026-11-17）

- 主程序：`GOMAXPROCS=4 go build -p 2 -o /tmp/youban-local-server ./main.go` 通过。
- Collector 针对性测试通过，覆盖 handoff、指标、恢复游标、来源与队列过滤、恢复错误分类和执行归属保护。数据库用例实际连接隔离 PostgreSQL，未使用业务数据库。
- 新增 `TestFailEventRejectsStaleExecution` 通过：旧 owner 不得覆盖新 owner 或 ready 事件，当前 owner 可正常写入重试状态。
- ENV handoff 映射契约、Redis SetRotate 测试通过。
- Publish 指标针对性测试未取得结果：首次受 120 秒外层限时终止，第二次以退出码 130 中断，不能计为通过。
- 全仓架构检查失败，报告位于 `tg_media_fallback_handoff.go` 和 `tg_video_sanitize_cache.go`；这两个文件及检查器相对验证时 HEAD 均未改动。本次没有放宽 baseline。

这些是本地验证结果，不代表已完成线上验收或全仓测试。

## 恢复职责轮转隔离

来源登记集合仍为 `telegram_collector:recovery:scopes:v1`，不复制五份来源关系。
接收 Spool、Event 租约恢复、Event 重新入队、Delivery 租约恢复、Delivery
重新入队分别使用固定职责 `ingest_spool`、`event_lease`、`event_enqueue`、
`delivery_lease`、`delivery_enqueue` 保存轮转游标和待访问成员。

同一职责的不同实例共享进度，不同职责独立遍历完整来源集合。职责不能使用实例 ID，
未知或空职责直接报错，不允许悄悄回到旧共享游标。Redis Cluster 的派生 Key
保留原集合的 hash slot，脚本仍只执行一页 SSCAN 和有界取出。

升级后各职责从自己的新游标开始，不删除旧集合、旧游标或业务任务。已有来源登记仍有效，
未登记的旧来源不会因此自动出现，仍须精确补登记；索引前置条件也不变。
这项修复需要更新运行 Event 恢复的 collector 和运行 Delivery 恢复的 publish 角色。

本次本地验证已通过缓存轮转回归及 race 检查、Collector 恢复/交接/观测专项和受限并发主程序编译。
轮转回归包含五个职责以固定顺序各取 8 个来源，并断言每个职责均能覆盖同一批 40 个来源；
同一职责的实例共享进度，另一职责独立开始，已删除成员不再返回，非法职责拒绝执行。

2026-11-17 本轮 DBX 仅按主键和系统目录只读核对：

- 原始事件 `12682247` 仍为 received、attempt_count=0；交付 `40208008` 仍为 pending、attempt_count=0。
- 两者的恢复范围均为 tenant=78/account=0/source=351，不能以业务 account=615 替代。
- 五个预期来源恢复索引均未找到。进一步检查 Event/Delivery 现有来源与租约索引，
  未发现相同的来源范围恢复定义；因此索引前置条件尚未满足。
- 本轮没有创建生产索引、重放消息、改业务状态或部署镜像；上述记录不是修复后的线上验收。
