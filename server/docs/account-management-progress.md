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
- `account_renewal.go`：套餐选择、优惠码校验或跳过、订单确认、金额与支付链接。确认采用短事务认领；本地业务订单和支付记录先提交，再调用支付入口；同一会话复用稳定的订单标识。结果保存失败可在认领过期后恢复，旧认领不能覆盖新认领。
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

支付和授权服务在这些测试中使用替身；Telegram 使用本地 HTTP 服务。上述早期测试不覆盖真实支付回调与绑定后置钩子的完整投递。实际账户DAO与PostgreSQL并发已在后续专门集成测试补充，见末尾。

## 未完成，部署前必须补齐

1. 每日报告定时初始化、按中国时间切日、最近七日补偿已接入；本地 PostgreSQL 协调器测试通过。仍需实际 cron 安装与多连接并发回归。
2. 每日报告持久化租约、分段游标、失败退避、5次上限、关闭开关和七天清理已实现。启用配置但零活动的管理员会生成零值报告。Telegram发送成功但游标写入失败的窗口仍可能导致单个分段重复；不能宣称跨外部API的严格一次投递。
3. 报告口径：成功任务已按 TG job 的 `sent_at` 统计，失败按 job 最近更新时间回退记录时间，处理中按首次记录日期。跨日成功和失败回归已通过。历史任务若 job 已清理只能回退到记录时间；快照不是任意历史时刻状态的事件溯源。
4. 网页和 Bot 删除账户已接入共享删除服务：删除与通知记录同事务提交，Scheduler 独立投递，带租约、重试上限和30天清理。本地 PostgreSQL 已验证重复删除、跨租户拒绝、通知入库失败回滚、投递重试及关闭通知；实际公开接口和多连接并发仍需补测。
5. 频道权限熔断、采集账号频道访问异常、循环暂停已接入频道通知设置；协议号掉线已接入账户通知设置。关闭通知仍会执行暂停／停用，并完成正常去重。资料当前上架操作转为失败时，已在状态事务中保存 task 类通知；旧操作与重复失败不新增通知，投递前读取任务失败开关。仍需核对群聊推广等非资料任务是否应纳入同类通知。会员到期提醒固定开启，未接入可关闭的通用发送器。
6. 分享授权：实际账户创建、权限保存、真实绑定唯一约束和PostgreSQL并发已补测试；仍需绑定后置通知的失败恢复验证。分享确认提交后已复用普通绑定的底部键盘刷新、绑定后置钩子及平台管理员通知；邀请方的可靠通知仍需补齐。授权摘要检索与按场景过期索引已加入 Bot 安装／升级迁移；账户管理会话过期后保留30天，每10分钟最多硬清理200条，不删除资料上传草稿。PostgreSQL索引迁移本地重复执行通过，MySQL尚未实际执行。
7. 支付：本地订单／支付记录先于网关提交、稳定交易号重试、会话保存失败恢复、免费优惠码与实际会员业务回调已补本地验证，见末尾。仍未向真实网关付款，不能将本地测试宣称为所有第三方网关的实际端到端验收。
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

## 通知偏好接线回归

本地 PostgreSQL 测试 `TestAccountNotificationPreferenceLocalPostgres` 已验证四类偏好关闭、租户隔离、无配置／软删除配置默认开启、非法类型拒绝及发送错误透传。
`TestAccountChannelAccessPauseLocalPostgres` 新增验证：关闭所属用户通知仍暂停失权来源，独立配置的采集管理员通知保留。
两个插件的账户管理专项回归通过；本轮尚未重新执行整包所有测试。

## 本轮新增验证

- `TestPublishFailureNotificationLocalPostgres`：只通知当前失败操作、重复转失败不重复入队、接收者为同租户所属管理员、HTML转义、任务通知关闭后标记跳过、通知写入失败回滚状态。
- `TestAccountManagementSessionRetention`：每批最多200条、保留近期审计与有效链接、不触碰资料草稿、清理过期重置密码会话。
- Bot授权链接新增 `account_management.pgsql.sql` / `account_management.sql`，已接入Bot插件安装和升级。

本次失败通知仅持久化到数据库，由已有Scheduler账户通知Worker投递；不在上架状态事务内调用Telegram。

## 授权实际服务与并发验证

```sh
YOUBAN_LOCAL_COLLECT_PG=1 go test -race -tags youban_account_integration \
  ./addons/youban_bot/logic/sys -run TestAccountAuthorizationRealServicesLocalPostgres \
  -count=1 -timeout=60s
```

已通过。该测试使用隔离的本地PostgreSQL schema、生产建表SQL、实际Publish账户创建及设置服务，不调用Telegram／支付：

- 实际创建上架账户、随机12位密码与哈希保存、绑定TG、归属邀请管理员及租户。
- 仅授予TG绑定权限，未额外授予共享／群聊／资料查看权限；账户设置可由所属管理员继续修改。
- 注入绑定数据库约束失败，验证实际账户创建、权限保存、链接消费与确认状态一起回滚；解除故障后原确认可重试成功。
- 8个并发确认只创建1个实际账户，竞态检测通过。

另一个使用生产Bot表结构／绑定唯一索引的 `TestAccountAuthorizationConcurrencyLocalPostgres` 已在 `-race -count=3` 下通过。
本轮两个完整logic包再次回归仍只有文档中列出的5项既有基线失败，未发现新增失败。日志：`/tmp/youban-account-management-full-notification-test.log`。

## 会员续费持久化与失败恢复

- Bot确认先以短事务保存2分钟认领，支付流程最多45秒；结果以认领载荷为条件保存。宕机或保存失败后可在认领过期后重新确认，旧执行者不能覆盖新认领。
- Bot专用内部 `RequestKey` 按租户、账户、会话派生业务订单号；账户行锁保证首次并发创建只产生一份本地订单。参数与原订单不一致时拒绝复用。
- Pay创建支持内部 `DeferGateway`：只保存支付记录，提交后调用现有支付入口；重试沿用已保存的 `out_trade_no`。其他未提供内部标识的创建入口保留原流程。
- 修复无HTTP请求上下文时自动选择支付类型的空指针风险；未初始化支付配置返回中文错误。

验证命令：

```sh
YOUBAN_LOCAL_COLLECT_PG=1 go test -race ./addons/youban_publish/logic/sys \
  -run TestTenantVipRenewalDurableLocalPostgres -count=1 -timeout=45s
go test ./addons/youban_bot/logic/sys -run TestAccountRenewal -count=1 -timeout=90s
go test ./internal/library/payment/epay -count=1
```

已验证：网关配置异常后本地订单与支付记录保留；8并发重试复用订单和交易号；参数变化拒绝；Bot会话保存故障后支付身份不丢、活跃认领不重复调用、过期恢复、旧执行者不能覆盖新认领；单次免费优惠码在延期失败后可恢复，优惠码只计数一次，会员不重复延期；真实已支付业务回调失败回滚、重试与重复回调幂等。
易支付签名与订单URL测试使用本地库测试，会员回调使用实际业务处理器和本地PG事务；未向第三方真实下单／付款。

本轮整包回归仍为原有5项失败，日志：`/tmp/youban-account-management-renewal-full-test.log`。

## 2026-10-25：账户管理交互简化（本地验证）

- 账户管理首页把会员状态和到期时间拆成独立行；正文账户名称统一 HTML 转义并使用 `<code>`，不再显示内部账户 ID。
- 我的账户菜单每行一个功能。上架账户列表每行一个账户按钮，分页保留；点击进入该账户的账号设置、重置密码、删除账户。
- 自身与上架账户共用设置分类页：发送后缀、标题标识；上架账户额外显示账户权限。点击分类才展开对应设置，修改后返回原分类。标题选项显示 ✅。
- 创建、授权创建、重置与删除确认移除展示 ID，确认页显示所选账户名称；编辑文本提示显示账户名称、字段名称和原值。
- 回调仍使用内部 ID；所有目标读取先走 AdminAccountSettingView 的管理权限检查，显示名称复用现有账户服务。旧设置回调继续可用，不修改权限、密码或删除的事务实现。
- 本地 SQLite + 模拟 Telegram HTTP 测试通过：账户列表/详情/分类、名称转义、消息换行、无可见 ID、分页返回、旧回调、开关修改回到原分类、越权目标拒绝；同时跑过授权、续费、报告、创建、密码重置和删除现有专项用例。
- 验证命令：`go test ./addons/youban_bot/logic/sys -run 'TestAccountAuthorization|TestAccountRenewal|TestAccountReport|TestAccountCreation|TestAccountManagement|TestOwnPasswordReset|TestManagedPasswordReset|TestManagedAccountDeletion' -count=1 -timeout=90s`；`git diff --check`。本轮未操作生产、未发消息给真实用户、未部署。
