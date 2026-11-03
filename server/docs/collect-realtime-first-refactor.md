# 采集链路实时优先重构方案

状态：设计稿

适用范围：Telegram Collector、采集媒体、采集处理、审核生成和采集推送链路。

## 1. 背景与已确认问题

线上事件 `10161732` 暴露了两个独立延迟：Telegram 收到源消息后约 18 小时才进入采集事件；事件媒体在数秒内完成后，又等待约 3 小时才被处理 Outbox 消费。最终 Telegram 发送本身只耗时几十秒。

当前系统已经存在 realtime/history 队列名称，但部分处理预算仍共享，历史恢复和实时任务可能竞争同一批消费者。媒体阶段也曾因全局恢复背压导致 ready 事件长期孤儿。

目标是让新消息先落库、先进入实时处理链路；历史采集、恢复和批量任务只能使用明确预算，不能阻塞实时链路。

## 2. 目标和非目标

目标：

- Telegram Update 到采集事件落库 P95 小于 1 秒；
- 实时事件进入 process queue P95 小于 3 秒；
- 历史任务不能占满实时消费者；
- 实时 backlog 超阈值时自动暂停历史任务；
- 媒体、处理和推送均保留幂等、租约接管和迟到 ACK fencing；
- 每个 tenant、account、source 可以观察等待时间、最老任务和处理速率。

非目标：

- 不新增 SAE 角色或实例；
- 不清理 Redis 队列；
- 不批量重置线上事件；
- 不在第一阶段删除仍被 CLI 使用的 BOT 恢复入口。

## 3. 目标架构

```text
Telegram Update
    |
    v
幂等 Ingest + ingest_spool（事务落库）
    |
    +--> realtime event queue --> realtime process workers
    |                                  |
    |                                  +--> realtime media queue
    |                                  +--> profile/review/dispatch
    |
    +--> history queue --> history workers（固定低预算）

Recovery scheduler
    +--> ready orphan（小预算，始终允许）
    +--> expired lease（小预算）
    +--> pending media（实时空闲时）
    +--> history process（实时空闲时）
```

每一层都以数据库状态为最终事实，Redis/Asynq 只负责唤醒和调度。重复投递、任务丢失或租约过期都必须可以安全重放。

## 4. 分阶段技术改动

### 阶段 A：实时采集先落库

改动：

1. Collector Update 入口只执行幂等键校验、原始事件落库和 realtime outbox 写入。
2. 新 Update 固定写入 `history=false`、高 priority、`available_at=now()`。
3. 历史分页和恢复任务固定写入 `history=true`、低 priority。
4. ingest_spool 的 claim 查询按 `priority DESC, next_run_at, id` 排序。
5. collector runtime 拆成 realtime/history 两个消费者池，实时池固定保留容量。

建议初始预算（单现有实例）：

```text
realtime consumers: 8
history consumers:   2
```

### 阶段 B：Process Outbox 严格实时优先

改动：

1. `collect_process_outbox` 领取分两段：先 realtime，再 history。
2. 单轮最多领取 realtime 500、history 50。
3. realtime pending >= 100 或 oldest age >= 3 秒时暂停 history。
4. realtime 使用独立 worker semaphore；history 不能借用 realtime semaphore。
5. recovery 只能恢复对应原始队列类别，禁止把 history 重标成 realtime。

数据库索引：

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_collect_process_realtime_due
ON hg_youban_publish_collect_process_outbox (status, available_at, id)
WHERE COALESCE(history, FALSE) = FALSE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_collect_process_history_due
ON hg_youban_publish_collect_process_outbox (status, available_at, id)
WHERE history = TRUE;
```

实际迁移前必须先检查线上索引是否已存在，并通过限定范围 EXPLAIN 验证，不执行全表 ANALYZE。

### 阶段 C：媒体实时预算和账号公平性

改动：

1. realtime media 与 bulk media 使用独立消费者预算。
2. realtime media 固定 16 个 worker，bulk media 固定 4 个 worker，具体值通过 ENV 调整并受代码上限保护。
3. 账号媒体并发初始设为 8，source 并发初始设为 4。
4. 先检查可复用缓存，再获取 Telegram owner slot；缓存命中不占用账号配额。
5. owner slot 获取最多等待 2 秒，失败进入公平重试，不阻塞 worker。
6. Telegram FloodWait、下载超时和来源删除分别记录，不把永久失败无限重试。

建议 ENV：

```text
YOUBAN_PUBLISH_MEDIA_ACCOUNT_CONCURRENCY=8
YOUBAN_PUBLISH_MEDIA_SOURCE_CONCURRENCY=4
YOUBAN_PUBLISH_MEDIA_REALTIME_WEIGHT=24
YOUBAN_PUBLISH_MEDIA_BULK_WEIGHT=1
```

### 阶段 D：Delivery 实时优先

改动：

1. dispatch 产生后按 `history` 分类进入 realtime/history delivery queue。
2. realtime delivery 固定消费者，history delivery 限速。
3. Telegram FloodWait 只暂停当前账号/目标频道的发送槽，不暂停全局 delivery。
4. 明确成功、明确失败和结果未知分开处理，迟到回执必须使用 lease fencing。

## 5. 恢复策略

恢复器拆为四个有界步骤：

| 步骤 | 每轮预算 | 是否受 realtime 背压影响 |
|---|---:|---|
| ready orphan | 100 | 否，始终执行 |
| expired lease | 50 | 否，始终执行 |
| pending media | 50 | 是，实时繁忙时暂停 |
| history process | 50 | 是，实时繁忙时暂停 |

恢复必须写结构化日志，包含 `source_id/account_id/event_id/queue_class/selected/reenqueued/error`。数据库状态更新和重新入队必须幂等；不能依赖 Redis unique key 作为唯一事实。

## 6. 可观测性

新增指标：

```text
xiaohuiji.collect.ingest_to_event_seconds
xiaohuiji.collect.event_to_process_seconds
xiaohuiji.collect.realtime_backlog
xiaohuiji.collect.history_backlog
xiaohuiji.collect.realtime_oldest_age_seconds
xiaohuiji.collect.history_oldest_age_seconds
xiaohuiji.collect.media_owner_wait_seconds
xiaohuiji.collect.media_pending_oldest_age_seconds
xiaohuiji.collect.process_wait_seconds
xiaohuiji.collect.dispatch_wait_seconds
xiaohuiji.collect.delivery_wait_seconds
```

指标统一带以下维度：

```text
tenant_id, account_id, source_id, queue_class
```

日志关键节点：

```text
ingest_persisted
realtime_enqueued
process_claimed
media_owner_acquired
media_ready
profile_committed
review_created
dispatch_created
delivery_sent
```

## 7. 本地线上数据验证

部署前必须使用脱敏线上快照，至少覆盖：

- 1000 个 realtime event 与 1000 个 history event；
- 同一 account 多 source；
- grouped display/verify 配对；
- ready orphan、pending media、expired lease；
- 重复 Update、重复 enqueue、迟到 ACK；
- Redis 入队失败后由 outbox 恢复；
- realtime backlog 高于阈值时 history 自动让出。

验收指标：

```text
realtime ingest P95 < 1s
realtime process claim P95 < 3s
history backlog 不影响 realtime oldest age
重复事件不产生重复资料/推送
ready orphan 在两个恢复周期内下降
```

当前仓库已有恢复、背压、租约和 replay 测试；完整线上 dump 回放需要启动临时 PostgreSQL 并提供 `YOUBAN_COLLECT_PROCESS_DUMP`，不能用生产库直接作为本地测试库。

## 8. 部署和回滚

每阶段只部署必要角色：

1. 阶段 A/B：collector、publish、scheduler；
2. 阶段 C：media、publish、scheduler；
3. 阶段 D：publish。

每次部署前记录旧镜像 digest、目标镜像 digest、ENV 差异和回滚命令。部署后观察 15 分钟：realtime oldest age、media owner wait、process backlog、review 创建速率和 Telegram 错误率。任一指标恶化或实时 P95 超过 10 秒，立即回滚镜像和 ENV。

## 9. 实施顺序

1. 先实现并测试 collector realtime/history 消费分离。
2. 再改 process outbox 领取算法和索引。
3. 再上线媒体实时预算和账号/source 并发 ENV。
4. 再改 delivery realtime/history 分流。
5. 连续线上验证通过后，删除旧的混合领取路径；保留 CLI 修复入口直到历史 backlog 清零。

