# 采集入库耗时与积压排查

## 统计口径

一条 `collect_profile_committed` 日志表示一次顶层事务确认的新建资料。
起点是该展示消息组已保留的可信本地接收时刻，终点是资料事务成功返回。
同一资料复用、事务回滚和未确认提交不进入新建耗时统计。
通过 `time_basis` 区分起点，不把不同起点的分位数混算：

- `webhook_received`：Webhook 处理入口，时间随原有磁盘队列任务保留。
- `gateway_ingress`：Gateway 原有 `EnqueuedAt`。新 Webhook 任务会从处理入口采样；
  polling 与旧任务仍是原来的入队采样，故保守标记为网关入口，不能全称 HTTP 接收。
- `collector_ingress`：只有 Collector 入口时钟的协议账号、旧路由等样本。

Gateway 复用既有 wire 字段和队列选项，不新增 Asynq Unique 的观测字段；
磁盘队列新增可选接收时间，老任务缺失该值时不会伪造。
历史消息在 Telegram 上的发布时间不等于本地采集接收时间。

生命周期 Metrics 使用有限的 `action/source_type/collection_mode/time_basis` 标签；来源、频道、
Bot、账号、事件、资料 ID 仅放日志和 Span。不得添加这些 ID 到 Histogram 标签。
不为埋点增加数据库查询、Redis 扫描或独立 exporter。

## 可直接使用的 Metrics

全局或按 Bot/协议号、实时/历史筛选的平均入库耗时：

```promql
sum by (source_type, collection_mode, time_basis) (
  rate(xiaohuiji_collect_lifecycle_duration_seconds_sum[5m])
)
/
(
  sum by (source_type, collection_mode, time_basis) (
    rate(xiaohuiji_collect_lifecycle_duration_seconds_count[5m])
  ) > 0
)
```

匹配的三个边界独立展示，不能把嵌套耗时相加：

- `material_pair`：展示组与验证组配对的本次执行；
- `rule_match`：本次规则匹配；
- `content_extract`：本次资料文本内容提取。

平均值：

```promql
sum by (stage) (
  rate(xiaohuiji_collect_pipeline_duration_seconds_sum{
    stage=~"material_pair|rule_match|content_extract"
  }[5m])
)
/
(
  sum by (stage) (
    rate(xiaohuiji_collect_pipeline_duration_seconds_count{
      stage=~"material_pair|rule_match|content_extract"
    }[5m])
  ) > 0
)
```

P95 使用相同过滤器的 bucket 做 `histogram_quantile(0.95,sum by(le,stage)(rate(...[5m])))`。
重试各记一次阶段执行；阶段执行不包含先前排队和等待验证组的全部驻留时间。
新建资料端到端包含这些等待，因此二者不能互相替代。

生命周期没有有效样本时保留“无可信时钟样本”；同时展示
`xiaohuiji_collect_lifecycle_missing_total` 的 `reason`，禁止补 0。
旧任务不会凭空补回接收时间。新增长耗时 bucket 部署后才生效，
SDK View 若覆盖边界，须核对实际导出的 `le`；滚动更新期间避免比较不同边界实例。

## 入库明细与 TOP10

结构化事件：`log_type=telegram_collect_pipeline`、
`event=collect_profile_committed`。所有新建提交均记录，不依赖 Trace 采样，
快速成功也不会被阶段日志的 100ms 阈值过滤。

字段：

| 字段 | 含义 |
|---|---|
| `lifecycleSample` | valid / missing_received_at / future_received_at |
| `timeBasis` | webhook_received / gateway_ingress / collector_ingress 等受控本地起点 |
| `receivedAtUnixMs` / `committedAtUnixMs` | 本地接收及事务确认时间 |
| `lifecycleSeconds` / `lifecycleMs` | 仅 valid 样本提供，不以零代替缺失 |
| `slow` | 有效耗时达到 600 秒；慢成功不意味着业务失败 |
| `sourceId` / `sourceChatId` / `canonicalChatId` | 来源与频道关联 |
| `collectEventId` / `upstreamEventId` / `deliveryId` | 两套事件 ID 与交付记录 |
| `profileId` / `ruleId` / `traceId` | 资料、规则与 Trace |
| `sourceType` / `collectionMode` | Bot/协议号及实时/历史 |

以下 SQL 适用于字段已建立索引、提取为同名顶层字段的日志存储。
如采集器将 `g.Map` 保留为 JSON 字符串，应在日志接入侧解析该结构，
不能把未解析字段的空结果解释为无慢任务。

最慢的 10 次入库：

```sql
event:collect_profile_committed AND lifecycleSample:valid
| SELECT sourceId, sourceChatId, collectEventId, profileId,
         sourceType, collectionMode, timeBasis, lifecycleSeconds, traceId
  ORDER BY CAST(lifecycleSeconds AS double) DESC
  LIMIT 10
```

按采集源平均耗时排名（每组均附样本数）：

```sql
event:collect_profile_committed AND lifecycleSample:valid
| SELECT sourceId, sourceType, collectionMode, timeBasis,
         COUNT(*) AS created_samples,
         AVG(CAST(lifecycleSeconds AS double)) AS avg_seconds,
         MAX(CAST(lifecycleSeconds AS double)) AS max_seconds
  GROUP BY sourceId, sourceType, collectionMode, timeBasis
  ORDER BY avg_seconds DESC
  LIMIT 10
```

应在日志查询界面限制时间，例如最近 1 小时或最近 24 小时。
不要为 TOP10 查询生产资料大表。

以上排行仅统计**已经完成入库**的耗时，不能发现完全没有完成的任务。
未完成积压需结合媒体 pending 数量、最老年龄、队列等待、
`collect_stage_completed` 的 `stage/result/waitReason` 和恢复日志。
区分三种现象：等待原因计数、真正执行超时、耗时很长但最终入库成功。

阶段日志在有可信本地接收时间时同时提供 `ingressAgeSeconds` 和
`ageTimeBasis`（与上述实际起点一致）。它与 `durationMs` 不同：前者含历次排队/退避，
后者只是当前执行阶段耗时。可先按 `collectEventId/sourceId` 取每条链路最近
一条阶段结果，再筛 `pending/retryable` 排序；不能把同一任务的历史 pending
日志全部当作当前积压，已完成后的旧 pending 记录也必须排除。
完全没有被消费者执行的任务仍需队列库存与最老年龄观测，阶段日志无法覆盖。

Grafana 当前仅接入 VictoriaMetrics 时，可以直接显示类型均值、分位数、
超时阶段 TOP10；具体资料/采集源的明细 TOP10 需要接入日志数据源后查询上述事件，
不能通过向 Metrics 添加高基数 ID 标签来替代。

## 配置数量快照的覆盖范围

`telegram_collector_sources_loaded` 和
`telegram_collector_message_entries_loaded` 复用现有 account runtime provider
的加载结果，分别统计当前实例加载的有效来源、至少挂一个来源的协议账号。
固定 `coverage=instance_loaded`、`entry_type=protocol_account`。
看板用 `telegram_collector_inventory_snapshot_valid == 1` 按实例关联过滤；
加载失败、上下文结束、超过 90 秒未刷新时不展示旧数量。

它们不是全局启用来源数量、不是在线连接数量，也不包含完整 Bot 入口清单。
各实例的配置可能重叠，不能通过 sum 或 max 将它们包装成全局总数。
在“不为监控额外查库”的约束下，全局精确数需要配置提供方暴露已有的完整配置快照，
再在同一快照边界上报；不能使用最近有消息的来源数量冒充全部已启用来源。
