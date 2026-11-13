# 8. HotGo 实现规范

- 使用 GoFrame v2 的 `gdb、gerror、gctx、gtime`；
- Controller 只透传，业务写在 logic，数据库访问通过 DAO/Model；
- 业务错误使用中文 `gerror.New/Wrap`；
- 缓存通过 `cache.Instance()` 或基础设施层统一封装，业务代码不得直接调用 Redis；
- 队列 Topic 和缓存 Key 集中定义；Key 必须带租户和频道；
- 定时任务通过 `cron.Register` 注册，消费者通过 `queue.RegisterConsumer` 注册；
- 新逻辑必须有中文注释、结构化日志和可运行回放；
- DAO 生成文件禁止手改；
- 不以单元测试替代 PG、Redis、TG Fake 的真实链路验收。

## Redis 操作

业务 logic 不得直接调用 `g.Redis().Do()`。普通缓存使用项目统一的 `cache.Instance()`：

```go
err := cache.Instance().Set(ctx, key, value, 15*time.Second)
value, err := cache.Instance().Get(ctx, key)
_, err = cache.Instance().Remove(ctx, key)
```

ZSET、Stream 等基础设施能力必须封装在 `internal/library/cache` 或 `internal/library/queue`，业务层只调用语义方法，例如 `cache.ZSetAdd`、`cache.ZSetRemove`。Key 必须集中定义，并包含租户、账户或频道隔离字段。Redis 只做热索引，写入失败不能回滚 PostgreSQL 事实任务，必须由 Outbox 回填。

## SQL 与代码生成

优先使用 GoFrame 生成的 DAO、DO、Entity 和 `dao.Table.Columns()`：

```bash
gf gen dao
```

生成文件禁止手改。业务查询通过 DAO/Model 完成，写入使用 DO 白名单；不要在业务写入中使用 `g.Map`。只有跨数据库方言、迁移脚本或性能诊断 SQL 才允许使用参数化原生 SQL，并且必须限定租户/账户/来源范围。

```go
row, err := dao.YoubanPublishTgJob.Ctx(ctx).
    Fields(dao.YoubanPublishTgJob.Columns().Id,
        dao.YoubanPublishTgJob.Columns().Status).
    Where(dao.YoubanPublishTgJob.Columns().TenantId, tenantID).
    Where(dao.YoubanPublishTgJob.Columns().ChannelId, channelID).
    WhereIn(dao.YoubanPublishTgJob.Columns().Status, statuses).
    OrderAsc(dao.YoubanPublishTgJob.Columns().Id).
    One()
```

更新使用生成的 DO：

```go
_, err := dao.YoubanPublishTgJob.Ctx(ctx).
    WherePri(jobID).
    Data(do.YoubanPublishTgJob{Status: terminalStatus}).
    Update()
```

索引先用 DBX `EXPLAIN` 验证，再通过迁移脚本使用 `CREATE INDEX CONCURRENTLY`；执行后检查 `pg_index.indisvalid` 和 `indisready`。禁止用全表 `COUNT(*)`、无条件 `SELECT *` 或无范围历史关联作为线上常规路径。
