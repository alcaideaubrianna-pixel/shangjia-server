# 百花阁 2 群发送成功后等待租约到期

环境：SAE Publish，镜像 `30d0784`。频道 229，用户 615，租户 78。
以下时间均为北京时间。

## 证据

| 时间 | 事实 | 来源 |
|---|---|---|
| 20:54:01 | Job 2065649 取得频道发送锁，固定有效期 630 秒 | SLS `channel.send_lease.acquire`，`acquired=true` |
| 20:54:14 | 展示相册 API 返回成功，API 耗时 1567 ms | SLS `api.sendMediaGroup` |
| 20:54:21 | 验证视频 API 返回成功，API 耗时 895 ms | SLS `api.sendVideo` |
| 20:54:22 | Job 成为 `sent + verify_confirmed` | PG `tg_job.sent_at=20:54:22.633561` |
| 20:54:24 | 原执行最后阶段日志，未记录频道租约释放 | SLS execution `1wmu2a21000dm6a0fphzpxlwv6ik8kw3` |
| 20:54:26 | 持锁实例 f542g 的容器重新启动 | SAE `CreateTimeStamp=1792155266000`，`InstanceContainerRestarts=1` |
| 20:54:27 | 同实例重新启动 push-worker / Asynq | SLS 启动日志 |
| 20:54:57—21:04:19 | Job 2065654 多次获取锁失败，没有发送 Attempt | SLS、PG |
| 21:05:15 | Job 2065654 取得锁 | SLS `acquired=true` |
| 21:05:36 | Job 2065654 整组完成，随后频道继续推进 | PG `sent + verify_confirmed` |

根因：进程在成功落库后、defer 释放 Redis 租约前重启。Redis
只保存随机令牌，其他实例不知道锁对应的任务已经完成，必须等待固定租约到期。
本次证据没有证明重启由 OOM、健康检查或人工操作触发，不作该项结论。
也没有证据把本次故障归因于删除任务。

## 修复边界

- 新发布租约仍使用原 Redis key、相同 630 秒有效期，并保存带随机 nonce 的任务归属。
- 正常取得锁的路径不增加数据库查询。
- 锁繁忙时，只回收归属同租户、同目标频道，且为
  `sent + (verify_confirmed / completed_without_verify)` 的资料发布任务租约。
- 回收使用 Lua 比较完整令牌后删除；旧执行迟到解锁和两个恢复器并发均不会删掉新租约。
- 发送中、结果未知、失败、下架、模板发送和旧版无归属令牌均不回收。
- 数据库或 Redis 出错时保留锁，不猜测发送是否完成，不重发已成功资料。
- 不修改发送 API、媒体组、验证视频或频道前序任务检查。
- 旧版遗留锁仍需自然到期一次；本修复不能安全识别旧随机令牌对应哪个任务。

恢复时长由“等剩余 630 秒租约”缩短为“下一次正常调度时完成归属校验和原子释放”，
不承诺固定秒数，不以扩大同频道发送并发提速。

## 本地验证

使用本地 miniredis 执行真实 Lua，不连接生产 Redis、PG 或 Telegram。
覆盖成功后未解锁、发送中不回收、未知/失败/跨租户/跨频道拒绝、数据库失败保留锁、
旧调用迟到解锁、同 owner 不同 nonce、旧版令牌兼容、32 个恢复消费者同时竞争。
相关测试与 race 检测通过。完整业务包测试存在既有失败，须单独记录，不能称全量通过。

未改动 HEAD 的独立源码副本，在同一配置下复现以下失败：
`TestNormalizeAntiScanMattingConcurrency`、`TestDuplicatePHashScanBucketsBoundLookupCost`、
`TestClassifyProfileMessage`（两个子场景）。完整业务包另有需要本地 PG 的测试因
127.0.0.1:5432 未启动而失败。本次未修改这些业务或测试。

21:35:51 的生产只读查询：频道 229 待推送 1085、发送中 1，最近 10 分钟
完成 15 组，最后成功时间 21:35:27；不是依靠本地修复恢复，本地代码尚未上线。

本次仅修改本地代码，未推送、部署或删除生产锁。
