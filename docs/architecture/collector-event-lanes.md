# 原始采集事件的实时与历史执行预算

## 路由

新 Bot 更新进入 `tg.collect.live`；新实时账户消息继续进入
`tg.collect.realtime`，由同一实时 Worker 池处理。

旧 `tg.collect.urgent`、`tg.collect.urgent.v2` 不再接收新版本入口任务，与 `tg.collect.history`
共同使用独立历史 Worker 池。无需清空、搬动或扫描旧 Redis 任务。
原始事件恢复、来源重放、旧 Spool 恢复一律进入历史队列。

新 Bot 通道任务携带独立的调度时间 `liveEnqueuedAtUnixMs`，不依赖是否开启
观测载体。超过 `YOUBAN_COLLECTOR_REALTIME_MAX_WAIT_SECONDS`（默认 120，
有效范围 5～3600 秒）后，先确认稳定 TaskID 的历史任务已入队，再 ACK 当前
实时任务；交接失败保留重试，不领取数据库事件、不运行资料窗口。超龄转历史
旨在控制实时工作量，不等于超龄资料已经完成，也不是任意流量下的延迟保证。

消费者按队列确定历史执行属性，不根据当前时间推测消息类型。历史交付持久化
低优先级和 history pool，Delivery 消费及恢复不能因原始消息为 Bot 再升为实时。
不同队列仍可能包含同一旧事件；现有数据库原子领取和交付身份检查负责业务幂等。

## 去重与故障

新任务采用队列内稳定 TaskID `collector-event-{eventID}`，没有短时 Unique
或成功后保留时间。排队超过五分钟仍不会重复添加同队列任务；完成记录由 PG
原始事件和链路日志保留，显式重放不会被旧 completed TaskID 阻断。

恢复先确认新实时队列是否已有可执行的相同任务；存在则不补发历史副本。
TaskID 冲突只在确认 pending/active/scheduled/retry 后视为已有执行保障。
归档任务使用 Asynq 的 RunTask 唤醒；读取失败、任务消失不能伪装成成功。
Inspector 复用连接池，不为每个恢复候选重新连接 Redis。

## 配置

`YOUBAN_TELEGRAM_COLLECTOR_CONCURRENCY`：实时原始事件并发，沿用当前设置。

实时总预算至少为 2 时，Bot 与账户使用独立消费者：默认总预算 12 拆成
Bot 8、账户 4，不增加总并发。`YOUBAN_COLLECTOR_BOT_REALTIME_CONCURRENCY`
可指定其中分配给 Bot 的预算，最多为总预算减 1；总预算为 1 时维持单槽
共享，启动日志 `realtimeCapacityShared=true` 明确表示不能提供保留容量。
更新 collector 即加载该分配，无需迁移 Redis 任务或新增 SAE 角色。

Delivery 的实时与历史账户额度按 lane/account 分开，避免历史领取占用实时
账户名额；两者仍受原有各自 Worker 总并发限制。该隔离不是来源独占配额，
同一 Bot 的多个来源仍会竞争 Bot 实时预算，不能据此承诺 351 已恢复实时。

`YOUBAN_TELEGRAM_COLLECTOR_HISTORY_CONCURRENCY`：历史原始事件并发，默认 2，最小 1。
这只控制原始事件阶段，Delivery 历史池和业务来源池仍有自己的容量限制，
不等于端到端数据库并发预算。不能据此保证 PG 总压力不会升高。

## 部署与回滚

沿用现有角色，不新增 SAE 应用或 SQL 表、索引。需要更新：

1. publish、media：先启动 fresh Delivery 消费者及媒体实时预留槽位。
2. collector：启动新实时队列消费者，以及旧队列历史消费者。
3. web：最后切换新 Bot 事件生产队列。

部署过程中未升级的 web 仍写旧队列，只能使用历史预算；需尽快完成入口升级。
旧 Collector 仍存活时可能继续消费旧队列并使用旧路由，不应将滚动中间态视为最终隔离。
回滚时先恢复入口生产路由；新队列中仍有消息时保留新消费者，不能直接撤掉消费者。

首次 queued 实时交接进入 `tg.delivery.fresh.v1`，从原 Delivery 实时总预算
中预留容量（默认 8 拆为 fresh 6、旧实时 2），不增加总并发。恢复、重放和
失败重试不升级 fresh；direct handoff 保留原路径。总预算为 1 时共享执行槽，
启动日志明确标记。Collector 和 Publish 中实际启动 Delivery 的角色都要升级，
不能只更新生产者。

媒体的账户/来源配额沿用原 Redis key、总槽数和租约 token，总额度大于 1 时
为实时预留最后一个槽，Bulk 只能使用其他槽。总额度为 1 时仍共享，避免历史
永久停跑；新旧消费者共存时旧历史任务可能仍持有预留槽，需要等正常释放。
更新实际运行媒体消费者的 media 角色，不新增角色、不删除租约。

## 验收

同时观察新队列最老等待时间、旧队列积压、实际 event/Delivery/资料/dispatch
阶段，不用 completed 任务计数替代资料入库数量。检查
`event.recovery.live_task_preserved` 和 `event.enqueued` 的 `enqueueOrigin`、
`outcome`；重复恢复应为 `existing_task_confirmed`。

本改动解除旧 Bot 原始队列对新入口的排队影响，不代表不同来源已经获得独占
CPU/PG，也不代表 175 万旧任务已经恢复。原始事件的业务完成状态是实际恢复依据。

## 历史 PostgreSQL 写入合并

历史任务使用进程内短窗口合并数据库操作，任务身份、Asynq 领取和重试不变。
队列交付的明确主键进入批次，不从数据库发现更多历史任务。

- Event：`UPDATE ... RETURNING` 合并领取与读取；同一批重复 ID 只交付给一个执行者。
- Delivery：批量幂等写入并返回 ID，保留已有记录的 Event 归属和已完成状态。
- Event ready：按 ID、tenant、lease owner、attempt 精确匹配后合并写回。
- 失败路径复用已经领取的重试次数，省掉一次重复读取；CAS 更新仍检查当前 owner。

这三类历史批次 SQL 共用一个串行执行锁，单进程最多同时执行其中一个批次。
这不是所有数据库访问的总预算：业务窗口、Delivery 消费和其他角色仍有独立 SQL。
调用方只有收到本次数据库结果后才继续；进程退出或结果未知时由原有任务、租约恢复。
不在内存缓冲后提前 ACK，也不删除 Event/Delivery 的可靠交接状态。

配置：

```text
YOUBAN_TELEGRAM_COLLECTOR_HISTORY_BATCH_ENABLED=true
YOUBAN_TELEGRAM_COLLECTOR_HISTORY_BATCH_SIZE=20
YOUBAN_TELEGRAM_COLLECTOR_HISTORY_BATCH_WAIT_MS=10
```

批次大小限制 1～50，合并等待限制 0～50ms。设置 enabled=false 可回到原单条路径。
低流量或并发为 2 时通常合并不到 20 条，不能按满批次推算实际吞吐。

实时原始事件的领取也复用按主键的 `UPDATE ... RETURNING`，直接取得本次
租约归属和原始消息，省去领取后的独立 SELECT；不等待历史微批次，也不获取
历史批次写入锁。实时单条领取保留行锁等待，不能因暂时锁冲突成功 ACK 一个
尚未领取的任务。历史微批次保留既有 `SKIP LOCKED` 和来源级恢复。
这只减少领取阶段的数据库往返，不代表端到端吞吐翻倍或历史积压已经清空。

频道进度使用相同合并器，在 5ms 窗口内按 tenant/channel 合并最大消息 ID，
通过同步批量写回防止较旧消息覆盖较新进度，也避免仅写首条的 TTL 节流漏掉末条。
无新增 SAE 角色、后台数据库扫描或 SQL 迁移；更新 collector/publish 消费实现。

本地验证使用独立 Docker PostgreSQL 的 127.0.0.1:55489，测试仅通过显式
YOUBAN_COLLECTOR_BATCH_TEST_PORT=55489 启用，不读取生产 DSN。

## 已知拒绝任务的入口路由

Publish adapter 在来源、实际频道审批或自身发布频道检查明确拒绝后，按
tenant/source/actual-chat/config-generation 写入一分钟的后台路由标记。
source 是全局唯一主键；版本沿用既有配置失效机制，账户、来源、频道配置变更后
旧标记失效。检查前捕获版本，防止旧请求把拒绝结论写进新配置版本。

Collector 通过可选 service 接口只读取缓存，不查询 Publish 实现或表。
新 Bot 更新仍完整写入原始 Event；命中标记只把执行预算改为 history，
不丢弃更新、不伪造资料成功、不绕过审批或资料去重。
已排在实时队列中的已知拒绝任务也可交给后台 Delivery；Handler 命中相同标记
则直接记录 skipped，无需再次读取来源、发布目标和审批关系。

缓存缺失、异常或未知枚举保持原实时路径。配置重新启用后，原始事件可由
既有指定来源重放入口恢复，不需要重建或借用其他租户的媒体身份。
上线需更新 collector/publish 消费实现及 web 入口，不新增 SAE 角色或 SQL 迁移。
