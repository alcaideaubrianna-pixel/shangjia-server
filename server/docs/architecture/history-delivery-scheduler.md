# 历史采集 Delivery 调度重构方案

## 目标

历史采集调度与 Publish 执行解耦。background 负责公平领取和投递 delivery，publish 只负责消费 delivery 并执行规则、资料和媒体处理。队列保持三条，避免为账户或 source 创建大量 Redis 队列。

## 运行边界

```text
account -> collector event/delivery -> background scheduler -> tg.delivery.history -> publish worker
```

- account 保持 Telegram 协议单实例约束。
- collector 只接收原始事件并创建 delivery。
- background 负责账户/source 公平调度、任务配额、进度统计和失联恢复。
- publish 负责 delivery 业务执行和资料入库。
- media 继续负责媒体下载及处理。

## 队列

只保留 `tg.delivery.realtime`、`tg.delivery.history`、`tg.delivery.retry` 三条逻辑队列。公平性由 PostgreSQL 调度器保证，Redis 只承载已被调度的短期执行任务。

## 配额

- 单历史采集任务默认并发 4。
- 单账户默认并发 8。
- 单 source 默认并发 4。
- 新历史任务、旧积压、失败重试分别使用 50%、40%、10% 的调度额度。

配额只限制同时执行数，不限制任务总量。任务数量增长时通过增加 publish worker 扩大执行容量。

## 数据库调度字段

delivery 增加 `account_id`、`source_id`、`scheduler_pool`、`max_concurrency`、`running_count`、`dispatch_owner`、`dispatch_until`、`dispatch_at`。旧数据分批回填，回填和索引创建不得阻塞线上 delivery。

## 调度规则

调度器每轮按“有效优先级、账户轮转、source 轮转、创建时间、ID”领取任务。领取使用短事务和 lease，Redis 入队成功后才记录 dispatch；入队失败保留 pending，下一轮恢复。Publish 执行前仍使用 delivery lease 做幂等保护。

## 灰度

先增加字段和回填，再以开关启用 background scheduler。关闭开关时保留现有 Publish recovery。单 source 灰度验证无重复、无丢失后，逐步扩大到账户和全量历史任务；稳定一个发布周期后再移除旧调度路径。

## 扩容指标

以 pending 数、最老任务等待时间、每分钟完成数、Redis 延迟、数据库连接池和 Telegram flood wait 为依据扩缩 publish worker。background 调度器保持 1 个 active leader，可运行备用实例但必须使用数据库租约选主。

## 验收

验证单账户十万任务不能独占队列；新任务能在目标时间内开始；processing lease 过期可恢复；Redis 短暂异常可补派；100、1000 和更大数量任务下 completed 吞吐随 publish worker 扩展提升。
