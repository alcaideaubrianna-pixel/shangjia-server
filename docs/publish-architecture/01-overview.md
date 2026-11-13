# 1. 总体架构

```text
Telegram/BOT
  -> collect_event（事实落库）
  -> realtime/source/history queue（来源分片）
  -> 规则匹配、资料投影、媒体处理
  -> collect_dispatch
  -> tg_job + tg_outbox
  -> channel publish ZSET
  -> Stream/Asynq publish worker
  -> Telegram
  -> 状态回写、ZREM、唤醒下一条
```

删除链路独立为：

```text
下架/自动删除 -> delete job/outbox -> channel delete ZSET -> delete worker -> 终态回写
```

生产者只负责创建事实任务并写入 Outbox；消费者只负责领取、发送和收口。账户负责派发和凭据隔离，频道负责顺序，不允许账户级全局锁阻塞所有频道。

频道队列 Key 必须包含 `tenantId + channelId`，任务成员只保存 Job ID 或删除消息 ID，完整数据从 PostgreSQL 读取。
