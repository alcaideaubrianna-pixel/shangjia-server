# Redis Stream 灰度适配层

这个目录只负责 Redis Stream 的事件协议和传输适配，业务层不直接调用 Redis 命令。

## 责任边界

- `ReadyEvent`：描述数据库任务已经就绪；
- `RedisProducer`：封装 `XADD`；
- `RedisGroupConsumer`：封装读取、解码、handler、ACK；
- `RunBothStreams`：同时运行 realtime/bulk 两条消费流；
- `StreamForOperation`：选择 realtime/bulk lane。

Stream 消息只携带 `job_id`、`channel_id`、优先级和原始队列提示。消费者必须通过现有 Asynq 幂等入队逻辑执行任务，不能直接调用 Telegram API。

## 灰度和回滚

运行开关为 `youbanPublish.redisStream.grayEnabled`，默认 `false`，且只在
`Push Worker` 角色读取。账户还必须将 `push_engine` 设置为 `unified_stream`；关闭
运行开关或将账户切回 `legacy` 都不会影响旧链路。

账户配置使用 `unified_stream` 时才启用该传输层。`legacy` 和 `unified` 路径继续使用现有 Outbox/Asynq 实现。回滚账户配置到 `legacy` 即可停止该账户的 Stream producer；历史 Stream 消息保留给恢复任务处理。

## 当前接入状态

1. Push Worker 通过 HotGo cache 基础设施创建 Redis Producer/Consumer。
2. Outbox 领取成功后，灰度账户发布 realtime/bulk Ready Event。
3. Consumer Group handler 重新校验账户状态，再调用现有 Asynq 入队入口。
4. 成功处理后 ACK；Redis 或 handler 失败时保留 Outbox/Stream pending 供恢复。
5. 目前仅完成代码接入和本地回放，生产账户仍需逐个配置 `unified_stream` 灰度。

删除整个目录不会影响现有发送、删除、循环、批次和 Outbox 代码。
