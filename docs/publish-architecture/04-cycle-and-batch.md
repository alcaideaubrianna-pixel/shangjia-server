# 4. 循环上架与批次上架

## 三层模型

```text
cycle_plan（规则） -> cycle_run（一次批次） -> tg_job（资料×频道任务）
```

每分钟只扫描到期计划，但扫描必须按租户/账户分片：

```sql
WHERE tenant_id = ?
  AND account_id = ?
  AND enabled = true
  AND next_run_at <= now()
ORDER BY next_run_at, id
LIMIT ?
```

使用 `id` 或 `profile_id` 游标分页生成任务，不能按页码扫描 300 万资料表。每批 100～500 条，`cycle_run` 持久化 `cursor_id、queued_count、sent_count、failed_count`。

循环和批次都复用统一的 Job 创建接口，区别只体现在 `run_type`：`cycle、manual_batch、collected、recovery`。生成任务必须幂等，唯一键至少包含 `run_id、profile_id、channel_id`。

资料已下架、已删除、审核拦截或目标频道失效时，循环生成阶段直接记录跳过原因，不生成发送 Job。`publish_at` 只能按规则覆盖有效时间，不能无条件清空。
