# OpenObserve 第一批告警

OpenObserve 部署完成后创建 Telegram Webhook Destination，并按以下规则建立告警。所有规则使用 2 次连续命中、5 分钟去重，并配置恢复通知。

## P0

| 告警 | 条件 |
|---|---|
| API 全部不可用 | `xiaohuiji.process.up{service_name="xiaohuiji-api"}` 2 分钟无数据 |
| Worker 全部停止 | `xiaohuiji.runtime.heartbeat{role="worker"}` 2 分钟未更新 |
| Scheduler 无心跳 | `xiaohuiji.runtime.heartbeat{role="scheduler"}` 2 分钟未更新 |
| 数据库采集失败 | `xiaohuiji.publish.jobs` 与 `xiaohuiji.collect.events` 同时 2 分钟无数据 |
| 全局停止发送 | publish pending > 0，10 分钟 sent 增量为 0 |
| TG 账号全部离线 | authorized 账号数为 0 且 publish pending > 0 |

## P1

| 告警 | 条件 |
|---|---|
| 同频道多活动任务 | `xiaohuiji.invariant.channel_multiple_active_jobs > 0` |
| idle 无活动队头 | `xiaohuiji.invariant.idle_channel_without_active_head > 0` |
| DB Job 缺少 Asynq Task | `xiaohuiji.invariant.db_job_missing_asynq_task > 0` |
| Asynq Task 对应 DB 已结束 | `xiaohuiji.invariant.asynq_task_terminal_db_job > 0` |
| sending 超时 | `xiaohuiji.invariant.stale_sending_jobs > 0` |
| 恢复器失败 | `xiaohuiji.recovery.runs{result="failed"}` 5 分钟增量 > 0 |
| Gateway Bot 不一致 | configured 与 running 两条 `xiaohuiji.tg.gateway_bots` 不相等 |
| 租约冲突 | `xiaohuiji.tg.account_lease_events{action="conflict"}` 5 分钟增量 > 10 |
| lock failed | `xiaohuiji.tg.account_lease_events{action="lock_failed"}` 5 分钟增量 > 30 |
| Bot Webhook 未产生后续事件 | webhook queued 增量明显高于同周期 bot collect_event 增量 |
| Bot ID 为 0 | `xiaohuiji.invariant.collect_bot_id_zero > 0` |
| 采集源长期无事件 | `xiaohuiji.collect.configured_sources_without_events > 0` |
| FILE_REFERENCE_EXPIRED | `xiaohuiji.tg.file_reference_expired > 0` |
| 视频预览图缺失 | `xiaohuiji.collect.video_preview_missing > 0` |
| PHash/媒体处理积压 | `xiaohuiji.media.phash_oldest_age_seconds > 300` |
| 单频道清空时间过长 | `xiaohuiji.publish.channel_estimated_clear_minutes > 120` |

## Telegram 消息字段

```text
环境、服务、角色、告警等级、当前值、阈值、首次发生时间、持续时间、Railway 部署 ID、OpenObserve 查询链接
```

恢复消息必须包含故障持续时间和恢复后的当前值。
