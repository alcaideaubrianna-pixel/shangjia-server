# 待入队任务恢复查询优化

## 故障与范围

2026-11-18 生产 Publish（镜像 `33addec`）的 `channel_queue_recovery` 连续达到约 8 秒步骤预算。频道 231 的终态误阻塞修复后，积压仍可能因恢复查询超时停止推进。

本次保持恢复条件、频道划分、候选上限、重试时间和发送保护不变，不增加全局扫描频率或调大超时。

## 修改

- `telegramPendingRecoveryPostgresSQL` 在频道候选去重阶段只读取 ID、租户、账号、频道、目标和创建时间。
- 候选与占用检查使用 MATERIALIZED CTE 明确执行边界，选定后才通过主键读取完整 Job。
- 可用 Bot/协议号、计划有效期和到期时间仍在频道候选 LIMIT 之前检查，避免不可用的候选遮挡同频道可执行任务。
- 继续按原频道维度去重，不改成先限制原始任务行数；否则一个大频道可能占满整批，影响其他频道。
- 频道执行保护保留非终态和未知结果检查，不因任务创建时间较新就忽略正在执行的任务。

配套 PostgreSQL 索引：

| 索引 | 用途 |
|---|---|
| `idx_ybp_tg_job_pending_recovery_head` | 仅索引 pending/failed_retry、idle、无队列任务 ID 的记录；顺序匹配频道 DISTINCT ON；包含恢复筛选字段 |
| `idx_ybp_tg_job_execution_blocker` | 仅索引真正可能占用频道的非终态记录；避免反复查询大量历史终态 |

执行文件：`server/addons/youban_publish/resource/sql/tg_pending_recovery_indexes.pgsql.sql`。必须在事务之外执行 CONCURRENTLY，并检查 `indisvalid / indisready`，不能把 IF NOT EXISTS 当成无效索引修复工具。

## 线上执行与验证

索引已在生产 `railway` 并发创建，均为 valid/ready：

- 候选索引约 7560kB。
- 占用索引约 16kB（随活跃任务变化）。
- 不修改 Job、Outbox、消息或资料状态。

旧执行计划为候选 Bitmap 合并扫描、宽行排序去重，加历史任务 Bitmap 占用查询。补索引后，有界只读 `EXPLAIN (ANALYZE, BUFFERS)` 样本：

| 查询 | 数据库耗时 | 主要访问 |
|---|---|---|
| 当前部署 SQL + 新索引 | 359.856ms | 候选部分索引 + 占用部分索引 |
| 窄行查询等价 SQL + 新索引 | 233.703ms | 候选 Index Only Scan + 占用部分索引 + 主键回表 |

样本在不同瞬间分别返回 15、16 个可恢复频道头，不应将其当作严格并发基准。候选索引仍读取约 4.16 万条待处理记录；此优化没有把全局工作集变成恒定大小。后续若积压规模继续增长，应另行设计持久化频道游标或活跃频道恢复入口，不能简单先截断原始任务行数。

生产 SLS：

- 10:22:59：恢复步骤仍有一次约 8.31 秒超时。
- 10:23:17：`channel_queue_recovery result=ok`，1358ms。
- 10:23:36：`channel_queue_recovery result=ok`，1498ms。
- 10:23:33：频道 231 为 208 pending、1 sending，较上次 210 pending 继续推进。

步骤耗时包含查询后的 Outbox 重投递，不等于数据库单条 SQL 耗时。上述成功轮次由索引带来，窄行代码尚待部署 Publish。另一个 `operation_state` 恢复步骤仍有约 8 秒超时，本次不宣称已解决。

## 验证限制与回滚

遵照用户要求，不编译、不运行单元测试或消费者；补充回归用例源文件并执行格式化、差异和独立静态 review。生产只读查询验证不写入业务数据。

代码可回退本次提交。两个索引可在确认负载与依赖后分别 `DROP INDEX CONCURRENTLY`；回滚索引会失去已验证的线上查询性能收益。不得为了回滚重置已完成任务。

## 后续：恢复批次的重复工作与预算

部署窄行查询后进一步定位到三个应用层问题，本轮代码修复如下：

1. **恢复投递统一入口。** `rearmTelegramJobOutbox` 对已有和新建 Outbox 都负责投递。待入队恢复器移除外层第二次 dispatch；延迟 intent 仍由 `available_at` 约束。未修改 stale Asynq 恢复中为绕过失效唯一键而显式新建任务的逻辑。
2. **取消恢复复用删除事实。** 每 job 最多读取一次已删除消息 ID；只有 confirmed、有响应、预期数量有效、完整 audit 且所有 ID 已持久化 deleted 的回执才跳过 capture。活跃/缺失消息、部分/未知回执走原路径，settle 的所有完成条件保留。重复等待提示不再反复更新同值。
3. **资料状态恢复减少无效工作。** 仅 failed_retry 需要判断恢复年龄时查询日志；推导出的 pending/publishing 与同一 operation 的扫描快照相同则不启动写事务。真正转换仍通过原权限/operation/取消 fence。
4. **主动让出预算。** 待入队处理在不足 1 秒时停止开启新项；资料状态扫描/处理给游标保存预留 500ms，不足 1 秒时保存并轮转游标。已过期 context 不再启动 deferred 旧失败扫描，且不延长父预算。单个已启动操作仍可能超时，本次没有扩大总超时。

新增或完善的日志：

- `channel_recovery_batch_finished`：候选扫描耗时、选中/处理数、错误数及 yielded。
- `profile.publish_state.recovery_summary`：游标、扫描、处理和总耗时，游标写入数及 yielded。
- `profile.publish_state.recovery_cursor / recovery_scan / recovery_item / recovery_yield`：细分定位；游标写入仅慢于 100ms 或出错时逐项记录，资料处理仅慢于 250ms 或出错时逐项记录，避免正常路径高频日志。
- `profile_cancel_receipts_reused`：每 job 跳过的已删除确认回执数。

这些优化未新增表、索引或环境变量；需部署 Publish 验证。保留逐行游标 checkpoint，未引入并发恢复或新的消费者。回归源码覆盖预算、相同状态跳过以及删除回执复用的边界；遵照禁令，未编译、未运行测试。
