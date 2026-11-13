# 6. 状态机、恢复与幂等

任务状态至少包含：`pending、queued、sending、confirming、sent、failed、superseded、timeout_cancelled`。Outbox 状态包含：`pending、processing、delivered、failed`。

任何发送超时都必须：

1. 结束当前租约；
2. 将 Job 收口为失败或超时终态；
3. ZREM 当前频道索引；
4. 唤醒下一条；
5. 对发送结果未知的任务保留对账信息，禁止盲目重复发送。

恢复扫描必须按账户/频道/来源分片，小批量、游标化，并有独立超时。恢复失败不能阻塞正常发送队列。
