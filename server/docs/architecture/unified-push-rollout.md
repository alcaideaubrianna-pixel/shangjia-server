# 统一频道推送灰度手册

## 发布前检查

1. 先执行对应 PostgreSQL 在线升级脚本，确认 `hg_youban_publish_account_setting.push_engine` 存在并有 `(tenant_id, account_id, push_engine)` 索引。
2. 发布 `publish` 角色，确认发送 Worker、Cleanup Worker、AutoDelete Worker 都已启动。
3. 在队列观测中确认以下队列都有消费者：
   - `youban_publish_push_send`
   - `youban_publish_push_delete_manual`
   - `youban_publish_push_delete_keyword`
   - `youban_publish_push_delete_cycle`
4. 保持所有账户为 `legacy`，先观察旧链路没有新增失败。

## 单账户灰度

只为已确定的测试账户设置 `push_engine=unified`。修改后账户配置缓存会主动失效，不需要重启服务。

按以下顺序验证：

1. 手动上架一条资料，确认任务进入统一发送队列并成功发送。
2. 在同一频道连续上架两条资料，确认发送顺序和历史实现一致。
3. 触发关键字自动删除，确认任务进入关键字删除队列且不阻塞发送队列。
4. 等待或手动触发一次循环翻新，确认旧任务完成后生成下一次循环时间。
5. 重启一个 publish 实例，确认 Outbox 能重新投递未完成任务。
6. 模拟 Telegram 超时或失败，确认任务进入 Asynq 重试，最终状态不会停留在无消费者队列。

## 观察指标

灰度期间至少观察一个完整循环周期，重点记录：

- 各统一队列的 pending、active、retry 和 oldest task age；
- 同一频道是否出现并行发送或顺序反转；
- `hg_youban_publish_tg_job` 的 `dispatch_status`、`status`、`next_retry_at`；
- Outbox 的 pending 数量、租约过期数和投递失败数；
- 循环资料的 `cycle_enabled`、`next_cycle_at` 和失败计数。

## 回滚

发现发送延迟、顺序异常、重复发送或删除任务堆积时，将该账户的 `push_engine` 改回 `legacy`。旧队列仍由现有 Worker 消费，已经创建的统一队列任务继续由新 Worker 完成，不能直接删除 Redis 任务。

回滚后保留统一队列中的任务和日志，先确认没有 active 任务，再决定是否需要按任务状态做定向补偿。禁止全量重试或批量重置发送状态。

## 放量和收口

单账户稳定后按账户逐步放量，每次放量都记录账户、时间、队列延迟和循环结果。所有账户稳定运行后，再单独提交旧调度代码的删除变更；在此之前必须保留 `legacy` 回退路径。
