# 采集来源档案与历史回填

默认只读，单次 1–500 条，必须指定租户和账号；只写来源档案，不修改资料、媒体、排序时间，不入队或重发。工具不启动采集、发布 Runtime。

## 上线顺序

1. 先构建并完成本地专项回归。当前默认 `youbanPublish.profileOriginEnabled: false`，旧版本可以继续运行。
2. 使用现有安全连接，在显式短事务中运行 `addons/youban_publish/resource/sql/migrations/profile_origin.pgsql.sql`（PostgreSQL）；MySQL 对应 `profile_origin.sql`。设 `lock_timeout`/`statement_timeout` 并检查提交。不要加入请求/启动自动 DDL，不运行其他历史维护 SQL。
3. 配置文件设置 `youbanPublish.profileOriginEnabled: true`，或通过已有 ENV 映射设置 `YOUBAN_PUBLISH_PROFILE_ORIGIN_ENABLED=true`。先确认表/索引存在，再更新相关 Web/Collector/审核处理 writer。所有 writer 更新前不能宣称新资料 100% 写永久档案。
4. 对每个租户/账号先 preview，再按相同游标 apply；由人工/受控作业推进下一批，工具不会自动扫全库。
5. 验证资料列表、详情与 MCP 返回的 `collectMethod`、`originCompleteness`、`origins`；检查新写入 origin 的缺失率和 partial/unknown。原来源字段保留，`origins` 保存当时快照，名称改动不覆盖快照。

从 `server/` 执行，使用现有受保护配置/ENV，不在命令中传递凭据：

```sh
go run ./tools/profile_origin_backfill --tenant-id 56 --account-id 561 --after-id 0 --limit 100
# 确认上面批次后才执行：
go run ./tools/profile_origin_backfill --tenant-id 56 --account-id 561 --after-id 0 --limit 100 --apply
```

`nextId` 是下一批 `--after-id`。`origins/complete/partial/unknown` 描述本批候选证据；`inserted` 是实际新增档案数，重复 apply 为 0；`skipped` 为应用时资料已被删除的候选数。没有来源证据的资料只计入 unknown，不伪造档案。

来源事实以事件 ID 在租户/账号/资料作用域内幂等，不以可重建的 dispatch/rule ID 去重。真实新事件可追加来源；同资料多规则/重试不重复。同事件再次读取不会覆盖已有快照。历史回填 evidenceKind 与实时入库分开标记。

`complete` 要求来源坐标、消息、方式以及执行 Bot/协议号/关注账号可证；只有频道/媒体组 key 的记录是 partial，方式为 unknown。关注采集的 `chatId=account:<id>` 是站内账号来源，不伪装成 TG 频道。Bot 私聊正数 ID 原样保存。

## 回滚与边界

关闭开关可停止新表写入/读取，来源筛选回到旧分支；保留新增表与档案，不删除字段/表来回滚。回滚期间旧 writer 不提供新资料强保证，必须记录起止时间并补档。

采集配置软删除、失败事件清理和工作流重置不删除永久档案；授权的资料硬删除/清理会在同事务删除对应档案，采用 parent→origin 锁顺序避免回填复活来源。关闭开关或使用旧镜像删除资料后，可能需单独审计残留档案，不能宣称旧镜像也具备新生命周期逻辑。

TG 文件导入仍走原有多阶段导入，不写采集档案，也不会伪装成 collect。本期未重构导入事务、未增加前端页面交互；返回字段供页面/MCP复用。

历史数据无法无条件保证 100% 恢复；频道 ID 可以定位来源身份，不保证私有频道/已删除消息的链接仍可访问。`receivedAt` 是事件接收时间，不是 TG 消息发布时间。`origins` 按可证时间排列，历史记录的最早可证来源不等于无法证明的真实首次来源。

## 本地验证

只连接本次独立临时 PostgreSQL `127.0.0.1:55484`，不连接生产；数据库测试默认跳过，并在子进程隔离 GoFrame 配置。

```sh
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys -run '^TestProfileOrigin' -count=1
go test ./internal/bootstrap/envconfig ./addons/youban_publish/install -count=1
go build ./...
```

测试覆盖：永久/历史查询、来源软删、改名、事件与派发删除、MCP来源消息查询、来源筛选、同资料多个 collector、16个并发事务的来源幂等、事务故障回滚、回填 preview/apply/重复执行、跨归属过滤、私聊 ID、无证据不猜测，以及关闭开关且新表不存在的兼容行为。尚未进行生产迁移、回填、部署及真实 Bot/HTTP 端到端验收。
