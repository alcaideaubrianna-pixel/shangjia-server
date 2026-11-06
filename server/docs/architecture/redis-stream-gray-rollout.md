# Redis Stream 频道推送灰度清单

## 启用前

1. 仅部署 `Push Worker` 版本，确认 `youbanPublish.redisStream.grayEnabled=false`。
2. 确认 Redis 使用项目统一 cache 适配器，Consumer Group 可执行 `XGROUP CREATE ... MKSTREAM`。
3. 确认旧 Outbox/Asynq worker 正常，记录待处理任务数、发送延迟和失败率基线。

## 单账户灰度

1. 在账户设置中将目标账户 `push_engine` 设置为 `unified_stream`。
2. 仅在指定 Push Worker 实例开启 `grayEnabled=true`，避免 Scheduler/Web 角色启动消费者。
3. 观察 realtime/bulk 两条 Stream 的 pending、ACK 延迟、Outbox 重试和频道发送顺序。
4. 逐步验证手动上架、采集推送、循环翻新、全量推送和删除任务。

## 回滚

1. 先将账户 `push_engine` 改回 `legacy`。
2. 再关闭 `grayEnabled` 或重启 Push Worker。
3. 历史 Stream 消息会被账户门禁跳过；未完成任务由 Outbox 恢复旧 Asynq 链路。
4. 确认该账户频道队列恢复、无持续 pending、无重复发送后再处理遗留 Stream。

## 放量条件

- 连续观察一个完整业务高峰周期；
- 无频道乱序、任务卡死或重复发送；
- Outbox 重试、Stream pending 和 Asynq 延迟均回到基线范围；
- Redis 重启和 Worker 滚动发布后 Consumer Group 能自动恢复。
