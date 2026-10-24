# Bot 账户管理实现与验证清单

更新：2026-10-24。此文档是交接清单，不表示功能全部完成，不能作为部署通过依据。

## 用户确认的入口

账户管理顶部显示账户名称、会员状态和到期时间。

- 我的账户：当前账户、会员续费、重置密码、账号设置、推送设置、邀请注册。
- 上架账户：账户列表、创建账户、删除账户、账号设置（含权限）、分享授权链接。
- 每日推送报告：最近七个完整中国自然日，包含采集来源频道明细、采集推送目标频道明细、循环推送目标频道明细。

管理员管理入口必须验证 TG 绑定的上架系统管理员身份，不能凭回调中的账户 ID 授权。接收分享授权的入口允许未绑定者使用，但必须拒绝已绑定者。

## 当前已接入代码

- `addons/youban_bot/logic/sys/account_management.go`：菜单、管理员回调校验、账户分页、设置与权限按钮、报告浏览。
- `account_management_input.go`：创建账户、修改设置、重置密码与删除的一次性确认。
- `account_renewal.go`：套餐选择、优惠码校验或跳过、订单确认、金额与支付链接。确认会话和本地订单落库共用事务；重复确认返回已保存订单。
- `account_authorization.go`：30分钟单次授权链接。链接仅存摘要；单独的 `available` 状态不占用普通输入会话。重新生成／主动撤销会使旧链接失效。接收者确认后创建上架账户、授予 TG 绑定权限、写入绑定，并消费链接。所有本地变更在同一事务中。审计载荷保存接收者和创建的账户 ID，不保存初始密码。
- Publish 设置复用原有保存逻辑，部分修改先读取当前值。权限白名单仍由管理员及所属租户、父账户校验控制，修改后刷新可见性版本。

## 本地验证

```sh
go test ./addons/youban_bot/logic/sys ./addons/youban_publish/logic/sys \
  -run 'TestAccountAuthorization|TestAccountRenewal|TestAccountReport|TestAccountCreation|TestUploaderCreate|TestAccountSettingPatch|TestAccountNotification|TestOwnPasswordReset|TestManagedPasswordReset|TestManagedAccountDeletion|TestDailyReportSevenCompletedDays' \
  -count=1 -timeout=60s
```

该组测试已通过。主要边界：

- 创建账户不绑定操作者自己的 TG，失败时不消费创建会话。
- 重置密码／删除确认绑定目标和租户，重复点击不重复执行，失败回滚。
- 授权链接过期、撤销、重复确认、错误 Bot／用户／会话、邀请方归属改变、已有 TG 绑定、权限写入失败。
- 授权链接不被普通输入取消误撤销，重新生成和主动撤销有效。
- 优惠码无效反馈、有效确认、不使用优惠码、支付金额与链接消息、重复订单确认、失败回滚、会话隔离。
- 设置修改保留其他字段，权限字段仅接受 0/1；会员到期提醒不能关闭。

支付和授权服务在这些测试中使用替身；Telegram 使用本地 HTTP 服务。尚不能据此证明真实支付回调、实际账户 DAO 创建及绑定后置钩子、多个 PostgreSQL 连接并发均已验证。

## 未完成，部署前必须补齐

1. 每日报告定时初始化、按中国时间切日、最近七日补偿已接入；本地 PostgreSQL 协调器测试通过。仍需实际 cron 安装与多连接并发回归。
2. 每日报告持久化租约、分段游标、失败退避、5次上限、关闭开关和七天清理已实现。启用配置但零活动的管理员会生成零值报告。Telegram发送成功但游标写入失败的窗口仍可能导致单个分段重复；不能宣称跨外部API的严格一次投递。
3. 报告口径：成功任务已按 TG job 的 `sent_at` 统计，失败按 job 最近更新时间回退记录时间，处理中按首次记录日期。跨日成功和失败回归已通过。历史任务若 job 已清理只能回退到记录时间；快照不是任意历史时刻状态的事件溯源。
4. 网页和 Bot 删除账户已接入共享删除服务：删除与通知记录同事务提交，Scheduler 独立投递，带租约、重试上限和30天清理。本地 PostgreSQL 已验证重复删除、跨租户拒绝、通知入库失败回滚、投递重试及关闭通知；实际公开接口和多连接并发仍需补测。
5. 通知设置的频道异常、账户操作、任务失败开关需要接入实际通知生产者。会员到期提醒固定开启，不能被普通通知开关误关闭。
6. 分享授权：需补实际账户服务、实际绑定唯一约束和 PostgreSQL 并发测试；需接入绑定后置钩子与管理员通知。现有会话存储的审计保留与授权摘要检索索引需核对长期容量。
7. 支付：需验证外部网关成功而本地事务失败时的补偿；本地事务幂等不代表外部支付请求具有幂等性。还需真实支付回调／零金额优惠码验证。
8. 日报独立迁移已加入 Postgres/MySQL 安装和在线升级入口；Postgres本地连续执行两次通过。DAO/DO/Entity由GoFrame CLI生成。MySQL实际迁移尚未执行，不能宣称已验证。相关包完整测试仍有下述已验证的5项基线失败。

未执行线上 SQL、未部署，也未宣称完整功能验收通过。

## 2026-10-24 整包回归结果

完整执行两个相关包的测试后，5项测试失败。本次专项测试仍通过。

使用隔离工作区的已提交基线 `8f97f79e` 重跑，以下5项同样失败，错误一致：

- `TestRelayAuditControlsStayOutOfDedicatedHelp`：帮助文案暴露审计入口。
- `TestRelayHelpAndSettingsValidation`：帮助文案暴露审计入口。
- `TestNormalizeAntiScanMattingConcurrency`：当前机器归一化结果128，断言期望64。
- `TestClassifyProfileMessage`：混合媒体分类断言不一致。
- `TestParseProfilePublishMark`：小写 `xxy6400` 未识别。

基线 Publish 包使用仓库跟踪的 `config.example.yaml` 作为本地配置，避免复制线上凭据。

另外，`YOUBAN_LOCAL_COLLECT_PG=1` 的本地 PostgreSQL 日报统计、七天完整自然日测试已通过（临时表事务）。此结果仅证明现有统计查询可执行，不消除上文所列跨日业务口径缺口。

## 每日报告自动投递新增验证

```sh
YOUBAN_LOCAL_COLLECT_PG=1 go test ./addons/youban_publish/logic/sys \
  -run 'TestDailyReportGenerationLocalPostgres|TestDailyReportDeliveryLocalPostgres|TestBuildDailyReportLocalPostgres|TestDailyReportSevenCompletedDays' \
  -count=1 -timeout=60s
```

已通过：零活动管理员、禁用账户／子账户／跨租户源排除、中国零点切日、七日报告补偿、快照不被重复生成覆盖、重复轮询不重发、七天清理、分段失败后续发、5分钟退避、过期租约恢复、活跃租约保护、旧租约不能覆盖新状态、关闭开关停止发送，以及跨日任务成功归属。

实现位置：`daily_report_delivery.go`；安装初始化：`install/cron.go` 的 `ensureDailyReportCron`；迁移：`resource/sql/daily_report.pgsql.sql` 和 `daily_report.sql`；渲染共用 `library/dailyreport`。

调度每分钟检查一次，单轮有50秒截止时间，最多生成100份报告、处理50份投递；每次失败后按次数延后5分钟，最多5次。数据库租约2分钟，进度逐段保存。注意发送API本身另有35秒超时，因此单次发送可能超过协调器剩余时间；租约仍留有余量。
