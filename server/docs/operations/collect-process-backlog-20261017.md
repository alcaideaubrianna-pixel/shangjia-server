# 采集处理积压修复与验证（2026-10-17）

## 修复范围

- 已 processed/dispatched 或普通 ignored 的事件按幂等成功退出；未匹配验证事件仍允许恢复，不可恢复失败复用现有失败分类。
- 同一事件的旧队列任务和新任务使用事件锁；带 Outbox owner 的旧调度在消费前检查归属，完成确认也检查 owner。
- 事件投递仅持久化到 Outbox，由 scheduler 统一入队。排队租约过期时复用 owner 和 Asynq TaskID，避免复制同一调度。
- 恢复扫描不覆盖 pending/processing 的 Outbox（生产 PostgreSQL 路径）；业务续处理可以建立新调度。
- realtime_v2 独立消费者，recovery_v2 与旧队列由 background 消费。实时额度预留，历史恢复限于 background 并发。每轮 Outbox 扫描为最近10分钟事件预留20%额度。
- 验证子事件直接进入已有验证处理流程；暂不满足窗口条件使用延期调度；活动事件残留完成时间按状态条件清理。

## 验证证据与边界

通过 SSH server2 对生产执行 REPEATABLE READ READ ONLY 事务，导出324事件、344媒体、324 Outbox、5采集源。URL、文件凭据和错误中的临时地址已移除。完整快照仅存本地临时文件，不提交仓库。

实际回放将事件和采集源载入本地 PostgreSQL 临时表：300终态事件走真实 processCollectSourceTask，24活动事件检查归属及适用窗口查询。媒体与 Outbox 快照已导出，但本回放没有调用真实下载、上传或发送；不能宣称验证了完整入库推送。

另用本地 PostgreSQL 临时 Outbox + miniredis 验证20次通知仅保留一个调度、连续10次租约超时只有一个Redis任务、恢复不覆盖队列 owner、过期完成不覆盖新调度。15条状态快照作为固定回归用例提交。

运行：

```sh
YOUBAN_LOCAL_COLLECT_PG=1 YOUBAN_COLLECT_PROCESS_DUMP=/tmp/youban-collect-backlog-snapshot-20261017.json go test -race ./addons/youban_publish/logic/sys -run 'TestCollectProcess|TestCollectMaterial|TestCollectRecovery|TestPairCollectMaterial' -count=1
go build ./...
```

依赖本地127.0.0.1:35432 PostgreSQL，测试事务使用临时表；需要项目测试配置文件，禁止填入生产数据库或Redis凭据。miniredis只模拟本地队列，不启动推送worker。

## 发布顺序与回滚

此变更尚未发布。先更新 background（启用v2消费者和旧队列清理消费），再更新 scheduler（统一Outbox投递），随后更新其他采集投递/续处理角色。旧代码仍可能直接投递旧队列或无条件完成Outbox，全部相关角色对齐前不应声称去重保证已经生效。account发布必须停旧后起新，全程最多一个实例。

旧队列保留，不直接清空。新代码先通过终态幂等退出逐步消化旧任务；如需批量清理必须另做限速、状态核对和审计。回滚不能简单停掉v2消费者：必须保留消费者直到v2任务处理完，或暂停投递并将v2有效任务对应的Outbox恢复为待投递后再切回。

上线验收观察：新消息首次处理延迟、v2 pending最老等待、同事件有效任务数、retry新增速率、Outbox逾期数量、采集到入库及发送耗时。上线前的离线回归不构成零故障保证。
