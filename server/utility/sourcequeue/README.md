# 来源独立队列

边界：本包只提供 Redis 队列设施，不启动 Worker，不读取业务配置或执行规则。
生产构造使用 `New(namespace)` → `cache.RedisClient()`，不建立另一份 Redis 配置。
同一消费阶段的所有生产者、消费者必须使用相同 namespace。

## 契约

- `Scope{TenantID int64, SourceID int64}` 均要求正数。
- `Lane` 接受 `Realtime`、`History`、`Fresh`，没有超时后自动降级。
- `Enqueue` 等价于零延迟 `EnqueueAfter`；ID 是 string，payload 是二进制。
- 同 ID 在 namespace/scope/lane 内合并直到 Ack，payload 最新写入胜出。
- 重复 enqueue 不增加 pending list 成员，也不覆盖当前 token/租约。
- Claim 每次只领取一个任务；每来源每 lane 最多一个 inflight。不同 lane
  刻意允许并行，调用方须在同一总预算内限制各通道资源；跨 lane 业务写入仍需幂等。
- Claim 无任务返回 `(nil,nil)`；高竞争下有限次尝试后也可返回空，调用方退避轮询。
- Ack/Nack/Renew 必须携带原始 Claim，租约过期或 token 不符返回 `ErrLeaseLost`。
  消费者收到该错误应取消工作，不得继续确认；token 只保护队列操作，外部业务
  事务仍需唯一键或版本保护，不能用 Redis 租约代替数据库幂等。
- Enqueue 发生在 Claim 期间会增加 generation。旧 Ack 释放旧槽，但保留并重新
  投递最新通知；即使重复通知内容相同，也允许额外处理一次。
- Claim 期间最后一次 EnqueueAfter 的 delay 在 dirty Ack 时保留。Nack 的显式
  delay 则决定本次重试时间，包括期间新增的同 ID 通知；payload 仍是最新版本。
- 已在延迟索引中的任务重复入队可提前到期，不推迟原到期时间。已 pending 的
  任务不因 EnqueueAfter 被推迟。延迟任务不占槽，来源其他任务可继续处理。
- 失联消费者到期恢复后按重试处理，立即重新排队；外部媒体等待应主动 Nack。
- Ack 后同 ID 可重新创建；重复 Ack 返回 ErrLeaseLost，不承诺 exactly-once。

## 恢复和公平性

调用方必须周期性对所有启用的 lane 调用 `Recover(ctx,lane,limit)`，limit 为 1..1000。
Recover 每轮最多读取两个索引各 limit 条，最多尝试恢复 limit 条；两种索引
交替选择，即使 limit=1 也不会使租约或延迟任务永久饥饿。多个实例同时恢复会
再次原子校验 deadline，重复恢复不会重复推入 pending。

活跃来源使用按激活序号排序的有序集合，Claim 取一个来源、一个任务，完成后
把仍有积压的来源放到队尾。没有来源注册全量扫描，没有每来源常驻消费者。
来源 pending list 物理独立；Redis key 使用 namespace SHA256 hash tag，所有
Lua key 显式通过 KEYS 声明且同槽。共享一个 namespace 的索引仍由一个 Redis
分片承载，不表示跨分片吞吐无限扩展。

原子脚本覆盖入队/激活、领取/租约、确认/重新唤醒，避免 enqueue 与 Ack 交错
丢唤醒。Claim 已提交但响应丢失，通过到期租约恢复。业务为至少一次交付。
任务、payload、索引均不设置 TTL。Redis 的持久化、复制和禁淘汰策略由部署
保障；本包不改变服务端配置，也不替代原始消息的数据库持久化。

## 本地测试

测试通过内部 `newQueue(namespace,client)` 注入独立客户端，不调用 New，不读取
项目生产配置。默认临时 miniredis；真实 Redis 仅接受显式 loopback IP 和端口：

```sh
cd server
SOURCEQUEUE_TEST_REDIS_ADDR=127.0.0.1:16391 GOMAXPROCS=4 \
  go test -p 2 ./utility/sourcequeue -count=1 -timeout=60s
```

固定 DB 0、无密码。每测试使用独立 namespace，清理仅删除已知测试 key，不做
全库枚举。测试覆盖公平性、lane 隔离、并发唯一领取、同 ID dirty Ack、延迟
释放槽、续租与迟到 token、恢复有界性、取消重试、响应丢失和并发丢唤醒。

## Collector 新交付隔离

`YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED=true` 仅允许首次实时 Event 交接使用 Fresh；默认关闭。
来源、租户、Delivery ID、原始消息和业务分组键均不变。恢复、历史和已有任务不会批量升级。
Fresh 与旧 Realtime 共享原实时 worker 总额度：额度8时分配6/2，额度1时同一worker轮转两个通道。
不同通道可并行，业务端仍须使用既有唯一键、消息组租约和数据库领取CAS，不承诺跨lane严格FIFO。

上线先部署支持三个通道的 collector，确认消费者启动，再通过ENV启用。
观察 Fresh 入队到领取延迟、旧实时队列净变化、业务资料生成、重复率和PG内存/IO。
已存在的积压继续原地消化；本开关不会立刻清掉它们，也不保证下游下载/分组/发布没有等待。
停用时设置开关false，新版本仍会消费Fresh残留；必须等Fresh待执行/处理中/延迟索引排空后，
才能回退到不认识Fresh的旧镜像，否则已接收任务将无人消费。不得直接删除旧队列或批量重放。
