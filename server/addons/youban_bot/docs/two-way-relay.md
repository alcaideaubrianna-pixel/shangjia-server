# youban_bot 双向机器人

实现归属 `youban_bot` 插件，复用现有 Bot Token、Gateway、用户表及群资料缓存。独立的 `youban_two_way_bot` 插件和前端源码均未修改。

## 启用及配置

1. 发布前显式执行 `resource/sql/relay.pgsql.sql`（PostgreSQL）或 `relay.sql`（MySQL），再通过插件升级执行菜单权限同步。安装/升级流程已包含新表，但普通应用启动和消息处理不会执行 DDL。
2. 在「Bot 管理 → 插件开关与配置」打开「双向机器人」（`two_way_relay`）。现有通用配置协议会展示新增字段，无需前端改动。插件首次注册默认停用；完成迁移并保存目标Bot设置后再启用。
3. 选择“应用到Bot”，配置双向功能、管理员、双向列表、黑名单、屏蔽词、跳转开关、数据库存档及审计转发群。
4. 该弹窗是配置应用入口：切换目标 Bot 不会自动请求该 Bot 的设置。保存将把当前完整表单应用到目标 Bot；独立读取和编辑某个 Bot 请使用下文 settings 接口。弹窗重新打开时回读上次所选 Bot 的数据库状态，包括 Telegram 命令做出的修改。
5. 管理员、黑名单、关键词每行一项。双向列表每行两个对象，以空格分隔；保存后完整替换目标 Bot 对应列表，其他 Bot 不变。任何一项校验失败，整次配置回滚。
6. 整个 Bot 的运行状态仍由原 Bot 列表启停控制；双向插件另有全局开关和单 Bot 开关。全局开关单独切换不会用旧配置覆盖聊天命令刚修改的规则。

首次配置该 Bot 的后台账号为默认管理员，身份通过现有 `app=admin` 的有效 Telegram 账号绑定识别；后台账户必须先绑定 TG，或在附加管理员中填写自己的 TG ID。附加管理员支持多个 ID 和用户名；未知用户名为待确认状态，当前 Bot 观察到其私聊后固定为 ID。用户名后续被其他人注册，不会继承原管理员权限。

用户需先私聊当前 Bot；群聊需添加当前 Bot 并确保其可读取普通消息。资料在现有用户、群缓存表中复用；绑定时检查当前 Bot 是否可以访问对象。用户 ID 和群 ID 均按字符串返回，前端不应转为 JavaScript number。

## 双向传递与过滤

- 支持用户—用户、群—用户、用户—群、群—群；频道不作为绑定对象。
- 每个聊天端点在一个 Bot 内最多属于一个绑定。端点唯一索引和配置行锁共同防止并发冲突。
- 用户名用于查找，持久化使用 Telegram ID。新绑定不补发历史聊天。
- 通过 `copyMessages` 传递，保留原有媒体、媒体说明、实体格式和相册分组，不暴露原始转发链接。
- 文本、图片、视频、文件、语音、贴纸等由 Telegram 复制能力支持。不支持、已删除或不可访问的消息记录失败并提示来源聊天。Telegram 可能跳过消息，部分成功记录为不确定，不错误关联返回的消息 ID。
- 黑名单拦截发送人以及绑定两端。用户在群中发言也受用户黑名单约束；群黑名单暂停该端点双向传递。
- 跳转过滤覆盖正文、媒体说明、mention/text_mention 和隐藏链接；字面 `@`、`t.me/`、`telegram.me/`、`telegram.dog/`、`tg://` 会被拦截，邮箱中的 `@` 也会命中。
- 关键词按包含关系匹配，英文不区分大小写。待发送消息会重新检查当前规则；相册成员被屏蔽时，其余尚未发送的成员不再绕过过滤。
- 管理命令及斜杠菜单命令不作为普通聊天内容传递。管理命令在私聊执行；`/id`、`/help` 可用于群聊。
- 编辑、撤回暂不双向同步；新 Update 中的服务消息不转发。

## 聊天审计

数据库存档和实时群转发可以独立开启，也可以同时开启。存档保存收到的用户/群消息原始 JSON，包括被屏蔽的消息和管理命令；不包含机器人自动菜单回复。媒体 JSON 保存 Telegram 文件标识及元数据，不下载媒体二进制。原有 Bot 消息日志仍遵循原来的 24 小时保留规则，独立审计存档不参与该清理。

审计转发群必须是开启 Forum 话题的超级群，机器人必须是管理员并具有管理话题权限。审计群禁止同时作为当前 Bot 的业务绑定端点，其消息不会再次进入业务转发。

会话键为排序后的双方 Telegram Chat ID。A→B 与 B→A、解绑后重新绑定同一双方，均复用 `bot_id + audit_group_id + pair_key` 唯一话题，默认标题为“昵称-昵称”。未绑定的访客按“访客-Bot”建立独立话题。标题变化不改变会话身份。

审计使用 `forwardMessages` 保留来源信息。受 Telegram 内容保护等限制而不可转发的消息仍可在开启数据库存档时保留 JSON，审计投递显示失败。审计设置及命令不出现在机器人 `/help` 或“当前设置”回复中。

## 投递、重试与恢复

- 所有发送任务先保存到 `hg_youban_bot_relay_delivery`，不依赖临时 goroutine 或 30 秒媒体缓存。
- 双向传递、审计、存档分别建立唯一记录；一个成功、另一个失败不会重复发送成功项。
- Runtime/Account 角色运行投递循环，每实例 4 个消费者；支持多个实例通过数据库行锁及状态条件更新抢占。API 只落库，独立 Worker 角色不会持有额外 polling 客户端。
- 相册延迟约 2 秒聚合；单次最多 100 条，实际分组以已到达 Gateway 的消息为准。极晚到达的成员可能分批发送，不无限等待。
- Telegram 429 按 RetryAfter 延迟，确定失败终止。网络中断、超时、部分发送及过期发送租约标记 `uncertain`，避免盲目重发。
- 发送/取消完成后清理投递任务的原始 payload，存档只保留在独立 archive 记录；失败/不确定任务保留 payload 供核对。投递去重元数据继续保留。
- 话题创建前持久化 `thread_id=-1` 创建意图，防止进程崩溃或回执丢失后重复创建话题。确定创建失败可重试；长期 -1 需核对群中话题后恢复映射。话题被删除时将旧映射失效并重建。
- `failed`/`uncertain` 记录通过 records 接口查看。人工补偿前必须核对目标消息；对不确定任务不得直接批量重置 pending。无自动死信重放。
- 单 Bot 停用后消费者会取消读到的待发送任务；插件总开关停用时暂停消费并保留已有任务。已在途的外部请求不能撤回。重新启用不会补采停用期间聊天。

## 后台接口

以下路径均在既有插件后台前缀下，由 `AdminAuth` 和菜单权限控制：

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/youban_bot/relay/bots` | Bot 列表及双向状态，不返回 Token |
| GET | `/youban_bot/relay/settings` | 按 botId 读取独立设置、管理员 |
| POST | `/youban_bot/relay/saveSettings` | 保存设置，首次保存确定默认后台管理员 |
| POST | `/youban_bot/relay/switch` | 仅修改指定 Bot 的双向开关，enabled=0/1 |
| GET | `/youban_bot/relay/bindings` | 分页双向列表 |
| POST | `/youban_bot/relay/bind` | left/right 使用 ID 或用户名，新增绑定 |
| POST | `/youban_bot/relay/unbind` | 按 botId/id 解绑并取消待投递聊天 |
| GET | `/youban_bot/relay/rules` | 分页规则，kind=user/group/keyword |
| POST | `/youban_bot/relay/saveRule` | 增加规则 |
| POST | `/youban_bot/relay/deleteRule` | 按 botId/id 删除规则 |
| GET | `/youban_bot/relay/records` | 分页聊天存档和投递状态，kind=archive/audit/relay |

列表沿用 `page`/`pageSize` 分页契约。新数据枚举由 `YoubanBotRelaySwitchOptions`、`YoubanBotRelayRuleOptions`、`YoubanBotRelayDeliveryStatusOptions`、`YoubanBotRelayDeliveryKindOptions` 提供。

专用 Bot 表格、设置页及分页审计查看器只提供后端接口，本次遵照不修改前端源码的要求，未增加专用前端页面。

## 验证

默认测试使用临时 SQLite 验证入库去重、过滤、配置回读、Telegram 请求。需要 PostgreSQL 行锁的测试显式使用本机隔离库：

```sh
YOUBAN_RELAY_TEST_PG='pgsql:relay_test:@tcp(127.0.0.1:55439)/youban_relay_test' \
GF_GCFG_PATH=/path/to/test/config \
go test -race ./addons/youban_bot/logic/sys ./addons/youban_tg_bot_gateway/logic/sys
```

测试库必须是本机且命名为 `youban_relay_test`；测试会重建 relay 表并清空测试用 Bot 资料，禁止指向生产库。覆盖四种绑定、并发绑定冲突、相册排他抢占、话题双向复用、默认/附加管理员、用户名转让、跳转/关键词、重复 Update、配置回滚与 Gateway 接管失败后的重试。

DAO/DO/entity 由 GoFrame CLI 基于显式迁移创建的本地 PostgreSQL schema 生成，不手写生成目录。新功能按命令、目录解析、规则、绑定、投递、话题和后台配置拆分，新增及修改的 Go 文件均不超过 500 行。

## 主机器人隔离与缺失迁移保护

双向插件仅作为全局配置入口，不向主机器人全局命令或回复键盘注册 `/help`。
Gateway 只给明确启用双向配置的 Bot 返回 `/help` 和 `/id` 菜单。
未配置、停用或双向配置读取失败时，不认领主机器人的消息，原有插件继续处理。
只有完成正向范围识别、开始处理该 Bot 的双向消息后，业务失败才通过队列重试。
插件默认停用，停用的投递循环不访问双向迁移表。
