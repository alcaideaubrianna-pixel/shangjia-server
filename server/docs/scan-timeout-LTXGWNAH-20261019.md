# LTXGWNAH：Redis 适配器差异导致查询降级

线上版本 b00a950，account 单实例。2026-10-19 16:49:40–48 的追踪日志确认：Telegram 解析 1383ms、图片下载 61ms、冷指纹处理 83ms，MD5 434ms、精确 pHash 48ms，随后近似查询约 4605ms 超时。5 秒查询配置已生效。

关联窗口日志发现，新增策略缓存使用 `GetOrSet(ctx,key,func,ttl)`。GoFrame 内存适配器兼容执行函数值，Redis 适配器则尝试序列化该函数并返回 `can't marshal func(context.Context) (interface {}, error)`；优化 SQL 因而没有执行，回退旧分桶并超时。之前内存缓存回放未覆盖这一差异，这是漏测。

修复为专门的回调接口 `GetOrSetFunc`，仅缓存返回的策略布尔值。阈值 12、权限过滤及 5 秒可配置预算均未变。

## 验证

- 独立临时 Redis（127.0.0.1:16387、上限 32MB、不持久化）+ 本地 PostgreSQL：修复前精确复现线上序列化错误，修复后真实 Redis 集成测试通过。
- 针对性 PostgreSQL 集成测试和两个业务包 race 检查通过。
- LTXGWNAH 真实缓存指纹 + 真实账户权限 + 生产只读 PostgreSQL + 独立 Redis：首次预检约 2.02s；独立 SQL 核对后，重新启动临时 Redis 清空测试缓存的正式回放首次 366ms、重复 9ms，候选集合一致。独立基准采用完整权限范围扫描计算距离，不使用 LSH 邻域筛选。此次没有人工逐张判断视觉一致性，不将距离匹配称为完全相同图片。
- TL3W7PG0 四张真实指纹的单图和 2/3/4 图组图，在 Redis 上共 14 次正式回放通过：首次 52–226ms、重复 5–7ms。

测试没有发送 Telegram 消息，没有清理或重启生产 Redis。上述为搜索服务实测，不包含 Telegram 收发，不保证任意网络和数据库负载下的端到端耗时。

## 后续回归入口

`YOUBAN_SCAN_STORAGE_PG=1 YOUBAN_SCAN_TEST_REDIS_ADDRESS=127.0.0.1:16387 go test -race ./addons/youban_publish/logic/sys -run '^TestMediaSearchStorageLocalPostgres$' -count=1`

Redis 必须是独立测试实例。生产只读指纹回放 `TestBotSearchProductionFingerprintReplay` 也支持该环境变量；每个用例使用不同 Redis DB 保持隔离，重新测冷查询应重启临时实例，禁止对生产 Redis 执行 FLUSHDB。

应用修复需构建发布 account，先停旧实例、确认 0，再启动新版本单实例。线上尚未完成该代码修复的部署验证时，不宣称生产已恢复。
