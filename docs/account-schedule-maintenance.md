# 账户调度维护

账户调度保存 tenant/account 合并状态，不保存每次扫描的独立任务。真实发布 Job、Outbox 与 Telegram 发送对账不由这里删除。

## 扫描与恢复

Scheduler 的目录扫描以及 Cron、启动恢复入口共用目录租约和缓存 TTL 闸门。默认每 300 秒最多一批 100 个启用账户；完整遍历时间随账户数量增加。每批在 10 秒预算内执行；失败不推进游标，等待下一次间隔重试。闸门使用现有缓存适配器，生产多实例须共用 Redis 缓存。

Background 账户消费者领取租约 45 秒，业务预算 25 秒。ACK 检查 token 和租约期限，过期任务在后续领取时恢复。Context 超时依赖下游遵守取消，不会强杀 Go goroutine。

- Success：保留一份账户状态，默认 300 秒后兜底。
- More：前 3 次快速续跑，持续续跑逐渐让步，最多等待 1 秒；不能据此断言业务没有进展。
- Deferred：账户锁争用，保留 1 秒短等待。
- Stalled：本轮总预算耗尽，1 秒起指数退避，最多 300 秒；输出中文 Warning。
- Failure：真实错误保留已有 1～60 秒退避，不能被重复目录发现绕过。

Changed 可提前唤醒空闲或预算退避账户；不会把已就绪的早期账户排到队尾。真实失败退避继续保留。到期计划可以提前唤醒等待账户。执行中的变更通过 generation 保存。

## 失效状态回收

每轮先按 tenant/account 主键核验账户存在性。查询出错时保留状态并退避；只有确认账户不存在或软删除才申请 Retired。Redis ACK 原子验证当前 token、未过期租约、没有新增 generation 且没有未来计划后移除状态和索引。

禁用账户、切换引擎账户仍保留状态，避免误跳过其清理和恢复工作。Retired 不清理数据库业务记录，不删除尚未确认的 Telegram 发送任务。不设置整个账户状态 Hash 的 TTL。

## 历史积压

继续使用 tools/account_schedule_migrate 工具；先确认旧生产者停止且新消费者运行。默认 dry-run，apply 必须显式确认。每批最多 200 条，至少暂停 100ms，每批 5 秒预算；只迁移起始快照和 max-items 限定的旧账户唤醒。先成功标记新调度，再核验旧归属并删除旧 Hash；异常项隔离，work 支持恢复。不自动在应用启动时扫描历史积压。

## 配置与发布

- YOUBAN_PUBLISH_ACCOUNT_SCHEDULE_INTERVAL_SECONDS：目录批次间隔，默认 300。
- YOUBAN_PUBLISH_ACCOUNT_SCHEDULE_IDLE_INTERVAL_SECONDS：成功空闲兜底间隔，默认 300。

发布 Scheduler 和 Background；Cron 如在其他角色运行，也需更新对应角色。旧实例仍可能使用旧频率，必须核验镜像和已有 ENV。上线后观察 ready/running/state/wake 数量、旧 pending/work/quarantine、预算耗尽告警、数据库调用及 Redis 内存；代码测试不能替代线上验证。
