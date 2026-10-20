# 后台扫描、统计与连接池优化（2026-10-20）

## 环境与范围

生产 7 个 SAE 角色镜像均为 `d36bfa5`。本轮代码在独立 worktree `/tmp/youban-sql-optimization-20261020` 开发，基线 `8729a3b3`，未触碰原工作区的未提交修改。数据库为 PostgreSQL `railway`，通过已配置 DBX / SSH MCP 操作。

## 已执行的线上 SQL

显式、逐条执行 `background_scan_indexes_20261020.pgsql.sql` 中两个 `CREATE INDEX CONCURRENTLY`，无业务数据更新。锁等待上限 3 秒、单语句 5 分钟、禁用并行索引维护。20:39（北京时间）确认两个索引 `indisvalid=true / indisready=true`，无索引构建残留；连接总数 142，阻塞连接 0。这是当时快照，不代表持续窗口零错误。

| 索引 | 大小 | 目的 |
|---|---:|---|
| idx_ybp_collect_media_invalid_ready | 8 KB | 仅索引缺少持久化文件的 ready 媒体，保留原 SQL 的 NULL/备份判断语义 |
| idx_ybp_collect_event_retention | 20 MB | failed/ignored 事件按 created_at、id 查找过期批次 |

在 `statement_timeout=3s` 下对有 LIMIT 的候选查询执行 `EXPLAIN (ANALYZE,BUFFERS)`：

- 无持久化媒体原查询：使用新 Index Only Scan，0 条候选，执行 0.060ms，shared hit=1，Heap Fetches=0。历史窗口同类 SQL 最长 23.694 秒；不同时间、负载，不能当作严格基准倍率。
- 过期事件新排序查询：created_at <= now()-7days，ORDER BY created_at,id LIMIT 1000，0 条候选，执行 0.039ms，shared hit=3，Heap Fetches=0。该查询排序需发布本轮代码才会被清理器使用，当前旧代码仍按 id 排序。

回滚索引须单独执行 `DROP INDEX CONCURRENTLY public.idx_ybp_collect_media_invalid_ready` / `DROP INDEX CONCURRENTLY public.idx_ybp_collect_event_retention`，不得放入事务。回滚代码不依赖删除索引；索引无字段或数据兼容性变更。同名索引重试前必须检查 indisvalid，IF NOT EXISTS 不会修复无效索引。

## 代码改动

1. 过期清理改为 created_at,id 排序，优先最早过期事件；保留原状态、时间条件、批量上限和事务删除逻辑。
2. PostgreSQL 统计采用 GROUPING SETS，一次扫描任务历史生成队列、频道、Bot 三组统计，再关联小维度表。保留全历史统计口径、NULL 语义、空队列名归一化、原错误信息 MAX 口径及写入事务。查询失败不会提交空统计。普通生产 EXPLAIN 确认任务表仅一个 Seq Scan；未执行生产全量 ANALYZE，也未验证新版本线上实际耗时。
3. 统计刷新增加事务级非阻塞 advisory lock，覆盖新版 scheduler 滚动重叠，事务结束自动释放；旧版刷新器尚无此锁，首次新旧混跑仍可能并发。MySQL 保留独立统计分支，频道/Bot 改为聚合后关联元信息。

## 连接池核对

7 个 SAE 角色均未发现 maxOpen/maxIdle/maxLifetime 的环境变量覆盖。Dockerfile 构建会将 config.example.yaml 复制为 config.yaml；d36bfa5 的该文件没有池配置。锁定依赖 GoFrame v2.9.4 的默认值为 maxOpen=0（无限制）、maxIdle=10、maxLifeTime=30s。此结论来自部署配置、构建配方与锁定版本源码，尚未从每个运行进程导出 sql.DB.Stats 或核对运行时文件挂载。

本轮镜像配置设 maxOpen=20、maxIdle=5、maxLifeTime=10m，并加入 YOUBAN_DB_MAX_OPEN / YOUBAN_DB_MAX_IDLE / YOUBAN_DB_MAX_LIFETIME 以及对应 GF 别名。当前仍运行旧镜像，**这些上限尚未在线上生效**。

| 角色 | 副本 | 新主库连接预算 |
|---|---:|---:|
| web | 2 | 40 |
| background | 1 | 20 |
| collector | 1 | 20 |
| media | 2 | 40 |
| publish | 2 | 40 |
| scheduler | 1 | 20 |
| account | 1 | 20 |
| 总计 | 10 | 200 |

数据库 max_connections=500，superuser_reserved_connections=10。即使保守按全部副本双份重叠，上限 400，仍需为运维、监控、其他应用预留连接。后续扩容必须重算预算；部署后观察连接池等待、请求延迟及队列吞吐，按角色调整，不能只看总连接下降。没有调高数据库 max_connections。

飞牛外部只读数据源另有独立池，现有 sourceDB 显式 maxOpen=3、maxIdle=1；它不计入本主库预算，未修改。

## 验证与发布状态

本地仅使用专用 PostgreSQL 127.0.0.1:55484（test/postgres）；没有操作其他测试集群。

- 新旧频道/Bot 查询双向 EXCEPT ALL 比较通过；合并统计与旧队列/频道/Bot 三组逐字段对比通过。1000 条构造任务覆盖不同租户/账号、全部常见状态、缺失元信息、NULL、限流错误、默认队列名归一化。
- 统计租约测试通过：第二连接直接跳过刷新，第一事务结束可重新获取。
- 池配置环境变量实际覆盖与别名优先级测试通过。
- 原统计错误截断、预期退出错误和日志保留相关测试通过；git diff --check 通过。

复现：在 server 目录执行：

```sh
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys -run 'Test(TelegramObserve|TruncateTelegramObserveError|ExpectedTelegramObserveShutdownError|PublishTaskLogRetention)' -count=1
go test ./internal/bootstrap/envconfig
```

本轮两个索引已生效；应用代码尚未合并、构建或部署。后续发布须保持 `YOUBAN_PUBLISH_PROFILE_ORIGIN_ENABLED=true` 和各角色现有环境变量，account 继续严格单实例交接。不要把源码提交当作上线。

## 尚需持续排查

本轮未改动指纹历史回填算法、TG 恢复任务、采集重试扫描及 pg_exporter 聚合 SQL；这些仍是上次窗口的慢 SQL 来源。没有移除账号行锁，也没有更改去重、发送或采集来源业务状态机。应在首轮上线后复查慢查询窗口，再按剩余热点推进，不承诺所有 SQL 超时已消除。
