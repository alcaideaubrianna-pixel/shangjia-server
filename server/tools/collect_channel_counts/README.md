# 采集频道计数迁移与对账

列表只读 `hg_youban_publish_collect_channel_count`。资料归属仍保存在原有
`hg_youban_publish_collect_channel_profile`，不在 HTTP 请求中扫描 dispatch/job/origin。

数据库触发器在同一事务中维护计数。`count_member` 保存规范化频道、资料与规则的引用数，
防止正负频道 ID 别名或多规则造成重复计数；rule_id=0 表示跨规则去重数量。
计数只包含资料和账户归属仍有效的关系。单纯上下架不会改变数量。
资料软删除、物理删除、恢复及账户归属变化均触发同步。

## 发布顺序

1. 显式执行 `addons/youban_publish/resource/sql/collect_channel_counts.pgsql.sql`。
   迁移可重复执行，设置了 3 秒锁等待和 30 秒语句超时；失败会整笔回滚。
   不得在服务启动或 HTTP 请求中自动执行迁移。
2. 回填已有关系。凭据通过受保护的 `YOUBAN_COLLECT_COUNT_DSN` 环境变量传入，工具不输出凭据。

```sh
go run ./tools/collect_channel_counts --tenant 78 --account 615 --batch 200
go run ./tools/collect_channel_counts --tenant 78 --account 615 --apply --batch 200 --max-batches 100
# 明确全库范围；每次最多 20 万行，未完成可重复调用。
go run ./tools/collect_channel_counts --all --apply --batch 200 --max-batches 1000
```

`count_projected=false` 是持久化检查点。每批独立提交、失败停止；重跑不重复加数。
新旧应用写入均由触发器覆盖，不依赖单实例或 Redis 锁。
初始化中的频道返回明确错误，不会把未初始化计数当成 0，也不会回退历史大表扫描。

3. 如历史来源索引缺失，逐频道预览、补全。只修复仍有效且有精确规则归属的关系，
   不重放消息、不重新入库或推送。没有规则/来源证据的记录不猜测归属。

```sh
go run ./tools/collect_channel_counts --tenant 78 --account 615 --rule 338 --channel 4386299825 --reconcile-history
go run ./tools/collect_channel_counts --tenant 78 --account 615 --rule 338 --channel 4386299825 --reconcile-history --apply --batch 200 --max-batches 10
```

4. 对账通过再更新 **web**。触发器已覆盖 collector/publish 的既有写入路径，计数维护不需要改变角色职责。

```sh
go run ./tools/collect_channel_counts --tenant 78 --account 615 --rule 338 --audit
go run ./tools/collect_channel_counts --all --audit
```

审计对比有效归属明细与计数，发现差异返回非零退出码。历史预览用于发现明细本身缺失，
与计数审计是不同检查；不能把“计数审计通过”声称为所有历史来源已完整恢复。

## 本地验证

只接受本机 `127.0.0.1:35432` 的独立测试 schema，退出后清除 schema。
测试不会读取生产 DSN，也不启动业务消费者。

```sh
YOUBAN_LOCAL_COLLECT_PG=1 YOUBAN_SOURCE_DOWN_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys \
  -run 'TestCollectChannelCountPostgres|TestIntroFee|TestCollectRuleLifecycleLocalPostgres|TestCollectSourceDownHistoricalLocalPostgres' -count=1
YOUBAN_LOCAL_COLLECT_PG=1 go test ./tools/collect_channel_counts -count=1
```

覆盖 20,074 条历史关系、并发回填/重复写入、频道别名、多规则去重、事务回滚、
软删除/恢复、物理删除、账户归属变更、跨租户隔离、重复迁移、历史补全与计数漂移检测。
线上汉庭元数据与 70 份资料索引还覆盖两种列表路径和关键词搜索。

## 回滚

优先回滚 Web 读取代码，保留计数结构与触发器，旧版写入与读取仍兼容。
若触发器自身异常，需要先停止使用新计数读取，再显式移除本迁移创建的四个触发器；
保留基础资料和归属明细。停用期间计数会失效，重新启用前必须重建并对账，不能直接使用旧计数。
不得清空资料、dispatch、发送审计或 Redis 来恢复计数。
