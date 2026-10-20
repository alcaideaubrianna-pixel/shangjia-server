# 2026-10-20 生产 SQL 错误复核与本地修复

记录时区：Asia/Shanghai。生产观察窗口为 2026-10-20 08:00–13:55，来源为数据库节点实际 `pg_current_logfile()` 指向的 PostgreSQL CSV 日志。只读解析约 205 万条日志记录，未导出业务参数或凭据。

## 数据库操作

按用户授权，使用 DBX `PostgreSQL_Xiaohuiji` 固定会话，设置 `lock_timeout=2s`、`statement_timeout=10s` 后执行：

```sql
ALTER TABLE public.hg_youban_publish_account_setting
  ADD COLUMN IF NOT EXISTS profile_detail_view_enabled smallint NOT NULL DEFAULT 0;
```

数据库返回字段已存在并跳过；实际新增字段 0，业务数据更新 0。回查确认 `smallint NOT NULL DEFAULT 0`，无锁等待，DBX 会话已关闭。本次没有新增生产字段或修改其权限值，无需数据库回滚。

规则表缺少 `intro_fee_suffix` 并非漏迁移：当前配置存于 `hg_youban_publish_collect_rule_item`，线上已有 122 条后缀规则项。旧查询还引用了规则表上不存在的 `intro_fee_suffix_enabled`、`bot_id`，应修复查询而不是增设另一套配置列。

## 本地修复

| 生产证据 | 根因与改动 |
| --- | --- |
| 7 次 `column r.intro_fee_suffix does not exist` | 新采集频道通知查询改为读取绑定的有效规则，并用已有 `attachCollectRuleItems` 加载后缀；保留租户、账号、来源、启用和软删除限制。 |
| 4 次 `syntax error at or near "p"` | 排名 CASE 表达式被 `OrderAsc` 当作标识符引用，生成 `ORDER BY "CASE" p.id`。改用 `Order(gdb.Raw(...))`，纯无效 ID 列表不生成 CASE。 |
| 8 次 `uk_content_media_profile_asset` 冲突 | 数据库唯一键是 `(profile_id, source_asset_id)`，原代码同时匹配类型、用途和排序。正附件按唯一键定位并更新，重排、改用途、恢复软删除均保留投影 ID；没有附件的媒体写 NULL，兼容识别历史的 0，避免多个 Bot 媒体冲突。未删除或修改唯一索引。 |
| 1 次采集性能统计事件唯一键冲突 | 原先 COUNT 后 INSERT 存在竞争。改为按 `event_id` 原子 upsert，更新时保留 `id`、`created_at`。 |
| 5 次死锁；日志明确包含 Attempt 更新与升级建索引/改表 | 在线升级执行器先通过目录查询识别已存在且有效的索引、已存在字段，跳过会额外持锁的无效 DDL。PostgreSQL SQL 报错后立即返回回滚，不继续忽略已失败事务。历史规则清理、来源去重与活跃时间回填移到显式维护脚本，避免在线升级长事务夹带这些 DML。 |

新建或真正缺失的字段/索引仍可能与业务竞争锁，应通过显式迁移命令与受控窗口执行。本次没有运行生产维护脚本，也没有重试、重置或重发生产任务。

## 其余错误判定

- 52 次账号详情权限字段错误发生于 13:42–13:44，复核时字段已存在。当前工作分支没有引用该权限字段的业务代码，未擅自移植其他工作区的权限功能。
- 采集内容池唯一键冲突已有“重读并发胜出记录”的分支；会员事件唯一键冲突已有事务回滚后返回幂等结果的分支。本次未改动其业务语义。
- 162 次 `pg_exporter` 查询取消、64 次认证超时、7 次事务空闲超时需要结合监控/客户端生命周期继续排查，现有证据不足以断言为某个 Go 函数泄漏。未更改数据库超时或放宽连接认证。
- 应用节点 SSH 超时，未取得应用侧日志；未验证真实 HTTP/Bot 端到端行为。

## 验证

使用本次创建、仅监听 `127.0.0.1:55484` 的独立临时 PostgreSQL，测试不连接生产，也不启动消费者。

```sh
cd server
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./internal/logic/sys \
  -run 'Test(ProfileRankOrder|NormalizeHomeProfileFeedRecommended)' -count=1
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys \
  -run 'Test(BotMediaProjectionLocalPostgres|BotCollectDiscoveryLocalPostgres|CollectMediaStatUpsertLocalPostgres|TenantVip|CollectRule|CollectSource)' -count=1
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/install -count=1
go build ./...
git diff --check
```

上述检查通过。真实数据库测试覆盖 CASE 排序、无效排名 ID、规范化后缀及归属过滤、生产唯一索引下多 Bot 媒体重试、附件重排/用途变化/软删除恢复、统计 upsert 保留创建时间，以及持有业务写锁时重复升级不等待 DDL 锁。

扩大检查未全绿：

- 发布逻辑包完整测试的范围外失败：`TestNormalizeAntiScanMattingConcurrency`（期望默认 64、当前返回 128，单独执行也失败）、`TestMediaPHashBucketCandidatesRejectProfileOnlyScope`、`TestDuplicatePHashScanBucketsBoundLookupCost`、`TestClassifyProfileMessage`、`TestParseProfilePublishMark`。未修改这些功能来迎合测试。本次会替换默认数据库配置的集成测试在子进程执行，避免污染其他 SQL 生成测试；既有 SQL 生成测试单独执行通过。
- `go test ./... -run '^$'`：队列基础库 `internal/library/queue/disk.go:128`、`kafkamq.go:101` 的非恒定格式字符串未通过 vet。这些文件本次未改动。
- `go build ./...` 仅有第三方 `go-m1cpu` 的 C 编译警告，不影响成功退出。

## 交付状态与回滚

本记录随本次修复提交；未合并、未推送、未构建生产镜像或部署；现网 SQL 错误仍须在部署本次修复后按真实业务样本验收。已有其他未提交修改保持原样。

回退应仅撤销本次相关差异，尤其 `media.go` 含用户既有的图片搜索阈值修改，禁止整体还原该文件。生产未新增字段、未运行历史清理，无数据回滚动作。若后续部署，记录新旧镜像 SHA，并按部署平台切回已验证旧镜像；不要通过批量重发验收。
