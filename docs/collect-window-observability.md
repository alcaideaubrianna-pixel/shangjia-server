# 采集窗口查询与目标批次复用观测

本文约定 `collect_window_query_completed` 和 `collect_target_batch_reuse` 的统计口径，并给出 CLS 查询模板。**模板未执行，未核实任何生产主题索引，也不代表字段已经部署或已被解析。**执行前先确认镜像、角色、实际日志样本、字段位置和索引。

日志载荷与字段提取遵循 [频道队列 CLS 手册](publish-queue-cls.md)，环境选择遵循 [生产排障入口](production-debugging.md)。本页不包含生产写操作、队列恢复或日志索引变更。

字段已按本工作区 `collect_window_observability.go` 及调用点静态核对。两类事件的 `log_type` 均为 `telegram_collect_pipeline`。没有新增 Redis 统计计数器；以下统计直接聚合每次执行输出的日志，不读取 Redis 计数值。

## 事件与字段口径

### 窗口查询完成：collect_window_query_completed

每次实际执行 primary 或 retry 窗口查询输出一条完成事件；查询失败同样记录。未执行查询的路径不能伪造查询完成事件。事件量表示查询调用量，不表示去重后的采集事件量。

| 字段 | 类型/取值 | 口径 |
|---|---|---|
| event | string | `collect_window_query_completed` |
| stage | string | `collect.window.primary` 或 `collect.window.retry` |
| queryKind | string | `primary` 或 `retry`，与 stage 一一对应 |
| durationMs | integer | 本次独立查询的耗时毫秒；整数精度不足以表达亚毫秒，不把 0 当成未执行 |
| durationUs | integer | 同一次查询的独立耗时微秒，用于亚毫秒分析；不是追加耗时 |
| rows | integer | 本次查询实际返回行数；失败时不把部分结果当成完整窗口 |
| emptyQueryCount | integer | 成功查询且返回 0 行时为 1，其余为 0；错误不能记为空结果 |
| queryCount | integer | 每条实际查询完成事件恒为 1 |
| queryErrorCount | integer | 查询失败为 1，成功为 0 |
| queryOutcome | string | `success` 或 `error` |
| rowLimit | integer | 本次查询的探测上限，当前为 maxRows+1 |
| queryLimitReached | boolean | 返回行数超过 maxRows；与合并窗口的 windowTruncated 分开解释 |
| tenantId / accountId | integer | 队列 payload 的租户及账户 scope；accountId 不是事件原始归属字段 |
| eventAccountId | integer | 事件记录的 account_id；Bot 事件可为 0，不应与队列 accountId 混为一列 |
| sourceId | integer | 当前采集来源，不按同频道其他来源替换 |
| sourceChatId | string | 源 Telegram Chat ID，保留日志实际格式，不能与内部频道主键混用 |
| canonicalChatId | string | 规范化 Chat ID，用于合并同频道短/长号统计；仍须保留 tenant/account/source 作用域 |
| eventId / collectEventId | integer | 当前采集事件 ID，两者同值，不重复计数 |
| anchorMessageId | integer | 目标事件的 source_message_id |
| traceId | string | 执行链路标识；实际载荷若为 traceID，需映射大小写 |

计数约束：`queryCount=1`，`emptyQueryCount`、`queryErrorCount` 均为 0/1，二者不能同时为 1。重试查询是一笔新的实际查询：同一 eventId 可以有多条 primary/retry 日志，不用 `count(distinct eventId)` 代替查询次数。

错误率为 `sum(queryErrorCount) / sum(queryCount)`。成功查询空结果率为 `sum(emptyQueryCount) / (sum(queryCount) - sum(queryErrorCount))`，成功次数为 0 时返回 NULL。若要展示“全部尝试中的空查询占比”，分母可用全部查询次数，但必须使用不同名称。

查询计时只包围一次 ORM 调用往返，包含连接等待、网络往返和结果解码，**不是 PostgreSQL 服务端纯执行时间**。不包含前置查询、SQL 构造、缓存操作或本条完成日志输出；ORM 内部调试开销仍属于调用耗时。定位 PG 执行耗时仍需独立数据库证据。

### 批次复用：collect_target_batch_reuse

每次目标窗口处理在退出时输出一条 batch 终态日志，携带复用决策及本次执行结果；不额外输出一条用于累计的 Redis 计数日志。这里的“终态”指本次调用结束，不能解释为采集事件已永久完成。是否命中、是否实际评估、是否进入 run 需要区分：

| 字段 | 类型/取值 | 口径 |
|---|---|---|
| event | string | `collect_target_batch_reuse` |
| stage | string | `collect.window.batch_reuse` |
| reuseOutcome | string | `hit`、`miss`、`not_evaluated` |
| reuseReason | string | 复用或未评估的具体原因；枚举以实现和实际日志为准，不在查询中假定未发布的原因值 |
| reuseLookupCount | integer | hit/miss 时为 1，not_evaluated 时为 0 |
| reuseHitCount | integer | 仅 hit 为 1，其余为 0 |
| reuseMissCount | integer | 仅 miss 为 1，其余为 0 |
| rowsLoaded | integer | 仅 batchRunStarted=true 时表示合并后的窗口行数；不是新查询行数，也不是该批次新插入行数 |
| windowTruncated | boolean | 仅 batchRunStarted=true 时有效，表示窗口受到上限截断；不能凭一次截断窗口宣称完整覆盖历史 |
| batchGeneration | string | 批次代次标识，用于同作用域关联；不假定是全局唯一或连续整数 |
| coveredEventCount | integer | 已读取的缓存完成记录中 CoveredEventIDs 的去重数量；不是本轮新覆盖数量，也不是 TG 发布成功数量 |
| batchRunStarted | boolean | 是否进入本次批次 run；miss 后若执行前失去租约，可以为 false |
| batchResult | string | `completed`、`deferred`、`failed`；描述返回结果，不表示 Telegram 已发送 |
| durationMs | integer | 本次复用决策/操作记录的耗时毫秒，不是窗口 SQL 的独立耗时 |
| durationUs | integer | 与 batch durationMs 同一完整处理边界的微秒精度，二者不相加 |

batch 事件同时携带前表的身份字段，包括 canonicalChatId、eventAccountId、accountId 和 traceId。batch duration 从目标窗口处理入口计到退出，可能包含协调、查询及业务处理，不能当成纯 Redis lookup 延迟。

**复用命中率 = hit / (hit + miss)**，等价于 `sum(reuseHitCount) / sum(reuseLookupCount)`，前提是计数约束成立。租约忙、Redis 失败等未评估路径为 `not_evaluated`，不能归为 miss，也不能进入命中率分母。它们必须另报次数、占比及原因，避免命中率掩盖不可用。

上述 not_evaluated 指**形成 hit/miss 决策之前**发生的租约忙或 Redis 错误。已经形成 miss 后，即使续租/后续缓存写入失败，reuseOutcome 仍保留 miss，结合 batchRunStarted、batchResult 和 errorMessage 判断停点，不能把所有 Redis 错误重新归类。

**miss 只证明复用条件未满足，不保证执行过 SQL。**run 前租约丢失时可以是 miss、batchRunStarted=false；run 已开始也不能替代实际查询完成事件作 SQL 次数。SQL 调用量只统计 collect_window_query_completed 的 queryCount。

`rowsLoaded` 和 `coveredEventCount` 是每次观测的值；hit 未重新加载窗口时 rowsLoaded 为 0，不代表缓存没有资料。同一个 batchGeneration 的 hit 可能多次报告相同缓存覆盖数量。批次关联应至少结合 tenant/account/source/canonicalChatId 及实际存在的其他目标维度；仅凭 generation 不足以全局去重。

**rowsLoaded/windowTruncated 只在 batchRunStarted=true 时有效。**hit 或其他未进入 run 的路径中，0/false 是未执行的默认值，不能记为空窗口或未截断窗口。进入 run 后仍需结合 batchResult 判断加载是否成功；失败不能自动解释为空窗口。

### 耗时不可重复相加

父阶段 duration 覆盖子查询时长。**不能把 parent.batch duration 与 primary/retry 两条 query.duration 求和**，也不能把同一查询的 durationMs 与 durationUs 相加。查询精细耗时统一使用 `durationUs / 1000.0` 转为毫秒；`durationMs` 用于兼容现有面板。

多个查询耗时之和表示累计查询工作时间，可能包含重试；它不等于父链路墙钟耗时。并发时累计工作时间还可能超过墙钟时间。批次复用事件的 durationMs 与查询耗时也不能未经边界核对直接相加。

## 先核对实际载荷与索引

沿用已有文档的实际约束：普通日志加 `g.Map`、JSON 文本、CLS 顶层字段并非同一件事。新增事件名不代表能直接执行 `event:...` 或 `SELECT durationUs`。

在 [生产排障手册](production-debugging.md#3-cls) 的业务主题内检索近 5–15 分钟，全文关键词使用 `"collect_window_query_completed" OR "collect_target_batch_reuse"`。读取少量样本核对事件、字段位置及数值类型，再通过主题的索引配置或 `tccli cls DescribeIndex` 确认可统计字段。原文搜索按 `ListOver` 和 `Context` 判断完整性。

本页列名是逻辑字段名，执行时按实际索引映射。解析失败保留 NULL，避免将缺失数据显示为零。

## 已提取为可统计顶层字段时的 SQL

以下采用 CLS `索引检索 | SQL` 形式；仅在对应字段确实已提取且启用统计后使用。先在 CLS 控制台验证语法和字段，再将经验证的语句传给 `SearchLog --QueryString`。

### primary/retry 查询量、错误、空结果和独立耗时

```sql
event:collect_window_query_completed
| SELECT stage, queryKind,
    sum(try_cast(queryCount AS bigint)) AS queries,
    sum(try_cast(queryErrorCount AS bigint)) AS errors,
    sum(try_cast(emptyQueryCount AS bigint)) AS empty_queries,
    sum(try_cast("rows" AS bigint)) AS returned_rows,
    avg(try_cast(durationUs AS double)) / 1000.0 AS avg_query_ms,
    approx_percentile(try_cast(durationUs AS double), 0.95) / 1000.0 AS p95_query_ms,
    max(try_cast(durationUs AS double)) / 1000.0 AS max_query_ms,
    100.0 * sum(try_cast(queryErrorCount AS bigint))
      / nullif(sum(try_cast(queryCount AS bigint)), 0) AS error_pct,
    100.0 * sum(try_cast(emptyQueryCount AS bigint))
      / nullif(sum(try_cast(queryCount AS bigint))
        - sum(try_cast(queryErrorCount AS bigint)), 0) AS successful_empty_pct
  FROM log
  GROUP BY stage, queryKind
  LIMIT 10
```

`returned_rows` 可能因重试重复读取相同行，不表示唯一消息数。平均与分位数包含成功及失败查询，用于观察整体调用成本；若只分析成功查询，另加 `WHERE try_cast(queryErrorCount AS bigint)=0` 并明确面板名称。

### 慢查询定位：超过 100ms

```sql
event:collect_window_query_completed
| SELECT tenantId, accountId, eventAccountId, sourceId,
    sourceChatId, canonicalChatId, eventId, traceId,
    stage, queryKind, "rows", queryErrorCount, durationMs, durationUs
  FROM log
  WHERE try_cast(durationUs AS bigint) > 100000
  ORDER BY try_cast(durationUs AS bigint) DESC
  LIMIT 100
```

按 durationUs 判定阈值，避免整数毫秒截断漏掉 100.xms。先限定来源或更窄窗口，再扩大范围；LIMIT 只限制输出，不保证底层扫描成本有界。

### 批次命中率与未评估量

```sql
event:collect_target_batch_reuse
| SELECT
    sum(try_cast(reuseLookupCount AS bigint)) AS evaluated,
    sum(try_cast(reuseHitCount AS bigint)) AS hits,
    sum(try_cast(reuseMissCount AS bigint)) AS misses,
    sum(CASE WHEN reuseOutcome='not_evaluated' THEN 1 ELSE 0 END) AS not_evaluated,
    100.0 * sum(try_cast(reuseHitCount AS bigint))
      / nullif(sum(try_cast(reuseHitCount AS bigint))
        + sum(try_cast(reuseMissCount AS bigint)), 0) AS evaluated_hit_pct,
    avg(try_cast(durationMs AS double)) AS avg_batch_call_ms
  FROM log
```

分母为 0 时命中率为 NULL，不能显示为 0% 或 100%。平均 batch 调用耗时包含三种 outcome 及其执行路径，需要细分时按 outcome、batchRunStarted、batchResult 分组，不能解释为 Redis 平均查询时延。

### 按真实频道汇总并区分 miss 后未执行

```sql
event:collect_target_batch_reuse
| SELECT tenantId, accountId, sourceId, canonicalChatId,
    reuseOutcome, batchRunStarted, batchResult,
    count(*) AS batch_calls,
    sum(try_cast(reuseHitCount AS bigint)) AS hits,
    sum(try_cast(reuseMissCount AS bigint)) AS misses
  FROM log
  GROUP BY tenantId, accountId, sourceId, canonicalChatId,
    reuseOutcome, batchRunStarted, batchResult
  ORDER BY batch_calls DESC
  LIMIT 100
```

canonicalChatId 合并短/长号，sourceChatId 留作原文排查；不按 canonicalChatId 单独聚合来判断某个租户/来源是否健康。miss 且 batchRunStarted=false 提示尚未进入 run，应继续看 batchResult/errorMessage；该数量不能记为 SQL 查询数。

### 未评估/未命中原因与截断情况

```sql
event:collect_target_batch_reuse
| SELECT reuseOutcome, reuseReason, batchRunStarted, batchResult, windowTruncated,
    count(*) AS observations,
    avg(CASE WHEN cast(batchRunStarted AS varchar)='true'
      THEN try_cast(rowsLoaded AS double) ELSE NULL END) AS avg_run_merged_rows,
    max(try_cast(coveredEventCount AS bigint)) AS max_reported_coverage
  FROM log
  GROUP BY reuseOutcome, reuseReason, batchRunStarted, batchResult, windowTruncated
  ORDER BY observations DESC
  LIMIT 100
```

`max_reported_coverage` 仅为单次缓存已覆盖去重数的最大报告值，不是跨批次唯一覆盖数。lease busy 或决策前 Redis 错误从 not_evaluated 原因观察；决策后的错误同时检查 batchResult 和 errorMessage。

上例只将已进入 run 的 rowsLoaded 纳入平均，未执行组返回 NULL。windowTruncated 仅在 batchRunStarted=true 的组内解释，其他组的 false 没有窗口语义。示例按布尔值字符串 `true` 适配；若实际索引是原生 boolean，可改为 `batchRunStarted=true`，若采集转换为其他形式则按样本调整。需要“成功加载窗口”指标时，还需依据加载成功的实际日志证据限定，不能单凭 rowsLoaded=0 或 batchResult 推断。

## JSON/map 字段未平铺时的适配

若索引实际是嵌套字段，例如 `fields.durationUs`，SQL 中使用实际列名（如 `"fields.durationUs"`），索引过滤也替换为实际配置的路径。不要把 Go 的 map 变量名推测成 CLS 列名；以样本和 `DescribeIndex` 为准。

仅当 `content` 是**完整合法 JSON** 且该列可用于 SQL 分析时，可以先提取为标准列，再复用上述聚合。以下示例假设 JSON 中指标在根节点；若在 `fields` 下，应将 `$.durationUs` 改为 `$.fields.durationUs` 等。真实载荷列为 `message` 时也需替换：

```sql
"collect_window_query_completed"
| SELECT query_kind, count(*) AS observations,
    avg(query_us) / 1000.0 AS avg_query_ms
  FROM (
    SELECT json_extract_scalar(content, '$.event') AS event_name,
      json_extract_scalar(content, '$.queryKind') AS query_kind,
      try_cast(json_extract_scalar(content, '$.durationUs') AS double) AS query_us
    FROM log
  )
  WHERE event_name='collect_window_query_completed'
  GROUP BY query_kind
  LIMIT 10
```

上述全文前置过滤需要全文索引；不存在时改用已验证的载荷字段索引过滤。带时间/级别前缀或普通 map 打印文本不属于完整 JSON，不能直接套 `json_extract_scalar`；应先按现有采集解析规范提取，未完成解析时只做原文排障，不报告精确聚合。

## 发布后验收口径

- 样本中 stage/queryKind 配对正确，查询成功/失败均有完成事件；同一次实际查询只记一条。
- queryCount 均为 1，空查询与错误计数互斥；durationUs 可转换为数值，亚毫秒查询可见。
- emptyQueryCount 仅成功且 rows=0 时为 1；空查询率分母为 queryCount-queryErrorCount，分母为 0 返回 NULL。
- hit/miss 的 lookup=1，not_evaluated 的 lookup/hit/miss 均为 0，hit+miss=lookup。
- 业务身份按 tenant/account/source 隔离；accountId 为队列 scope，eventAccountId 为事件归属；用 canonicalChatId 合并短/长号，sourceChatId 与内部频道 ID 分开解释。
- miss 不直接累计成 SQL 次数；batchRunStarted=false 的 miss 可由执行前丢失租约产生。只用查询完成事件衡量 ORM 往返，并且不将其当作 PG 纯执行时间。
- rowsLoaded/windowTruncated 只在 batchRunStarted=true 时解释；hit 的 0/false 不计为空窗口。父 batch 与 primary/retry 查询耗时分别展示，不求和。
- 先对比相同窗口、相同角色/版本覆盖和相同业务范围，再判断查询次数、空查询率及命中率是否改善。缺少日志、采样或索引延迟均需标注。
- 仅靠这两个事件不能证明资料已入库或 TG 已发送；沿 eventId/traceId 继续关联采集分支、dispatch 和 Job 回执。
