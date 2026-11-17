# 原始采集事件的实时与历史执行预算

## 路由

新 Bot 更新进入 `tg.collect.urgent.v2`；新实时账户消息继续进入
`tg.collect.realtime`，由同一实时 Worker 池处理。

旧 `tg.collect.urgent` 不再接收新版本入口任务，与 `tg.collect.history`
共同使用独立历史 Worker 池。无需清空、搬动或扫描旧 Redis 任务。
原始事件恢复、来源重放、旧 Spool 恢复一律进入历史队列。

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

`YOUBAN_TELEGRAM_COLLECTOR_HISTORY_CONCURRENCY`：历史原始事件并发，默认 2，最小 1。
这只控制原始事件阶段，Delivery 历史池和业务来源池仍有自己的容量限制，
不等于端到端数据库并发预算。不能据此保证 PG 总压力不会升高。

## 部署与回滚

沿用现有角色，不新增 SAE 应用或 SQL 表、索引。需要更新：

1. collector：先启动新实时队列消费者，以及旧队列历史消费者。
2. publish：加载历史 Delivery 属性传递及恢复修复。
3. web：切换新 Bot 事件生产队列。

部署过程中未升级的 web 仍写旧队列，只能使用历史预算；需尽快完成入口升级。
旧 Collector 仍存活时可能继续消费旧队列并使用旧路由，不应将滚动中间态视为最终隔离。
回滚时先恢复入口生产路由；新队列中仍有消息时保留新消费者，不能直接撤掉消费者。

## 验收

同时观察新队列最老等待时间、旧队列积压、实际 event/Delivery/资料/dispatch
阶段，不用 completed 任务计数替代资料入库数量。检查
`event.recovery.live_task_preserved` 和 `event.enqueued` 的 `enqueueOrigin`、
`outcome`；重复恢复应为 `existing_task_confirmed`。

本改动解除旧 Bot 原始队列对新入口的排队影响，不代表不同来源已经获得独占
CPU/PG，也不代表 175 万旧任务已经恢复。原始事件的业务完成状态是实际恢复依据。
