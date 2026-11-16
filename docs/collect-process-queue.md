# 采集处理队列

采集处理链路分成两个边界：

```text
Telegram 更新
  → collect_event 业务事件
  → Redis/Asynq collect-process 队列
  → 规则处理 Worker
  → collect_event / collect_dispatch 业务投影
```

接收层的旧 `hg_tg_collector_ingest_spool` 也默认使用 `telegramCollector.ingestSpoolMode=redis_first` 绕过。实时 webhook 只保留一次 `collect_event` 幂等写入，再把事件交给 Redis/Asynq；接收 Spool 仅在迁移旧部署或回放接收积压时通过 `postgres_spool` 显式启用。

实时采集默认使用 `youbanPublish.collect.processOutboxMode=redis_first`。任务进入 Redis/Asynq 后，由 Asynq 的唯一任务键、延迟任务和重试机制负责执行交接；Worker 只在带有旧 Outbox 租约的任务上回写 Outbox 状态。这样实时事件不会先对 `hg_youban_publish_collect_process_outbox` 执行一次 PostgreSQL upsert，也不会为每个来源启动 PostgreSQL Outbox 轮询器。

`hg_youban_publish_collect_process_outbox` 保留给旧版本任务和显式迁移/恢复。新的实时任务和新的历史任务都不再写入这张表。迁移旧任务时可以临时把配置切换为：

```yaml
youbanPublish:
  collect:
    processOutboxMode: "postgres_outbox"
```

切换后重启负责采集处理的 publish worker，等待旧 Outbox 积压收口，再切回 `redis_first`。切换期间不要同时让旧版本和新版本处理同一个来源，否则旧版本仍可能为实时事件写入 Outbox。

Redis-first 模式的边界是：`collect_event` 仍是业务事实，Redis/Asynq 是执行交接。Redis 不可用时，任务入队会失败并由上游/Asynq 调用方重试；系统不会伪造 PostgreSQL Outbox 行来掩盖 Redis 故障。需要长期离线接收保障时，应先恢复 Redis 高可用，再考虑增加独立 Redis Stream 或接收层队列，不能把 PostgreSQL Outbox重新放回每条实时事件的热路径。

排查实时采集时，优先查看 `collect_event` 的事件状态和 Asynq 队列深度，再查看 `hg_youban_publish_collect_process_outbox`。Redis-first 下没有对应 Outbox 行属于预期；只有带 `outboxOwner` 的历史/恢复任务才需要核对 Outbox 租约。

启动日志会输出 `telegram_collector / collector_worker_started` 和 `telegram_collect_queue / collect_process_queue_mode`，其中 `ingestSpoolMode`、`outboxMode`、`legacySpoolActive` 和 `postgresOutboxUsed` 用于确认实际镜像是否加载了 Redis-first 配置。
