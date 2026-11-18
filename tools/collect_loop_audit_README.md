# 采集回流只读审计

`collect_loop_audit.py` 仅通过用户指定 SSH 节点的本机 PostgreSQL 导出 ID 证据，不写业务数据，不调用 MCP apply，不启用来源或重放，不读取正文。单一持久 SSH/psql 连接，数据库强制只读，statement_timeout=3s、lock_timeout=500ms、禁用查询内并行。每条数据查询先 EXPLAIN；大表 Seq Scan 或总计划成本超限即停止并保存失败。channel/source/account 小表允许已检查计划的顺序读取。

```sh
python3 tools/collect_loop_audit.py --priority-only
python3 tools/collect_loop_audit.py --exclude-manifest /path/to/already-handed-off/manifest.jsonl
```

默认按 tenant/account/chat 索引扫描 origin，复合游标 `(profile_id,id)` 分页；每轮串行执行包含最多 24 个独立范围的 UNION ALL。profile/event 按 PK 批量读取，全部 origins 按 profile 索引批量读取。包括已软删 account/source 的现存小表行及软删 target 的证据。短号和 `-100` 两种精确 ID 表示分别查询，不做相似度推断。

结果写入 `tmp/collect-loop-audit-20261118/run-时间/`，目录 0700、文件 0600。`manifest.jsonl` 为证据快照，`queries.jsonl` 保存 SQL 与 EXPLAIN，`summary.json` 包含失败与覆盖范围。执行中可用独立离线分类器生成固定交付快照：

```sh
python3 tools/collect_loop_audit_classify.py /path/manifest.jsonl /path/new-output-directory \
  --exclude-manifest /path/already-handed-off/manifest.jsonl
```

只使用 `time-verified/`（或离线分类器输出目录）交付安全 ID：

- `safe-scope-ids.json`：按 tenant/account 分组去重，排除已交付 ID 和 834927。
- `safe-tenant*-account*-batch*.json`：每批最多 50 个 profile 的证据。
- `timeUncertain.json`：event 缺失、目标创建更晚、目标在事件前已软删、时间缺失等。
- `mixedOrMultipleOrigins.json`：混合来源或多 origin，保守交人工复核。
- `otherEvidenceUncertain.json`：其他不一致。

安全条件：event/origin 的 tenant、account、source、message 和规范化实际 chat 一致；origin 为 collect/event 且仅有一个 origin；同租户启用 up target 必须满足 `created_at <= event.created_at AND (deleted_at IS NULL OR deleted_at >= event.created_at)`。这是存储配置在事件时刻的存在证据，无法证明历史 status/direction 曾经如何变更；没有这些字段的历史版本。原始事件时间口径按主 agent 采用的 `event.created_at`，不把 profile 后续创建时间当成采集时间。

可选补查（默认不运行）：`--dispatch-only --dispatch-source 351`，按 `(source_id,profile_id,event_id)` 索引分页 dispatch，再 PK 查询 profile/event；不扫描全局 event。省略 `--dispatch-source` 会补查所有小表已知来源，运行时间可能较长。缺失 origin 的 dispatch 证据隔离人工复核，不自动删除。

范围限制：默认 origin 路径不能恢复没有 origin 的现存 profile；account 和 source 都已物理删除的历史 scope 无法仅靠小表发现；没有全站一致性快照，生产并发删除/新增可能使结果变化；本工具不证明 Telegram 消息已删除。旧 run 的 safe/confirmed 命名若没有时间分类，禁止直接自动删除。
