# 5. 数据模型与索引

## 300 万资料表

循环扫描必须使用覆盖索引，至少具备：

```sql
(tenant_id, account_id, enabled, next_run_at, id)
(tenant_id, account_id, id)
```

实际索引名称和字段以生产表结构为准，使用 `CREATE INDEX CONCURRENTLY`，先 DBX `EXPLAIN` 再执行。

## 必须避免的查询

- `SELECT *` 读取采集事件；
- 无租户、账户、来源条件的 COUNT/聚合；
- 按频道对全量 Job 做历史自关联；
- 账号任务使用大范围 `OR`；
- 每条消息同步写运行日志表；
- 高频更新 Bot 频道统计累计触发 WAL。

计数和状态展示优先使用增量计数表或 Redis 统计，定期低频对账；事实表仍保留完整状态和审计记录。
