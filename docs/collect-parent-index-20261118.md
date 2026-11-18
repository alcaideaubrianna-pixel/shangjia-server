# 父事件关联索引执行记录（2026-11-18）

## 环境与执行

生产数据库 `PostgreSQL_Xiaohuiji` / `railway`，主库；
SSH 节点 `sg-xiaohuiji-pgsql`。时间为 Asia/Shanghai，约 08:01–08:07。
仅创建一个非唯一索引，不更新、删除业务记录，不重发任务。

DBX 核对旧索引与执行计划后，先通过固定会话执行 CREATE INDEX CONCURRENTLY。
DBX 在 60 秒客户端上限处取消，留下 indisvalid=false/indisready=false 的产物。
确认没有正在运行的建索引进程后，通过 DBX DROP INDEX CONCURRENTLY 清除该产物；
没有重复创建、没有删除旧业务索引。

DBX 无法完成超过 60 秒的构建，因此改用已授权 SSH 后台执行相同 DDL，
随后全部回到 DBX 校验。执行前剩余磁盘约 44GB，没有其他并发建索引。
参数：lock_timeout=3s、statement_timeout=15min、maintenance_work_mem=128MB、
max_parallel_maintenance_workers=0。

索引 `idx_ybp_collect_event_parent_role_id`：
`(material_parent_event_id, material_role, id)`。
最终 DBX 确认 indisvalid=true、indisready=true，约 288MB，无剩余建索引任务。
不设状态或 role 部分条件，以支持父事件单查、批量关联、完成/忽略、回放重置、
审核及发送阶段对所有关联子事件的读取。

DDL 保存于
`server/addons/youban_publish/resource/sql/collect_event_parent_lookup.pgsql.sql`。
远端审计文件为 `/tmp/youban-collect-parent-index-20261118.sql`、
`/tmp/youban-collect-parent-index-20261118.log`。

## DBX 验证

实际执行验证均设置 statement_timeout=1500ms，先检查普通 EXPLAIN，
不将数据库执行耗时与 DBX 网络/工具耗时混用。原故障父事件为 19672778。

| 查询 | 结果 |
|---|---|
| 索引前：parent + verify + 非终态 | 27.016ms，1 行；925 次索引搜索，4306 hit/1 read |
| 索引后：同一原查询 | 1.348ms，1 行；1 次搜索，6 hit/2 read |
| parent + verify 的 EXISTS | 6.805ms；新索引 Index Only Scan；含约 6.7ms 磁盘读取 |
| 仅按 parent 查 id | 0.037ms，1 行；新索引 Index Only Scan |
| 两个 parent 的 IN 查询 | 3.892ms，2 行；新索引 Index Only Scan |
| 审核媒体真实字段投影，id IN 或 parent IN | 106.631ms，11 行；主键与新索引 BitmapOr，再按媒体 event_id 索引读取；约 102ms 是磁盘读取 |

缓存与磁盘负载不同，以上为有界样本，不是全量 P95 保证。
原日志中的取消可能发生在父 Context 剩余预算内，不能把批次 4.9 秒全部归于这条 SQL。

## 同类查询检查

- collect_event.go：忽略、完成配对验证事件，及资料回写判断的 parent+role EXISTS。
- collect_message.go：回放重置时按 parent 查子事件。
- collect_process_queue.go、collect_material_window.go：验证组关联查询。
- collect_review.go：主事件与关联子事件的批量媒体读取。
- tg_send_repo.go：验证发送阶段通过 parent 找采集事件，再关联媒体。
- ai_ops_repair_plan.go：修复计划中的 parent+role 相关子查询。

这些查询的父事件定位可复用本索引；未执行任何 UPDATE/DELETE 验证，
也不声称其余连接、过滤、排序都因此获得相同延迟。

另外，ai_ops_telegram_message.go 的媒体计数：
`em.event_id=? OR (e.material_parent_event_id=? AND e.material_role=?)`，
涉及媒体表与 LEFT JOIN 事件表的跨表 OR。普通 EXPLAIN 仍显示并行大范围
Hash Left Join 和百万级估算扫描，成本约 558936。本次未对其执行 ANALYZE，
未修改业务代码。后续应先形成根事件 ID 与验证子事件 ID 集合，再按 event_id
索引关联媒体，保留原统计与 LEFT JOIN 的缺失事件语义。

## 后续应用查询优化

经用户授权，AI 运维媒体计数已改为单条 SQL：先 MATERIALIZED 根事件 ID
与验证子事件 ID 的 UNION，再通过媒体 event_id 索引关联媒体，最后
LEFT JOIN 事件读取角色。根 ID 不依赖事件表存在，保留孤立媒体的原 display
统计；UNION 防止根事件自关联导致重复计数。NULL/非 verify 角色、verify 根
事件、无媒体和多个子事件的口径均保留。

DBX 在 1500ms 保护下对父事件 19672778 执行同形 SQL 的 EXPLAIN ANALYZE：
执行 3.993ms，ID 集合 2 行、关联媒体 5 行，33 shared hit/9 read；
使用 parent_role_id、media_event 和事件主键索引，以 Nested Loop 访问，
未再出现原计划的百万级并行 Hash Left Join。该样本不是全量吞吐保证，
也不替代未执行的应用编译及单元测试。

新增固定 localhost 临时表的 PostgreSQL 回归测试源码，覆盖 8 个场景，
与原查询核对统计结果；本轮遵守禁止编译和运行测试，未执行该测试。
独立静态审查未发现阻断项。该测试不覆盖 GoFrame 参数转换和结果映射。
此次应用修改需部署 web（AI 运维 HTTP 接口所在角色）后生效，
已完成的数据库索引不需要重复执行。

## 收尾与回退

索引在线即时生效；上述后续应用代码修改仍需部署。遵守禁止编译、运行测试的要求。
本次不修改其他 agent 的业务代码。
若确需回退，显式事务外执行：
`DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_parent_role_id`。
本次未执行最终索引回退。
