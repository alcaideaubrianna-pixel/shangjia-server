# 发布链路修改边界

本文补充仓库根目录 `AGENTS.md`，不替代 HotGo 分层规范。修改前阅读
`docs/publish-architecture.md`（仓库根目录）及对应状态机、租约和队列实现。

- 沿用现有 TG Job、Outbox、Attempt；不得另建平行发布引擎或恢复消费者。
- Job 生命周期、调度状态和发送阶段使用 `JobStatus`、`DispatchStatus`、`SendPhase`。
  不得通过改回 `string`、增加类型别名或任意强转绕过编译错误；旧协议适配入口必须解析校验。
- 生命周期修改经过 `telegramJobTransition`。附加更新字段不得包含 `status`、
  `dispatch_status`、`send_phase`；发送阶段不得回退已确认的远端进度。
- 频道消费使用带 token 的 claim、续租、settle 协议。不得恢复直接 ZREM 出队的旧生产入口；
  ACK、续租和清理必须核对当前 token，判空和移除活跃标记必须原子执行。
- unknown 先对账。Redis/数据库超时不代表 Telegram 没有发送，禁止以本地失败直接重发。
  对账 checkpoint 不是排他锁，同任务对账需持有归属租约。
- 恢复步骤必须有界、可轮转；历史回填不得阻塞实时消费、Watchdog 或请求。
  SQL 的 LIMIT 不保证扫描有界；不得新增全局历史 Job 扫描或启动自动回填。
- 禁止通过批量刷新架构基线消除报错。基线变更必须逐函数、逐表达式说明职责和必要性；
  不得增加目录通配豁免，不得削弱 guard 或 CI 来通过构建。
- 新增后台入口要说明运行角色；新旧实例共存、取消、崩溃恢复和迟到 ACK 都需要验证。
- 本地测试不得读取生产 DSN、启动生产消费者或清空共享 Redis。
- 采集事件的 `source_id` 是规则归属，不得按频道重新选择其他来源。同一 Bot、
  同一频道允许存在多个独立来源；共享频道元数据不能当作来源唯一性约束。
  规则、后缀、目标频道及缓存按 tenant/account/source 隔离。某来源被禁用、
  未绑定规则或规则被禁用时，不得改用其他来源的规则继续执行。

从 `server/` 执行 `go test ./tools/architecture_guard`、
`go run ./tools/architecture_guard` 和 `go build ./...`；再按架构文档运行受影响的隔离测试。
通过编译不等于线上恢复，未经部署验证不得声称生产故障已经解决。
