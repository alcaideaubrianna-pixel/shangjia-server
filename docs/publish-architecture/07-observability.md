# 7. 结构化日志与观测

采集和推送日志使用统一 `log_type`，关键字段保持一致：

```json
{
  "traceId":"...", "operationNo":"...", "tenantId":1,
  "accountId":2, "sourceId":3, "collectEventId":4,
  "profileId":5, "jobId":6, "outboxId":7,
  "channelId":8, "targetChatId":"...", "runType":"cycle",
  "queueName":"...", "stage":"media.ready", "status":"ok",
  "durationMs":12, "queueAgeMs":20, "dispatchAgeMs":30,
  "attempt":1, "errorType":"", "error":""
}
```

阶段必须连续：`message.received → event.persisted → media.ready → profile.persisted → dispatch.created → outbox.claimed → queue.enqueued → job.started → telegram.sent → job.terminal`。

禁止把消息正文、Token、连接串写入日志。每个阶段成功和失败都要能通过 `traceId + jobId/profileId` 串起来。
