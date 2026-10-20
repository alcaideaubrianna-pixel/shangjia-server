# 线上调试与 Agent 交接

最后核对：2026-10-20。连接和应用清单是定位入口，不是实时健康证明；开始工作时重新读取实际配置。本文不保存密码、Bot Token、登录 Token 或云密钥。

## 1. 接手顺序

1. 完整阅读仓库根目录 `AGENTS.md`，查看 `git status --short`、当前分支及 `git worktree list`。现有未提交修改不等于本任务修改，禁止整体暂存或清理。
2. 用户通常要求合并到 `feat/railway-runtime-ci-split`，不要自行改为 develop。确认本次要求及两分支差异后再合并；构建、部署状态分别记录。
3. 先确定租户、账号、频道、资料、消息、job/event ID，获取有界的真实样本。区分已证实原因、推测、修复和待验收项。
4. DB 优先 DBX MCP，远程系统使用 SSH server 2 MCP。不要因为终端缺少 curl/wget 就判断服务故障。

## 2. 已核对的连接入口

| 入口 | 名称/地址 | 用途 |
|---|---|---|
| DBX PostgreSQL | `PostgreSQL_Xiaohuiji` | 生产业务库 railway |
| DBX connection ID | `436a8ca1-a13c-4a26-aa6f-08633096b8a4` | 当前配置的连接 ID，重建后可能变化 |
| PostgreSQL 内网地址 | `10.3.0.10:5432` | 以 DBX 当前配置为准，不要求本机直接可达 |
| SSH server 2 | `b-xiaohuiji-sg`，`ubuntu@43.159.54.73` | PostgreSQL 主机，已验证本机 psql 可用 |
| SSH server 2 | `aliyun-sg-Redis`，`root@47.79.227.11` | 阿里云新加坡 Redis 主机；逻辑 DB、端口及当前实际连接从应用 ENV 确认 |
| SSH server 2 | `b-xiaohuiji-Redis-sg`，`ubuntu@43.153.199.31` | 另一个已配置 Redis 节点；不能据名称认定仍承载生产 |
| API | `https://api.xiaohuiji.cc` | 生产 Web API |
| 本地后端 | `/Users/ley/Desktop/CodeProject/youban/youban-server/server` | Go module hotgo |
| 本地前端 | `/Users/ley/Desktop/CodeProject/youban/tdesign-starter/vue_admin_demo` | 用户指定的前端目录，进入前重新核对工作区 |

连接发现：`mcp__dbx__dbx_list_connections`、`mcp__mcp_server_2__list_servers`。工具名以当前会话提供的 schema 为准，未注入时先通过可用工具发现机制查找，不能声称已经执行。

DBX 示例（工具参数，不是 shell 命令）：

```json
{"connection_name":"PostgreSQL_Xiaohuiji","sql":"SELECT current_database(), now();","max_rows":10}
```

SSH 示例：

```json
{"connectionName":"b-xiaohuiji-sg","cmdString":"sudo -u postgres psql -d railway -X -P pager=off -c 'SELECT now();'","timeout":20000}
```

优先使用已配置的连接凭据。应用 Redis、COS、Bot 配置从对应线上应用 ENV 或现有配置读取，在进程内使用；禁止整份 ENV、docker inspect 环境或带凭据的连接串输出到日志、对话、文档。

## 3. DB、磁盘、日志排查

先查连接与锁等待，再查慢查询和磁盘；不能只凭 API 503 判定 Redis 或 DB 已挂。

```sql
SELECT pid, state, wait_event_type, wait_event,
       now()-query_start AS elapsed, left(query,400) AS query
FROM pg_stat_activity
WHERE datname=current_database() AND pid<>pg_backend_pid()
ORDER BY query_start LIMIT 20;

SELECT pid, pg_blocking_pids(pid) AS blockers
FROM pg_stat_activity
WHERE cardinality(pg_blocking_pids(pid))>0 LIMIT 20;

SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) AS total
FROM pg_stat_user_tables ORDER BY pg_total_relation_size(relid) DESC LIMIT 20;
```

查询文本可能含业务数据，分享结果时只保留排查必要字段并脱敏。大表先查看字段和索引，不默认执行全表 COUNT 或 EXPLAIN ANALYZE。EXPLAIN ANALYZE 会实际执行查询，必须限定范围与执行时间。

本次 DBX 显式 PostgreSQL BEGIN/COMMIT 被策略拒绝（SQL_BLOCKED）。需要原子迁移时使用已经授权的 SSH 本机 psql 事务，并设置 lock_timeout / statement_timeout。跨 DBX 调用不能假设 SET 会保留；需要状态时按当前工具支持使用固定 session，或 SSH 在同一 psql 会话完成。

DB 主机检查 `df -h`；已知 PostgreSQL CSV 日志目录 `/pg/log/postgres/`，本次证据文件为 `postgresql-Tue.csv`。按实际日期及日志配置核对，不要硬编码周二文件为“当前日志”。CSV 慢 SQL 可能含多行和长参数，用 CSV 解析器读取，避免 grep 截断后误判 SQL。

历史故障涉及数据库磁盘、日志大表、自建 Bot API 缓存。先核对各路径/表大小和保留需求；不得把消息审计、Attempt、采集事件和普通运行日志当成同一类直接清空。

## 4. SAE 角色与部署约束

区域 `ap-southeast-1`；以下 ID 来自现有 `deploy/sae-deploy-all.sh`。

| 角色 | AppId | 主要排查范围 |
|---|---|---|
| web | b4565e3c-6d08-4297-bf8b-037884d54ee4 | HTTP、鉴权、Bot 入口；具体消费者归属查当前启动配置 |
| background | d12e3d86-e368-48ae-bac1-bd41b3a08ef2 | 后台任务 |
| collector | 0deb4862-2af5-4c0f-9e82-ffa67364843e | 采集处理 |
| media | a34c2ae5-6e59-428b-8aa5-1277cbecc9ae | 下载、处理、媒体缓存 |
| publish | 2fb5d3db-0975-4dea-b086-805d255131fa | Telegram 发布 |
| scheduler | 596e5f62-0715-499e-ba71-46f00c9869d3 | 调度、恢复 |
| account | f8d4a7a9-d30f-46a5-89ac-dc57eb48fb4f | Telegram 协议号任务，严格单实例 |

- Account 不能在本地再启动一个生产消费者；切换时先确认旧实例停止，再启动新实例。其他角色本地连接生产也会消费真实任务，必须有任务范围、数量上限和停止条件，不能把“调试”理解成无影响。
- Web 使用滚动部署并验证 readiness，保留可服务实例；不要为升级 Account 停掉 Web。
- 部署前确认构建成功、不可变镜像 SHA、现网旧镜像和回滚目标；部署后分别验证应用状态与真实业务。
- **脚本当前陷阱**：即使 `SAE_DEPLOY_ROLES=web`，脚本末尾仍无条件执行 `deploy_account_singleton`。不能用此变量当作“仅部署 Web”的保证。先读脚本，不在排查中顺手执行全角色部署。
- 不存在名为 Bot 的独立 SAE 角色；按代码入口和当前 ENV 判定需要升级的应用。

## 5. 推送、采集与媒体排查

沿同一个业务 ID 检查：源消息 → collect_event / collect_dispatch → content_profile / media / profile_state → tg_job → Outbox → delivery Attempt / 已发送消息。实际表名及字段先通过 DBX 描述表或源码确认。

常用代码位于 `server/addons/youban_publish/logic/sys/`：

| 文件/范围 | 用途 |
|---|---|
| `tg_outbox.go`、`tg_channel_order.go` | 调度、频道前序任务与发送槽 |
| `tg_send_job.go`、`tg_delivery_attempt.go` | 发送状态、成功确认、未知结果与恢复 |
| `tg_send_media.go`、`telegram_collector_adapter.go` | Bot 媒体发送、协议号降级 |
| `collect_process_queue.go`、`collect_process_outbox.go` | 采集处理与持久化任务 |
| `note_index_query.go`、`note_index_projection.go`、`profile_query.go` | 列表读模型及补全 |

这些是查找入口，不代替当前代码事实。定位规则：

- `dispatcher selected jobs:N` 只能证明选中任务；继续检查入队、前序检查、槽、消费者开始、API 返回、落库完成各节点。
- `failed + display_confirmed`、unknown、租约过期、账号未登录等要分别判断；不能批量重置所有任务或重新上架所有资料。
- API 明确成功与发送结果未知分开处理。明确成功无需远端对账；超时不能当作“肯定未发送”。任何降级/重发都要检查迟到回执与重复消息风险。
- `chat not found` / `bot is not a member` 是目标可见性/成员资格线索，先核实 chat、Bot 及历史成功记录；不是下载超时，不能靠二进制上传解决。
- `WEBPAGE_CURL_FAILED` 检查 URL 回源与链路降级；媒体 404 检查 COS/CDN 与源消息能否恢复。
- `mode=reuse,hasFileId=true` 是发送选择证据，但需同时确认同 Bot、最终实际分支和结果。防扫图修改图片后不可复用未经处理的原始媒体。
- 验证视频缺失先核对来源上下文、事件接收/编辑时间、匹配、入库及发送阶段；不能把展示消息发布时间当成源视频 webhook 接收时间。

Redis 增长按应用实际 endpoint、逻辑 DB、队列类型检查 INFO memory/stats/keyspace、待处理/重试/归档数量、TTL 与生产/消费速率。计数很大不代表都是有效待发送业务；先分类。禁止以清库、KEYS *、全量重试来代替定位。

## 6. 已部署运维 MCP 与本地工具

当前会话可发现的 `youban_ops` 工具：

- `resolve_profile_by_message`：消息链接或 chat/message ID → 资料、来源。
- `collect_diagnose_message`：采集与发布诊断。
- `collect_preview_repair` / `collect_apply_repair`：先生成有界修复方案，再按方案应用。
- `profile_preview_operation` / `profile_apply_operation`：下架、重新上架、删除方案。
- `profile_media_preview` / `profile_media_apply`：媒体恢复。
- `verification_preview_missing` / `verification_apply_repair`：验证媒体恢复及完整资料替换发布。
- `ops_plan_status`：查询执行结果。

先读取当前工具 schema，preview 不等于 apply 成功；apply 必须符合用户授权范围并核对方案、租户、数量。不把历史验证恢复做成散发孤立验证视频。MCP 是否可用要看调用响应，只有配置文件不代表通链。

本地可复用工具：`server/tools/collect_repair/README.md`、`collect_edit_audit/README.md`、`media_md5_backfill/README.md`；另有 `media_salvage`、`media_salvage_enqueue`、`tg_dispatch_repair`。先读说明/参数和默认是否写入，不能直接运行到生产。

## 7. 列表慢查询最新交接

提交 `93f6326b` 优化 `/api/youban_publish/publish/admin/note/list`。原请求 `accountScope=all&sourceScope=collected&sortBy=createdAt&page=1&perPage=20` 的历史 SQL 为 30.57 秒：空结果仍扫描约 40 万索引行并逐条关联 profile/state。

线上已完成：`note_index.is_collected` 字段、旧写入兼容触发器、分批回填、来源与创建/发布时间组合索引；完整来源一致性校验 mismatch=0。应用查询改为读模型来源过滤。细节、回滚、SQL 及样本结果见：

`server/addons/youban_publish/resource/sql/note_index_source_scope.md`

完整原权限参数的新 SQL 执行 0.201ms；发布时间样本修复后 0.032ms。这不是 HTTP 总耗时。城市、标签、来源等部分组合仍扫描较多行，尚未证明所有组合“秒回”。截至本次交接，未完成登录后的端到端测试，不能把本地 commit 当成已发布。

真实接口探针（从 server 目录执行）：

```sh
python3 tools/note_list_probe.py --token-file /path/to/existing-token --repeat 3
```

使用用户现有登录身份，或环境变量 `YOUBAN_PROBE_TOKEN`；无凭据时不能通过伪造 Token/修改鉴权绕过。`--cases-file` 可传 case 名到参数对象的 JSON，覆盖来源、账号、状态、地区、标签、关键字、采集来源、两种排序和下一页 cursor。记录成功响应、数量、P95/最大耗时与 traceID；401、healthz、单条 SQL 快都不等于接口验收通过。

专项本地测试需要独立 PostgreSQL，测试仅接受固定本机临时库连接，默认跳过，不连接生产：

```sh
YOUBAN_NOTE_INDEX_TEST_PG='pgsql:test:@tcp(127.0.0.1:55483)/postgres' \
go test ./addons/youban_publish/logic/sys \
  -run 'Test(NoteIndexSourceScopePostgres|NormalizeAdminNoteSortBy|AdminNoteCursor|AdminNotePublished)' -count=1
git diff --check
```

本次临时集群曾使用 `/tmp/youban-random-media-pg.aIDnxn`，已停止，临时目录可能被清理；后续先确认存在及用途再启动，不依赖它长期存在。

## 8. 收尾记录

每次记录：时间与时区、环境/角色/镜像、样本业务 ID、证据、影响范围、变更与行数、验证结果、未验证事项、回滚方法、commit/合并/构建/部署各自状态。不要将历史观察写成永久事实，也不要承诺一次测试覆盖所有生产故障。
