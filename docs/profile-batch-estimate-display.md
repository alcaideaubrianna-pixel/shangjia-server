# 资料详情的批次循环排位说明

展示格式为：

> 预计 3 天后轮到 · 前方 210 份 · 每天 70 份

这次仅补充排期解释，继续使用现有批次循环算法、日期、轮次边界和发布队列。
“3 天”只是文案示例，不是把所有资料的等待时间改成 3 天。

## 接口

`profile/channels` 及详情的 `pushChannels` 使用同一个 `ProfilePushChannelModel`。
保留 `nextPushAt`、`nextPushAtEstimated`、`cycleBatchSize`，新增：

| 字段 | 含义 |
|---|---|
| `batchAheadCount` | 预计轮到之前，需要先轮播的资料次数；跨轮可包含同一资料再次轮播，不是 Redis 队列长度 |
| `batchRoundRemaining` | 等待下一轮时，当前轮剩余的预计资料次数 |
| `batchWaitingNextRound` | 目标资料是否需要等当前轮结束，再进入下一轮排位 |
| `batchInCurrentRun` | 根据当前排位，估计属于已创建的活动批次；不代表正在发送 Telegram 消息 |

两个数量字段使用可选整数。缺失表示无法预估，`0` 表示已知为零。
查询超时、目标不满足资格、批次大小无效、未启用批次循环时不伪造数量。
旧前端会忽略新字段；新前端收到旧接口时保留日期或等待排期提示。

## 与日期一致的计算

复用 `estimateProfileBatchPushAt` 已查询并缓存的 rank，不增加 COUNT、Redis 扫描或额外请求。

- 当前轮、当前活动批次范围内：标识为预计属于当前批次，前方数使用本批次内的排位。
- 当前轮、活动批次之后：前方数为 `max(aheadInRound - activeBatchSize, 0)`。
- 等待下一轮：本轮剩余为 `max(roundLeft - activeBatchSize, 0)`，
  前方总次数为本轮剩余加下一轮的 `beforeTarget`。

日期继续由原有整批估算公式计算，不能再用“前方总数 ÷ 每天数量”覆盖日期：
当前轮末批可能不足每日额度，而下一轮在后续调度日重新开始。

例如排查 F50025 时，频道 300 的快照为：

```json
{
  "cycleBatchSize": 70,
  "nextPushAtEstimated": true,
  "batchAheadCount": 507,
  "batchRoundRemaining": 247,
  "batchWaitingNextRound": true
}
```

含义是当前轮还有 247 次，下一轮目标前还有 260 次，
所以应同时说明“需等待当前轮结束”，不能把 507 展示成真实待消费 Job 数量。
这些快照会随资料上下架、频道配置及调度变化，不是发送承诺。
