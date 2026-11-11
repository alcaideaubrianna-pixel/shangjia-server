# Telegram 推送链路

## 正常路径

1. 业务事务写入 `tg_job` 和 `tg_outbox`。
2. 事务提交后，Outbox dispatcher 将就绪事件写入 Redis Stream。
3. Stream consumer 按 `job_id` 短窗口去重。
4. 频道唤醒入口按租户和频道短窗口去重。
5. 发送前获取频道 lease，检查频道队首和容量。
6. 任务使用稳定 Asynq TaskID `tg-publish-job-{jobID}` 入队。
7. Asynq worker 执行 Telegram 发送，完成后更新任务和资料投影。

## 可靠性边界

- PostgreSQL job/outbox 是最终事实来源。
- Redis Stream 只负责快速通知，丢失后由 Outbox recovery 补发。
- Redis 去重失败时继续走持久化幂等路径，不能阻断任务。
- Asynq `ErrDuplicateTask` 和 `ErrTaskIDConflict` 都表示任务已经存在，按成功幂等处理。
- 频道 lease 只保护单频道串行，不跨频道共享。

## 性能控制

`youbanPublish.queue.streamOutboxBatchLimit` 控制 Stream 单轮通知数量，默认 500；可按 worker 实例规格灰度调整。频道 wake debounce 默认 200ms，Stream job debounce 默认 300ms。

## 故障恢复

- pending/idle 任务由 Outbox 定期补发。
- queued 状态超过执行 lease 且未进入 sending 的任务会清理失效 Asynq 引用并重新唤醒。
- Redis 重启只影响通知速度，不影响 PostgreSQL 中的任务事实。
- Telegram 未知结果由对账和 webhook 路径收口，频道内后续任务保持顺序屏障。

## 本地验证

使用 `tools/local-regression/run-clean-tgfake-regression.sh` 重建独立 PG、Redis、tgfake 和角色后执行真实回归。测试角色统一使用 `Asia/Shanghai`，避免 `timestamp without time zone` 产生时间偏移。
