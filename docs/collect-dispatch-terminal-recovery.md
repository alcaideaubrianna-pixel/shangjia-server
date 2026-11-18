# 采集分发与取消恢复收尾修复

## 2026-11-18 证据

三个 dispatch `450203 / 450204 / 450211` 的频道清单分别为 1、3、3 个目标。其 exact tenant/account/profile/event/channel/`collect:<dispatchId>` 任务均已 sent/done，但 dispatch 仍 pending。

SLS 中任务 `3408602 / 3408604` 已明确发送成功，之后资料投影报 `sql: no rows in result set`。`completeProfileTelegramOperation` 的 failed 状态可选查询将 `sql.ErrNoRows` 当成异常返回，阻断正常完成路径。全局 `reconcileCollectDispatchesFromSentJobs` 只有定义，未接入恢复。

取消意图 9 的 Attempt `3430026` 已 superseded，仍保留规范证据 `bot_definite_rejection`；response/confirmed/audit/durable messages 均为 0。取消恢复复用只接受 retry_wait 的发送恢复分类函数，错误地继续等待这个已证明未发送的 Attempt。

## 修复边界

- failed 状态可选查询仅忽略 `errors.Is(err, sql.ErrNoRows)`；真实数据库错误仍返回，确有 failed 状态仍走原有证明与保护。
- 采集 dispatch 收尾移到当前资料操作投影之前。资料锁竞争、旧 operation 或资料投影失败，不再阻断已确认的历史发送事实收口。
- 新 helper 从已知 `collect:<dispatchId>` 按主键读取，并核对 tenant/profile/account。收尾 UPDATE 可限定 dispatch ID；原频道清单及 exact operation、sent/done 条件全部保留。
- dispatch 已 sent 时仍允许重试 event 收尾，避免 dispatch 更新后发生中断导致 event 永久 dispatched；重复调用不重写 dispatch 发送事实。
- 资料状态恢复器已选中的完成 operation 也调用该 helper，然后才 clear。收尾失败时保留原候选供下一轮重试，不把它当作发送失败，也不重发。
- 未启动全局 dispatch 扫描。已 clear 的全部历史 pending 不在现有状态游标范围内，本次不声称自动覆盖；已知样本按主键单独补偿。
- 取消仅识别 bot/protocol 的规范 definite_rejection 和 preparation_failed_before_send 标记，且要求 retry_wait/superseded、无 audit/response/confirmed/webhook deadline。任一矛盾证据仍等待，不修改 Attempt 生命周期，不触发发送。
- 取消新增 `profile_cancel_recovery_stage` 日志：stop_jobs、stop_cycle、settle、state_lock、attempt_evidence。仅记录 scope、耗时、结果和错误类型，不增加 SQL/回执正文输出。

## 线上样本补偿

生产 PostgreSQL `railway`，2026-11-18 14:58:42（北京时间）：

- 单事务仅限定上述 3 个 dispatch，同时核对各自 tenant/account/profile/event 原值及 pending 状态。
- 每个目标均要求存在频道清单，且每个频道均有 exact operation 的 sent/done Job。
- 事务断言实际更新必须为 3 条，否则回滚。实际更新 3 条 dispatch 为 sent，3 条满足所有分支完成条件的 event 为 processed。
- 未修改资料、Telegram Job、Attempt、队列或消息，未触发重发。

这些更新只补齐已有发送事实，不应回滚为 pending；如需纠正，必须重新核对原始发送事实及审计，不能覆盖后续真实变化。

## 验证与部署

补充了规范拒绝证据边界的测试源码，以及使用独立 loopback PostgreSQL 的可选 failed 状态空结果、dispatch/event 分步收尾重入、租户隔离和缺失/废弃目标回归源码。遵照用户禁令，未编译、未运行测试，也未启动任何本地消费者。格式化、差异检查及静态 review 不等于运行验收。

部署涉及 Publish 和处理资料投影队列的 Background。没有新增 schema、索引或环境变量迁移。部署后应核对新采集 dispatch/event 能否及时收口、取消意图 9 是否在其他门槛满足后完成，以及取消分段日志是否仍有慢项。
