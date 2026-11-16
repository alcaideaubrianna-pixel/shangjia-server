# collect_channel 关系级重复预览

这个工具只用于审查历史 `collect_channel` 重复记录，不执行线上迁移。它把短 Chat ID 和 Bot API 的 `-100...` Chat ID 按 canonical 值归为同一个频道组。

canonical 分组键：

```text
tenant_id + regexp_replace(trim(telegram_chat_id), '^-100', '')
```

不会跨租户合并，也不会把 BOT 缓存记录当成采集授权。

## 运行

```bash
DATABASE_URL='postgres://...' \
./server/tools/collect_channel_dedupe/run.sh --preview
```

`run.sh --preview` 只执行 `preview_collect_channel_duplicates.sql`。本次预览 SQL 只包含查询和 psql 输出控制命令，不执行 `UPDATE`、`DELETE`、`INSERT`、`ALTER` 或 `DROP`。不需要、也不应设置迁移确认变量。

## 输出内容

预览按重复 canonical 频道组输出：

1. **重复组决策摘要**

   每组输出：

   - `survivor_channel_id`：按未软删、完整 `-100...`、资料完整度、较小 ID选择的候选主记录；
   - `old_channel_ids`：需要迁移引用的旧记录；
   - `account_relation_count`、允许关系数、阻断/人工关系数；
   - BOT、规则、资料索引、计数投影引用标记；
   - `merge_decision`：`safe_auto_merge` 或 `manual_conflict`。

2. **频道实体明细**

   输出 survivor 和旧 `collect_channel`，包括 Chat ID、名称、用户名、状态、软删除时间。

3. **account_channel 明细**

   输出完整关系键：

   ```text
   tenant_id + account_id + source_id + canonical_chat_id
   ```

   同时输出 `relation_id`、`channel_id`、`collect_enabled`、`status`、`approval_status`、`approved_at`、`disabled_at`、`disabled_reason`、创建和更新时间。这样可以看到同一 source 下是否存在历史别名关系，以及是否有人工禁用证据。

4. **BOT 关系引用**

   输出 `bot_id`、BOT 频道关系 ID、状态和首次/最近发现时间。BOT 关系只用于汇总和展示，不决定采集授权。

5. **规则关系引用**

   输出 `rule_id`、账户、规则关系 ID和引用的旧频道实体。

6. **资料与计数投影引用**

   `channel_profile`、`channel_count`、`channel_count_member` 保存 Telegram Chat ID 文本，不是 `collect_channel.id`。预览按 tenant + canonical Chat ID 统计，避免漏掉短 ID与 `-100...` 别名。

7. **汇总**

   输出重复组总数、关系层面可安全处理的组数和需要人工确认的组数。

## 决策含义

`safe_auto_merge` 只表示关系层面没有发现人工状态冲突，也没有旧频道上的 BOT、account 或 rule 引用阻塞。它仍然需要迁移脚本再次检查唯一键、资料/计数投影和数据完整性，不能把预览结果当成已迁移。

`manual_conflict` 表示至少存在以下一种情况：

- `disabled_at` 或 `disabled_reason` 表明有人工禁用证据；
- 同一 canonical 组同时存在允许和阻断关系；
- 旧频道实体仍有 `bot_channel`、`account_channel` 或 `rule_channel` 引用，归并会触发唯一键或状态合并；
- 需要人工确认规则、账户或 source 的关系归并方式。

人工禁用优先于自动默认放行。预览不会猜测哪个状态应该覆盖另一个状态，也不会修改线上数据。

## 与迁移脚本的关系

`migrate_collect_channel_duplicates.sql` 仍然是受保护的历史工具，不能用来绕过关系级审查。执行任何迁移前必须保存本次 preview 输出，并逐组确认：

- survivor 频道实体；
- account_channel 的状态合并结果；
- BOT 和 rule 引用迁移结果；
- profile/count 投影是否需要重建；
- 是否存在跨租户或悬空引用。

本次范围不包含迁移执行、线上 SQL、业务代码、编译或提交。
