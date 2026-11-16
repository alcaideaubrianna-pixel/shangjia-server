# Telegram 协议号降级热修复（2026-11-17）

## 线上证据

`6cbb30b` 线上验收中，Job `3329238` 的展示消息发送成功，但验证 Bot 请求遇到 429。
交接 Account Task `7113610` 后，检查点写入生成了
`status=EXCLUDED."running"` 以及包含任务键的非法 SQL，最终 PostgreSQL 报语法错误。
其他 Account 任务还出现了频道槽忙就消耗一次尝试、五次后失败的问题。

## 修复

- 检查点的 `OnDuplicate` 改为列名列表，实际值统一通过 `Data` 绑定。
  插入、冲突更新和重复执行共用同一生产 SQL 构造函数。
- 频道租约忙、频道前序等待属于发送前准入；通过显式契约复用
  `AccountTaskFailure.NotStarted`，退还本次领取扣除的次数。
  真实网络失败、取消及未知结果仍按原有失败与对账规则处理。
- 准入等待使用已经持久化的 Job 截止时间；不因重试创建新预算。
- 普通 stale Attempt 扫描和事务内复核均排除 Account 接管标记；
  继续由现有 Account 任务恢复与 Job 总预算机制处理，不新增扫描器。
- MCP 已下架资料重新上架传入真实租户、账号。
  `ai-republish:` 在 Go 和 SQL 调度中统一使用手动最高优先级。

## 本地验证

- `GOMAXPROCS=4 go build -p 2 ./...` 通过，使用正常编译优化。
- 相关单元测试 577 项通过、37 项跳过。
  大型 publisher 测试包使用 `-gcflags=hotgo/addons/youban_publish/logic/sys=-l` 控制编译资源。
- 临时本地 PostgreSQL（仅 `127.0.0.1:35432`）8 项集成测试通过，覆盖：
  检查点首次插入与冲突更新、带冒号和引号的任务键、重复写入、
  交接归属隔离、展示与验证 Outbox 原子提交、普通 Job 失败收尾、
  Go/SQL 手动优先级一致、准入退款与旧租约拒绝、持久预算不延长、
  普通恢复不接管 Account Attempt。
- 用旧 `OnDuplicate(map)` 写法的隔离 overlay 执行新增检查点测试，
  实际复现相同 PostgreSQL 语法错误；修复后同一测试通过。
- `git diff --check` 通过。

测试边界：仍排除此前已记录的三个无关失败测试：
`TestTelegramWebhookAttemptIndexRoundTripAndRedisFallback`、
`TestTelegramPendingRecoverySelectsChannelHeadsBeforeLimit`、
`TestTelegramPendingRecoveryPostgresSelectsHeadsBeforeDueFilters`。
架构工具自身测试通过；业务扫描仍有 14 条存量发现，修改前为 15 条，
逐条对比无新增发现，未修改扫描器或基线。不能称为全仓所有检查通过。

## 部署与线上验收

不需要新增 SQL 迁移。需更新 `publish`、`account`、`web`；
为保持共享调度、Outbox 和恢复代码版本一致，部署时同时更新
`scheduler`、`background`。Account 保持严格单实例。

部署后对一个已确认有验证媒体的失败资料整组重新上架：
先确认正常 Bot 分支，再跟踪真实 429 的协议号交接。
验收必须看到 Account 实际发送结果、非空验证消息 ID 和
`sent / verify_confirmed`，不能只看交接任务已创建。
本地验证不代表线上故障已恢复，失败历史任务也不会因升级自动全部重发。
