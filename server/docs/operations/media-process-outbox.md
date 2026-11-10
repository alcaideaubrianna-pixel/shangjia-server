# 媒体处理 Outbox 上线与验证

## 链路

媒体 INSERT / 重新置为 uploaded → 同事务触发器写 Outbox → media worker 每秒领取到期 Outbox → 固定 Asynq task ID → 入队 → 条件确认 sent。

媒体处理失败、进程退出、任务超时由 Asynq 原有机制重试和回收。Outbox 只重试投递，不重置媒体执行次数。旧的每分钟扫描媒体表恢复器已删除，应用启动不执行建表或媒体回填。

每次领取最多 50 条，使用到期部分索引及 FOR UPDATE SKIP LOCKED；投递预算 10 秒、租约 30 秒。异常退避，单次状态更新须匹配 generation 和 lease_token，迟到的旧持有者不能覆盖新请求。

任务 ID 在 Redis 入队前持久化。沿用 media-process:<id> 和 :repair:<n> 命名，与旧生产者/队列兼容。已归档或已完成的同一投递 ID 不会被自动复活；显式修复请求才产生新投递代次。完成任务保留七天，原归档记录不删除。

## 顺序（当前生产 PostgreSQL）

1. 手动执行 `addons/youban_publish/resource/sql/media_process_outbox.pgsql.sql`，例如在受保护的数据库节点用 `psql -X -v ON_ERROR_STOP=1 -f <file>`。该迁移原子安装表、函数和触发器，设有 3 秒锁超时，失败整体回滚，勿忽略失败继续部署。
2. 手动执行 `media_process_outbox_backfill.pgsql.sql`，每次最多补录 1000 条。先查看同条件样本与计划，核对现网 `idx_ybp_media_processing_recovery_v2` 存在且有效；每批核对插入数，未处理完可分批重复。不得变成运行时定时回填。
3. 构建并部署 media；新显式入队入口跟随调用它的应用升级。旧上传入口仍受数据库触发器保护。若同时发布本次日报和 FeiNiu 修复，还需 web、scheduler。不要改角色配置。
4. 观察 Outbox 到期数量和 last_error、Asynq 队列、媒体 ready/failed 及相关资料投影；不以仅有 readiness 200 为业务验收。

补录任务带 bootstrap 标志：已有旧 active/pending/retry/scheduled 任务时沿用原任务；旧任务全部归档或完成时记录 blocked 和原因，人工核对后再显式修复，不擅自重置次数。旧 archived generations 由原实现保留并连续编号；不要在迁移期间手工删除中间归档任务。

触发器只对 uploaded 的首次写入、状态变化或源文件变化记录意图，retry/processing/ready 更新不创建额外投递。显式入队与尚未投递的触发器记录合并。队列故障时媒体和待投递记录仍已持久化。

## 查询

```sql
SELECT status,count(*) FROM hg_youban_publish_media_process_outbox GROUP BY status;
SELECT media_id,generation,status,available_at,attempt_count,last_error
FROM hg_youban_publish_media_process_outbox
WHERE status IN ('pending','leased') AND available_at<=statement_timestamp()
ORDER BY available_at,media_id LIMIT 50;
SELECT media_id,generation,last_error FROM hg_youban_publish_media_process_outbox
WHERE status='blocked' ORDER BY media_id LIMIT 50;
```

聚合状态查询用于有界人工验收，不是运行时热路径。投递表保留每个媒体的最近意图，sent/blocked 不在到期索引内。PostgreSQL 之外的部署须先提供对应迁移及方言适配，本次迁移不能直接用于 MySQL。

## 本地验证

固定隔离 PostgreSQL 127.0.0.1:35432，Redis 127.0.0.1:16382 DB15，不连接生产，不启动业务消费者：

```sh
YOUBAN_LOCAL_MEDIA_OUTBOX=1 go test -race ./addons/youban_publish/logic/sys -run TestMediaProcess -count=1 -timeout=90s
```

覆盖真实迁移重复执行、事务回滚、重复入队合并、执行重试不产生新意图、并发领取、租约回收、陈旧持有者、Redis 接收但 DB 未确认、归档不复活、显式修复、Redis 失败退避、旧队列补录和 5 万已完成任务下的索引计划。

## 回滚

先回滚应用到原 Asynq 实现，再停用新 media dispatcher。Outbox 及触发器可暂时保留；不要删除媒体、队列或归档。回滚期间新增的 pending 意图仍持久化，恢复新版时会继续投递。不要清空 Outbox 后再重放媒体。
