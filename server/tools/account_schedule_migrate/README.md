# 旧账户调度唤醒的有界迁移

只处理 `asynq:{youban_publish_account_schedule}:pending` 中的旧账户调度唤醒。
真实 TG Job、Outbox、频道任务及 PostgreSQL 均不读取/修改。
默认 dry-run；本工具不会自动部署，也不会启动任何生产消费者。

## 运行前提

1. 确认新的 Background `accountschedule` V2 已在消费：ready/running、turn 有变化，业务账户租约有效。
2. 确认旧 Asynq 账户调度生产者已升级，pending 不再净增长；不要同时运行其他旧清理脚本。
3. 核对 `REDIS_ADDR`、DB（默认 3）、ACL；密码只放环境变量 `REDISCLI_AUTH`，不要传命令行或写入日志。
4. 先执行 dry-run，核对类型及来源。第一次 apply 限 100 条，验证 V2 保留账户唤醒、pending/work/quarantine 和内存再加大轮次。

从 server 目录运行：

```sh
# 连接信息由当前运维环境设置；工具不自动加载 SAE/线上配置。
export REDIS_ADDR='127.0.0.1:16379'
export REDIS_DB=3
# REDIS_USER 可选；REDISCLI_AUTH 通过安全环境注入。
go run ./tools/account_schedule_migrate --batch 100 --max-items 100

# 仅在已验证新 scheduler 后由操作者显式执行：
go run ./tools/account_schedule_migrate --apply --confirm-new-scheduler --batch 100 --max-items 100 --pause 200ms
```

默认 dry-run 只有 LLEN/LRANGE/HGET，不创建租约或临时 key。只抽样 pending 尾部
最多 batch 条；recognized 只是样本，不代表全队列去重数。max-items 每次必须为正，
batch 1～200，批间 pause 至少 100ms。apply 每批共享 5 秒上下文预算，命令失败立即
停止，不循环重试；不要一次把 max-items 改为千万。TCP dial 显式 3 秒、读写 2 秒；SIGINT/SIGTERM 取消上下文，保留未完成 work。

## 协议和恢复

- apply 持有迁移租约后设置旧 queue 的 Asynq `paused` 键，阻止旧消费者继续领取；不取消或删除已经 active 的 8 个任务。每批/删除都检查 paused 存在，操作者解除 pause 时立即停止。退出故意保留 paused，迁移期间不要手动恢复旧消费者。
- 锁、work、quarantine 都在旧 queue 的 Redis hash tag 中，使用固定 v1 名字。
- 先取得 30 秒 token 租约；每批检查并续租（批预算 5 秒，小于租约 30 秒），所有迁移 Lua 都检查 token。
- 有 work 时优先恢复；否则最多批量 LMOVE batch 个 ID，从 pending RIGHT 到 work LEFT。
  保留项绝不放回 pending RIGHT，避免旧脚本反复取同一个任务。
- 仅识别 Asynq v0.25 TaskMessage：type、queue、id 必须匹配，非聚合任务；payload
  仅允许正数 tenantId/accountId 和可选 sourceId=0。重复/未知 JSON 字段及未知格式不删除。
- 相同 scope 每小批仅 Mark 一次，不建立全量 seen 集合。使用已有
  `accountschedule.Queue.Mark(Changed)`，在当前账户正运行时也递增 generation，避免迟到 ACK 吞掉旧唤醒。
- 元数据采用有界 LRANGE + HMGET pipeline，每个 scope 的 Mark 全部成功后再提交 finish pipeline。finish 按 work RIGHT 出队顺序执行；每一条 Lua 都重新检查 token/paused/state/msg。pipeline 某条报错时后续命令可能仍执行，因此逐项检查所有结果；遇到所有权冲突时该 work 尾项保留，后续顺序检查失败，重启从未完成 work 恢复。
- Mark 成功后再 Lua 原子检查 work 尾部 ID、task state=pending、msg 原样以及无其他
  lease/retry/scheduled/archived/completed 所有权；仅 UNLINK 此 task Hash、RPOP 此 work ID。
  不扫描 active 巨大列表；pending 已被原子移出，正常 Asynq consumer 无法再领取此 work ID。
- 未识别/不再 pending 的项 LMOVE 到 quarantine；Hash 原样保留。其他 ownership 冲突直接
  停止，work 原样保留，需人工调查。不要直接删除 quarantine。
- 如果进程在 Mark 后/删除前断电，下次重复 Mark 是幂等的合并唤醒；如果 Mark 失败，旧
  Hash 和 work ID 仍保留。若丢租约，迟到进程不能删除数据；退出仅释放自己 token 的锁。
- work/quarantine 不设 TTL，不清空；锁 TTL 不作为任务淘汰机制。工具不删除 Asynq unique
  string（可能被别的同 scope 任务拥有）；原来的 Unique TTL 自然释放，不影响迁移唤醒。
- 不用 LREM 大列表、KEYS、MONITOR、全量 LRANGE、Task Hash 任意 EXPIRE 或全库淘汰。

当前默认 batch 100/pause 200ms 是限速起点，不保证迁移时间。观察 Redis latency、CPU、
AOF、V2业务执行；有超时先暂停，不通过提高并发追速度。历史内存减少与实时命令降压需分别验收。

## 隔离验证

```sh
go test -race ./tools/account_schedule_migrate -count=1
```

测试只使用每个用例新建的 miniredis（DB15），不读生产地址。覆盖有界处理、相同账户合并、
未知项隔离、Mark失败保留、Mark后丢租约不删数据、中断work恢复、running ACK不吞唤醒，以及 pipeline 顺序、部分失败恢复、元数据变化不删和同 scope 每批一次 Mark。

性能上限：batch=200/pause=100ms 的理论长期上限为 2000 条/秒，实际还需扣除 Redis 命令和 Mark 耗时；不能宣称此限制下实测超过 2000 条/秒。先评估 Redis 延迟再决定是否调整安全限速。

## 可选进程级唤醒合并

默认仍每批调用 Mark，保持原保守行为。确认旧 Asynq producer 全部退出后，才可显式开启：

```sh
./account-schedule-migrate-linux --apply --confirm-new-scheduler \
  --coalesce-wakes --confirm-legacy-producers-stopped \
  --batch 200 --max-items 1000 --pause 100ms
```

此模式把成功 Mark(Changed) 的 `(tenantId,accountId)` receipt 留在本进程内，后续批次
同账户不重复递增 generation。最多 10,000 distinct scopes，无 LRU/淘汰；达到上限
立即停，当前 work/Hash 保留。只有 Mark 明确成功后才建立 receipt。进程中断 receipt
丢失，重启会再次安全 Mark，不会漏唤醒。它只适用于已停止生产的历史、无时间/资料请求
语义的账户唤醒；不能作为实时任务的系统级永久去重。

所有 apply 都在持有租约时原子 pause 并读取初始 `pending + work`，本次处理上限为
`min(max-items, initial_count)`。从 RIGHT 消费旧项，后续 producer LPUSH 到 LEFT 的新增项
不会进入本次初始额度。该界限用于防止追着队列新流量持续执行，并不替代停止旧 producer
的前提。未知 payload 仍隔离，实际 TG Job/Outbox 不变。
