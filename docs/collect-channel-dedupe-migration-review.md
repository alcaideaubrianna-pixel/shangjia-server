# collect_channel 重复归并迁移审查

日期：2026-11-17

本报告审查 `server/tools/collect_channel_dedupe/` 下的预览和迁移脚本，范围包括：

- `hg_youban_publish_collect_rule_channel`
- `hg_youban_publish_collect_channel_profile`
- `hg_youban_publish_collect_channel_count`
- `hg_youban_publish_collect_channel_count_member`
- 与 `collect_channel`、`collect_account_channel`、`collect_bot_channel` 的引用迁移顺序和唯一键风险

本次只读审查，没有执行线上 SQL、没有修改线上数据、没有编译、没有提交业务代码。

## 结论

当前通用 `migrate_collect_channel_duplicates.sql` 不能直接用于线上已有重复频道关系的归并。原因不是事务保护不足，而是它还没有实现“关系级归并”。它目前遇到业务引用就整体拒绝，这能避免误删，但无法处理线上最常见的情况：短 Chat ID 和 `-100...` 两个 `collect_channel` 都被关系表和投影表引用。

迁移必须把两种标识严格分开：

```text
使用 collect_channel.id 的表：
- collect_bot_channel
- collect_account_channel
- collect_rule_channel

使用 Telegram Chat ID 文本的表：
- collect_channel_profile
- collect_channel_count
- collect_channel_count_member
```

不能把所有表的 `channel_id` 当作同一种字段处理。

## 当前唯一键和字段语义

### `collect_rule_channel`

当前 PostgreSQL 结构是：

```sql
PRIMARY KEY (id)
UNIQUE (rule_id, channel_id)
INDEX (tenant_id, account_id, rule_id)
```

`channel_id` 指向 `hg_youban_publish_collect_channel.id`，不是 Telegram Chat ID。

当前唯一键没有包含 `tenant_id`、`account_id`。这会产生两个风险：

1. 如果同一个 `rule_id` 在多个租户或账户之间复用，写入会发生跨租户/跨账户唯一键冲突。
2. 迁移时不能直接把旧 `channel_id` 更新为 survivor 的 `channel_id`，因为同一 `rule_id + survivor_id` 可能已经存在，必须先按租户、账户、规则和 canonical Chat ID分组去重。

不能在没有全库冲突统计的情况下直接删除旧索引并创建新索引。索引变更应单独作为结构迁移，先确认 `rule_id` 是否全局唯一，并处理历史重复行。

### `collect_channel_profile`

当前唯一键是：

```sql
UNIQUE (tenant_id, account_id, rule_id, channel_id, profile_id)
```

`channel_id` 是 `varchar`，保存 Telegram Chat ID 文本。它可能出现：

```text
3715905169
-1003715905169
```

所以归并时不能更新 `collect_channel.id`，而应按：

```text
tenant_id + account_id + rule_id + canonical_chat_id + profile_id
```

去重。冲突行不能直接覆盖，应先决定保留哪一行，再合并 `source_id`、投影标记、时间字段和软删除状态。

已有索引：

```sql
(tenant_id, account_id, rule_id, channel_id, updated_at)
```

它不支持按 canonical Chat ID高效归并。大表迁移不能直接对每行执行 `regexp_replace` 全表扫描，应先使用受限候选集合，或分批建立临时映射；如果长期需要 canonical 查询，应考虑增加规范化列和相应索引，而不是在实时请求中反复函数计算。

### `collect_channel_count`

当前唯一键是：

```sql
UNIQUE (tenant_id, account_id, rule_id, channel_id)
```

`channel_id` 也是 Telegram Chat ID 文本，`profile_count` 是投影值，不能简单把两行的 `profile_count` 相加。正确做法是：

1. 先归并 `collect_channel_profile`；
2. 按 canonical Chat ID重新统计有效资料；
3. 用重建结果更新 count；
4. 最后校验 `profile_count = count(distinct active profile_id)`。

直接把短 ID行的 `profile_count` 加到 `-100`行会重复计算，因为两个投影可能指向同一资料。

### `collect_channel_count_member`

当前唯一键是：

```sql
UNIQUE (tenant_id, account_id, rule_id, channel_id, profile_id)
```

`reference_count` 表示资料被多个规则或投影引用的次数，不能把两条同资料记录直接相加，除非已经确认它们属于不同规则引用。安全处理方式是：

1. 先按 canonical Chat ID、租户、账户、规则、资料分组；
2. 明确重复来源是否是别名重复，还是合法的多次引用；
3. 别名重复只保留一条逻辑成员记录；
4. 合法多引用才累加 `reference_count`；
5. 重新计算 `collect_channel_count.profile_count`。

### `collect_channel_profile` 与计数触发器

`collect_channel_counts.pgsql.sql` 为 profile 增删改注册了 count 触发器。迁移期间直接批量更新 profile 的 `channel_id` 可能触发大量计数更新，产生长事务、锁竞争和中间计数状态。

迁移不能依赖触发器自动完成所有修复。推荐：

- 迁移窗口内关闭或绕过批量触发器的重复计算逻辑；
- 分批迁移 profile；
- 每批提交后重建对应 count/member；
- 最后执行一次受限范围的校验和补偿重算。

如果当前实现无法安全绕过触发器，应先暂停相关后台投影任务，不能在实时采集高峰执行。

## 当前脚本发现的风险

### 1. 统一 refs 查询混淆了两类 channel_id

现有预览/迁移脚本把 `collect_channel_profile`、`collect_channel_count`、`collect_channel_count_member` 的 Chat ID 文本尝试转换成 bigint，再与 `collect_channel.id` 比较。这种比较没有语义依据：

```text
Chat ID 3715905169 ≠ collect_channel.id 3715905169
```

即使数值碰巧相同，也只是偶然相等，不能视为引用关系。

后果包括：

- 真正的 profile/count 引用被漏报；
- 错把 Chat ID 数字当成主键引用；
- 遇到非数字或空值时 cast 失败；
- 迁移可能错误判断“无引用”并软删除旧频道实体。

这些表必须单独按 `tenant_id + canonical_chat_id` 检查。

### 2. `rule_channel` 迁移存在唯一键冲突

把旧 `channel_id` 更新成 survivor 时，以下情况会冲突：

```text
旧关系：rule=338, channel=old_id
已有关系：rule=338, channel=survivor_id
```

由于唯一键是 `(rule_id, channel_id)`，必须先分组决定保留关系，再迁移或软删除重复关系。不能依赖 `ON CONFLICT` 随意覆盖，因为该表没有明确的状态字段可供安全合并。

### 3. `account_channel` 不能按“允许优先”自动合并

同一个：

```text
tenant + account + source + canonical_chat_id
```

可能同时有：

```text
enabled + approved
disabled + rejected
```

如果后者是用户手动禁用，自动选择 enabled 会恢复用户已经关闭的频道。只有能证明禁用关系是旧别名自动生成、且没有人工操作证据时，才可以自动合并；否则必须进入冲突清单。

### 4. count/member 不能直接相加

`count` 和 `count_member` 是派生投影。归并时直接 `UPDATE channel_id` 或直接累加会导致重复资料计数。应以 profile 明细为事实源，先完成 profile 归并，再重建投影。

### 5. 事务范围过大

对数千甚至数万条 profile 做一次事务会：

- 长时间持有行锁；
- 与采集触发器、后台投影互相等待；
- 增加 deadlock 和 statement timeout 风险；
- 让线上采集延迟扩大。

应按租户/频道组分批，每批限制行数并提交。单组仍必须在同一事务内完成关系冲突检查和映射切换，避免半迁移状态。

## 安全迁移顺序

### 阶段 0：只读预览

先生成每个重复组的稳定映射：

```text
tenant_id
canonical_chat_id
survivor_collect_channel_id
old_collect_channel_ids
```

同时输出：

```text
rule_channel 引用
account_channel 状态
bot_channel 引用
profile 行数
count 行数
count_member 行数
人工禁用证据
跨租户/跨账户唯一键冲突
```

没有完整预览结果时禁止 apply。

### 阶段 1：冻结并加锁

只锁定当前重复组，不锁整张大表：

- 获取 advisory lock，锁键包含 `tenant_id + canonical_chat_id`；
- 对参与迁移的 `collect_channel` 行 `FOR UPDATE`；
- 设置短 `lock_timeout` 和 `statement_timeout`；
- 迁移前确认没有新的别名关系在并发写入。

不能只使用全局 advisory lock 代替行锁，也不能在没有应用侧写入协调的情况下更新频道主记录。

### 阶段 2：建立临时映射表

映射表至少包含：

```text
tenant_id
canonical_chat_id
old_channel_id
survivor_channel_id
```

映射表必须有主键/唯一约束，并记录批次号。所有后续更新只允许通过映射表进行，禁止重新按模糊字符串 join 全表。

### 阶段 3：处理 `account_channel`

按以下键分组：

```text
tenant_id + account_id + source_id + canonical_chat_id
```

处理规则：

- 只有一条关系：更新 `channel_id` 到 survivor；
- 多条状态完全一致：保留审计信息更完整的一条，其他软删除/归档；
- 存在人工 disabled/rejected：保留人工阻断，进入审计记录；
- 允许与阻断冲突但无法证明来源：该组中止，不影响其他组；
- 不同 source：绝不合并；
- 不同 account/tenant：绝不合并。

### 阶段 4：处理 `bot_channel`

按：

```text
tenant_id + bot_id + survivor_channel_id
```

去重。频道名称、用户名、最近消息时间应选择信息更完整或更新时间更新的一行，不能无条件使用旧行覆盖新行。

### 阶段 5：处理 `rule_channel`

先按：

```text
tenant_id + account_id + rule_id + canonical_chat_id
```

分组。冲突时保留规则配置完整、更新时间最新且属于当前租户/账户的关系。由于旧唯一键只有 `(rule_id, channel_id)`，处理前必须验证：

- 不存在另一租户/账户占用同一 `rule_id + survivor_channel_id`；
- 不存在同规则不同 owner 的历史共享；
- 需要时先扩展唯一约束，或安排停写窗口后重建索引。

不能在关系未去重前更新 survivor `channel_id`。

### 阶段 6：处理 `collect_channel_profile`

按 canonical Chat ID迁移文本字段：

```text
tenant_id + account_id + rule_id + canonical_chat_id + profile_id
```

先合并重复逻辑行，再更新到 canonical `-100...`。`source_id` 不得被任意覆盖；如果同一逻辑资料来自多个 source，应按现有业务规则保留来源或拆分处理。

建议按 `id` 分页批处理，避免按 `regexp_replace(channel_id)` 扫描整张大表。

### 阶段 7：重建 count/member

不要直接迁移或相加 count。顺序应是：

1. 以已归并的 profile 明细重建 member；
2. 按去重后的 member 计算 `profile_count`；
3. 更新 `ready`、`last_seen_at`、`updated_at`；
4. 校验 count 与 profile 明细一致。

如果必须保留旧投影行，应先写入 canonical key，再确认唯一键无冲突后删除旧别名行。

### 阶段 8：规范化 `collect_channel`

所有引用迁移并校验通过后：

- survivor 写成完整 `-100...` canonical Chat ID；
- 旧记录软删除；
- 不删除仍被历史审计需要的原始缓存；
- 检查租户边界和唯一索引。

### 阶段 9：最终校验

至少执行：

```text
同租户同 canonical Chat ID只有一个活动 collect_channel
同 tenant/account/source/canonical 只有一条活动 account_channel
同 tenant/account/rule/canonical 只有一条活动 rule_channel
profile 的 channel_id 全部为 canonical 格式
count/member 与 profile 明细一致
不存在跨租户 channel 引用
不存在重复的 survivor 唯一键
```

## 建议的脚本改造方向

当前通用 `--apply` 应继续保持拒绝有引用重复组的行为，直到关系级迁移实现完成。下一版应拆成：

```text
preview：只读，输出迁移计划和冲突
plan：只读，生成带批次号的映射/动作清单
apply：只接受已审核的 plan_id，逐组、分批、可恢复执行
verify：只读，验证唯一键、引用和计数一致性
```

`apply` 不应接受一个“全库自动合并”开关。每个组应具备：

```text
survivor_id
动作类型
授权决策
是否允许迁移
冲突原因
```

只有 `allowed_to_apply=true` 的组才可执行，人工禁用冲突必须保留为待处理状态。
