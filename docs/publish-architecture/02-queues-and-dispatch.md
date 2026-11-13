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

## 统一入口落地状态

资料类手动、采集、循环和批次入口统一调用资料 Job 创建工厂，工厂在同一事务内完成：

```text
资料来源 + 目标频道 + operationNo
  -> TG Job
  -> TG Outbox
  -> 频道发布索引
```

消息模板/AI 类入口也通过统一的消息发布创建包装进入相同的 Job/Outbox 调度链。底层发送不区分生产者，只根据 `priority、operationType、runType` 选择队列策略。

当前仍需验收的边界：

- 执行阶段原子频道发送闸门；
- 跨账户但同一 `target_chat_id` 的严格顺序；
- Redis 索引丢失后的 Outbox 回填；
- 手动、采集、循环混合任务的优先级和不同频道并发。

定向验证命令：

```bash
cd server
go test ./addons/youban_publish/logic/sys \
  -run 'Test.*(Channel|Priority|Queue|Recovery|Send)' -count=1
```

## 失败退避

失败次数使用指数退避并设置上限；频道锁失败只延后当前频道的 `available_at`，不阻塞其他频道。超过可配置队首等待时间后，任务进入明确失败/超时终态并唤醒下一条。
