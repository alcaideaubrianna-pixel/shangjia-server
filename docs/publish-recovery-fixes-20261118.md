# 发布恢复异常修复记录（2026-11-18）

环境：生产，Asia/Shanghai；PostgreSQL 节点 `sg-xiaohuiji-pgsql`，数据库 `railway`。排查起点 Publish 镜像 `9652d3d`。本记录不包含凭据、完整 ENV 或媒体地址。

## Outbox 公平扫描返回零值身份

SLS 样本：每 30 秒扫描约 810 条记录，却只选择 1 条，`cursorFromId=0`、`cursorToId=0`；随后出现 `jobId=0`、`outboxId=0`、`TG任务ID不能为空`。

扫描候选使用包含未导出匿名嵌入记录的结构体。数据库反射扫描没有正确恢复嵌入身份字段，导致公平分片键都变为零。修复为复用现有字段转换器，显式读取同一次 SQL 返回的行；在保存游标前校验 Outbox、Job、租户、频道和游标字段。分发入口另设非零身份保护。SQL、扫描上限、任务去重和业务状态机均不变。

已有错误游标会按原有校验规则自动重新开始扫描。不需要清空 Redis、重置任务或重新发送已经确认的消息。

## 采集交付恢复查询被取消

SLS 样本：来源 `25:0:78` 的来源级恢复查询在约 1 秒预算内被取消。后续也观察到其他来源出现同类查询取消。

生产检查发现，仓库已有的来源／pool 恢复索引尚未执行。原计划使用 BitmapOr，再过滤租户、pool、到期条件并排序；来源恢复代码因此仍可能反复访问大量不合格候选。

本次已在主库事务外执行：

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tg_delivery_scope_pool_recovery
ON public.hg_tg_collector_delivery
    (tenant_id, account_id, source_id, scheduler_pool, priority DESC, created_at, id)
WHERE status IN ('pending', 'failed_retry');
```

执行设置：`lock_timeout=3s`、`statement_timeout=15min`、`maintenance_work_mem=128MB`、`max_parallel_maintenance_workers=0`。没有更新或删除业务行。完成后 `indisvalid=true`、`indisready=true`，索引约 3640 kB。

有界只读验证，范围 `25:0:78/realtime`：

- 原恢复 SQL 已使用新索引；空结果样本执行时间 1.343ms。
- 新候选读取 SQL 使用同一索引；空结果样本执行时间 0.034ms。
- 以上不是有任务时的吞吐或全租户延迟保证。
- 06:09:58–06:12:58 SLS 查询 `collect_delivery_recovery AND "canceling statement"` 为 Complete、0 条。

代码继续保留每来源／pool 的扫描配额与 1 秒外层预算，将到期与尝试次数判断移到有界候选读取之后。使用持久游标跨过尚不可执行的候选，保留固定创建时间上界与原租约所有权校验，不增加全局扫描。

本次只创建上述查询所需索引；同一历史迁移文件中的 processing 租约索引不在本次已执行范围内。

## 发布权限恢复查询

另外观察到 `permission_backlog` 在约 5 秒后取消：原查询从 Job 历史表全局寻找符合权限暂停条件的任务。改为从小页权限 circuit 领取租户／频道范围，再读取有界 Job 候选；安全条件仍在操作前复核，不删除已有发送证据的任务。

复用生产已有 `idx_ybp_tg_job_tenant_channel_status`。范围 `tenant_id=2/channel_id=118/status=pending` 的候选 SQL 经普通 EXPLAIN 确认后，执行有界 EXPLAIN ANALYZE：Index Only Scan，空结果样本 0.098ms、4 个 shared buffer hit、0 次 heap fetch。无需为该修复增加索引；代码运行时表现仍待部署后验证。

## 验证与发布边界

本次遵守禁止编译／测试的限制：仅进行格式、差异和静态审查；新增回归测试源码未运行。数据库索引已在线生效，代码修复仍须部署到相应角色后，通过 SLS 复查公平扫描的有效 ID、游标前进及恢复错误率。

索引回退命令为 `DROP INDEX CONCURRENTLY public.idx_tg_delivery_scope_pool_recovery`；仅在确认需要回退时执行，本次未执行。代码可回退对应修复提交，但回退将恢复已定位的扫描／解码问题。
