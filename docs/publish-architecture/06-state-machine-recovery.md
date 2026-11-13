# 6. 状态机、恢复与幂等

任务状态至少包含：`pending、queued、sending、confirming、sent、failed、superseded、timeout_cancelled`。Outbox 状态包含：`pending、processing、delivered、failed`。

任何发送超时都必须：

1. 结束当前租约；
2. 将 Job 收口为失败或超时终态；
3. ZREM 当前频道索引；
4. 唤醒下一条；
5. 对发送结果未知的任务保留对账信息，禁止盲目重复发送。

恢复扫描必须按账户/频道/来源分片，小批量、游标化，并有独立超时。恢复失败不能阻塞正常发送队列。

## 执行阶段队首闸门验收

Outbox/Stream 入队检查不能作为最终顺序保证。Worker 获取频道发送租约后，必须再次读取主库队首；只有当前 Job 仍是队首时才允许进入 Telegram API。非队首任务需要释放租约、退避并重新激活 Outbox。

真实 TG Fake 回放曾发现同频道越序：后续 Job 在前序 Job 仍为 `pending` 时提前发送。修复提交为 `51eb0a05`，并补充跨账户共享 `target_chat_id` 的队首测试。后续验收必须同时满足：

- 同一 `tenant_id + target_chat_id` 的 Job 按创建顺序发送；
- 不同目标频道可以并发；
- 不同账户投递到同一目标频道仍然串行；
- 非队首任务不会调用 `sendMediaGroup`；
- 1000 条混合任务全部进入终态，TG Fake 收包数与成功 Job 数一致。
