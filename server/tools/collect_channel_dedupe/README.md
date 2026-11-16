# collect_channel 关系级重复归并

这个工具用于把同一租户下的短 Chat ID 和 Bot API `-100...` Chat ID 归并为一个频道实体。迁移是**单组、短事务、失败回滚**，不会执行全库长事务。

## 预览

```bash
DATABASE_URL='postgres://...' \
./server/tools/collect_channel_dedupe/run.sh --preview
```

## 执行单组迁移

当前迁移 SQL 不允许无参数执行，也不通过 `run.sh --apply` 执行全库迁移。必须先从预览结果确认一组，再直接调用 psql：

```bash
COLLECT_CHANNEL_DEDUPE_CONFIRM=I_UNDERSTAND_COLLECT_CHANNEL_MERGE \
psql "$DATABASE_URL" \
  -v tenant_id=78 \
  -v canonical_chat_id='3715905169' \
  -v confirm='I_UNDERSTAND_COLLECT_CHANNEL_MERGE' \
  -f server/tools/collect_channel_dedupe/migrate_collect_channel_duplicates.sql
```

`canonical_chat_id` 可以传 `3715905169` 或 `-1003715905169`，SQL 会统一比较为同一组。该命令只处理一个 `tenant_id + canonical_chat_id`，每组单独执行、单独校验。

> 这个文件只实现迁移 SQL 和使用说明；不会自动连接线上数据库，也不会自动执行。

## 迁移规则

### 频道实体

- 只在同一个 `tenant_id + canonical_chat_id` 内合并；
- 活跃记录优先，canonical `-100...` 优先，其次按更新时间、ID稳定选择 survivor；
- 旧 `collect_channel` 记录软删除。

### account_channel

关系键按以下维度判断同一组：

```text
tenant_id + account_id + source_id + canonical_chat_id
```

同组按 `updated_at DESC, id DESC` 选择最新活动关系，旧关系软删除，winner 指向 survivor。不同租户、账户、source 永远不会合并。最新关系为 `disabled/rejected` 时会原样保留，不会被旧的 enabled 关系恢复。

### bot_channel

按以下维度合并：

```text
tenant_id + bot_id + canonical_chat_id
```

同组按 `updated_at DESC, id DESC` 选择 winner，旧关系软删除，winner 指向 survivor。BOT 汇总关系不参与采集授权判断。

### rule_channel

`rule_channel` 冲突是硬失败条件：同一租户、账户、规则在旧频道和 survivor 上同时存在关系时，整组事务回滚，不自动猜测 suffix 或规则状态。无冲突时才迁移到 survivor。

### profile/count/member

- `channel_profile` 按 `tenant_id + account_id + rule_id + canonical_chat_id + profile_id` 去重，保留 `updated_at` 最新记录，旧记录软删除；
- 活跃 profile 的 Chat ID 统一为 `-100...`；
- `channel_count` 和 `channel_count_member` 先删除该 canonical 组投影，再从去重后的活跃 profile 明细重建；
- count 不做简单相加；
- 每个 profile 只在对应的 `tenant/account/rule` 下计数一次。

## 安全保护

- 必须提供 `tenant_id` 和 `canonical_chat_id`；
- 单组事务；
- `pg_advisory_xact_lock` 使用 tenant + canonical Chat ID 独立锁；
- `lock_timeout=5s`；
- `statement_timeout=60s`；
- `rule_channel` 冲突整组回滚；
- 任何唯一键、引用或最终一致性校验失败都会回滚；
- 不跨租户、不跨账户、不跨 source 合并；
- 不修改 BOT 缓存表；BOT 缓存只用于频道展示和发现记录。

## 迁移前必须检查

1. 保存对应组的 preview 输出；
2. 确认最新 `account_channel` 状态确实是最终状态；
3. 确认 `rule_channel` 没有冲突；
4. 确认 profile/count/member 是当前需要重建的投影；
5. 迁移完成后重新执行 preview 和一致性检查。

不要对全库直接执行 SQL。必须逐组执行，并在每组成功后记录 tenant、canonical Chat ID、winner relation 和执行时间。
