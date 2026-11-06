# Telegram 采集分发重构

## 目标

频道消息只负责接入和形成统一的 Telegram Collector 事件。采集 source、规则和频道审批分别维护自己的状态，任何一个 source 或 rule 被禁用，都不能影响同频道的其他订阅者。

## 当前事实

现有链路已经具备 source 级分发：

```text
Bot webhook
  -> tg_collector_event（source_id 维度）
  -> tg_collector_delivery
  -> Publish.HandleCollectorDelivery
  -> collect_event（source_id 维度）
```

线上样本 `tenant=78、source=351、chat=-1004375380986、message=105129` 证明：

- Bot webhook 已收到 105129～105132 四条相册消息；
- 四条消息拥有同一个 `media_group_id`；
- Collector event 和 delivery 都完成；
- Publish 没有创建 collect_event；
- 原因是旧的 `collect_account_channel` 关系 `24143` 为 disabled/rejected。

## 新的状态边界

```text
collect_channel
  频道是否存在、是否可访问

collect_source
  source 总开关和 source 归属

collect_account_channel
  source 与 channel 的订阅/审批关系
  唯一键：tenant_id + account_id + channel_id + source_id

collect_source_rule
  source 与 rule 的绑定关系

collect_event / collect_event_media
  source 独立的业务事件和媒体处理状态
```

`collect_account_channel` 不再代表“频道全局开关”。查询和写入必须始终带 `source_id`。

## 分发规则

1. Collector 按 source 生成事件和 delivery。
2. Publish 读取当前 delivery 的 source。
3. 只检查该 source 的频道订阅关系。
4. 只加载该 source 的启用规则。
5. 规则处理失败只影响自己的 rule delivery。
6. 相册按 `media_group_id` 聚合，聚合完成后再执行规则匹配。

## 已完成

- 采集关系唯一键迁移为 `tenant_id/account_id/channel_id/source_id`。
- Bot 频道审批查询按 source 隔离。
- Publish 采集事件状态检查按 source 隔离。
- 所有关系写入冲突键按 source 隔离。
- AI 运维 source 重放支持明确事件 ID、频道和消息区间。

## 后续删除旧逻辑

迁移完成并观察一轮线上数据后，删除以下全局假设：

- 按 `tenant_id/account_id/channel_id` 查询审批关系；
- 不带 source_id 的频道启停接口；
- 用频道清理操作直接修改其他 source 的关系；
- 在 Publish 消费者中通过频道状态推断 source 状态。

## 运维重放

统一使用 AI 运维计划接口：

- 指定多个 Collector event ID；
- 指定 `chat_id + start_message_id + end_message_id`；
- 预览候选事件；
- 确认令牌执行；
- 查询进度和失败事件。

重放只重新进入 Collector delivery，不直接修改资料或发送 Telegram 消息。资料重新推送必须经过已有的资料重发布接口，避免重复发送。

## 本地验证

```text
go test ./addons/youban_publish/logic/sys ./addons/telegram_collector/logic/sys
git diff --check
```

线上样本应导出为脱敏 JSONL，再在独立 PostgreSQL 中回放，覆盖单消息、四消息相册、多个 source 共用频道、单 source 禁用和区间重放五种场景。
