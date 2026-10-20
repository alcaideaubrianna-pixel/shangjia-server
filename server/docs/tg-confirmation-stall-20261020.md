# TG 成功响应后确认停滞（2026-10-20）

## 已确认事实

频道 203（Huasheng1883），任务 2236331：22:14:11 Bot API 明确返回
16495–16500；22:14:12 开始本地 message 写入后，没有该执行的结束日志。
后续恢复执行仍被同一任务持有的频道租约阻挡。22:23:54 超时清理后生成替代
任务 2236506，22:25:07 完成展示及验证发送。

因此不是“Bot 没有返回成功”。现有证据不能区分数据库等待、进程中断等
导致原执行停止的位置，不能宣称 ORM 连接池死锁：单连接实测未复现。

## 本次变更

- API 成功确认事务使用 PostgreSQL 事务局部时限：锁等待最多 2 秒，单条语句
  最多 8 秒，事务空闲最多 10 秒。已有更严格时限保持不变。
- 继续保留原有 10 秒 Go 确认预算、原子事务及发送租约保护。
- 记录数据库 backend PID、每条消息媒体组检查及写入的开始/完成/错误；确认
  阶段超过 2 秒告警。锁归属日志增加 sameJob，区分自阻挡和其他任务占用。
- 不强行回收可能仍在运行的发送租约，不修改线上状态或重新发送消息。

## 本地验证

隔离 PostgreSQL：127.0.0.1:35432，测试临时表，不连接生产。

```sh
YOUBAN_LOCAL_COMMIT_PG=1 go test ./addons/youban_publish/logic/sys \
  -run '^(TestTelegram|TestRestoreConfirmedTelegram)' -count=1 -timeout=90s
go test ./utility/ownedlease -count=1
```

新增用例验证单连接消息重复保存、媒体身份保存、禁止展示/验证混组、数据库
锁等待超时后连接可用，以及已有 100ms 语句时限不会被放宽。

全业务包执行另有失败：防扫图并发默认值、指纹桶数量、资料分类及编号解析。
这些不在本次修改范围内，不能将全量测试报告为通过。

## 下次排查

SLS 按 jobId 查询 telegram_publish_stage，通过 executionId 区分恢复执行；
检查 api_success.database_budget 的 databaseBackendPid，再查数据库活动及锁。
逐条定位 api_success.message_group_check / api_success.message_upsert 的未完成
messageId。channel.send_lease.owner 的 sameJob=true 表示原执行租约尚在。

本次时限只保证数据库锁/语句等待受限，不能解决进程被终止或运行时整体暂停；
这类仍由原有租约过期及任务恢复机制处理，需要实例生命周期证据继续判断。
