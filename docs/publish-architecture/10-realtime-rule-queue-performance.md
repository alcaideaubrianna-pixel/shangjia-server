# 实时采集规则队列与性能稳定方案

## 1. 目标与约束

目标是支持约 3000 个采集规则持续运行，每条规则每分钟产生约 10 条消息，即约 500 条/秒，同时满足：

- 实时消息优先于历史回放、循环和批次任务；
- 一个规则、租户或账户变慢时，不阻塞其他规则；
- 采集事件不丢失，Redis 故障后可以从 PostgreSQL Outbox 恢复；
- 实时路径禁止全表扫描、全局 COUNT、无范围 SELECT *；
- 代码遵守 HotGo/GoFrame v2 分层和 `cache.Instance()`、`queue` 封装规范；
- 队列、媒体、规则匹配、发布均有结构化链路日志。

## 2. 总体模型

```text
Telegram/BOT
  -> collect_event（事实记录）
  -> collect_process_outbox（可靠 Outbox）
  -> Redis 规则逻辑队列
  -> 采集处理 Worker Pool
  -> 资料/媒体意图/规则分发
  -> 媒体下载 Worker Pool
  -> collect_dispatch
  -> 发布任务创建入口
  -> 频道发布队列
  -> Telegram 发送
```

Redis 只保存待处理索引和租约，PostgreSQL Outbox 是最终事实源。任何入队失败都必须保留 Outbox 状态并进入补偿，不允许只写 Redis。

## 3. 队列键与分片

### 3.1 实时规则队列

每条采集规则拥有独立逻辑队列，不为每条规则创建独立进程：

```text
collect:rule:{tenant_id}:{rule_id}:realtime
collect:rule:{tenant_id}:{rule_id}:history
media:rule:{tenant_id}:{rule_id}
publish:channel:{tenant_id}:{target_chat_id}
delete:channel:{tenant_id}:{target_chat_id}
```

规则队列解决公平性和隔离问题；租户、来源、账户分片继续用于数据库补偿、限流和故障隔离。两者不能混为一个维度。

### 3.2 队列消息

消息只携带主键和版本，不携带大文本或媒体内容：

```json
{
  "outboxId": 123,
  "eventId": 456,
  "tenantId": 16,
  "ruleId": 789,
  "sourceId": 10,
  "profileId": 20,
  "attempt": 0,
  "traceId": "..."
}
```

Worker 必须按主键读取必要字段，禁止 `SELECT *`。

## 4. Worker 调度

采用共享 Worker Pool 和公平调度器：

1. 调度器维护活跃规则集合，不扫描 3000 条规则的业务大表；
2. 每次从一个规则队列最多领取 10 条；
3. 处理完成后立即释放该规则配额并轮转到下一条规则；
4. 实时队列优先，历史队列仅使用保底配额；
5. 单规则、单租户、全局均有并发上限；
6. 单条消息失败只退避当前消息，不阻塞该规则后续消息；
7. Redis 队列丢失时，按 Outbox 的 `(tenant_id, rule_id, id)` 游标补入，不允许从表头重新扫描。

推荐初始值：实时批次 10，单资料媒体并发 2～4，历史 Worker 不超过总容量的 10%。

## 5. 数据库降压要求

### 5.1 实时路径禁止

- `COUNT(*)` 判断是否有待处理任务；
- 无条件或大范围 `SELECT *`；
- 按历史表全局排序后取少量数据；
- 以 `OR` 拼接多个账户路径；
- 每个事件重复查询规则、来源和频道配置。

### 5.2 正确查询方式

- 使用 `WHERE tenant_id=? AND rule_id=? AND status=? AND id>? ORDER BY id LIMIT ?`；
- 使用 `FOR UPDATE SKIP LOCKED` 领取 Outbox；
- 使用 `EXISTS/LIMIT 1` 替代 COUNT；
- 只读取队列处理所需字段；
- 规则、来源、频道配置使用短时缓存，更新后主动清理；
- 统计改为增量计数表，低频对账按租户/规则游标执行。

### 5.3 必备索引

最终索引名称以实际表结构为准，至少覆盖：

```sql
-- 实时 Outbox
(tenant_id, rule_id, status, available_at, id)

-- 租约恢复
(tenant_id, rule_id, status, lease_expires_at, id)

-- 事件去重
(tenant_id, rule_id, source_id, source_message_id)

-- 规则分发
(tenant_id, rule_id, status, id)

-- 频道发布队首
(tenant_id, target_chat_id, status, priority, created_at, id)
```

上线前使用 DBX `EXPLAIN (ANALYZE, BUFFERS)` 验证，线上使用 `CREATE INDEX CONCURRENTLY`，并检查 `pg_index.indisvalid` 和 `indisready`。

## 6. 媒体处理

媒体下载不能占用采集规则处理槽：

- 采集 Worker 只创建媒体意图和媒体 Outbox；
- Media Worker 独立消费 `media:rule:{tenant}:{rule}`；
- 单资料媒体并发限制为 2～4；
- 媒体失败必须写明确终态和重试时间；
- `media_ready` 只表示媒体阶段完成，必须继续触发规则匹配或明确记录“规则无匹配/审核拦截/媒体失败”等终态；
- 媒体完成后通过事件唤醒后续处理，禁止依赖全局扫描。

## 7. 发布与优先级

发布仍按频道独立队列，保证同频道顺序、不同频道并行：

```text
priority=1  手动上架
priority=10 实时采集
priority=50 循环上架
priority=80 批次翻新
priority=100 历史补偿
```

频道队列只负责本频道顺序。多频道手动上架应同时创建多个频道队列消息，不得串行等待前一个频道完成。

任何频道任务超过配置的队首等待时间，必须：

1. 标记当前租约过期；
2. 记录失败原因和退避时间；
3. 从 Redis 队列移除失效索引；
4. 唤醒下一条任务；
5. 继续保留 PostgreSQL Outbox 事实记录，等待补偿。

## 8. ENV 配置

```text
YOUBAN_COLLECT_RULE_WORKERS=128
YOUBAN_COLLECT_RULE_BATCH_SIZE=10
YOUBAN_COLLECT_RULE_MAX_CONCURRENCY=4
YOUBAN_COLLECT_TENANT_MAX_CONCURRENCY=64
YOUBAN_COLLECT_HISTORY_WORKERS=8
YOUBAN_MEDIA_WORKERS=256
YOUBAN_MEDIA_PER_PROFILE_CONCURRENCY=4
YOUBAN_PUBLISH_WORKERS=128
YOUBAN_COLLECT_REHYDRATE_BATCH_SIZE=200
YOUBAN_COLLECT_HEAVY_SCAN_ENABLED=false
YOUBAN_COLLECT_STATS_ENABLED=false
YOUBAN_PUBLISH_RECOVERY_BATCH_SIZE=10
YOUBAN_PUBLISH_HEAD_TIMEOUT_SECONDS=900
```

所有配置必须有安全默认值、启动日志和运行时指标。达到数据库连接池或 Redis 延迟阈值时，只降低新任务领取速度，不暂停已领取任务的终态处理。

## 9. 结构化日志与指标

每个阶段必须携带：

```text
traceId, operationNo, tenantId, accountId, sourceId, ruleId,
collectEventId, profileId, outboxId, jobId, channelId,
stage, status, queueAgeMs, dispatchAgeMs, durationMs,
attempt, workerInstanceId
```

必须统计：

- 事件接收到入库耗时；
- Outbox 入队等待时间；
- 规则队列等待时间；
- 媒体下载耗时；
- 规则匹配耗时；
- 发布队列等待和 Telegram 发送耗时；
- 每规则 pending、processing、failed、终态数量；
- Redis 入队失败和 Outbox 回填数量。

## 10. 迁移步骤

1. 先增加规则队列 Key、Outbox 分片字段和索引；
2. 新消息采用“PG Outbox + Redis 规则队列”双写；
3. 启动规则 Worker Pool，实时队列优先；
4. 运行新旧路径对账，禁止重复发布；
5. 对历史积压按租户/规则游标分批回填；
6. 关闭实时路径上的全局统计、恢复和历史扫描；
7. 完成回放后再删除旧路径，不允许先删事实源或补偿逻辑。

## 11. 验收标准

必须使用真实本地 PostgreSQL、Redis、TG Fake 和当前二进制完成：

- 3000 个规则、每规则 10 条消息回放；
- 实时规则全部入库，不能被历史任务阻塞；
- 任一规则暂停时，其他规则继续处理；
- 媒体成功、失败、无媒体、审核拦截均进入明确终态；
- 多频道发布并行，同频道保持顺序；
- Redis 清空后 Outbox 能按游标恢复；
- 失败重试不会重复创建资料或重复发送；
- 实时路径不出现全局 COUNT、全局排序或无范围 SELECT；
- DBX 验证热点查询使用目标索引，单批领取查询稳定在毫秒级到低百毫秒级；
- 记录命令、数据量、耗时、终态数量和 TG Fake 收包证据。
