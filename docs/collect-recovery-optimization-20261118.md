# 采集来源恢复优化（2026-11-18）

## 证据与范围

生产环境：SAE `771ee3e`，PostgreSQL `sg-xiaohuiji-pgsql` / `railway`。
本轮仅优化来源范围内恢复查询，不批量重置资料、不重新发送、不清理队列。

普通事件恢复样本 `tenant=23/account=469/source=95`：原索引按来源消息排序，
简化为仅事件表的查询仍需 1019.281ms，过滤 4564 行，读取 3956 个块，
其中读取耗时 988.206ms。LIMIT 2 没有限制排序前的扫描量。

BOT 媒体恢复样本 `87:628:1237`：完整查询在独立 1500ms 保护下超时。
普通 EXPLAIN 显示 BitmapOr、媒体半连接和最终排序。现有来源媒体恢复索引、
event_id/cache_status/next_retry_at 索引均已存在，不重复创建。
只读候选方案先固定 12 条，再校验来源和媒体，执行 6.586ms，结果为空；
该空页不代表整个来源没有待恢复任务，必须通过游标访问后续候选。

## 已执行的生产索引

显式迁移文件：
`server/addons/youban_publish/resource/sql/collect_event_scope_active_recovery.pgsql.sql`。

创建前确认：主库、剩余磁盘约 43GB、目标索引不存在、没有其他并发建索引。
使用 CREATE INDEX CONCURRENTLY，lock_timeout=3s、statement_timeout=15min、
maintenance_work_mem=128MB、max_parallel_maintenance_workers=0。
未更新或删除业务记录，未创建其他索引。

完成后：`idx_ybp_collect_event_scope_active_recovery` 为 valid/ready，约 86MB。
原完整查询已使用新索引，无需排序；同一来源、有结果的样本执行 0.586ms，
shared hit=13/read=1，返回 2 行。与修改前的简化查询不完全相同，
不将该对比当作所有来源的延迟保证。

SLS 07:11:51–07:13:51（Asia/Shanghai）检索
`"读取来源采集恢复事件失败"` 返回 Complete、0 条。仅是短窗口观察。

远端审计文件：
`/tmp/youban-collect-active-index-20261118.sql`、
`/tmp/youban-collect-active-index-20261118.log`。

## 代码优化与验收边界

BOT 媒体恢复已改为先按来源读取有界候选，随后仅校验候选的媒体与权限。
每页最多 12 条，一次调用最多 50 条候选（调用方传更小配额时沿用该配额），
不再读取 raw_text/media_json 等宽字段，保留原 5 秒总工作预算。
Redis 游标保留微秒精度，按 updated_at/id 推进，固定每轮时间上界，
跳过不合格候选并在扫完后回绕；数据库或入队异常不确认失败候选的进度。
来源级租约串行化游标写入，保存前刷新并验证所有权；
损坏的缓存调度提示会在持锁后记录警告并重置，不因 Redis 读取失败而重置。
保留原有来源轮转、恢复去重与账号 0 兼容校验。

结构化事件 `collect.recovery.bot_media_page` 包含来源范围、候选、合格、
入队、预留去重跳过数、游标、固定时间上界、回绕、失败和耗时；
未新增高基数 Metrics 标签。新增八个隔离测试函数的源码覆盖跨过不合格前缀、
入队失败保留位置、微秒精度、回绕、候选边界、损坏提示与游标回退校验。

独立静态审查未发现提交阻断项。保留的限制：不合格候选在完整来源扫描回绕后
才会重试；大积压会延长重试间隔；永久入队错误仍需单独处理，不能靠跳过失败
位置丢弃重试。账号 0 的来源兼容判断仍保留，现有入队层不接受账号 0 的行为
本轮未改变。租约检查与缓存保存沿用分步方式；极端长进程暂停后仍存在游标提示
被旧持有者覆盖的可能，业务幂等不能依赖该游标。

本轮继续遵守禁止编译、运行测试的要求；仅允许 gofmt、静态审查和差异检查。
代码部署后仍需核对候选/合格/入队数、游标推进、超时与实际媒体恢复。
按 `771ee3e` 的运行时边界，Collector 与 Scheduler 均可启动 runCollectRecovery，
需更新 `collector`、`scheduler` 角色，避免保留旧恢复查询。
数据库索引已独立生效，不能将本地代码完成当作已部署。

## 回退

仅在确认需要时，事务外执行：
`DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_scope_active_recovery`。
本轮未执行回退。代码回退须使用对应优化提交，不能清空恢复队列代替回退。
