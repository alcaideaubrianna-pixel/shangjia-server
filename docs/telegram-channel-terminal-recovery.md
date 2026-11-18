# 频道发送超时与终态恢复

## 流程与职责

1. 上架、重新上架或循环调度创建 `tg_job`，通过 Outbox 可靠投递。
2. Outbox 投递到 Redis 频道队列；`delivered` 只代表投递完成，不代表 Telegram 发送成功。
3. Worker 获取执行权，按发送预算执行展示、验证；Bot 或协议号明确成功后持久化消息与阶段。
4. 成功、失败或废弃后，任务进入终态，调度收口并释放频道执行权。发送结果不确定的 `unknown / confirming / reconciling` 仍须保留对账保护。
5. 执行 Context 取消后，已有 `telegramFinalizeContext` 提供独立有界 Context 完成收尾；执行超时不等于收尾必然成功。
6. `recoverPendingIdleTelegramJobs` 有界读取待恢复任务，校验频道、重试时间及计划条件，经 `rearmTelegramJobOutbox` 与原 Outbox 投递入口恢复。禁止用批量重发替代恢复。

## 占用规则

内存 `telegramChannelJobExecutionState` 与 SQL `telegramChannelBlockingJobStateSQL` 必须一致：

- `sent / failed / superseded / cancelled / deleted` 不再占用发送槽；历史阶段残留不得覆盖终态。
- 非终态的 `sending / confirming / unknown / reconciling`，或 `dispatch_status=processing`，或执行中阶段，仍阻塞后续恢复。
- 交互优先级使用既有独立条件，本次未扩大其抢占范围。
- 终态释放频道不代表删除历史消息或孤立补发验证；部分发送失败仍按既有清理、完整重新上架策略处理。

这层规则用于容忍中断收尾留下的旧阶段，不能依赖事后全局扫描修复数据。

## 2026-11-18 频道 231 修复记录

环境：生产 PostgreSQL `railway`，tenant 78 / account 615 / channel 231（`baihuagexuanfei`）。

修复前：

- 两个任务 `3248684 / 3291193` 均为 `failed / done`，残留 `verify_sending`。
- 旧 SQL 通过阶段 OR 条件将两者视为占用者；内存判定已正确识别为终态。
- 频道范围内有 271 个 pending 任务；样本为 pending/idle，Outbox 已 delivered，但 Redis 队列无对应任务。
- 实时任务仍有展示和验证成功证据，故障集中于历史待恢复链路。

线上止血：

- 单事务、主键及租户/账号/频道/原状态精确限定，清空上述两条任务的 `send_phase` 并更新 `updated_at`。
- 事务断言影响行数必须为 2，实际影响 2 行。
- 保留 `failed / done`、错误原因、消息及 Attempt；未把旧失败任务重置为 pending，未批量重发 271 条资料。
- 后续由现有恢复器执行；移除阻塞不等于积压已全部恢复，需分别观察 pending 数、入队、开始执行及 sent。

第一次清理后，恢复器步骤仍正常返回，但 pending 未下降。进一步按频道核查发现另外 172 条更早的终态残留：`failed/done` 68 条（展示 64、验证 4），`superseded/done` 104 条（展示 98、验证 6）。因此最初两个样本并非全部阻塞者。

第二次止血限定同一租户/账号/频道、上述终态和 done 调度状态：

- 单事务保存原始 ID、状态、阶段、更新时间；断言候选必须为 172 条。
- 以全部原字段做 CAS，仅清空阶段并更新更新时间；断言实际更新必须为 172 条，否则整体回滚。
- 实际更新 172 条；加上首批 2 条，总计 174 条历史终态元数据。
- 原始阶段审计保存在操作者本机 `/tmp/youban-channel231-terminal-repair-20261118.json`，不包含凭据，不提交业务明细到仓库。
- 终态、发送记录、错误信息均保留；未触碰其他频道或重置待发送任务。

恢复验收（北京时间）：

- 09:42:45.422：任务 `3323275` 经 `channel_queue_dequeue` 开始消费。
- 09:42:46.128：Bot 展示发送成功，消息 `1168845–1168848`。
- 09:42:46.637：Bot 验证发送成功，消息 `1168849`。
- 09:42:46.799：`channel_queue_ack`，状态 sent，执行耗时 1379ms。
- 09:43:13 数据库复查：该任务为 `sent / done / verify_confirmed`，pending 从 271 降至 270，旧占用条件匹配项为 0。
- Trace：`29ccb9df01953b207216570367777a54`。这证明历史任务恢复链路已推进，不代表其余 270 条已全部完成。

代码修复仅涉及公共 SQL 频道占用条件和回归用例源文件。部署主要涉及 Publish；无需新增表或索引迁移。

回滚：代码可回退该提交；数据修复前阶段均为 `verify_sending`，不建议恢复这一错误占用标记。如确需回滚，必须先重新核对两个任务仍处于原终态并限制主键，避免覆盖后续变化。

验证边界：遵照用户要求，不编译、不运行测试；执行格式化、差异检查与静态审查。线上恢复结果以实际业务状态和日志为准。
