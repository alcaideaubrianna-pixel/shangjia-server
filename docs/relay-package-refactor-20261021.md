# 上架中转整包重构交接（2026-10-21）

## 实现与边界

代码位于独立工作树 `/tmp/youban-relay-package-20261021`，分支 `refactor/relay-package-publish-20261021`，基于 `b9aa4117`。原工作区已有的改动未合并进来。本次改动在独立分支提交；未合并、推送或部署，也未访问生产数据库或真实 Telegram 发送接口。

开启后，每个上架 Job 固化发布 Bot 的实际 Telegram 用户 ID、展示媒体、验证媒体、防扫图配置和最终文案。先在中转频道准备全部消息，清单完整后才清理上一版目标消息，然后使用一次 `copyMessages` 将展示和验证复制到同一目标频道。

- 保留独立相册，不显示原始转发来源。Telegram 不能保证严格原子投递或所有消息同毫秒可见。
- 消息总数必须在 1–100 范围内，来源 ID 严格递增。超过上限明确报错，不拆成多次目标投递。
- file_id 缓存按租户、实际 Bot Telegram 用户 ID、素材指纹隔离；防扫图开启时继续按现有保护配置生成。不同 Bot 不混用 file_id。
- 复用同一个中转频道与已验证的 file_id；每次 Job 仍准备自己的带恢复标记的消息包，以避免不同任务共享文案或确认标记。
- 中转准备成功以前保留原有目标消息。目标复制仍会经历旧版删除到新版复制的短暂窗口，不是零停顿原子替换。

## 调度与接入

频道设置包含协议账号、初始 Bot 和准备容量。前端支持选择服务端缓存中的频道、新建频道、编辑配置、状态轮询、启停、冷却/错误展示。租户/账号归属由后端重新校验，access_hash 从服务端缓存读取。

管理任务和资料准备任务都提交至协议账号常驻 Runtime，不另开协议连接。实际发布 Bot 首次使用时检查频道成员和发布/删除权限，缺少权限时由该频道所属协议账号邀请并授予所需权限。账号无邀请/授权权限时返回明确错误，不绕过 Telegram 限制。

容量由数据库活跃准备租约计算，不使用进程本地计数。短事务只锁定中转频道调度数据，不锁业务账号行，不在事务内调用 Telegram。PostgreSQL 调度锁等待限制 2 秒，单语句限制 5 秒；负载使用一次聚合查询。

限流范围：

| 错误范围 | 处理 |
| --- | --- |
| 明确频道权限/来源错误、SLOWMODE_WAIT | 冷却该频道，立即调度其他可用频道 |
| 协议账号 FLOOD_WAIT | 冷却该账号所属中转频道，可切换其他账号所属频道 |
| Bot API 429 | 遵守 retry_after，不通过更换来源频道绕过限制 |
| 准备阶段网络失败 | 递增代次后重建内部资料包，目标频道尚未发送 |
| 目标复制超时或返回部分消息 | 保留证据，等待 Webhook/协议号对账，禁止盲目重发 |

## 恢复与兼容

新增阶段为 `relay_preparing → relay_ready → package_sending → package_confirmed`，最终恢复为既有的 `verify_confirmed` 或 `completed_without_verify` 再完成 Job。整包确认阶段单独持久化，进程重启不会误回旧版展示/验证分别发送流程。

- 资料包有租约、心跳、任务键、代次和清单；旧代次不能覆盖新包。提交任务前后崩溃可由确定性任务键修复。
- 每条媒体都有独立标记和资料包标记，包括相册非首条消息。文案截断预留两个标记的空间。
- 每次目标投递仍创建独立 Attempt；资料包标记与 Attempt 的唯一编号分开，避免明确失败后重试触发唯一键冲突。
- 复制返回数量不足时不按位置猜测展示/验证归属。迟到部分响应不能覆盖已映射的 Webhook 证据。
- 对账核对完整消息数量、唯一目标 ID、来源 ID、媒体 ID 和用途。历史只读取最近 100 条；空或部分结果不证明未发送，最终转人工核对。
- 目标复制不确定时，超时处理不再自动创建替代尾部 Job。用户新建业务操作仍需先核实旧操作的目标消息。
- 旧版中转链路额外修复：现成单媒体索引正确返回；媒体组提前失败时释放已取得但未完成的租约。
- 已有全量推送的用户任务记录聚合入口继续复用，没有新增每条媒体的用户任务记录。

中转内部失败可能留下孤立消息，本次未增加自动删除策略，以免删除仍用于恢复的来源。现阶段保留来源消息和清单；后续清理必须排除未完成 Job、对账中 Attempt 及旧版 relay_media 引用。不要按频道消息年龄批量清理活跃包。

## SQL 与发布顺序

新增迁移：

- PostgreSQL：`server/addons/youban_publish/resource/sql/migrations/20261021_relay_package.pgsql.sql`
- MySQL：同目录 `20261021_relay_package.sql`（本次未实际运行 MySQL 验证）

迁移扩展 relay_channel，新增 relay_bot、relay_package、relay_file 及负载索引。均为增量变更，不删除原字段。已有安装先具备 `relay.pgsql.sql`/`relay.sql` 的旧表；全新安装也需要显式执行本迁移。

发布顺序：先备份并执行对应数据库迁移，确认字段和索引；再统一部署后台、推送 Worker、协议账号 Worker 的新版本；检查频道、账号和 Bot 权限后，按租户灰度开启。

```text
YOUBAN_RELAY_PACKAGE_ENABLED=true
YOUBAN_RELAY_PACKAGE_TENANTS=租户ID1,租户ID2
```

默认不开启。开启但不设置租户名单时覆盖所有新 Job，生产首次启用应设置名单。已进入旧发送阶段的 Job 不切换新模式；已进入新阶段的 Job 在开关关闭后仍按新模式恢复。

回退先关闭开关、等待已进入新阶段的 Job 和账号任务排空或完成核实。旧二进制不认识新任务和阶段，不能直接覆盖仍有活跃新流程的 Worker。数据库增量字段保留。

## 验证记录

本地已完成：

- GoFrame CLI 生成四个中转表对应 DAO、entity、do、table 元数据。
- PostgreSQL 17 在 `127.0.0.1:55484/youban_relay_package_test` 应用基础中转 SQL 和新迁移；重复执行新迁移成功。
- `YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys -run '^TestRelay' -count=1`：通过。覆盖 12 并发容量争抢、租户隔离、冷却频道排除、旧代次隔离、部分响应与 Webhook 竞态。
- 专项单测覆盖完整/部分/重复投递证据、消息归属、相册每条标记、Unicode 文案截断、灰度与关闭后的恢复、限流范围和快照字段。
- 模拟 Bot API 覆盖 10 条相册加 1 条尾单消息，使用已有 file_id，无真实外部发送。
- `go test ./addons/telegram_collector/logic/sys -count=1`：通过。
- `go test ./addons/youban_publish/... ./addons/telegram_collector/... -run '^$'`：编译通过。
- 仓库内兼容管理端：两个修改文件 ESLint、Vite 生产构建通过。
- 实际管理端位于 `tdesign-starter/vue_admin_demo/apps/web-antdv-next`，独立工作树 `/tmp/youban-relay-ui-20261021`，分支 `refactor/relay-package-ui-20261021`，基线 `3f36ffcfd`；已适配 Bot ID、容量、服务端状态文案、编辑、异步任务轮询、组件卸载清理。保留旧 API 导出，拆分超长 API 文件和页面逻辑；新增文案提供中英文。
- 实际管理端 `typecheck`、`build`、修改文件 Oxfmt / Oxlint / ESLint 通过。当前本机 Node 26 超出项目声明的 Node 22/24 范围，生产制品仍应由项目锁定的 CI 环境构建。

完整 publish/sys 测试有 4 个顶层测试失败（共 6 条含子测试的失败记录），在未修改的 `b9aa4117` 独立基线中全部复现：`TestNormalizeAntiScanMattingConcurrency`、`TestDuplicatePHashScanBucketsBoundLookupCost`、`TestClassifyProfileMessage` 的两个场景、`TestParseProfilePublishMark`。本次未更改其业务实现；全量测试不能标为全绿。

尚未执行真实 Telegram 端到端测试。启用生产灰度前需在专用测试频道完成：展示+验证整包发布、再次发布、下架删除、新 Bot 自动接入、权限撤销恢复、明确频道限流切换、目标复制超时对账、开关关闭与 Worker 重启恢复。不能将本地模拟测试表述为线上验证成功。

## 收尾 review 与发布判断

- 输入校验归入 input Filter，容量默认 2、上限 20；新增状态集中在插件 consts 并注册字典。DAO/entity/do 保持 CLI 生成，HTTP 控制器和现有任务记录入口不增加业务逻辑。
- 修正实际租户管理端入口的身份授权：先区分系统 AppAdmin 与租户 AppApi，避免不同账号体系数字 ID 重叠误判；租户管理员只可查询、管理自身租户及其协议账号的频道，系统入口仍要求超管。
- 管理任务处理中禁止配置覆盖；创建频道不可逆 RPC 前以任务 ID 和 pending 状态条件更新抢占，旧任务不能重复创建。提交失败记录 failed，避免界面永久显示等待提交。任务读取走主库以避免副本延迟。
- 兼容旧频道 bot_id=0：仍可停用，启用前需编辑选定 Bot 并检查权限。
- 参数校验和匿名/未知身份拒绝测试已补齐，最终专项测试与 PostgreSQL 并发测试通过；采集插件测试及两个插件全包编译通过。

代码可以提交并进入 CI。部署代码需要先执行增量 SQL、同步相关 API / publish-worker / account Runtime 版本及实际管理端；初次部署保持新链路关闭。**尚不满足全量启用条件**：真实 Telegram 端到端场景仍待验证，全量测试存在已在基线复现的历史失败。完成专用频道验证后才按明确租户名单灰度，不能直接打开全部租户。
