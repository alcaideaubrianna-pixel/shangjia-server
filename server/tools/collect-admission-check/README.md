# 采集准入二进制回归

在仓库根目录执行；不使用 `go test`：

```sh
python3 server/tools/collect-admission-check/build.py
python3 server/tools/collect-admission-check/postgres.py
```

`build.py` 保持原有媒体准入、发布准入及并发检查，完成后自动执行 `postgres.py` 的独立本地 PG 回归。任一断言失败则整体非零退出；原有 `publishCases` 保持不变。`postgres.py` 仍可单独运行。

## 本地 PostgreSQL 边界

- 每次创建独立临时 PostgreSQL 数据目录，监听随机的 `127.0.0.1` 端口；结束后停止并清理。
- 默认使用 `/opt/homebrew/opt/postgresql@17/bin`。其他安装位置用 `COLLECT_ADMISSION_PG_BIN` 指定包含 `postgres/initdb/pg_ctl` 的服务端目录。
- 不读取应用配置，不使用现有 DB 连接参数，不启动应用、Redis、媒体下载或 Telegram 发送。
- 使用临时 Go overlay 编译实际业务包并运行二进制，不改动业务包以导出测试符号。
- 仅将 `enqueueCollectMediaCache` 的外部队列投递替换为本地 spy，记录投递事件 ID；`enqueueCollectProcessDeferred` 的外部队列边界转接实际 `ensureCollectProcessOutbox`，验证真实 schedule 和 Outbox SQL 而不启动 sourcequeue。真实 ensure／prepare／commit、窗口查询、调度探测和状态更新保持原实现；不模拟准入判断，也不连接 Redis。fixture 使用现有 cache 的内存适配器以满足 HEAD 的目标窗口前置检查。

## PostgreSQL 用例及覆盖范围

使用 source351 / tenant78 / account615 的事故标识作为本地合成 fixture；不读取生产数据。

1. 两天前的展示事件缺少配对验证：实际调用 `commitCollectMaterial` 返回可重试错误；最小资料／分发表均保持空，窗口关闭也不允许就绪。
2. 重复展示映射到 canonical 事件：缺配对时不准入。
3. 配对验证事件已提交，但视频媒体行缺失：不准入。
4. 视频引用存在但缓存资产尚未准备：实际调用 `ensureCollectPairedVerifyReady`、`prepareCollectMaterial`、`commitCollectMaterial`，验证不准入且真实表名的资料／分发哨兵表仍为空；验证投递对象是配对视频事件。
5. 另一个未提交事务写入 ready 视频：准入事务不可见；回滚后仍无资料／分发。
6. 视频只绑定 duplicate：不能授权 canonical 事件；必须存在 canonical 配对。
7. canonical 配对和视频资产均提交后，原事件与 duplicate 重试十次：fixture 资料／分发表各一条。
8. fixture 分发插入失败：资料写入随事务一起回滚。
9. 暂时重命名本地事件表注入查询失败：实际 ensure 和 settlement 必须返回错误；窗口关闭也不能把失败当作成功。
10. 暂时重命名本地媒体表注入查询失败：实际 ensure 必须返回错误，禁止 `ready=true, err=nil`。

真实执行了配对查询、媒体读取、媒体就绪检查、合并和媒体准入校验。缺配对分支执行真实资料提交入口；成功落库使用明确隔离的 `admission_fixture_profile/dispatch` 最小事务 sink，duplicate 映射由 `admission_fixture_alias` 表表达。

**这些用例不证明完整生产 `commitCollectPreparedProfile`、真实 duplicate 解析、完整 dispatch／发布任务创建链路或生产并发幂等已通过集成验证。** 成功路径的唯一约束与事务断言属于 fixture。它们用于验证真实准入函数对 PostgreSQL 已提交／未提交事实的判断，不能替代端到端回归。

## 数据库错误回归（2026-11-19）

曾复现媒体查询失败时 `ensureCollectPairedVerifyReady` 返回 `ready=true, err=nil`。准入现直接读取媒体行并传播查询错误，空媒体集合保持未就绪。严格失败断言保留，修复后完整 `build.py`（含 PostgreSQL 二进制）运行通过。

此修正不修改通用布尔缓存 helper，不新增查询：替换原先该 helper 内部的媒体查询。成功落库范围仍限于上面说明的 fixture sink。

## 等待验证调度回归

已统一现有事件重试、来源／目标窗口筛选和 `nextCollectProcessDelay`：本地入库时间尚未超过验证窗口时沿用 10 秒，过期仍未配对时复用 `collectMaterialVerifyRetryDelay`（1 分钟）。缺失入库时间使用短重试，不用历史 Telegram 时间猜测过期。已配对事件离开 waiting_verify，保留原有定向唤醒能力。

Outbox 的 pending 截止时间按最早值合并；processing 当前租约重试可以设置下一轮时间，旧租约不能覆盖其他正在执行的租约或重开已完成事件。只有 schedule 入口的显式唤醒意图可以重开 delivered 事件，保留 HEAD 对普通生产者不得重开 delivered 的保护及 history 记录。

本地 PG 直接调用实际来源过滤、目标窗口、next-delay、waiting 更新、`scheduleCollectDisplaySettlement` 和 `ensureCollectProcessOutbox`，验证：

- 旧事件一分钟退避、新事件十秒重试，以及 NULL 入库时间时，来源 SQL 筛选与 Go 延迟一致。HEAD 的显式 target 则始终保留锚点，靠持久化任务截止时间控制自动重试，不因来源退避屏蔽新验证的定向唤醒。
- 两类事件混合时取较早截止，不让旧事件遮挡新事件。
- 到期可读取，处理后更新时间恢复退避，避免来源调度空转。
- 显式唤醒提前 available_at，晚到的旧重试保留早截止；16 个并发重试也不能推迟它。
- 旧任务不能错误确认新唤醒、覆盖新租约或重开 delivered；当前租约可正常延后重试。
- 持久化配对后绕过未配对退避，已有调度入口可以重新唤醒。

未新增全局扫描、缓存或 ENV。隔离 tree 的集成基于线上 dfad8b3，保留 HEAD 的 candidate index、schedule probe（最多 100 条）及一次有界 PG fallback；仅在现有候选页计算中调整等待间隔，没有带入 root 的旧版 next-delay 多查询实现。未进行线上性能压测或索引执行计划验证，不据此宣称生产 DB 负载已有下降。
