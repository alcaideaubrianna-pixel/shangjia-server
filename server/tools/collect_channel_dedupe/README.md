# collect_channel 重复归并工具

这个工具用于审计历史 `collect_channel` 记录中，短 Chat ID 与 `-100` Chat ID 形成的重复记录。

## 判断规则

项目现有 canonical 规则是去除 Telegram channel Chat ID 的 `-100` 前缀，因此脚本按以下键分组：

```text
tenant_id + regexp_replace(trim(telegram_chat_id), '^-100', '')
```

脚本不会按全局 Chat ID 合并租户数据，也不会跨租户迁移关系。

## 只读预览

必须显式提供数据库连接串：

```bash
DATABASE_URL='postgres://...' ./run.sh --preview
```

预览输出八部分：

1. 重复 canonical channel 分组及候选记录；
2. 每条重复记录在 bot/channel、account/channel、rule/channel、channel/profile 中的引用数；
3. 同一 `tenant + account + source + canonical channel` 下的授权冲突；
4. 迁移阻塞统计。
5. 计数投影（`collect_channel_count` / `count_member`）中的 Chat ID 别名。
6. `rule_channel` 当前唯一索引无法表达租户/账户作用域时的所有者冲突。
7. 没有外键时的悬空引用统计。
8. 计数投影中的 Chat ID 别名和重复组阻塞情况。

默认模式就是 preview。脚本不读取项目默认配置，也不连接线上数据库，除非调用者明确传入线上 `DATABASE_URL`。

## 迁移保护

当前 `--apply` 只允许处理**没有任何业务引用的重复记录**。只要重复记录仍被以下任一表引用，脚本就会报错并回滚：

- `hg_youban_publish_collect_bot_channel`
- `hg_youban_publish_collect_account_channel`
- `hg_youban_publish_collect_rule_channel`
- `hg_youban_publish_collect_channel_profile`
- `hg_youban_publish_collect_channel_count`
- `hg_youban_publish_collect_channel_count_member`

执行需要双重显式确认：

```bash
COLLECT_CHANNEL_DEDUPE_CONFIRM=I_UNDERSTAND_COLLECT_CHANNEL_MERGE \
DATABASE_URL='postgres://...' ./run.sh --apply
```

迁移会在事务内获取 advisory lock，设置 5 秒锁等待和 60 秒语句超时；选择优先级为：未软删记录、完整 `-100` 记录、较小主键。无引用时只会把保留记录规范化为 `-100...`，并对重复记录执行软删除。

## 有引用的重复记录

有引用的记录禁止自动归并。必须先根据 preview 输出人工确认：

- 哪条 `collect_channel` 是保留记录；
- 每个引用表如何迁移；
- 同 source 的授权状态如何合并；
- `rule_channel`、`account_channel` 的唯一键冲突如何处理；
- 资料索引和 TG 消息记录是否需要保持原 channel ID。
- 计数投影是否按 `channel_profile` 重建，而不是直接相加。

特别注意：`collect_channel_profile.channel_id`、`collect_channel_count.channel_id`
和 `collect_channel_count_member.channel_id` 是 Chat ID 文本，不是
`collect_channel.id`。脚本不会把这些字段误当成主键迁移。

`collect_rule_channel` 当前安装索引是 `(rule_id, channel_id)`，没有
`tenant_id/account_id`。本工具只报告跨所有者冲突，不会擅自删除或重建该索引；
因为直接重建前必须先确认业务是否允许同一规则 ID跨租户复用，并处理现有冲突。

确认后应新增独立、可审查的迁移脚本，先在数据库备份或临时库演练，再安排线上窗口。不要直接执行 `UPDATE collect_channel SET telegram_chat_id=...`，也不要把跨租户频道关系改写到当前租户。

## 回滚说明

`--apply` 只在一个事务中执行；任何保护条件失败、唯一键冲突、超时或 SQL 错误都会自动回滚。脚本还会在变更前检查计数投影表和非 canonical 活跃频道。提交后，软删记录的恢复只能使用迁移前备份或根据审计输出生成反向脚本，因此线上执行前必须保存 preview 输出和数据库备份。
