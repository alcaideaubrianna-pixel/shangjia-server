# 循环调度分片迁移

循环计划分为两类：按天循环使用 `hg_youban_publish_tg_job.next_cycle_at`，批次循环使用 `hg_youban_publish_channel.cycle_next_run_at`。两类计划使用同一套 64 分片编号，但各自保留独立游标、租约和执行状态。

## 迁移顺序

1. 在生产 PostgreSQL 执行 `resource/sql/manual/20261107_cycle_scheduler_shards.pgsql.sql`。
2. 校验两个分片字段都在 `0..63`，并统计每个分片的到期数量。
3. 部署包含分片查询的 publish/后台角色。
4. 观察每个分片的领取、生成 `cycle_batch` 和发送完成速率。
5. 确认连续两个调度周期没有全局扫描后，删除旧的全局扫描入口。

迁移脚本只新增字段和索引，并回填分片，不修改 `next_cycle_at`、频道游标或资料状态；出现异常时可以回滚应用版本，字段保留也不影响旧代码。

## 线上验证

```sql
SELECT cycle_scheduler_shard, count(*)
FROM hg_youban_publish_tg_job
WHERE cycle_enabled=1 AND next_cycle_at IS NOT NULL AND next_cycle_at<=NOW()
GROUP BY cycle_scheduler_shard ORDER BY cycle_scheduler_shard;

SELECT cycle_scheduler_shard, count(*)
FROM hg_youban_publish_channel
WHERE cycle_publish_enabled=1 AND cycle_next_run_at<=NOW() AND deleted_at IS NULL
GROUP BY cycle_scheduler_shard ORDER BY cycle_scheduler_shard;
```

单个租户或账户的积压只能占用所属分片预算。分片 worker 领取时必须使用条件更新和租约，不能恢复全局 `ORDER BY next_cycle_at LIMIT ...` 扫描。
