# 小灰机机器人管理

独立工作区：`worker/publish-bot-management-20261021`。机器人管理属于 `youban_bot` 的聊天功能插件，入口为插件菜单「机器人管理」或 `/bots`。仅要求绑定有效上架账户，不检查 VIP；频道管理仍要求用户所属协议号在线且有实际管理权限。

## 交互

首次进入按已配置上架频道创建同步任务，完成后通知并返回机器人列表。后续列表、搜索、翻页使用数据库，只有点击「同步机器人」才重新同步。搜索范围为本账户已同步机器人。

添加上架 BOT → 填写 Token / 选择已有机器人 → 选择候选上架频道 → 确定。频道每页 10 个，✅/⬜ 切换，跨页保存；首次默认全选，后续记住选择。每个 Bot 只分配一个候选频道，按机器人数量、管理员剩余名额、最近分配时间公平选择，并计入未完成添加任务的预留名额。现有管理员优先复用，不重复占名额。

机器人详情可以查看频道关系和已有访问链接、编辑发布/编辑/删除权限、移出频道、删除无引用的上架配置。不能获取的管理员上限显示「未知」；最终以 Telegram 的权限和容量校验为准，不凭空填写容量。Token 未提供的同步发现机器人只作关系展示，不能直接配置上架。

失败任务可以人工重试；已存在更新操作时禁止重新执行旧任务。同步失败通过重新同步恢复。Telegram 外部权限、限流、管理员容量限制会显示失败原因，不宣称必定添加成功。

## 存储和运行

- `hg_youban_bot_managed`：机器人身份与用户提供的凭据，按租户、账户隔离；已配置 Token 的 Bot 身份仅能被一个账户认领。
- `hg_youban_bot_managed_channel`：频道权限、访问链接、管理员数量和同步快照。
- `hg_youban_bot_managed_member`：机器人与频道关系、权限。
- `hg_youban_bot_manager_state`：用户跨页选择、输入状态、搜索词和独立的权限编辑草稿。
- `hg_youban_bot_manager_task`：同步、添加、移出和权限更新任务与通知状态。

协议操作复用现有 AccountTasks 和单实例 Account 会话，不启动新的 Telegram 登录。优先级 -200，每次执行预算 30 秒，最多尝试 3 次，未开始的任务最长等待 20 分钟；FloodWait 使用持久化延期，不在任务里 sleep。Account 重启依靠原任务租约恢复；等待任务轮转，防止阻塞后续账户。维护异常后退避一分钟。

凭据更新：已配置发送 Bot 以 `youban_publish_bot` 为发送凭据来源，管理入口在同一数据库事务中更新管理记录和该账户下同身份的发送记录，不改变 Bot 状态或用途；提交后复用现有缓存与 Gateway 刷新。频道同步只为无凭据记录补齐 Token，不覆盖已有 Token；添加/权限任务执行时读取当前发送凭据，绑定投影禁止修改已有凭据，防止延迟任务或旧快照回滚 Token。

完成通知使用独立循环，不占用任务投递的 4 秒预算，也不会触发任务投递的一分钟退避。每批只选择一个代表记录并轮转，单批通知预算 12 秒；临时失败保留数据库待通知状态，使用统一缓存退避一分钟（缓存不可用时仍轮转重试）。已被用户屏蔽或用户失效复用广播的不可达判断并终止该批通知；查询绑定临时失败不标记已通知。通知循环与投递循环一起取消和退出，不额外启动 Telegram Account 会话。

移出/降权共用既有频道发送租约；还有未完成发布任务时拒绝操作。修改三项权限时保留其他 Telegram 管理员权限。删除配置前检查频道、采集和发布任务引用，数据库变更在事务内完成。通知可能因进程恰好中断而重复，权限与添加操作会重新核实远端状态。

同一 Bot、同一频道的管理操作按任务 ID 串行：前序 pending/submitted（包括协议任务自动重试）结束后才投递下一条，执行入口再次检查。移出或关闭发布权限时先解除该频道的发送绑定，再修改远端；远端失败不自动恢复绑定，需重试移出或重新授予发布权限。不会全局停用共享 Bot，也不会影响它在其他频道的绑定。远端操作与必要绑定更新成功后，频道快照刷新失败只记录警告和失败状态，可通过同步按钮恢复，不触发重复权限操作；移出重试接受 USER_NOT_PARTICIPANT。

兼容现有发布链路：新管理数据归属 `youban_bot`，向原 `youban_publish_bot` 和频道 `bot_id_json` 投影，保留历史 Bot ID。尚未物理迁移或删除旧发布表；现有发送、采集和 Webhook Gateway 继续使用原入口。

## 部署

先执行 `addons/youban_bot/resource/sql/manager.pgsql.sql`（MySQL 对应 `manager.sql`），再部署处理 Bot Webhook 的 Web 和运行任务的 Account。Account 保持单实例；正常使用插件数据库升级入口也会执行这些文件。新表为增量，不修改现有业务表。

PostgreSQL 使用带时区的时间字段，避免驱动把无时区时间当 UTC 导致同步快照和任务超时偏差。MySQL SQL 已提供，当前没有 MySQL 实例实测，不宣称验证完成。

## 本地验证

全部使用一次性本地 PostgreSQL、SQLite 和本地假 Telegram HTTP/RPC；未连接线上或登录真实 Telegram。

```sh
go test ./addons/youban_bot/logic/sys -run '^TestBotManager' -count=1
YOUBAN_RELAY_TEST_PG='pgsql:ley:@tcp(127.0.0.1:35439)/youban_relay_test' go test -race ./addons/youban_bot/logic/sys -run '^TestBotManager' -count=1
GF_GCFG_FILE=config.example.yaml YOUBAN_RELAY_TEST_PG='pgsql:ley:@tcp(127.0.0.1:35439)/youban_relay_test' go test ./addons/youban_publish/logic/sys -run '^TestBotManager' -count=1
```

测试覆盖免费绑定、账户隔离、分页勾选与搜索、Token 校验及删除后重加、并发名额预留、重复确认、任务恢复/超时/轮转、同步中断保留原关系、失败通知、防止旧操作重试，以及原发布表的绑定和删除保护。真实 Telegram 的添加/权限/移出链路仍需要用测试账户和测试频道做验收。

完整构建通过。完整相关包测试中的旧失败项，已在基线 `b9aa4117` 独立复现：`TestRelayAuditControlsStayOutOfDedicatedHelp`、`TestRelayHelpAndSettingsValidation`、`TestNormalizeAntiScanMattingConcurrency`、`TestDuplicatePHashScanBucketsBoundLookupCost`、`TestClassifyProfileMessage` 的两项子用例、`TestParseProfilePublishMark`；没有把完整测试描述为全部通过。
