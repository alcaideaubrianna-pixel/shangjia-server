# Youban Telegram 采集与推送架构

本文档是采集、资料处理、循环上架、批次上架、手动上架、删除和 Telegram 发送链路的统一设计基线。后续代码、迁移、运行参数和验收必须以本目录为准。

## 章节

1. [总体架构](./01-overview.md)
2. [队列与调度模型](./02-queues-and-dispatch.md)
3. [采集与资料处理](./03-collection-pipeline.md)
4. [循环上架与批次上架](./04-cycle-and-batch.md)
5. [数据模型与索引](./05-data-and-indexes.md)
6. [状态机、恢复与幂等](./06-state-machine-recovery.md)
7. [结构化日志与观测](./07-observability.md)
8. [HotGo 实现规范](./08-hotgo-implementation.md)
9. [迁移、回放与验收](./09-migration-and-acceptance.md)
10. [重构 TODO 与验收状态](./TODO.md)

## 不可违反的原则

- PostgreSQL 是事实源，Redis 是高速调度索引，Redis 丢失不能造成业务丢失。
- 每个租户、每个频道拥有独立发布队列和删除队列；同频道串行，不同频道并发。
- 实时采集优先于历史采集，手动上架优先于循环任务，但不能打断已经开始发送的任务。
- 所有任务必须有明确终态；超时、账号未授权、目标失效、媒体失败、规则无匹配、审核拦截不能永久停在中间状态。
- 大表禁止无范围扫描；循环、恢复、统计必须按租户/账户/来源分片并使用游标。
- 每条链路必须使用同一组 `traceId、operationNo、eventId、profileId、jobId、channelId、accountId、sourceId` 贯穿日志。
