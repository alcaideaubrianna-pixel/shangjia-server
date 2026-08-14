# Railway 集群可观测性设计

> 目标：在从腾讯云单主机迁移到 Railway 集群后，能够回答“哪条链路、哪个阶段、哪个服务、哪个账号、哪个频道、哪条任务”发生了阻塞，并支持通过 `trace_id`、`flow_id`、`operation_no`、资料编号、采集事件号和 TG 任务号下钻。
>
> 本文只考虑 Railway 集群，不采集腾讯云主机指标。

## 1. 设计结论

采用三层结构：

```text
xiaohuiji-api
xiaohuiji-worker
xiaohuiji-account
xiaohuiji-scheduler
        │
        │ OTLP/HTTP 或 OTLP/gRPC（Railway 私网）
        ▼
xiaohuiji-otel-collector
        │
        │ OTLP（Railway 私网）
        ▼
xiaohuiji-observe（OpenObserve，第一阶段单实例）
```

同时由 `xiaohuiji-otel-collector` 直接连接 Railway PostgreSQL，使用 SQL Query Receiver 采集业务状态快照；Redis/Asynq 和 TG 运行态由应用通过统一指标埋点上报。

### 为什么不是先上 Prometheus + Grafana + Loki + Tempo

这套组合通常需要维护多个服务、多个存储和多套保留策略。当前最重要的是今晚迁移期间能够快速定位积压和失败，不是先建设一套复杂的监控平台。因此第一阶段统一落到 OpenObserve：

- Logs：结构化日志、流程事件、错误堆栈。
- Traces：HTTP、数据库、媒体下载、TG RPC、异步任务链路。
- Metrics：服务资源、接口性能、队列、采集、资料发布、账号租约、TG 发送。
- SQL：大盘可以直接查询原始 telemetry，不需要先开发一套后台监控 CRUD。

后续如果 OpenObserve 需要拆出 Railway，只需要更换 Collector 的 exporter，不改业务服务的上报地址。

## 2. Railway 服务拓扑

### 2.1 第一阶段服务

| 服务 | 副本 | 作用 | 对外暴露 |
|---|---:|---|---|
| `xiaohuiji-api` | 2 起 | HTTP API、后台、支付回调、健康检查 | 业务域名 |
| `xiaohuiji-worker` | 1 起 | HotGo 队列、资料处理、TG 上架任务 | 否 |
| `xiaohuiji-account` | 1 起 | Telegram Bot、账号常驻监听、账号租约 | 否 |
| `xiaohuiji-scheduler` | 1 | Cron、发布调度、恢复调度 | 否 |
| `xiaohuiji-otel-collector` | 1 | OTLP 接收、批处理、限流、SQL 指标、转发 | 否，仅私网 |
| `xiaohuiji-observe` | 1 | OpenObserve UI 和数据存储 | 仅 UI 域名 |
| PostgreSQL | Railway 托管 | 业务数据库 | 否 |
| Redis | Railway 托管 | 缓存、HotGo Queue、Asynq、租约 | 否 |

`xiaohuiji-observe` 第一阶段只允许 1 个副本，并挂 Railway Volume。它是监控平面，不参与业务请求；即使监控服务重启，也不能影响 API、采集和推送。

### 2.2 私网地址

业务服务统一使用：

```text
OTEL_EXPORTER_OTLP_ENDPOINT=http://xiaohuiji-otel-collector.railway.internal:4318
```

Collector 使用：

```text
OPENOBSERVE_ENDPOINT=https://xiaohuiji-observe-production.up.railway.app
```

不同 Railway Environment 必须各部署一套 Collector；Railway 私网域名只在同一 Environment 内有效。

### 2.3 端口

| 服务 | 端口 | 说明 |
|---|---:|---|
| API | Railway 注入的 `PORT`，当前应用监听 8080 | 唯一业务公网入口 |
| Collector | 4317/4318 | OTLP gRPC/HTTP，私网 |
| OpenObserve | 5080 | UI/API，使用独立域名并加认证 |
| PProf | 不对公网暴露 | 仅临时私网或关闭 |

## 3. 统一上下文模型

当前项目已有 `trace_id`、`operation_no`、`collect_event_id`、`profile_id`、`channel_id`、`account_id` 等字段，但不同链路使用方式不完全一致。后续所有阶段事件统一包含以下字段：

### 3.1 必填字段

```json
{
  "event.name": "publish.stage",
  "event.type": "flow_stage",
  "flow.id": "publish:operation:...",
  "flow.kind": "collect|publish|full_push|quick_push|scheduled_push|account_collect|listener",
  "flow.stage": "tg.send",
  "flow.result": "started|success|failed|retry|skipped|superseded",
  "flow.attempt": 1,
  "trace_id": "...",
  "operation_no": "...",
  "service.name": "xiaohuiji-worker",
  "runtime.role": "worker",
  "railway.environment": "production",
  "railway.deployment_id": "..."
}
```

### 3.2 业务关联字段

只有存在时才写入，不能为了凑字段写 0：

```text
tenant_id
member_id
account_id
tg_account_id
bot_id
source_id
source_chat_id
source_message_id
collect_event_id
collect_dispatch_id
profile_id
profile_no
channel_id
target_chat_id
tg_job_id
message_id
batch_id
plan_id
```

### 3.3 性能字段

```text
queue_wait_ms
processing_ms
download_ms
upload_ms
tg_rpc_ms
db_ms
redis_ms
bytes
media_count
retry_after_seconds
http.status_code
error.type
error.message
telegram.method
telegram.flood_wait_seconds
```

**重要：** `profile_id`、`channel_id`、`target_chat_id` 等高基数字段放在日志和 Trace 中，不作为 Metrics label。Metrics 只使用有限枚举，例如 `role`、`stage`、`result`、`queue`、`media_type`、`error_type`。

## 4. 完整业务链路

### 4.1 资料采集总链路

```text
Telegram Update / Bot 消息 / 账号监听
  → 账号或 Bot 租约获取
  → 原始消息解析
  → collect_event 写入
  → 媒体组等待窗口
  → 文本/来源/编号解析
  → 展示资料与验证资料配对
  → 去重/相似资料判断
  → collect_event_media 下载或转发
  → 媒体缓存 ready
  → 资料 profile/media 写入
  → 资料索引更新
  → 采集 dispatch 生成
  → 绑定默认/采集来源频道
  → 生成 TG publish job
```

必须观测的阶段：

- `collect.receive`：收到 Telegram 消息到写入事件的延迟。
- `collect.group_wait`：媒体组等待数量、等待年龄、超时数量。
- `collect.classify`：展示/验证/未知资料分类。
- `collect.dedupe`：重复、相似、误判和跳过数量。
- `collect.media_download`：下载中、成功、失败、重试、耗时和大小。
- `collect.material_commit`：资料、媒体、来源和索引落库耗时。
- `collect.dispatch`：采集事件进入后续发布流程的延迟。
- `collect.stuck`：事件长时间停留在某一状态。

### 4.2 Bot 采集链路

```text
Bot Update
  → Bot Handler
  → Source/Chat 识别
  → collect_event
  → collect_process_queue
  → media cache
  → material commit
```

重点查看：Bot 是否持续收到 Update、对应 `source_id` 是否有活跃任务、Redis 队列是否消费、事件是否卡在 `group_collect` 或 `media_pending`。

### 4.3 账号采集链路

```text
Telegram Account Session
  → account client lease
  → listener subscription
  → Update 接收
  → listener plan/target 匹配
  → collect_event
  → 后续进入采集总链路
```

重点查看：

- 账号授权状态、最近心跳、最近 Update 时间。
- 租约持有者、租约剩余时间、看门狗续期次数。
- 重连次数、授权失效、FloodWait、网络错误。
- 同一个账号是否被多个 `account` 副本重复消费。
- 监听器有无计划但没有收到消息。

### 4.4 全量推送链路

```text
API 全量推送请求
  → 创建 batch/operation
  → 读取资料分页
  → 读取频道映射
  → 展开 profile × channel
  → 去重或 superseded 判断
  → 生成 tg_job
  → Redis/Asynq 入队
  → worker 领取
  → channel/account 串行窗口
  → 媒体准备
  → Telegram SendMedia/SendMessage
  → tg_message 持久化
  → publish_success_record
  → profile publish state projection
  → batch 完成
```

必须能直接看到：

- 本次批次计划数、展开数、实际生成数、跳过数、superseded 数。
- 各频道任务数及最老任务年龄。
- `pending → queued → processing → sending → sent/failed_retry/failed` 各状态数量。
- 任务是否卡在数据库未入队，还是 Redis 未消费，还是 TG 限流。
- 单频道间隔导致的预计完成时间。

### 4.5 普通资料上架/下架链路

```text
资料编辑或状态变更
  → 解析发布频道映射
  → 使用手动映射；无手动映射时使用默认频道
  → 创建发布 operation
  → 生成频道任务
  → TG 推送/删除
  → 状态投影
```

重点检查“为什么推送到不应推送的频道”：同一个 `profile_id`、`tenant_id`、`channel_id` 的映射快照、映射来源和 operation_no 必须在事件中记录。

### 4.6 快速推送链路

```text
API 创建/执行快速推送计划
  → 计划状态与执行时间判断
  → target chat 解析
  → 消息模板序列化
  → urgent/bulk 队列选择
  → Inline / source copy / source forward / account send
  → media group 发送
  → message_push_sender 记录结果
```

重点查看：

- 计划是否到达执行时间，使用的时区和下一次运行时间。
- 目标频道解析数量是否为 0。
- 计划任务是否进入 urgent 队列。
- 是否逐条发送而不是媒体组发送。
- 账号是否忙、FloodWait、目标频道权限失败。

### 4.7 Telegram 下载、上传和发送链路

统一分为三段，不允许只记录一个总耗时：

```text
TG 下载/缓存读取
  → 本地临时文件或 COS 文件
  → TG 上传 file/media
  → SendMedia/SendMessage
  → 消息 ID 与发送记录落库
```

每一段记录：媒体类型、大小、来源、缓存命中、耗时、Telegram 方法、错误分类、重试等待。

错误分类统一为：

```text
FILE_REFERENCE_EXPIRED
FILE_REFERENCE_INVALID
FLOOD_WAIT
TIMEOUT
NETWORK
AUTH
PERMISSION
NOT_FOUND
DISK
DATABASE
UNKNOWN
```

## 5. Telemetry 采集方式

### 5.1 Trace

保留当前 GoFrame/gtrace 链路，统一切换为通用 OTLP 配置：

- 服务名由 `RAILWAY_SERVICE_NAME` 或显式环境变量生成。
- 资源属性包含角色、环境、部署 ID、Git SHA。
- 正常请求采样 10%～20%。
- 错误、慢请求、关键 TG 失败由 Collector tail sampling 保留 100%。
- 异步任务使用 `span link` 关联父请求，不把一个跨分钟任务保持为长 HTTP span。

首批 Trace：

- HTTP 请求和响应状态。
- PostgreSQL 查询耗时和错误。
- Redis/队列操作耗时和错误。
- 采集事件处理。
- 媒体下载、缓存、上传。
- Telegram RPC。
- TG 任务领取、重试和最终状态。

### 5.2 Logs

Railway 没有原生 stdout log drain，因此不能假设 Railway 日志会自动进入 OpenObserve。使用现有 `glog.SetDefaultHandler(LoggingServeLogHandler)` 作为唯一接入点，新增非阻塞 OTLP Log Handler：

- 继续写 Railway stdout，保留 Railway 原生日志。
- 同步向 Collector 批量发送结构化 JSON 日志。
- 上报失败不得阻塞业务，不得递归写日志。
- `access`、`cron`、`queue`、`tg`、`collect` 日志保留 `trace_id` 和业务关联字段。
- 上传媒体、请求体、session、token、支付密钥等敏感内容禁止进入 telemetry。

### 5.3 Metrics

Metrics 分为三类：

#### 服务指标

```text
xiaohuiji_http_requests_total
xiaohuiji_http_request_duration_ms
xiaohuiji_http_errors_total
xiaohuiji_process_heartbeat
xiaohuiji_runtime_role_up
xiaohuiji_goroutines
xiaohuiji_db_query_duration_ms
xiaohuiji_redis_command_duration_ms
```

#### 业务指标

```text
xiaohuiji_collect_events_total
xiaohuiji_collect_events_by_stage
xiaohuiji_collect_event_age_seconds
xiaohuiji_collect_media_pending
xiaohuiji_collect_media_download_duration_ms
xiaohuiji_collect_dedupe_total
xiaohuiji_collect_material_commit_total
xiaohuiji_publish_batch_total
xiaohuiji_publish_jobs_total
xiaohuiji_publish_job_age_seconds
xiaohuiji_publish_job_duration_ms
xiaohuiji_publish_success_total
xiaohuiji_publish_failure_total
xiaohuiji_publish_retry_total
xiaohuiji_publish_superseded_total
xiaohuiji_quick_push_total
xiaohuiji_quick_push_duration_ms
xiaohuiji_quick_push_failure_total
```

#### Telegram/账号指标

```text
xiaohuiji_tg_account_online
xiaohuiji_tg_account_lease_owned
xiaohuiji_tg_account_lease_waiting
xiaohuiji_tg_account_reconnect_total
xiaohuiji_tg_update_received_total
xiaohuiji_tg_download_bytes_total
xiaohuiji_tg_download_duration_ms
xiaohuiji_tg_upload_duration_ms
xiaohuiji_tg_send_duration_ms
xiaohuiji_tg_send_total
xiaohuiji_tg_send_failure_total
xiaohuiji_tg_flood_wait_seconds
xiaohuiji_tg_media_group_waiting
```

### 5.4 SQL Query Receiver

Collector 直接连接 Railway PostgreSQL，按 15～60 秒执行只读聚合 SQL，避免额外维护 `xiaohuiji-observer` 服务。采集内容包括：

- 采集事件按状态、来源、账号的数量和最老时间。
- 采集媒体按缓存状态、媒体类型的数量和最老时间。
- 采集 dispatch 按状态的数量和最老时间。
- TG job 按队列、优先级、状态的数量和最老时间。
- 每频道 Top 20 积压及预计完成时间。
- 快速推送计划待执行、执行中、失败数量。
- 账号在线状态、最后心跳、租约超时。
- Scheduler 最近心跳和最近调度时间。
- 失败错误分类 Top N。
- PostgreSQL checkpoint 次数、写入/同步耗时和后台刷盘缓冲区。
- PostgreSQL WAL records、FPI 和字节数。
- 当前数据库提交、回滚、临时文件、临时字节和死锁累计值。
- Top 50 高频写入表、更新次数、删除次数和死元组数量。

SQL 必须：

- 只读账号。
- 所有查询带时间窗口或状态索引。
- 单个查询超时不超过 3 秒。
- 不执行全表 `DELETE/UPDATE`。
- 大盘明细通过日志/Trace 下钻，不把每个频道作为高基数 Metric。

## 6. OpenObserve 大盘

### 6.1 总览：迁移指挥盘

顶部只放 12 个核心指标：

1. 四个角色在线/就绪实例数。
2. API 错误率、P95、P99。
3. Worker 心跳和队列总积压。
4. TG job 最老任务年龄。
5. 采集事件最老年龄。
6. 媒体下载失败率。
7. TG 发送成功率。
8. FloodWait 总等待秒数。
9. 在线账号/总账号。
10. Redis 内存、连接和队列错误。
11. PostgreSQL 连接、慢查询和锁等待。
12. Telemetry 最近到达时间。

### 6.2 发布大盘

分为普通上架、下架、全量推送、周期调度：

- 批次趋势：创建数、完成数、失败数、部分失败数。
- 状态漏斗：生成、入队、领取、发送、成功、重试、失败。
- 队列年龄分布：1/5/10/30/60 分钟以上。
- 频道 Top 20：pending、sending、retry、oldest、last_sent、预计剩余时间。
- 账号 Top 20：发送量、失败量、FloodWait、最近活跃时间。
- Operation timeline：输入 operation_no 后展示完整阶段时间线。

### 6.3 采集大盘

- Bot 采集与账号采集分别统计。
- 每个来源：最近消息时间、事件数、处理速率、最老事件。
- 状态漏斗：received、group_collect、prechecked、media_pending、media_ready、committed、ignored、failed。
- 媒体缓存：pending/downloading/ready/failed。
- 去重：重复、相似、来源已删除后重采集等结果。
- 验证资料配对：等待中、成功、未匹配、超时。
- 账号租约：owner、等待、过期、续期失败。

### 6.4 快速推送大盘

- 计划运行时间与实际开始时间偏差。
- 目标频道解析数量。
- urgent/bulk 队列积压。
- Inline、复制原消息、来源转发、账号直发的占比。
- 媒体组发送耗时和失败原因。
- “计划已到时间但没有任务生成”作为单独告警查询。

### 6.5 Telegram 媒体大盘

- 下载/上传/发送三段耗时 P50/P95/P99。
- 按媒体类型、大小区间、账号、错误类型统计。
- `FILE_REFERENCE_EXPIRED`、`FLOOD_WAIT`、`TIMEOUT`、`AUTH`、`PERMISSION` Top N。
- 大文件慢任务和重复下载。
- 媒体组等待 3 分钟内完成率。

### 6.6 数据库和 Redis 大盘

PostgreSQL：

- 活跃连接、空闲连接、连接等待。
- 慢查询 Top 20。
- 锁等待、长事务、死锁。
- 查询失败率和事务回滚。
- 关键业务表增长量。
- `xiaohuiji.postgres.checkpoint_write_time_ms_total` 与 `xiaohuiji.postgres.checkpoint_sync_time_ms_total` 持续增长时，表示刷盘压力上升。
- `xiaohuiji.postgres.wal_bytes_total` 增长过快时，结合 `xiaohuiji.postgres.table_updates_total` 定位高频写表。
- `xiaohuiji.postgres.table_dead_tuples` 持续增长时，检查 autovacuum 和重复更新。

Redis：

- 内存使用和 eviction。
- 命令延迟、连接数、blocked clients。
- HotGo Queue、Asynq、租约相关 key 数量。
- 队列入队速率与消费速率。
- Redis 错误和超时。

## 7. 告警规则

### P0：立即 TG 告警

- API 全部实例 `readyz` 失败超过 1 分钟。
- PostgreSQL 或 Redis 连续不可用超过 1 分钟。
- Worker 全部无心跳超过 2 分钟。
- Scheduler 无心跳超过 2 个调度周期。
- 有待发送任务但 5 分钟内全局没有成功发送。
- 账号全部离线且存在待发送任务。

### P1：聚合后 TG 告警

- 任一频道 pending 超过阈值且 oldest 超过 10 分钟。
- 任一频道 5 分钟内无发送，但 pending 持续增加。
- 发布失败率超过 10%，持续 5 分钟。
- 采集事件在同一 stage 停留超过 10 分钟。
- 媒体下载失败率超过 15%。
- 快速推送计划到期超过 2 分钟仍未生成任务。
- 账号租约等待超过 2 分钟或租约续期失败。
- FloodWait 告警按账号/频道聚合，避免每条错误刷屏。

### P2：趋势告警

- API P95、TG 发送 P95、媒体下载 P95连续 15 分钟上涨。
- PostgreSQL 连接使用率超过 80%。
- Redis 内存超过 80% 或出现 eviction。
- Telemetry 断流超过 2 分钟。
- 单频道产生任务速率显著超过发送速率。

告警必须使用去重键：

```text
rule + environment + role + tenant_id + channel_id
```

同一问题 5 分钟内只发一条，恢复时发送恢复通知。

## 8. 数据排查方式

后续排查不再只看一条错误日志，而是按以下优先级：

1. `trace_id`：定位单次 HTTP 或异步入口。
2. `operation_no/batch_id`：定位一次全量、上架或快速推送。
3. `collect_event_id`：定位一条采集消息从接收到资料落库的全链路。
4. `profile_no/profile_id`：定位资料处理与所有频道映射。
5. `tg_job_id + channel_id`：定位具体频道发送卡点。
6. `tg_account_id`：定位账号在线、租约和 Telegram RPC。

预置查询模板：

- “为什么资料 X 没有上架”：profile → mapping → operation → tg jobs → send logs。
- “为什么频道 Y 不推送”：channel → pending/queued/sending → last success → account/lease → error。
- “为什么采集后资料错乱”：source message → collect event → media group → classifier → commit。
- “为什么快速推送没反应”：plan → due time → target resolution → job creation → urgent queue → send。
- “为什么账号在线但采集不到”：account session → lease → listener heartbeat → update received → source match。
- “为什么全部重新上架”：operation generation → mapping source → superseded decision → publish state projection。

## 9. 迁移期间的安全原则

- 监控服务不参与业务事务，不写业务表。
- Collector 所用数据库账号必须只读。
- 监控服务单独使用 OpenObserve Volume；不与业务上传目录混用。
- `readyz` 只作为服务健康检查，不代表 TG 账号或队列健康；业务健康由业务 Metrics/告警判断。
- PProf 不对公网开放。
- 生产 Trace 不使用 `AlwaysSample`；使用可配置采样，错误和慢请求保留。
- 所有异步队列必须有 `started/heartbeat/success/retry/failed` 事件，不能只在最终失败时记录。
- 不把 session、token、手机号、支付密钥、完整消息正文和媒体内容发到 OpenObserve。
- Collector 发送失败时业务继续运行，Railway 原生日志继续保留。

## 10. 分阶段实施

### 阶段 A：今晚迁移前

1. 在 Railway 新增 `xiaohuiji-otel-collector`。
2. 在 Railway 新增 `xiaohuiji-observe`，单实例 + Volume，保留 3～7 天。
3. 四个业务服务统一设置 OTLP endpoint、服务名、角色、环境和 Git SHA。
4. 将当前 `InitTencentAPM` 改为通用 OTLP 配置，关闭生产 `AlwaysSample`。
5. Collector 配置 OTLP receiver、batch、memory limiter、tail sampling、OpenObserve exporter。
6. 配置 SQL Query Receiver，先采集 TG job、采集 event、账号、调度心跳四类核心指标。
7. 建立总览、发布、采集、账号、TG 媒体四个大盘。
8. 配置 P0/P1 告警和 Telegram Webhook。

### 阶段 B：迁移完成后

1. 在 HTTP 入口统一生成 `flow.id` 并写入 Context/Baggage。
2. 在采集、全量推送、快速推送和 Scheduler 边界增加阶段事件。
3. 在 TG 下载、上传、RPC、租约和重试处增加 Span/Metric。
4. 增加 Asynq/HotGo Queue 消费速率和延迟指标。
5. 预置资料、频道、批次和账号下钻查询。
6. 通过压测校准告警阈值，不直接使用固定经验值。

### 阶段 C：稳定运行后

1. OpenObserve 数据迁出 Railway Volume，改用 COS/S3-compatible 对象存储。
2. OpenObserve 迁移到独立低价 VPS 或托管实例；业务只保留 Collector 私网/公网出口配置。
3. Collector 按业务与基础设施拆分，只有在吞吐量确实不足时扩容。
4. 需要更强数据库分析时，再增加 PostgreSQL metrics receiver 或独立 PG 监控，不提前引入 Kubernetes。

## 11. 验收标准

迁移后必须能够在 5 分钟内回答：

- API 是否正常，哪个角色不健康。
- 任务是没有生成、没有入队、没有领取、没有发送还是 TG 返回错误。
- 哪个频道、哪个账号造成积压。
- 采集事件卡在分组、去重、下载、配对还是落库。
- 快速推送是否到期、是否生成目标、是否进入 urgent 队列。
- TG 下载、上传、发送分别耗时多少。
- PostgreSQL/Redis 是否是瓶颈。
- 最近一次部署后哪个服务或版本开始恶化。

如果做不到以上查询，不能认为可观测性接入完成。

## 12. Railway 应用前配置

GitHub Actions 会同时构建：

```text
ghcr.io/<owner>/youban-server:sha-xxxxxxx
ghcr.io/<owner>/xiaohuiji-otel-collector:sha-xxxxxxx
```

执行 Railway IaC 前设置两个不可变镜像：

```bash
export YOUBAN_RAILWAY_IMAGE=ghcr.io/<owner>/youban-server:sha-xxxxxxx
export XIAOHUIJI_OTEL_COLLECTOR_IMAGE=ghcr.io/<owner>/xiaohuiji-otel-collector:sha-xxxxxxx
npx -y @railway/cli@latest config plan
npx -y @railway/cli@latest config apply
```

首次应用后，需要在 Railway 设置以下 `preserve()` 变量：

```text
xiaohuiji-observe.ZO_ROOT_USER_EMAIL
xiaohuiji-observe.ZO_ROOT_USER_PASSWORD
xiaohuiji-otel-collector.OPENOBSERVE_AUTHORIZATION
```

`OPENOBSERVE_AUTHORIZATION` 使用 `Basic base64(email:password)`。完成后重新部署 Collector，并验证 `/healthz`、Collector `:13133/`、OpenObserve OTLP 数据流和 Telegram 告警恢复通知。

## HTTP 接口性能大盘

HTTP 性能大盘定义保存在 `deploy/observability/dashboards/http-performance.json`，导入后的大盘名称为“`小灰机 HTTP API 性能`”。

应用部署后，OpenObserve 会出现以下指标流：

- `xiaohuiji_http_server_duration_bucket`：接口耗时直方图桶，用于 P95/P99。
- `xiaohuiji_http_server_duration_sum` / `xiaohuiji_http_server_duration_count`：用于平均耗时。
- `xiaohuiji_http_server_requests`：按路由、方法、HTTP 状态码和业务 `code` 统计请求量。

大盘按“流量与可用性”“接口延迟”“业务响应”三个中文 Tab 拆分。查看接口耗时排行时，打开“接口延迟”，重点关注
“接口延迟 P50 / P95 / P99”和“接口平均耗时排行”。如果需要临时查询，可使用：

```promql
histogram_quantile(0.95, sum by (le, http_route) (rate(xiaohuiji_http_server_duration_bucket[5m])))
```

当前指标按 `http_route` 聚合，不使用原始 URL 作为标签，避免资料编号、频道 ID 等高基数路径污染监控。
统一 JSON 响应中的 `code` 会作为 `http_business_code` 标签写入 HTTP 指标和访问日志；打开“业务响应”中的
“业务成功率”“业务失败率”“业务响应码请求数（最近 5 分钟）”与“业务错误码 × 路由排行”，即可区分 HTTP 200
但业务失败的请求。业务响应码使用表格展示最近 5 分钟请求数，避免将每秒速率误读成业务码。非 JSON、流式和超过 128 KB 的响应不解析业务码，
避免下载接口和大响应增加额外开销。

## Telegram 运营大盘

TG 运营大盘定义保存在 `deploy/observability/dashboards/tg-operations.json`，导入后名称为
`xiaohuiji Telegram operations`，按 Tab 分为“异常总览”和五组详情：

1. **异常总览**：最近 15 分钟发送错误、TG 发送延迟分布、媒体下载错误/超时、频道重试 Top 20、频道积压 Top 20、无消费者任务。
2. **TG 账号租约**：活动租约数、租约事件速率、账号状态。
3. **网关 Bot 与 Webhook**：Bot configured/running 数量、Webhook 接收结果、更新分发结果。
4. **采集事件**：未处理事件数量、最老事件年龄、长期无事件采集源。
5. **媒体下载与验证资料**：媒体缓存积压、PHash/验证资料处理队列、文件引用过期和视频预览图缺失。
6. **频道推送与 TG 积压**：TG Job、Asynq 队列、频道积压 Top 20、消费者和无消费者任务。

异常总览中的错误类型已经做了低基数归类：`rate_limit`、`timeout`、`authorization`、`permission`、`media`、`other`。
发送延迟按 `<5s`、`5-30s`、`30-120s`、`>=120s` 分桶；出现 `30-120s` 或 `>=120s` 时，优先检查
频道积压、重试 Top 和消费者面板。媒体错误中的 `timeout` 表示下载超时，`file_reference_expired` 表示 Telegram
文件引用过期，需要重新获取文件引用或触发媒体重试。

导入后建议先将时间范围设为最近 1 小时。SQL 快照指标每 15 秒刷新一次，Asynq 和运行时指标按各自
采集周期更新；如果某个面板为空，应先检查对应指标是否已经进入 OpenObserve，而不是直接判断业务链路失败。

异常总览第一行固定显示“监控数据是否正常上报”和“展示与验证资料混组拦截”。心跳全部为 0 表示采集链路异常，
不是业务没有问题。面板查询统一使用 `or vector(0)`，没有异常时显示 0，避免空白图表造成误判。

Railway 私网 DNS 无法解析 Observe 服务名时，Collector 使用公共 OTLP 地址
`https://xiaohuiji-observe-production.up.railway.app/api/default`。该地址仍通过
`OPENOBSERVE_AUTHORIZATION` 鉴权，恢复后应检查 Collector 日志中不存在 `Exporting failed`。

常用排查顺序：

1. 先看“TG Worker 与消费者”，确认有 worker 且没有无消费者任务。
2. 再看“TG Job 积压”和“频道积压 Top 20”，确认任务是否停在数据库或 Redis。
3. 采集问题查看“采集事件”和“媒体下载与验证资料”，区分事件未入队、媒体未 ready、PHash 未完成。
4. Bot 问题查看“网关 Bot 与 Webhook”，区分 Webhook 未收到、入队失败和分发未完成。
5. 账号问题查看“TG 账号租约”，重点关注 `conflict`、`lock_failed`、`expired` 事件。
