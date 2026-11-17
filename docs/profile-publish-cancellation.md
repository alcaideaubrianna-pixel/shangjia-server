# 单资料发布取消

接口：`POST /publish/admin/profile/publish/cancel`

```json
{"uuid":"资料UUID","operationNo":"task-status 返回的不透明轮次标识"}
```

接口沿用管理端租户和可管理账号权限。必须同时提交 UUID 和操作号；
不接受“取消最新一轮”、频道或 Job ID。成功返回现有响应封装中的
`id / uuid / status / taskStatus / operationNo`。`taskStatus=canceling`
表示取消意图和清理意图已经持久化，不代表 Telegram 清理已经完成。

`GET /publish/admin/profile/task-status` 返回相同的操作标识，以及可选
`taskError`（等待发送回执或清理的说明）。此 GET 只读，不执行恢复或清理。
同轮重复取消返回当前状态；轮次不符或已经完成/失败的发布拒绝取消。

## 状态和消息范围

- 普通上架、重新上架和资料循环上架统一使用同一入口。
- 精确范围为 `tenant_id + account_id + profile_id + operation_no`。
- 取消记录是持久 tombstone，不是另一套发送队列。停止生成本轮 Job，
  活动 Job 经现有 `telegramJobTransition` 迁移为 `superseded`，
  发布 Outbox 收口；历史轮次不受影响。
- 受理事务只锁资料状态并保存取消/清理意图。Job 停止在提交后的独立、
  可重试事务中执行，不持有资料状态锁等待 Job 行锁；即使该阶段超时，
  已提交的取消意图仍会阻止新发送阶段，并由现有恢复周期继续停止 Job。
- 资料 `status / visibility` 不由取消流程修改，也不关闭频道循环。
  已删除的历史消息不能通过取消重建。
- 删除本轮新增消息使用现有 Cleanup Outbox 和删除 Worker。取消事务内
  持久化初始清理意图；迟到的每个新消息 ID 都有独立、幂等的清理意图，
  不依赖一次 best-effort Redis 入队。
- Bot / 协议号响应和 Webhook 仍保存审计、消息 ID；取消后不再继续验证、
  降级或把资料投影为成功。重复回执不会复活已经删除的消息。
- 现有发布恢复周期按 `next_check_at,id` 有界轮转取消记录，等待在途
  Attempt、协议号 processing/finalizing 任务和消息清理完成后写入
  `canceled`。展示状态与 operationNo 保留到下一轮发布开始。
- 未知发送结果不能按本地超时推断为未发送。缺少远端回执时保留
  `canceling` 并返回原因；明确拒绝且没有消息 ID 的结果可安全收尾。
  永久不可删消息也不会被伪报为清理完成，需要按日志人工处理。

资料循环的 `profile_cycle:<run_key>` 使用实际 `run_key` 定位 Run，
不是把后缀当数字 ID。停止 Run 在独立事务中执行，避免与循环创建的
Run → profile_state 锁顺序反转。频道批次可能包含多条资料，不取消整批。

## 启用和回滚约束

默认开启：`YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED` 未设、为空或为 `true`
时启用取消发布。显式设置为 `false` 可关闭；关闭时接口拒绝受理，
正常发送不会访问新取消表。其他非空且不是 `true` 的值也按关闭处理。

1. 默认开启要求迁移先于新版进程启动；分阶段升级时先统一显式设置开关为 `false`。
   执行 `server/addons/youban_publish/resource/sql/profile_publish_cancel.pgsql.sql`。
   现有发布状态字段、Attempt 表及 Cleanup Outbox 的
   `operation_no / payload / dedupe_key` 必须已完成原有迁移。
2. 升级全部 Web、发布、协议号执行和调度实例。旧实例不认识取消围栏，
   不能与启用后的新实例共同处理取消任务。
3. 默认无需配置开关。若升级期间显式设置了 `YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED=false`，
   全部实例升级完成后统一移除该配置或设置为 `true`。
   现有 publish 恢复周期和 Cleanup Outbox Worker 必须保持运行。
4. 有未完成取消时不能直接撤销开关或回滚至不认识取消的旧版本。
   先停止新取消受理并完成/人工处理既有取消，再安排回滚。

本改动不自动执行迁移，不部署，也不修改生产数据。

## 隔离验证

`TestProfileCancelContract` 无需外部服务。PostgreSQL 测试显式使用
`127.0.0.1:55487 / cancel_test / postgres`，每个测试创建独立 schema，
结束后删除。不会读取应用 DSN、启动消费者、访问 Telegram 或共享 Redis。

```sh
cd server
GOMAXPROCS=2 GOGC=30 YOUBAN_PUBLISH_CANCEL_TEST_PG=1 \
  go test -p 1 -gcflags='hotgo/addons/youban_publish/logic/sys=-l' \
  ./addons/youban_publish/logic/sys \
  -run '^TestProfileCancel' -count=1
```

测试覆盖权限下的精确归属在业务方法中的传递、事务回滚、取消幂等、
旧轮次拒绝、跨租户 Job 隔离、取消/生成并发、迟到消息持久清理、
重复回执、成功回调不能覆盖取消、终态轮次保留。HTTP 权限中间件
沿用原有管理端实现，不用这些隔离测试冒充线上验收。
