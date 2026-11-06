# Redis Stream 灰度分支

本分支用于验证 Redis Stream 作为“就绪通知”传输层，现有 PostgreSQL Outbox 和 Asynq 仍然保留为可靠执行链路。

```text
事务内写 tg_job + outbox
        ↓
提交后 XADD ready stream（仅灰度账户）
        ↓
Stream consumer 按 job_id 投递现有 Asynq 发送任务
        ↓
现有频道 lease、发送、重试、最终状态
```

## 灰度边界

- 默认账户继续使用当前 Outbox dispatcher，不读取 Stream。
- 只有明确开启灰度的账户写入 `youban_publish:tg:ready:realtime` 或 `youban_publish:tg:ready:bulk`。
- Redis Stream 只发送提示事件，事件丢失时由 Outbox recovery 补发。
- Stream consumer 必须调用现有幂等 Asynq 入队方法，不能直接执行 Telegram 发送。
- 同一频道的顺序仍由数据库队首判断和频道 lease 保证。

## 失败和回滚

- Redis 不可用：提交后的事件投递失败，Outbox recovery 继续补发；不能回滚已提交任务。
- Consumer 崩溃：消费者重启后从 consumer group pending entries 重试。
- 账户回滚到旧实现：停止该账户的 Stream producer，保留历史 Stream 消息和 Outbox；旧 dispatcher 接管未完成任务。
- 重复事件：通过 `job_id` 幂等，重复 XADD 不得产生重复 Telegram 发送。

## 上线顺序

1. 只部署支持 Stream 的 publish worker，所有账户保持旧传输路径。
2. 用本地线上样本验证提交失败、Redis 重启、consumer 崩溃、重复事件和回滚。
3. 开启一个测试账户的 Stream producer，观察 ready 延迟、Asynq 入队延迟、频道顺序和重复发送。
4. 稳定后逐账户放量；出现异常时只关闭该账户的 Stream 灰度开关。

本分支不直接修改生产账户开关，也不删除现有 Outbox 或 Asynq 代码。
