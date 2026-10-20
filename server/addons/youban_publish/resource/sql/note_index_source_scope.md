# 采集资料列表来源筛选迁移

旧查询对租户约 40 万条索引逐条关联 profile_state/content_profile 后才过滤来源，空结果也可能耗时 30 秒。
新查询使用 note_index.is_collected，从租户/账号、来源标记、创建时间组合索引直接定位候选项。
原有资料、所属账号、发布状态的软删除和权限检查仍然保留。

## PostgreSQL 部署顺序

1. 执行 `note_index_source_scope.pgsql.sql`。DDL 设置 3 秒锁等待，失败时整个事务回滚，可稍后重试。
2. 按主键游标分批回填，推荐每批 5000 行。每批独立提交；禁止单个大事务更新全表。
   记录返回的 cursor，作为下一批 `:after_id`；cursor 为 NULL 表示完成。

```sql
WITH batch AS MATERIALIZED (
    SELECT id FROM hg_youban_publish_note_index
    WHERE id > :after_id ORDER BY id LIMIT 5000
), changed AS (
    UPDATE hg_youban_publish_note_index i
    SET is_collected = COALESCE(p.source_type = 'youban_collect', false)
    FROM batch b, hg_content_profile p
    WHERE i.id = b.id AND p.id = i.profile_id
      AND i.is_collected IS DISTINCT FROM COALESCE(p.source_type = 'youban_collect', false)
    RETURNING i.id
)
SELECT (SELECT max(id) FROM batch) AS cursor,
       (SELECT count(*) FROM changed) AS changed;
```

3. 逐条、自动提交执行 `note_index_source_scope_indexes.pgsql.sql`；`CONCURRENTLY` 不可放在事务中。
   如创建失败，先检查同名索引的 indisvalid；无效索引不能被 IF NOT EXISTS 自动修复。
4. 校验下面的 mismatch 必须为 0，并确认新增索引均 `indisvalid=true`。

```sql
SELECT count(*) AS mismatch
FROM hg_youban_publish_note_index i
JOIN hg_content_profile p ON p.id = i.profile_id
WHERE i.is_collected IS DISTINCT FROM COALESCE(p.source_type = 'youban_collect', false);
```

5. 更新统计信息 `ANALYZE hg_youban_publish_note_index (tenant_id,account_id,is_collected,created_at);`。
6. 部署 Web 查询代码；其他角色只负责同步投影，兼容触发器允许它们逐步升级，不需要重启 Account。

触发器兼容旧版本写入，且 source_type 修改时同步所有相关索引行。它们是正确性保障，不能在仍有旧写入端时删除。
普通状态更新、发布时间更新不会触发新增来源查找；INSERT 和显式修改 profile_id/is_collected 才查一次资料主键。

## 回滚与限制

应用可回滚到旧版本，新增字段、索引和触发器应保留；旧查询仍可工作，但可能恢复原有慢查询行为。
本改动不修改资料来源、不删除数据、不触发重新上架，也不改变发送队列。
线上 PostgreSQL 已验证；MySQL 新装表含字段、应用会写入标记，但已有 MySQL 安装仍需对应字段迁移及回填，不能直接只升级应用。

## 本次验证记录（2026-10-20）

- 线上回填修正 185312 条投影；完整关联校验 mismatch=0。
- 两个列表组合索引均有效，各约 39MB；来源覆盖索引约 29MB。
- 原慢查询的完整权限参数，新 SQL 实测 0.201ms（数据库执行时间，空结果）；原历史日志为 30574.648ms。
- 有采集资料的账号，旧/新 SQL 分别 2.766ms / 0.845ms，21 条结果 ID 和顺序一致。
- 本地隔离 PostgreSQL 集成测试：迁移重复执行、历史回填、采集/手动/全部、租户和账号隔离、资料/状态/索引软删除、旧版本 INSERT、来源类型变更、资料引用替换。
- 本地 10 万条数据验证空采集列表走组合索引，不逐条查询 profile/state。
- 专项 Go 回归和 git diff --check 通过；应用尚未部署，未验证登录后的端到端 HTTP 延迟。

### 其他参数补充验证

- `publishedAt + collected` 原索引过滤 405485 行，执行 1173.746ms；补建 tenant/account 来源发布时间索引后，同一数据库查询执行 0.032ms。
- 手动来源创建时间排序：0.077ms；采集来源 + 状态：0.214ms。
- 大账号城市无匹配：301.080ms；审核状态无匹配：162.128ms。仍扫描 139547 行，仅证明这些样本低于一秒，不能外推所有筛选组合。
- 补充样本：省份 441.658ms、可见性 0.085ms、标签无匹配 172.724ms、标题/正文关键词无匹配 8.797ms、采集来源 618.077ms。关键词/标签为诊断谓词样本，不代表完整业务分词及多标签组合均已验收。来源条件仍有大量关联探测，是下一步优化对象。
- 上述补充样本为读取 note_index 的执行计划测试，不包含完整权限关联、媒体补全、缓存和网络时间。前面的完整关联查询结果另列。
- `tools/note_list_probe.py` 为只读 HTTP 探针，默认连续测试来源、账号范围、两种排序及无匹配关键词；可用 `--cases-file` 提供真实城市、标签、来源和游标等参数。使用现有登录 Token 文件，不输出凭据或资料正文。
- 例：`python3 tools/note_list_probe.py --token-file /path/to/existing-token --repeat 3`。必须在优化 Web 版本部署后验证成功响应、数量、P95 和最大耗时；401 或仅 SQL 变快均不算端到端验收通过。
