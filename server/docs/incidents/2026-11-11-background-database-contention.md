# 后台扫描与数据库竞争修复

后续媒体架构重构已替换本文记录的临时有界恢复扫描：媒体事务 Outbox → Asynq，旧媒体扫描器已删除。下文扫描/索引结果保留作为事故定位和临时止血记录；最终上线流程与验证见 [媒体处理 Outbox](../operations/media-process-outbox.md)。

## 已确认的原因

- 生产媒体表约 603 万行；恢复扫描需要 uploaded/retry/processing，但旧部分索引包含 uploaded/processing/failed，缺少 retry。生产 EXPLAIN 为 Parallel Seq Scan，现场扫描超过 4 分钟。
- 日报生成、投递、清理共用 50 秒上下文；前序超时后清理仍执行并覆盖前序错误。日报表约 262 行，无相关外键或业务触发器。
- FeiNiu 多个业务入口反复运行安装 SQL，包括 ALTER TABLE IF NOT EXISTS，触发表锁超时。

## 修改

- 新索引 idx_ybp_media_processing_recovery_v2，覆盖 uploaded/retry/processing；旧索引保留，避免影响其他查询。CONCURRENTLY 必须单独执行，不能放在事务中。失败后须检查 indisvalid/indisready，不能把同名无效索引当作成功。
- 媒体恢复各状态独立索引扫描，分别 LIMIT 200，再合并按 updated_at/id 取最早 200 条；查询及入队周期受 15 秒上下文约束。
- 日报生成/投递/清理分别预算 30/15/5 秒，共用原始父上下文，父取消仍向下传播；保留最早错误，同时报告清理失败。
- 日报 cron 使用 55 秒固定 Redis 租约，非阻塞竞争，避免多个调度实例同时扫描；Redis 故障返回错误而非无锁降级。线上 Web 的 web,runtime 配置会展开 scheduler，另有独立 scheduler；本次不修改角色配置。
- FeiNiu 每进程仅成功初始化一次，串行初始化、失败可重试，不缓存失败结果；配置定时任务同步保持原行为。

## 验证

本地隔离 PostgreSQL 127.0.0.1:35432：

```sh
YOUBAN_LOCAL_COLLECT_PG=1 go test ./addons/youban_publish/logic/sys -run 'Test(DailyReport|MediaProcess)' -count=3
go test -race ./addons/youban_feiniu_sync/logic/sys -count=1
go test ./addons/youban_publish/install -count=1
YOUBAN_LOCAL_REPORT_REDIS=1 go test -race ./addons/youban_publish/crons -run TestDailyReportLeaseLocalRedis -count=1 -timeout=30s
YOUBAN_LOCAL_COLLECT_PG=1 go test ./addons/youban_publish/logic/sys -run TestDailyReportStageTimeoutLocalPostgres -count=1 -timeout=60s
```

媒体专项构造 5 万条 ready、900 条混合待恢复状态、未来/已删除/终态数据，验证最早 200 条、三种状态、排除条件与索引计划。日报夹具补齐当前统一任务批次表及频道标题字段。FeiNiu 覆盖初始化失败后重试、50 个并发调用仅一次成功初始化、取消上下文。

真实 PostgreSQL ACCESS EXCLUSIVE 锁阻塞日报账号表，30 秒生成超时后仍成功删除过期日报，返回“读取日报账号失败”而非清理错误。本地 Redis 127.0.0.1:16382 DB15 验证 20 个竞争者不重复执行，持有者失败后下一次调用可成功获取租约。

## 线上操作及边界

已发起并发索引创建；必须待 pg_stat_progress_create_index 完成并确认 indisvalid/indisready 后，以真实查询验证计划。仅取消 SQL 精确匹配且运行超过 30 秒的媒体恢复 SELECT，未删除任务或修改媒体数据。

首轮 CREATE INDEX CONCURRENTLY 在校验阶段触及 15 分钟 statement_timeout，留下 indisready=true、indisvalid=false 的索引；已经确认停止后执行 REINDEX INDEX CONCURRENTLY，同一会话 statement_timeout 调整为 60 分钟、lock_timeout 为 3 秒。维护会话设置不改变应用连接的数据库参数。重建完成前不能宣称索引生效。

重建已完成，新索引 indisvalid=true、indisready=true。线上 EXPLAIN (ANALYZE, BUFFERS) 实测：旧恢复 SQL 改走新索引，Execution Time 0.140ms；新版分状态 LIMIT 查询走三路 Index Only Scan，Heap Fetches=0，Execution Time 0.076ms。两次实际候选均为 0，不能据此声称积压吞吐或接口延迟达到相同水平。

重建遗留 idx_ybp_media_processing_recovery_v2_ccold 为无效索引，DROP INDEX CONCURRENTLY 的 3 秒 lock_timeout 触发，未强行锁表；保留有效新索引。清理遗留索引和确认其状态需另行记录，不得删除有效新索引。

随后定位到索引生效前已经启动、仍使用旧计划的媒体恢复 SELECT，按精确 SQL 范围取消后，DROP INDEX CONCURRENTLY 成功清理 ccold。未删除业务行；下一轮恢复扫描可重新规划使用有效新索引。

代码尚需构建部署。媒体恢复代码归 media，日报及 FeiNiu 按当前启用相应运行角色的应用部署；不要仅根据应用名称推断运行职责。本修复不代表已消除全部慢查询；日报候选扫描仍需结合负载复验。
