# 采集来源独立队列

## 新执行路径

原始 Event、内部 CollectorDelivery 和资料处理使用三个队列命名空间；每个阶段都按 `tenant_id/source_id/realtime|history` 建立独立 Redis 待处理队列，不再先进入共享实时消息队列或按固定数量分片。

```text
Webhook → 原始 Event 持久化
        → 来源实时 Event 队列
        → 来源实时 CollectorDelivery 队列
        → 业务 collect_event 入库
        → 来源实时资料处理队列
        → 资料及 collect_dispatch handoff
        → 现有 Publish 引擎
```

`CollectorDelivery` 是内部采集交接，不是 Telegram 推送。它与原始 Event、资料处理均由现有 `collector-worker` 角色消费；Publish 继续拥有 dispatch 之后的发送执行。不会新增 SAE 角色。

每个命名空间使用活跃来源通知和有界执行池，不为每个来源常驻一个 Asynq Server。单次领取后来源持有带 token 的租约；ACK、NACK、续租都校验 token 和有效期。等待媒体或重试时释放执行槽。入队与来源激活原子完成；处理期间再收到同 ID 唤醒时递增版本，旧 ACK 不会删除新唤醒。

Realtime 与 History 使用独立队列、租约和执行预算。Realtime 不再因等待超过 120 秒降级历史。历史资料处理使用自身有界 worker 容量。内部 CollectorDelivery 遇到同来源实时队列非空时默认延期，可通过下文的来源级保底额度配置继续执行。资料历史单次业务执行上限仍为 5 秒，沿用跨通道的事件锁、窗口锁和配对写入锁，实际锁冲突或媒体依赖未就绪时保留重试。独立 worker 名额不等于数据库与锁资源完全隔离，应先保持低历史并发并观察实时延迟和 DB 压力。延迟任务、崩溃领取只通过有界 Redis 到期索引恢复，不通过枚举全部来源或全库 pending 查询来领取执行任务。现有来源级业务补偿仍保留其数据库幂等和范围约束。

## 配置与迁移

原始处理与内部交付沿用 `YOUBAN_TELEGRAM_COLLECTOR_CONCURRENCY`、`YOUBAN_TELEGRAM_COLLECTOR_HISTORY_CONCURRENCY`、`YOUBAN_TELEGRAM_DELIVERY_CONCURRENCY`、`YOUBAN_TELEGRAM_DELIVERY_HISTORY_CONCURRENCY`。资料处理池使用：

需要暂停历史执行时，把三个阶段的历史并发设为 `0`：`YOUBAN_TELEGRAM_COLLECTOR_HISTORY_CONCURRENCY`、`YOUBAN_TELEGRAM_DELIVERY_HISTORY_CONCURRENCY`、`YOUBAN_PUBLISH_COLLECT_SOURCE_HISTORY_CONCURRENCY`。历史任务保留，实时继续消费；来源级持久化补偿和 Redis 到期租约维护仍保留。旧资料队列迁移默认空名单，暂停历史时不要另行启用迁移名单。

| ENV | 默认值 | 用途 |
|---|---:|---|
| `YOUBAN_PUBLISH_COLLECT_SOURCE_REALTIME_CONCURRENCY` | 4 | 实时总执行槽 |
| `YOUBAN_PUBLISH_COLLECT_SOURCE_HISTORY_CONCURRENCY` | 1 | 历史总执行槽 |
| `YOUBAN_PUBLISH_COLLECT_SOURCE_LEGACY_CONCURRENCY` | 1 | 显式旧 Asynq 队列兼容预算 |
| `YOUBAN_PUBLISH_COLLECT_SOURCE_RUNTIME_LEASE_SECONDS` | 30 | 资料队列租约 |
| `YOUBAN_PUBLISH_COLLECT_SOURCE_RUNTIME_POLL_MS` | 200 | 空队列退避间隔 |
| `YOUBAN_PUBLISH_COLLECT_SOURCE_LEGACY_QUEUES` | 空列表 | 明确旧队列名单，不枚举全部来源 |

旧 Asynq 数据不删除、不清空；新生产者不再写旧共享队列。旧原始事件兼容读取只负责有界迁移到来源历史队列；动态来源旧资料队列通过明确队列名单或现有按来源原始消息重放入口推进，不在启动时全量查询 PG/Redis 回填。历史重放应指定租户、来源、事件范围和批次检查点，复用分类配对规则。

上线涉及现有 Web、Collector、Publish 三个角色，不新增角色：Web 包含原始消息生产者；Collector 消费新来源队列并拥有内部 CollectorDelivery；Publish 升级后退出旧内部 CollectorDelivery 消费，只负责发布。不能仅部署 Collector 后让旧 Web 持续投递旧共享实时队列，也不能长期保留旧 Publish 的交付恢复逻辑。先升级 Web 将新消息可靠写入新来源队列，再尽快升级 Collector 开始消费，并升级 Publish 收口旧内部交付消费者；切换窗口的新队列可能短时等待，需监控接收数、队列年龄和消费者 readiness。旧积压保留在历史边界，禁止直接清库。核验 `collector_worker_started`、内部交付和资料处理来源 Worker 启动日志，再选同一 webhook 样本检查 `event.enqueued → event.persisted → process.queued → 资料入库/明确终态 → dispatch handoff`。队列迁移成功、进程健康或发布 Job 日志不等于采集闭环成功。

Redis 使用既有项目 cache 适配器，不切换 endpoint、账号或逻辑 DB。持久业务事实仍在 PostgreSQL，Redis 持久化和高可用仍是运行前提；Redis 故障应保留可重试状态，不以全局 SQL 扫描代替队列。无需为队列引入新数据库表。

## 保留的业务边界

以下记录为既有业务语义，来源队列迁移不修改展示/验证分类、媒体组配对和审核规则。

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

### 历史交付保底和重放范围

| ENV | 所在角色 | 默认 / 范围 |
|---|---|---|
| `YOUBAN_TELEGRAM_DELIVERY_HISTORY_BUSY_SCOPES` | Collector | 默认空。逗号分隔的 `租户ID:来源ID`，例如 `25:78` |
| `YOUBAN_TELEGRAM_DELIVERY_HISTORY_BUSY_INTERVAL` | Collector | 明确设置 Go duration，范围 `1s`～`1m`。例如 `2s` 表示实时繁忙时，该来源最多每两秒开始一次历史交付 |
| `YOUBAN_COLLECTOR_REPLAY_BATCH_SIZE` | MCP 所调用的 Web API | 指定规则重放单次选择的原始事件上限，默认 200，范围 1～200 |
| `YOUBAN_COLLECTOR_REPLAY_MAX_MESSAGE_SPAN` | MCP 所调用的 Web API | 指定规则重放的消息号区间宽度，含两端，默认 200，范围 1～200 |

历史交付仍由历史 Worker 执行。保底额度由来源级 Redis 原子预留，扩 Collector 副本时不会倍增。普通实时领取和历史／实时交付共用的数据库身份、领取条件及终态保持规则沿用原行为。
