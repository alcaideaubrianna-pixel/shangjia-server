# 采集 source 关系迁移手册

## 迁移前只读检查

```sql
SELECT tenant_id, account_id, channel_id, COUNT(*) AS relation_count,
       COUNT(DISTINCT source_id) AS source_count
FROM hg_youban_publish_collect_account_channel
WHERE deleted_at IS NULL
GROUP BY tenant_id, account_id, channel_id
HAVING COUNT(*) > 1;

SELECT id, tenant_id, account_id, channel_id, source_id,
       collect_enabled, status, approval_status
FROM hg_youban_publish_collect_account_channel
WHERE deleted_at IS NULL AND source_id <= 0;
```

当前线上检查结果为：没有重复频道关系，存在 3 条 `source_id=0` 遗留关系。它们没有可匹配的 active source，迁移时必须保留原记录，不得自动绑定。

## 执行顺序

1. 先备份 `hg_youban_publish_collect_account_channel`。
2. 执行 `upgrade.pgsql.sql` 或 `upgrade_online.pgsql.sql`。
3. 确认新唯一索引存在：

```sql
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename='hg_youban_publish_collect_account_channel'
  AND indexname='uk_ybp_collect_account_channel_source';
```

4. 部署 `publish`。
5. 部署 `collector`。
6. 用 AI 运维接口预览目标频道的消息区间，再执行重放。

## 回滚

应用回滚只需要恢复旧镜像。数据库回滚必须先确认没有新增同频道多 source 关系，再删除新索引并恢复旧唯一索引。禁止直接删除关系数据。

## 验收

- 同一频道的两个 source 可以分别创建订阅关系；
- 禁用一个 source 不影响另一个 source；
- `tg_collector_event` 和 `tg_collector_delivery` 数量按 source 分开；
- 相册消息按 `media_group_id` 聚合后再进入规则处理；
- AI 重放支持明确事件 ID、消息区间和频道上限；
- 3 条 `source_id=0` 关系仍存在且未被自动归属。
