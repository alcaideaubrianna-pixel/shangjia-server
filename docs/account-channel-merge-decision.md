# account_channel 历史关系归并决策

本文只描述历史 `hg_youban_publish_collect_account_channel` 关系归并的只读审查结论和决策规则。它不执行 SQL，也不授权自动修改线上数据。

## 关系的真实作用域

一条采集授权关系的业务唯一键不是单独的 `channel_id`，而是：

```text
tenant_id + account_id + source_id + canonical_chat_id
```

其中：

- `channel_id` 是 `hg_youban_publish_collect_channel.id`，属于内部频道实体主键；
- `canonical_chat_id` 是频道实体的 `telegram_chat_id` 去掉可选的 `-100` 前缀后的比较值；
- `source_id` 必须保留，不能把同一个账户下的多个采集源合并；
- 不同租户、不同账户、不同 source 的关系永远不能互相覆盖。

频道实体归并和授权关系归并是两个不同动作：

```text
tenant_id + canonical_chat_id
    → 归并 collect_channel 实体

tenant_id + account_id + source_id + canonical_chat_id
    → 归并 account_channel 授权关系
```

## 当前字段能证明什么

当前表字段为：

```text
collect_enabled
status
approval_status
approved_at
disabled_at
disabled_reason
created_at
updated_at
deleted_at
```

字段只能提供以下证据：

| 证据 | 可以说明 | 不能说明 |
|---|---|---|
| `status=disabled` | 当前关系被阻断 | 一定是用户手动禁用 |
| `approval_status=rejected` | 当前审批结果为拒绝 | 一定是人工拒绝 |
| `collect_enabled=0` | 当前不允许采集 | 阻断来源是频道、source、审批还是历史迁移 |
| `disabled_at IS NOT NULL` | 关系曾进入禁用时间点 | 谁操作、为什么操作 |
| `disabled_reason` 非空 | 有较强的阻断原因证据 | 原因是否仍然适用于当前 source |
| `approved_at IS NOT NULL` | 曾经有通过时间 | 当前允许状态是否仍有效 |
| `created_at/updated_at` | 可用于时间线排序 | 不能作为人工操作证明 |
| `deleted_at IS NULL` | 当前关系仍是活动记录 | 不能证明状态正确 |

因此，不能使用下面这些条件单独认定“人工禁用”：

```text
status=disabled
approval_status=rejected
disabled_at IS NOT NULL
updated_at 较新
```

## 关系分类

迁移预览应把每条关系分类为以下四类之一。

### 明确人工阻断

只有满足以下至少一项时，才可以标记为 `manual_block`：

```text
disabled_reason 非空，且内容明确表示管理员/用户操作；
disabled_reason 非空，且对应业务原因是明确的人工下架、拒绝或频道禁用；
存在独立操作审计记录，能通过 relation_id 或业务唯一键关联到操作人；
```

当前已知的高可信原因包括：

```text
管理员手动禁用采集频道
频道删除并下架
```

这类关系具有最高优先级。即使同一 source/canonical channel 下另有 `enabled + approved`，也不能自动恢复，必须进入 `manual_decision_conflict`。

### 自动生成的允许关系

只能在满足全部条件时标记为 `auto_allow`：

```text
collect_enabled=1
status=enabled
approval_status=approved
disabled_reason 为空
没有人工阻断审计证据
```

这类关系可以作为默认允许的候选，但不能覆盖明确人工阻断。

### 自动生成的阻断关系

以下情况只能标记为 `auto_block`，不能当作人工禁用：

```text
source 当前暂停或关闭时自动生成的 disabled 关系；
关系创建时 source 本身不可用，导致 collect_enabled=0；
历史迁移根据 source 状态写入 disabled/rejected，但没有人工原因；
disabled_at 有值但 disabled_reason 为空，且没有操作审计；
```

这类关系表示当时的运行条件，不足以证明用户希望永久禁用。归并时要保留其阻断事实，同时报告为 `automatic_block_evidence`，不能直接与人工禁用等价。

### 未知或冲突关系

以下情况必须标记为 `unknown` 或 `manual_decision_conflict`：

```text
同一组同时存在 auto_allow 和 manual_block；
只有 disabled_at，没有原因和审计；
approval_status 与 status/collect_enabled 互相矛盾；
同一关系多次状态变化，但当前表无法还原操作者；
多个 enabled + approved 关系的来源时间线无法确定；
```

未知关系不得自动选择允许。迁移程序应停止该组，输出完整样本。

## 合并优先级

对同一组：

```text
tenant + account + source + canonical_chat
```

采用以下优先级：

```text
manual_block
  >
manual_reject
  >
manual_pending
  >
auto_block
  >
auto_allow
```

这里的优先级表示“迁移保护优先级”，不是把所有阻断都永久写成 disabled。

具体决策如下：

| 关系组合 | 迁移决策 | 是否自动执行 |
|---|---|---|
| 只有一条关系 | 只迁移 `channel_id` 到 survivor | 可以 |
| 多条 auto_allow，状态等价 | 合并成一条允许关系 | 可以 |
| 多条 auto_block，状态等价 | 合并成一条阻断关系 | 可以 |
| auto_allow + auto_block | 保留阻断并报告时间线 | 禁止，需确认 |
| auto_allow + manual_block | 保留人工阻断 | 禁止，需确认 |
| manual_block + manual_block，原因一致 | 合并并保留最完整原因 | 仅在审计字段一致时可以 |
| manual_block + manual_block，原因不同 | 不合并 | 禁止 |
| enabled + approved + rejected | 不看字段字面覆盖 | 禁止，需确认 |
| pending + auto_allow | 允许关系可保留，但记录 pending 历史 | 仅在无人工证据时可以 |
| pending + manual_block | 保留人工阻断 | 禁止 |
| 多条关系均无法分类 | 不迁移 | 禁止 |

## 当前实现暴露出的风险

### `disabled_reason` 不是完整审计

当前代码在多个入口直接写入状态：

- source 审批会写 `disabled_at`，但没有稳定的 `disabled_reason`；
- BOT 首次发现会自动创建 `enabled + approved`；
- 账户采集首次绑定会根据 source 状态创建 enabled 或 disabled；
- 频道清理会写入明确的 `disabled_reason`；
- 规则/频道操作路径存在只写状态、未统一写操作来源的逻辑。

所以迁移不能把 `approval_status=rejected` 直接等同于人工拒绝。

### 现有自动创建关系的状态组合不统一

历史代码可能生成以下组合：

```text
enabled + approved + collect_enabled=1
disabled + approved + collect_enabled=0
disabled + rejected + collect_enabled=0
enabled + rejected + collect_enabled=1
pending + pending
```

其中后两类是矛盾状态，必须进入 `unknown`，不能通过迁移“修正”为允许。

### 关系表的唯一键仍按内部 channel_id

当前唯一键是：

```sql
(tenant_id, account_id, channel_id, source_id)
```

因此短 Chat ID 和 `-100...` 会通过不同的 `collect_channel.id` 形成两条关系。归并顺序必须是：

```text
先确定 survivor collect_channel
再按业务唯一键分组 account_channel
再处理关系冲突
最后迁移/软删除旧 channel
```

不能先更新 `collect_channel` 或直接删除旧记录。

## 只读预览必须输出的字段

每个重复关系组至少输出：

```text
tenant_id
account_id
source_id
canonical_chat_id
survivor_channel_id
relation_id
old_channel_id
collect_enabled
status
approval_status
approved_at
disabled_at
disabled_reason
created_at
updated_at
deleted_at
classification
decision
blocking_reason
```

建议的 `classification` 值：

```text
manual_block
manual_reject
manual_pending
auto_allow
auto_block
unknown
```

建议的 `decision` 值：

```text
merge_keep_allow
merge_keep_block
keep_manual_block
manual_review
skip
```

## 迁移前的硬性阻断条件

以下任一条件成立时，当前组不得自动迁移：

```text
存在 manual_block 或 manual_reject；
存在 auto_allow + 任意阻断关系；
status、approval_status、collect_enabled 不一致；
无法确认 disabled_reason 的来源；
source_id 缺失或为 0；
发现跨租户 channel 引用；
发现同一 source 的多个活动关系仍无法稳定排序；
关联 rule_channel、bot_channel、profile/count 投影无法生成迁移计划；
```

迁移工具应按组拒绝，而不是因为一个冲突组让全库 SQL 静默选择一个状态。全局事务可以回滚，但预览必须先把可迁移组和不可迁移组分开，不能把“可迁移”误报成“已迁移”。

## 对当前线上问题的判断

对于：

```text
3715905169       enabled + approved
-1003715905169   disabled + rejected
```

仅凭现有字段不能断言第二条是人工禁用。正确分类应是：

```text
第一条：auto_allow
第二条：unknown_block
```

如果第二条的 `disabled_reason` 明确是用户禁用或存在操作审计，升级为：

```text
manual_block
```

在没有该证据前，不能自动删除第二条，也不能自动把第一条覆盖回第二条。该组应进入：

```text
manual_decision_conflict
```

代码层可以继续采用“有效关系优先”的读取兼容逻辑来止血，但数据迁移必须等待关系级预览和人工确认。

## 后续实现建议

为了让后续迁移可以可靠分类，建议新写入链路增加统一的决策来源字段或审计事件，例如：

```text
decision_source = auto_discovery
decision_source = source_pause
decision_source = operator_approve
decision_source = operator_reject
decision_source = operator_disable
decision_source = repair_migration
```

如果不增加字段，至少必须把每次状态变更写入已有审计日志，并携带：

```text
relation_id
tenant_id
account_id
source_id
canonical_chat_id
before_status
after_status
operator_id
reason
```

在此之前，迁移工具必须采用“无法证明就阻断”的策略，不能用字段猜测人工意图。
