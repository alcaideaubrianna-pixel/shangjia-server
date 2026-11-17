# 采集来源并发入口与多规则显式迁移

同一个 Bot 的匹配来源在单次调用内最多使用 4 个 worker；不会按来源数量创建 goroutine。每个输入来源最多调用一次，正常情况下恰一次。来源 error/panic 单独汇总，不取消其他来源；错误按输入顺序返回，保留 errors.Is 关联。租户/Bot 匹配、source 维度事件 ID 和重放幂等键保持不变。

共享 ctx 取消时，未开始的来源不再执行，按来源返回取消错误；已经开始的回调仍需自行遵守同一 ctx。函数等待已启动回调退出，不遗留后台 worker，也不将取消伪报成功。并发上限是每次 fan-out 调用的上限，不是进程全局限流；4 个都不响应取消的阻塞回调仍会占满本次 worker，因此不替代下游操作的超时设置。

## 数据库问题

旧 `uk_ybp_collect_source_single_rule UNIQUE(source_id)` 限制一个来源只能绑定一条规则。新代码允许多规则不代表线上旧索引已删除；不要为绕过唯一冲突删除关系、重置禁用或改成绑定别的来源。

独立脚本：

`server/addons/youban_publish/resource/sql/migrations/20261117_collect_source_multi_rule.pgsql.sql`

已于 2026-11-17 通过 DBX 在生产连接 `PostgreSQL_Xiaohuiji` 执行，结果见文末。目标固定为 `public.hg_youban_publish_collect_source_rule`。若部署 schema 不是 public，需要另行审核和调整，不能依赖隐式 search_path。

## 执行与保护

由运维在核对目标数据库后显式执行，使用已有受保护连接配置，不把密码写入命令：

```bash
psql -X -v ON_ERROR_STOP=1 \
  -f server/addons/youban_publish/resource/sql/migrations/20261117_collect_source_multi_rule.pgsql.sql
```

整个迁移在单个事务内，使用非等待 advisory lock、2 秒 lock_timeout、15 秒 statement_timeout、30 秒 idle-in-transaction 超时。会短暂获取目标关系表的 ACCESS EXCLUSIVE 锁；繁忙时超时回滚，停止后重选低流量时段，不持续自动重试。锁内只有系统目录查询和删除索引/约束，不读取业务关系行，也不建索引。

迁移顺序：

1. 核对 source_id/rule_id 字段及已有有效、立即生效、非部分、非表达式的 `UNIQUE(source_id,rule_id)` 索引；名称可以不同。
2. 若双列唯一索引不存在或失效，立即拒绝，不自动建索引/扫描大表。先单独评估并创建，再重新执行。
3. 核对旧索引确实属于目标表，且只对 source_id 唯一；异常结构拒绝处理。
4. 若旧索引由 UNIQUE constraint 管理，按实际 constraint 名执行 DROP CONSTRAINT；独立索引执行 DROP INDEX。
5. 所有删除均 RESTRICT，不使用 CASCADE。外键或其他依赖阻止删除时整笔回滚，需要单独评估依赖。
6. 如果仍有其他名字的 source-only 唯一索引，整笔回滚并报告，不擅自删除未知索引。
7. 确认双列唯一保护仍然存在，输出最多 50 行系统目录报告后提交。

重复运行时，旧索引已不存在且双列唯一保护正常会直接通过。脚本不 DELETE/UPDATE 任何绑定、不调整 status/tenant/account/source、不触发历史回放。成功仅解除旧单规则结构限制，不代表自动恢复以前失败的绑定或补发资料。

迁移失败会整体回滚；成功后不能盲目重新创建 UNIQUE(source_id) 回退，因为后续合法多规则关系可能已存在。需要应用回退时应先评估多规则数据，不能通过删关系来恢复旧约束。

## 验证范围

父 agent 使用独立本机 PostgreSQL 16 容器验证了：

- 独立旧索引、约束管理旧索引两种结构均可迁移，重复执行幂等。
- 同来源可以增加第二条规则，重复 source/rule 仍被双列唯一索引拒绝。
- 原有 status=2 的绑定保持禁用，不因迁移重置。
- 旧索引定义异常时拒绝执行，事务回滚并保留索引。

本地测试不读取生产连接、不启动业务消费者。

## 生产执行记录

2026-11-17，按用户指示通过 DBX 执行。连接 `PostgreSQL_Xiaohuiji`，
数据库 `railway`，用户 `dbuser_dba`。将脚本中的 psql 专用指令移除后，
在同一 DBX 批次执行 BEGIN、超时设置、受保护 DO、目录报告及 COMMIT。
事务正常提交；随后独立查询在北京时间 08:04:25 验证：

- `public.uk_ybp_collect_source_single_rule` 已不存在。
- `public.uk_ybp_collect_source_rule` 仍为有效、ready、立即生效的双列唯一索引。
- 目标表主键和两个普通索引保留。
- 精确读取的来源绑定前后一致：1203 / tenant 87 / source 457 / rule 449 / status 1；
  1202 / tenant 87 / source 487 / rule 476 / status 1。

迁移仅修改结构，没有 UPDATE/DELETE 业务绑定，没有改变人工禁用状态。
没有部署应用、重放历史消息或创建测试业务绑定。结构验证不代表采集链路已经在线恢复。
