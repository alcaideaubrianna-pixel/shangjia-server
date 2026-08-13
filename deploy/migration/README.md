# Railway 数据迁移工具

## 一键快速在线迁移（推荐）

`fast-migrate.sh` 是正式切换入口。它不会在本机保存 30GB 数据文件，而是通过
SSH/Railway 隧道使用 `pgcopydb` 并行复制 PostgreSQL，并使用 RedisShake 持续同步
Redis。所有密钥只保存在本机 `.migration-state/`，该目录已加入 `.gitignore`。

### 1. 迁移前只读检查

```bash
./deploy/migration/fast-migrate.sh precheck
```

检查内容：

- 本机 Docker、Railway CLI、SSH 私钥和必要命令；
- 腾讯云 PostgreSQL/Redis 容器和网络连通性；
- Railway PostgreSQL/Redis 变量和隧道；
- Railway 七个业务服务必须全部停止；
- Railway PostgreSQL 和 Redis 必须为空；
- 源库大小、表数量、Redis DB/Key 数量；
- PostgreSQL `wal_level`、复制参数和无主键表；
- 目标容量和迁移端口是否可用。

该命令不修改数据库、不重启 PostgreSQL，也不会启动同步。

### 2. 一条命令预同步并在凌晨切换

建议在迁移前一天执行：

```bash
./deploy/migration/fast-migrate.sh run 02:00
```

执行顺序：

1. 自动运行全部预检查；
2. 将源 PostgreSQL 配置为 logical WAL（首次会重启 PostgreSQL 一次）；
3. 自动创建独立迁移账号，并为无主键表设置 `REPLICA IDENTITY FULL`；
4. 启动 `pgcopydb clone --follow`：8 个表任务、8 个索引任务，大表按 CTID 并行拆分；
5. 启动 RedisShake：源 DB3 全量复制后持续追 AOF 增量；
6. 等待到下一次本地时间 02:00；
7. 禁止旧 PostgreSQL 业务账号登录、暂停源 Redis 写入；
8. 设置 PostgreSQL 最终 WAL 终点并等待 CDC 追平；
9. 同步 Sequence，优雅停止 RedisShake；
10. 精确校验全部 PostgreSQL 表行数、TG session、Redis Key/TTL/内容；
11. 将 Railway 服务统一切换到源 Redis DB，并按 Account → Worker → Scheduler → API 顺序启动；
12. 只有全部校验通过才执行可选切流 Hook。

> `run 02:00` 必须提前执行。若凌晨 02:00 才开始全量复制，30GB 数据不可能在分钟级完成。

### 3. 查看实时进度

另开终端执行：

```bash
./deploy/migration/fast-migrate.sh watch
```

或只显示一次：

```bash
./deploy/migration/fast-migrate.sh status
```

显示 PostgreSQL 表/索引复制进度、CDC Slot lag、目标数据库大小、Redis 源/目标 Key 数量和 RedisShake 最近日志。

### 4. 分步执行

如不希望命令一直等待：

```bash
./deploy/migration/fast-migrate.sh prepare
./deploy/migration/fast-migrate.sh watch
./deploy/migration/fast-migrate.sh schedule 02:00
```

也可以在确认停机后手动切换：

```bash
./deploy/migration/fast-migrate.sh cutover
```

### 5. 自动切换域名/Webhook

脚本不猜测 Cloudflare、Telegram Webhook 和支付平台的具体切换命令。可在执行前配置一个 Hook；
它只会在数据库和 Redis 全部校验通过、Railway 服务健康后运行：

```bash
export MIGRATION_CUTOVER_HOOK='./deploy/migration/switch-traffic.sh'
./deploy/migration/fast-migrate.sh run 02:00
```

### 6. 回滚与清理

切换异常时：

```bash
./deploy/migration/fast-migrate.sh rollback
```

该命令停止 Railway 业务服务、解除源 PostgreSQL 登录限制并解除 Redis 暂停；之后按原方式启动腾讯云业务。

确认迁移稳定后清理本机同步容器和隧道：

```bash
./deploy/migration/fast-migrate.sh cleanup
```

Telegram session 存储在 PostgreSQL，会完整复制。切换过程先停止旧账号运行时，再启动 Railway
`xiaohuiji-account`，不会主动注销 Telegram session，也不会让两个 Account 实例同时登录。

### 当前环境实测结果（2026-08-12）

- 源 PostgreSQL：约 30GB，190 张 public 表；
- 源 Redis：DB3，约 18.5 万 Key；
- Railway PostgreSQL/Redis：空；
- Railway 七个业务服务：全部停止；
- 源 `wal_level=replica`：`prepare` 首次会自动改为 `logical` 并重启一次；
- 无主键表：2 张，`prepare` 自动设置复制身份。

## 旧快照/单账号工具

目录包含四个工具：

- `pre-migrate.sh`：全量 PostgreSQL/Redis 快照，以及可选的 Railway 恢复；
- `system-snapshot.sh`：只导出系统配置、插件配置和权限配置；
- `account-snapshot.sh`：按 `tenant_id/account_id` 导出单个账号业务数据；
- `verify-migration.sh`：验证目标 PostgreSQL、Redis 和核心业务表。
- `compare-migration.sh`：停机迁移后，对比源端与目标端 PostgreSQL 全表行数，以及 Redis Key、TTL 和内容摘要。

默认 SSH 连接：

- 主机：`43.129.181.9`；
- 用户：`ubuntu`；
- 私钥：`~/.ssh/id_xiaohuiji`。

## 一、先做迁移能力检查

如果只想确认 SSH、Docker、PostgreSQL、Redis 和备份命令能否跑通，不生成全量文件，使用只读检查模式：

```bash
MIGRATION_MODE=check ./deploy/migration/pre-migrate.sh
```

该模式不会执行全量 `pg_dump` 导出、Redis RDB 导出、文件下载或目标写入。

## 二、预迁移快照

默认只读，不停止现网、不修改 PostgreSQL、不删除 Redis Key、不调用 Telegram API：

```bash
MIGRATION_MODE=snapshot \
MIGRATION_OUTPUT_DIR="$PWD/migration-artifacts" \
./deploy/migration/pre-migrate.sh
```

产物包括：

- `youban-postgres.dump`：PostgreSQL custom 格式全库备份；
- `youban-redis.rdb`：Redis RDB 备份；
- `postgres*.sha256`、`redis*.sha256`：校验文件；
- `manifest.txt`：迁移范围记录。

脚本会自动发现现网 PostgreSQL/Redis 容器，通过 SSH 下载备份，并显示每个阶段的进度。

## 三、测试恢复（不导入 TG session）

恢复前必须准备一个空的 Railway PostgreSQL 和 Redis。测试环境不要启动 `xiaohuiji-account`，避免尝试连接 Telegram：

```bash
MIGRATION_MODE=restore-test \
MIGRATION_REUSE_ARTIFACTS=1 \
MIGRATION_CONFIRM=I_UNDERSTAND_TARGET_WILL_BE_WRITTEN \
TARGET_PGHOST='<railway-pg-host>' \
TARGET_PGPORT='<railway-pg-port>' \
TARGET_PGDATABASE='<railway-pg-database>' \
TARGET_PGUSER='<railway-pg-user>' \
TARGET_PGPASSWORD='<railway-pg-password>' \
TARGET_REDIS_URL='redis://default:<password>@<railway-redis-host>:<port>/0' \
./deploy/migration/pre-migrate.sh
```

`restore-test` 会恢复业务库，但跳过 `hg_youban_publish_tg_session` 的数据。它用于验证数据库结构、资料、频道、订单、采集任务和 Redis 队列迁移，不会验证 Telegram 登录态。

恢复完成后执行：

```bash
TARGET_PGHOST='<railway-pg-host>' \
TARGET_PGPORT='<railway-pg-port>' \
TARGET_PGDATABASE='<railway-pg-database>' \
TARGET_PGUSER='<railway-pg-user>' \
TARGET_PGPASSWORD='<railway-pg-password>' \
TARGET_REDIS_URL='redis://default:<password>@<railway-redis-host>:<port>/0' \
MIGRATION_TEST_SKIP_TG_SESSION=1 \
./deploy/migration/verify-migration.sh
```

## 四、正式全量恢复（包含 TG session）

正式停机窗口内必须按顺序：

1. 停止腾讯云 API、Account、Collector、Media、Publish、Background、Scheduler；
2. 重新生成最终快照；
3. 恢复 PostgreSQL；
4. 复制 Redis 全部 Key 和 TTL；
5. 先启动 Railway Account 单副本并检查账号状态；
6. 再启动其他 Worker、Scheduler 和 API；
7. 最后切换域名、Bot Webhook 和支付回调。

正式恢复命令：

```bash
MIGRATION_MODE=restore-full \
MIGRATION_REUSE_ARTIFACTS=1 \
MIGRATION_CONFIRM=I_UNDERSTAND_TARGET_WILL_BE_WRITTEN \
TARGET_PGHOST='<railway-pg-host>' \
TARGET_PGPORT='<railway-pg-port>' \
TARGET_PGDATABASE='<railway-pg-database>' \
TARGET_PGUSER='<railway-pg-user>' \
TARGET_PGPASSWORD='<railway-pg-password>' \
TARGET_REDIS_URL='redis://default:<password>@<railway-redis-host>:<port>/0' \
./deploy/migration/pre-migrate.sh
```

`restore-full` 会导入 `hg_youban_publish_tg_session`。脚本不会调用 Telegram API，不会注销 session。迁移期间绝不能让腾讯云和 Railway 两套 Account 同时连接同一批账号。

## 五、系统配置单独迁移

系统配置不包含租户资料和 TG session：

```bash
MIGRATION_OUTPUT_DIR="$PWD/migration-artifacts/system" \
./deploy/migration/system-snapshot.sh
```

输出 `system-config.sql`，包含系统配置、插件配置、角色、角色权限和菜单权限。全量 PostgreSQL 恢复时不要再次执行它，只有目标库已经初始化、需要单独同步系统配置时才使用。

## 六、单个账号导出

单账号导出默认不包含 TG session，也不包含 Redis：

```bash
MIGRATION_TENANT_ID=123 \
MIGRATION_ACCOUNT_ID=456 \
MIGRATION_OUTPUT_DIR="$PWD/migration-artifacts/account-456" \
./deploy/migration/account-snapshot.sh
```

脚本输出 `account-data.tar.gz`，包含 `manifest.tsv`、表字段和 CSV 数据。当前版本只负责安全导出，不直接导入生产库；导入前必须人工检查 ID 和关联关系，避免资料缺媒体、采集源缺规则、任务缺频道映射。

## 七、单个账号恢复

先将导出包解压到目录，并默认执行 dry-run：

```bash
MIGRATION_ACCOUNT_INPUT_DIR="$PWD/migration-artifacts/account-456/unpacked" \
MIGRATION_TARGET_TENANT_ID=123 \
MIGRATION_TARGET_ACCOUNT_ID=456 \
TARGET_PGHOST='<railway-pg-host>' TARGET_PGPORT='<railway-pg-port>' \
TARGET_PGDATABASE='<railway-pg-database>' TARGET_PGUSER='<railway-pg-user>' \
TARGET_PGPASSWORD='<railway-pg-password>' \
./deploy/migration/account-restore.sh
```

确认 dry-run 输出无误后，再设置 `MIGRATION_DRY_RUN=0` 和确认字符串执行真实导入。导入保留源 ID，目标租户必须已存在、目标账号必须不存在；全程单事务，失败会回滚。Telegram session、Redis、Asynq 任务和运行时租约不会通过单账号包导入。

## Redis 说明

Redis 同时保存缓存、Asynq 队列、账号租约、分布式锁和去重数据。脚本生成 RDB 作为可靠备份；恢复时通过 SSH 隧道使用 `SCAN + DUMP + RESTORE` 复制**停机后的源 Redis**全部 Key 和 TTL，不执行 `FLUSHDB`。RDB 作为回滚和留档文件，不直接假设 Railway 支持加载 RDB 文件。

目标 Redis 必须是专用实例。脚本会覆盖同名 Key，但不会删除目标中源端没有的 Key，因此不要在已有测试数据的 Redis 上直接恢复。`redis://` 和 Railway 常见的 `rediss://` 均支持。

## 八、迁移后一致性对比

停机窗口内，源端服务停止并完成 PostgreSQL/Redis 恢复后，保持源服务器数据库可读，执行：

```bash
TARGET_PGHOST='<railway-pg-host>' \
TARGET_PGPORT='<railway-pg-port>' \
TARGET_PGDATABASE='<railway-pg-database>' \
TARGET_PGUSER='<railway-pg-user>' \
TARGET_PGPASSWORD='<railway-pg-password>' \
TARGET_REDIS_URL='rediss://default:<password>@<railway-redis-host>:<port>/0' \
./deploy/migration/compare-migration.sh
```

对比内容：

- PostgreSQL public schema 每张基础表的精确行数；
- Redis 全部 Key 数量；
- 每个 Redis Key 的 TTL（默认允许 5 秒迁移耗时漂移，可通过 `MIGRATION_REDIS_TTL_TOLERANCE_MS` 调整）；
- 每个 Redis Key 的序列化内容摘要；
- 源端没有写入权限要求，不会修改源端和目标端数据。

只有对比通过后才切换 Railway 域名、Webhook 和支付回调。若校验失败，脚本会输出 PostgreSQL 差异并停止切流。

## Telegram session 说明

只要完整恢复 PostgreSQL 中的 session 表，并保持 Telegram `AppId`、`AppHash`、项目签名/加密密钥和代理配置一致，迁移后通常不需要用户重新登录。迁移期间必须停止源 Account，Railway Account 首次启动只能保持单副本。

## 迁移后检查

- `/api/account/current`；
- PostgreSQL 核心表数量；
- TG 账号数量和 session 行数；
- Redis Key 数量和 Asynq 队列；
- 用户登录、资料、频道、采集源和订单；
- COS 图片/视频访问；
- 历史采集、媒体下载、Telegram 推送；
- 支付订单创建和回调。
