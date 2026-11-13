# 2. 队列与调度模型

## 队列层级

| 队列 | Key/队列 | 用途 |
|---|---|---|
| 发布 | `publish:{tenant}:{channel}` | 同频道发送串行 |
| 删除 | `delete:{tenant}:{channel}` | 删除消息，与发布发送独立 |
| 实时采集 | `collect:realtime:{tenant}:{source}` | BOT/协议号实时事件 |
| 历史采集 | `collect:history:{account}` | 历史拉取，低于实时优先级 |
| 规则分发 | `collect:rule:{tenant}:{source}` | 规则拆分和资料投影 |

Redis 使用 ZSET 保存待处理成员，分数为 `priority + sequence`。Outbox 是可靠回填入口。入队、终态、租约回收都必须幂等。

## 调度规则

```text
手动/AI        priority=1
实时采集       priority=10
批次上架       priority=30
循环翻新       priority=50
历史/恢复      priority=80
```

同频道只允许一个发送中的 Job；不同频道由 worker 并发。发送锁只覆盖单频道，不得使用账户级锁包住多个频道。

## 失败退避

失败次数使用指数退避并设置上限；频道锁失败只延后当前频道的 `available_at`，不阻塞其他频道。超过可配置队首等待时间后，任务进入明确失败/超时终态并唤醒下一条。
