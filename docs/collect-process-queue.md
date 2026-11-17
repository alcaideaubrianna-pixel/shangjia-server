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

SAE ENV 对应字段为：

```text
YOUBAN_PUBLISH_COLLECT_PROCESS_OUTBOX_MODE=redis_first
YOUBAN_TELEGRAM_COLLECTOR_INGEST_SPOOL_MODE=redis_first
```

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

## 已忽略验证的调度边界

`ignored + verify + 验证资料未匹配到前序资料组` 对该事件自己的处理任务是终态。
Worker 收口任务、清理候选索引和调度标记；来源扫描和下一次调度判断也不再把它视为主动工作。
仅仅等待十秒再次查询相同窗口，不能补出缺失的展示资料。

这不取消迟到配对能力。新的展示或验证消息到来时，既有有界窗口和向前回看仍可读取同租户、
同来源、同频道的旧验证记录。目标窗口读取被动验证时不再套用旧轮询的十秒冷却条件，
避免新展示刚到就漏掉验证。已完成展示后来绑定了验证，仍经过原有“是否需要修复资料”
检查和幂等分发，不因普通终态判断而被丢弃。

没有新消息的历史孤立验证不再自行无限扫描；确需修复时使用明确来源、事件或消息范围的
维护/原始消息重放入口。停止轮询不意味着已恢复这些历史记录，也不自动撤销用户禁用。
旧展示因错误授权被忽略、媒体被取消时，需要先核对当前实际频道授权，再恢复父展示、
已配对验证和可恢复的媒体状态，不能只把展示事件改回 pending。

SLS 事件 `collect_unmatched_verify_wakeup_closed` 表示已忽略验证结束本次主动唤醒，
其中 `passivePairingEligible=true` 表示它仍可作为后来消息的配对上下文。
`collect_target_batch_reuse` 的 `reuseReason=terminal_target` 表示窗口入口在 SQL/批次租约之前
识别了终态，不属于缓存命中或未命中。此类记录不进入批次命中率分母。

来源扫描和后续调度共用主动状态白名单：pending、group_collecting、waiting_order、
prechecked、media_pending、media_ready。processed/dispatched 即使残留空 processed_at，
也不能维持来源唤醒；failed 由既有的失败恢复路径按可恢复原因处理，不靠普通来源扫描反复轮询。

后续调度不再对来源执行 COUNT/MIN 聚合，只取最多 100 条必要字段，在内存计算下次等待。
Redis 候选子集没有查到主动事件时，允许一次同租户/账户/来源/频道范围的有界回退查询，
防止把“缓存候选都是旧终态”误判成“来源没有任务”。候选查询出错会保留错误，不以空页完成任务。
LIMIT 限制返回行数，不是扫描代价保证；上线仍需核对来源索引和实际执行计划。
