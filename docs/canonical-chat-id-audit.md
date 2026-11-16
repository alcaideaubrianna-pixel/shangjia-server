# Canonical Telegram Chat ID 链路审查报告

审查范围：

- `bot_channel_cache`
- `collect_channel`
- `collect_channel_profile`
- `profile_origin`
- `collect_event`、`collect_dispatch`
- TG Job、TG Message、删除与对账链路

本次审查只读取代码和现有测试，未修改业务代码、未执行数据库迁移、未编译、未提交。

## 结论

当前代码已经具备两套 ID 处理方式：

- 频道实体和 Bot API 目标逐步使用完整 Chat ID：`-100...`；
- 采集资料归属、事件快照、历史查询仍允许保存或读取原始短 ID。

这种兼容策略本身可以支持历史数据，但目前缺少一个明确边界：哪些字段必须 canonical，哪些字段允许保留 raw。最严重的风险是：

```text
历史 collect_channel 只有短 ID
    ↓
ensureAccountCollectChannelRelation 只按 canonical ID 查询
    ↓
已有频道没有命中
    ↓
被误判为未绑定，或创建重复关系
```

## 已确认正确的地方

### 频道缓存写入

`bot_channel_cache.go:149` 的 `upsertCollectChannel` 已先调用：

```go
normalizeTelegramChannelChatID
```

并对历史短 ID做了以下处理：

- 已存在 canonical 记录：优先使用；
- 只有一条短 ID记录：原地改成 canonical；
- 多条历史别名：拒绝在线自动合并，要求受控迁移。

这条策略可以避免新的在线写入继续产生短 ID重复记录。

### 缓存查询

`channel_cache_dialog.go:410` 的 `tgChannelCacheLookupIds` 已支持：

```text
输入 3715905169
查询 -1003715905169、3715905169

输入 -1003715905169
查询 -1003715905169、3715905169
```

这适合只读兼容查询。

### 采集资料归属索引

`collect_channel_profile.go:28` 的 `indexCollectChannelProfileModel` 使用：

```go
canonicalCollectSourceChatID(sourceChatID)
```

因此新写入的 `channel_profile.channel_id` 会以短 ID canonical 形式保存。这里的 canonical 是内部资料来源键，和 Bot API `-100...` 不是同一种表示，不能直接与 `collect_channel.telegram_chat_id` 混用。

### TG 目标 Chat ID

资料上架和下架创建任务时，`target_chat_id` 已通过：

```go
normalizeTelegramChannelChatID
```

写入完整 Bot API Chat ID。发送和删除 Telegram 消息时也有多处再次规范化，整体方向正确。

## 必须修复的剩余调用点

### P0：采集授权关系回写只查 canonical，可能误判历史短 ID

文件：

```text
server/addons/youban_publish/logic/sys/telegram_collector_adapter.go
```

当前调用约在 `ensureAccountCollectChannelRelation` 内：

```go
Where(channelCols.TelegramChatId, chatID)
```

这里的 `chatID` 已经是 `normalizeTelegramChannelChatID` 的结果，但数据库可能仍只有：

```text
3715905169
```

因此已有频道会查不到。这个调用点必须改成：

```text
canonical 优先，短 ID 兼容
```

或者确保它只调用一个统一的 `findCollectChannelByChatID` 方法，由该方法内部使用 `tgChannelCacheLookupIds` 并明确 canonical 优先级。

必须补的测试：

```text
数据库只有短 ID collect_channel
→ 采集消息到达
→ 命中原 channel_id
→ 不新建 collect_channel
→ 不新建错误 account_channel
```

### P0：采集事件、分发记录和 TG Job 的来源快照没有统一写入 canonical

以下位置仍然直接写入原始值：

```text
collect_event.go:1039
collect_profile_dispatch.go:158
profile_publish_job.go:585
collect_source_trigger.go:136
collect_process_queue.go:500
collect_process_outbox.go:397
```

这些字段包括：

```text
source_chat_id
source_chat_id_snapshot
collect_source_chat_id
```

它们是来源审计和历史回查字段，最好统一保存 Bot API canonical ID；如果产品需要保留 Telegram 原始输入，应增加明确的 raw 语义字段，而不是让同一字段有时存短 ID、有时存 `-100...`。

至少必须保证：

```text
同一个 source/channel 的新事件、dispatch、TG Job 使用同一种格式
```

必须补的测试：

```text
输入短 ID
→ collect_event.source_chat_id
→ collect_dispatch.source_chat_id_snapshot
→ tg_job.collect_source_chat_id
三者格式一致
```

### P1：历史资料归属查询同时使用两种 canonical 规则，容易出现漏查

`collect_channel_profile.go` 当前存在两种表示：

- `channel_profile.channel_id` 使用 `canonicalCollectSourceChatID`，即去掉 `-100` 的短键；
- dispatch、TG Job、origin 证据使用原始 `source_chat_id`，查询时再通过 `CASE` 或 `REPLACE` 兼容。

这种做法可以工作，但查询条件分散，后续很容易出现一个新查询只使用 `=`，漏掉另一种格式。

必须补一个集中式测试矩阵，覆盖以下证据来源各自使用短 ID和 `-100...` 的组合：

```text
channel_profile
collect_dispatch
tg_job
profile_origin
```

期望 `collectChannelProfileIDs` 和 `loadProfileOrigins` 返回相同资料集合。

### P1：TG Job 恢复、删除和对账仍有手写 REPLACE 逻辑

已发现的调用点包括：

```text
tg_job_recovery.go:1109
record.go:302
tg_job_recovery_test.go:36
tg_job_recovery_test.go:138
```

当前逻辑使用：

```sql
REPLACE(target_chat_id, '-100', '')
```

这不是严格的 Telegram Chat ID canonical 比较，存在以下风险：

- 会替换中间出现的 `-100`；
- 对非频道 Chat ID 语义不清；
- SQL 和 Go 的规范化规则可能不一致；
- `NULL`、空字符串、用户名和异常值处理不一致。

建议后续统一为：

```text
Go 层生成 canonical key 后作为参数查询
```

或在数据库中建立明确的 canonical 字段/表达式索引。当前阶段至少要补测试，证明短 ID和 `-100...` 在恢复、去重、删除查询中等价。

### P1：直接删除接口保留了双值 OR 查询，需确认租户与频道唯一性

`ai_ops_telegram_direct_delete.go:33-37` 当前使用：

```sql
target_chat_id = canonical
OR target_chat_id = raw
```

这个兼容查询方向正确，但必须同时确保：

- `tenant_id` 已进入查询条件；
- 同一租户不会因为两条历史频道记录返回多行；
- 返回的 Bot Token 与目标频道属于同一账户。

应新增测试覆盖：

```text
同租户 canonical + raw 两条历史记录
→ 只选择 canonical 主记录

不同租户相同 raw/canonical
→ 不能互相命中
```

### P2：BOT 缓存和采集授权关系的职责边界需要测试固定

BOT 缓存只负责：

```text
频道名称、用户名、最近消息等展示信息
```

采集授权必须由：

```text
tenant + account + source + collect_channel.id
```

决定。

必须补回归测试，确认：

```text
BOT 缓存存在
但 account_channel 不存在
→ 不能自动放行采集

A source disabled
B source enabled
→ A 被阻断，B 仍可采集
```

## 测试缺口清单

当前 `tg_chat_id_test.go` 已覆盖：

- 短 ID与 `-100...` 的规范化；
- 关系键的 tenant/account/source 隔离；
- 不同频道不合并；
- 缓存查找返回两种 ID。

仍缺少：

1. `upsertCollectChannel` 只有短 ID历史记录时的原地规范化测试；
2. `upsertCollectChannel` 同时存在 canonical 和短 ID时的 canonical 优先测试；
3. `upsertCollectChannel` 多条历史别名时拒绝自动合并测试；
4. `ensureAccountCollectChannelRelation` 短 ID历史记录命中测试；
5. 事件 → dispatch → TG Job 来源字段格式一致测试；
6. TG Job 恢复/删除/对账短 ID和 canonical 等价测试；
7. profile origin 与 collect channel profile 证据集合一致测试；
8. 不同租户相同 Chat ID 不互相命中测试；
9. 普通群、用户名、空值和异常负数 ID不被错误加上 `-100` 测试。

## 处理优先级

上线前必须完成：

```text
P0-1  修复授权关系回写的短 ID历史记录误判
P0-2  统一来源快照字段的写入语义，并补链路测试
```

随后处理：

```text
P1-1  收敛 REPLACE/双值 OR 查询
P1-2  补齐 TG Job 删除、恢复和对账测试
P1-3  补齐跨租户和多历史别名测试
```

数据归并仍然应该独立于业务发布执行，先 preview，再按冲突组迁移，不应由在线 ingest 请求自动合并。
