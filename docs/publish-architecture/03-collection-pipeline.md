# 3. 采集与资料处理

实时采集必须先写入 `collect_event`，再通过来源分片 Outbox 进入处理，不允许直接依赖内存队列。

```text
收到消息
 -> collect_event
 -> 媒体下载/缓存
 -> media_ready
 -> 文本与规则处理
 -> content_profile / content_media
 -> collect_dispatch
 -> tg_job/outbox
```

BOT 采集允许 `account_id=0`，此时分片键为 `tenant_id + source_id`；协议号采集使用 `tenant_id + account_id + source_id`。禁止因为 `account_id=0` 把 BOT 事件丢弃，也禁止把不同租户的来源混合。

每个事件必须落入以下终态之一：`processed、dispatched、ignored、failed`。`media_ready` 只是中间状态，验证超时必须重试或失败收口，不能无限停留。

实时队列必须优先于历史队列。历史任务使用独立配额，不能占满实时处理容量。
