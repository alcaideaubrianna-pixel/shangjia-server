# 采集推送架构 V2：分阶段替换计划

目标：实时采集持续接收、可恢复、可观测；历史恢复和发布故障不能阻塞实时链路。所有替换必须保留旧路径回滚能力，直到存量任务和影子对账完成。

## 当前问题边界

当前链路是：Telegram Update → Collector Event → Delivery → Collect Event → 媒体/分组/规则 → Process Outbox → Dispatch/Review → Telegram Publish。实时、历史和发布任务虽然有部分队列名称隔离，但共享数据库、Redis、source 锁、进程资源和恢复周期。旧任务缺少统一分类标记时会进入兼容恢复路径；发布锁竞争会消耗共享资源。

线上验收基线：7 个代理 source 持续产生 Event；历史样本回放 4,371 条、83 个 source；线上曾观察到 recovery queue 18,000+、旧 Delivery urgent 500,000+、过期媒体 Outbox 和频道锁竞争。

## V2 状态机

每一阶段只做一次职责确认：

1. `ingest_spool.received`：Telegram 更新已写入共享持久化接收表，之后才确认更新。
2. `collector_event.received/processing/ready/dead`：仅负责原始事件去重和转换。
3. `delivery.pending/processing/completed/quarantined`：仅负责把事件交给业务适配器。
4. `collect_event.pending/group_collecting/media_pending/processed/failed`：仅负责资料处理状态。
5. `process_outbox.pending/processing/delivered/dead`：仅负责业务处理任务投递；`lane=realtime|recovery` 持久化保存，不从创建时间猜测。
6. `dispatch.pending/processing/sent/failed`：审核和发布使用独立状态；发布结果未知必须对账后再决定重试。

每个状态都必须有：主键幂等键、租约、attempt、next_run_at、last_error、first_seen_at、updated_at。任何消费者只能用条件更新领取，ACK 只在下一阶段成功后写入。

## 阶段替换

### Phase 0：观测和影子对账

保留现有生产路径，新增按 source/tenant/lane 的指标：最老任务、入队速率、消费速率、P50/P95、失败类型、租约过期、重复 ACK、审核生成速率。对实时和 recovery 做只读影子分类，不能改变线上路由。验收条件：连续 30 分钟指标稳定，实时最老年龄和 recovery 最老年龄可区分。

### Phase 1：持久化接收边界

账号和 Bot 更新统一写 `ingest_spool`，唯一键为 `tenant_id + source_type + account/bot + source_chat_id + update_id/message_id`。写入成功后返回更新成功；数据库暂不可用时由 Telegram 客户端重放，应用不在更新回调中无限等待。新增 spool consumer 转成现有 Collector Event；旧入口保留只读旁路直到对账一致。

删除门槛：连续 24 小时 spool→event 数量一致、重复率可解释、零未分类更新、断开重连测试通过。

### Phase 2：实时/恢复资源隔离

实时和 recovery 使用独立 Worker 进程、Redis queue group、数据库连接池和并发配置。实时固定最小并发和 DB 预算；recovery 使用剩余预算，按 source round-robin，每批上限和单次执行预算固定。单 source 锁等待或失败超阈值进入 source circuit，不影响其他 source。

删除门槛：连续 24 小时实时 P95、最老年龄、source 饥饿、DB pool 使用率达标；recovery backlog 单调下降。

### Phase 3：媒体和分组隔离

媒体下载、媒体指纹、分组等待、审核规则分为独立队列；媒体失败只阻塞所属资料，不阻塞 source 全窗口。所有等待状态有最大年龄和可重入补偿；已删除媒体的非核心索引失败不能阻断发送。

删除门槛：线上真实媒体样本回放、下载失败/OSS 超时/媒体删除竞态、分组跨页案例全部通过。

### Phase 4：发布隔离

发布 Outbox、频道锁、Telegram API 重试独立于采集 Worker 和 Redis group。频道锁使用固定租期、可观测 owner、释放保护；未知发送结果必须先对账。发布 backlog 不得共享实时采集 DB 预算。

删除门槛：频道锁竞争、Bot API 超时、进程终止、重复发送、延迟回执测试通过。

### Phase 5：删除旧路径

只在所有删除门槛达成后删除：旧 urgent/ready 消费者、旧同步入口、按创建时间兼容分类、进程内缓冲、全局 recovery 扫描和无状态 ACK。删除前保留一个版本可回滚镜像和旧队列消费者；旧队列为空且数据库状态已对账后再删 SQL/配置。

## 回滚

每个 Phase 使用独立 feature flag 和不可变镜像。回滚只切换消费者和 flag，不删除 Redis task、Outbox、Event 或媒体。新字段和索引保留。任何实时最老年龄连续增长、spool→event 不一致、重复发送、数据库连接池耗尽、租约过期激增，立即停止新消费者并回滚该 Phase。

## 线上测试要求

每个 Phase 必须同时通过：

- 线上脱敏事件/Delivery/媒体样本在临时 PostgreSQL + miniredis 的完整回放；
- 入口失败、Redis 断开、DB 超时、Worker 终止、租约过期、重复投递故障注入；
- source 分布保持线上长尾比例，不用等量合成数据；
- 真实 source/tenant lane 的最老任务和处理速率断言；
- 影子计数与生产只读 SQL 对账；
- race、全量 Go 测试和部署后 5/15/30 分钟分段验收。
