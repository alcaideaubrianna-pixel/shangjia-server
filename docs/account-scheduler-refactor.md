# 账户调度重构评审与验收

更新时间：2026-11-16。本文是重构期间的独立审查记录，列出基线、接口约束和验收要求；不能作为已部署或已通过测试的证明。仅新增本文档，未修改生产数据。

## 范围与事实边界

目标是让每个账户获得有界的调度机会，避免全局候选、Asynq 唯一窗口和慢账户阻塞其他账户。真实发布继续使用持久 TG Job、Outbox、频道队列和发送状态机。账户唤醒可以合并，资料发布任务不能因合并而丢弃。

本次频道231的采集授权故障独立于公平调度：来源351本身启用，Bot64与采集频道1279的发现关系为 discovered，但账户615对应的关系24093为 collect_enabled=0、disabled、rejected。21:29至21:32实际消费的历史媒体事件仍被忽略。公平调度不能绕过明确禁用，也不能自动恢复这些 ignored 事件。

关系24093自10月30日禁用，原因字段为空，不能由当前行证明操作者或历史成因。仅开启采集源不等于批准账户频道；本轮采集修改仅补齐授权拦截日志并澄清错误提示，不自动授权、不覆盖disabled/rejected、不重放ignored事件。审批写入错误和源/关系不一致属于独立后续议题。

## 基线入口与角色

| 入口 | 基线行为 | 重构约束 |
|---|---|---|
| scheduled_publish.go / runScheduledPublishRuntime | 周期调用 enqueueAccountSchedules；旧引擎另行扫描定时发布 | 多实例只标记账户待处理；目录分页游标单独推进 |
| scheduled_publish.go / submitDuePublishTasks | 无 scope 且全局 stream 时转账户调度 | 保留转发，不可再产生第二套账户执行器 |
| cycle_run.go / RunChannelCycleScheduler | stream 模式转账户调度；旧模式执行分阶段扫描 | 旧 Cron 入口保持可调用，统一转发；混合引擎仍需过滤 |
| cycle_profile_schedule.go / runProfileCycleDueScan | stream 模式转账户调度 | 账户查询不能被全局 Redis 前 N 条候选截断 |
| cycle_profile_schedule.go / runProfileCycleStartupRecovery | 启动30秒后恢复；stream 转账户调度 | 多实例启动幂等，不允许全局改写有效任务 |
| profile_cycle_orchestrator_runtime.go / runProfileCycleOrchestrator | 无 scope 时转账户调度 | scoped 执行不得递归产生全局调度 |
| tg_queue_worker.go / backgroundMux、accountScheduleMux、cycleMux | 基线三处注册旧 AccountScheduleTask | background/cycle保留桥接；旧专用队列隔离，不自动drain千万级积压 |
| runtime.go / startTelegramBackgroundWorker | BackgroundQueue 启动账户调度消费者 | 新消费循环归属 background-worker；all/worker 为兼容角色 |
| runtime.go / startPublishSchedulers | Scheduler 启动目录生产与启动恢复 | Scheduler 负责生产；不能依赖 publish-only 实例消费账户调度 |

角色依据 runtime_config.go：background-worker、worker、all 开启 BackgroundQueue；scheduler 仅开启 Scheduler；publish-only、web、collector 不默认承担账户调度消费。最终上线以实际代码与 SAE 命令为准。

## 调度接口必须保证的性质

### 账户身份与公平性

- 调度身份使用 tenantId + business accountId，不能把 channel.tg_account_id 当作业务 accountId；源账号、发布账号与 Telegram 协议号必须通过既有映射解析。
- 目录游标只由本次目录页的末尾决定。额外到期账户不能混入排序后拉回游标。
- 每次领取、恢复、执行批次有数量与时间上限。热账户完成一轮后回到队尾；新唤醒不能无限抢到所有冷账户前面。
- 多个目录生产者并行时，游标倒退最多造成重复标记，不能造成永远漏过某一段账户；需游标租约/CAS，或证明全遍历上界。
- Redis 到期索引是加速器。当前账户无候选、候选过期或不匹配时，必须有按账户、有界的 PostgreSQL 回退与持久计划恢复。

### generation 防丢唤醒

generation 表示账户在执行期间新增了工作，不等价于 TG Job 的业务操作号。

1. mark 原子增加 generation 并记录最早需要处理时间；执行中的账户不得被无条件标成 idle。
2. claim 原子生成唯一 owner token、读取本轮 generation，记录租约截止时间。
3. 同账户多个 mark 可以合并，但 worker 在执行期间看到的新 generation 必须保留下一轮机会。
4. finish 只接受当前 token；只有 generation 未变化且本轮工作确实完成时，才能清除本轮唤醒。
5. generation 已变化时，finish 将账户重新放入 ready；不能在另一次 mark 之后执行无条件 ZREM/DEL。
6. 延迟与即时唤醒并存时取较早时间。一个失败重试不能把刚到来的手工或采集工作推迟到更晚。
7. 无待办、达到本轮预算、执行错误、未取得业务租约应有明确不同结果。尤其未取得旧账户租约不能被当作“执行完成”。

### 租约、fencing 与崩溃恢复

- ready → running 的领取与租约记录需原子完成；禁止先删除 ready 再单独写运行记录。
- renew、finish、release 均按 owner token 条件更新。A超时、B接管后，A的迟到完成不能清掉B。
- 每个 claim 的租约到期可被有界恢复扫描找回。领取后崩溃、执行后ACK前崩溃、Redis响应丢失都不能永久丢账户。
- Redis token 只能隔离 Redis 状态，不能直接隔离 PostgreSQL 事务。ownedlease 的续租失败会取消上下文，但不能撤销已提交事务。
- DB侧创建 Run/Job 仍须业务唯一键、operationNo、generation和条件更新；更新计划只能由对应持久任务创建结果推进。
- 保留原 AccountScheduleLeasePrefix 业务租约，确保滚动旧实例与新Redis调度器不能同时执行同账户。
- 账户调度不直接发送 Telegram。即使账户阶段重复执行，Telegram侧仍须通过现有频道发送租约、Attempt、Job终态及消息幂等避免重发。
- mark失败、finish失败不能只打印日志后成功返回；应保留可重试状态，或由目录重扫/租约恢复覆盖，并明确最长恢复时间。

## 滚动部署与回滚

1. 保留旧 Asynq 账户任务兼容 handler，仅转换为新 mark；background/cycle队列可桥接遇到的旧任务。旧专用账户队列的监听器本轮有意移除，用于隔离千万级历史积压，不能宣称其已drain。已有旧实例执行中通过原账户租约互斥。
2. 先部署具备新消费者和旧任务桥接能力的 background，再升级 scheduler 生产端。若同时滚动，必须验证新队列无人消费期间的唤醒仍保留。
3. 不清空旧 Asynq 队列，不删除真实 TG Job、Outbox、频道ZSET或消息记录。旧账户任务可合并，但只能在成功转换后确认。
4. account_schedule_v2 前缀与旧账户租约有不同用途；不能为了换前缀同时换掉互斥租约。
5. 新旧实例必须使用同一个 Redis endpoint/DB。不同 Redis 上的锁不能实现互斥；之前分Redis造成的故障不能靠代码token补偿。
6. 回滚到旧生产/消费者后，新ready/running中尚未完成的账户仍需保留，由目录周期重扫或明确桥接恢复；不能宣称回滚天然消费v2键。
7. 实际部署角色以代码修改为准：账户生产/消费重构至少涉及 scheduler、background；若修改共享队列handler及周期生成代码，其他可执行同路径角色也要核对。仅升级publish不能替换background账户消费者。

旧专用队列处理属于独立迁移阶段：先按账户合并并核对目录覆盖，制定有界preview、计数、迁移预算和回滚方案后再执行；本轮不自动遍历、drain或删除旧任务。新目录重扫恢复账户机会，不依赖旧专用队列出队。旧队列里的账户唤醒与持久TG发布Job必须明确区分，不能把隔离账户唤醒解释为删除真实发布任务。

## 持久业务任务不丢的边界

- DB事务提交 Job/Outbox 后再标记账户或频道；Redis写失败由有界Outbox/恢复补偿，不回滚“Telegram已明确成功”的业务结果。
- 批次游标、next_cycle_at、run状态只能在相应持久任务建立后推进；不能把“账户已被mark”当作资料任务已创建。
- 未就绪媒体、权限暂停、未找到频道、数据库超时分别保留可恢复计划或进入明确失败，不可统一清空计划。
- sending/confirming/unknown 任务不得由账户调度重建成新发送任务；继续既有确认/对账链路。
- 有效历史任务不能因队列等待超过15分钟而 superseded；已删除、明确取消、同频道新任务替代与确认成功按现有状态机处理。

## 需要独立验证的场景

| 场景 | 必须观察到的结果 |
|---|---|
| A执行期间mark同账户，A完成 | 下一代仍ready，至少再执行一轮 |
| A租约到期、B接管、A迟到finish/renew | A不能删除或续租B的状态 |
| claim后进程崩溃 | 到期恢复重新可领取，generation不丢 |
| DB已提交、ACK前崩溃 | 幂等恢复，不重复创建同操作Job |
| 新旧worker共存 | 同账户业务执行串行，其他账户可并行 |
| 旧Asynq任务被新worker领取 | 成功桥接并可追踪，非unknown-task归档 |
| 持续高频mark一个账户，同时存在冷账户 | 冷账户在有限轮次内被领取 |
| 全局Redis候选全属账户A、账户B在DB有到期计划 | B仍能创建任务，不能零结果后退出 |
| 前几个阶段持续慢查询 | 后续循环/恢复阶段仍有执行机会 |
| Redis错误、租约争用、阶段超时 | 无永久idle，存在重试/恢复日志 |
| Scheduler与Background独立滚动 | 待办不丢，停止后可恢复 |

测试必须使用独立本地Redis/PG与fake Telegram，不连接生产消费者。独立测试文件已与接口所有者协商，见本地验证记录。

## 观测与验收

建议关键日志统一携带 tenantId、accountId、schedulerVersion、generation、stage、elapsedMs、result、retryAt；owner token只记录安全的关联标识。分别统计 ready账户数、最老等待、running租约数、到期恢复数、各阶段超时/失败及实际创建Job数。mark成功或领取成功都不能当作发布成功。

上线先对指定账户核对：调度领取 → 到期计划命中 → Run/Job提交 → Outbox → 实际发送 → Job终态。资格拒绝与无新采集输入单独报表，避免把一切未推送都归因于调度公平性。

## 本轮审查状态

- 已完成基线入口、角色与旧worker注册审查，并向实现agent反馈滚动互斥、generation CAS、阶段饥饿及业务账号映射风险。
- 核心接口已落盘并完成本地队列故障审查；本地测试不覆盖数据库fencing或SAE滚动实测。生产尚未验收，不能以本地通过代替上线结果。
- 未执行生产变更，未清理任何队列，未运行生产消费或发送测试。

### 独立故障测试（本地）

新增 `server/addons/youban_publish/library/accountschedule/queue_fault_integration_test.go`，与接口实现者协商后独立写入；使用隔离的loopback miniredis执行Lua并控制Redis TIME，不读取生产ENV。

执行：`cd server && go test -race ./addons/youban_publish/library/accountschedule -run '^TestFaultAccount' -count=1`，最新7项通过（1.709s）：

- 重复Ensure/Changed不后移ready score；执行中热账户再次唤醒后，不抢过已等待的冷账户。
- 租约恰好到期、尚未被新owner接管时，旧ACK已被拒绝；接管后旧失败ACK同样不能覆盖新owner。
- 将业务租约busy映射为Deferred后，账户仍在ready；周期Ensure不后移既有重试时间，过期后可重新领取；反复busy维持短延迟，不增加业务失败退避。
- 执行中连续Changed、随后Failure，下一次领取保留全部新增generation。
- 多次Schedule只保留最早未来唤醒，未到期不可领取；运行中到期Schedule经claim处理后仍保留下一代。
- More续调排到已等待冷账户之后，并递增Turn，后续阶段可轮转。
- 20个账户并发Schedule和ACK后，到期均可领取，没有丢唤醒或同时重复领取。

前4项的普通测试及-race均通过；最新7项包含More/Schedule接口变更后重跑，busy测试已跟进当前Deferred接口。

测试边界：busy测试验证队列的Deferred状态转换，worker将errAccountScheduleBusy映射为Deferred另由代码审查确认；未注入真实业务DB租约。

主协调agent另行使用 `/tmp/redis-7.4.2/src/redis-server` 在本机16594端口运行Redis 7.4.2，配置 `YOUBAN_ACCOUNT_SCHEDULE_TEST_REDIS=127.0.0.1:16594` 执行 `go test -race ./addons/youban_publish/library/accountschedule -count=1`，回传全包通过（1.932s）。其中常规队列测试使用真实Redis；本文新增故障测试内部固定创建miniredis，不读取该ENV，因此不能称全部故障注入都在真实Redis运行。该结果不覆盖多SAE实例或数据库fencing。

主协调agent后续回传最新真实Redis配置下全包-race再次通过（1.796s），并回传focused采集权限测试通过（0.810s）。仍按上述方式区分常规真实Redis测试与内部miniredis故障测试。

当前实现的目录扫描已移除全局dueAccounts合并逻辑，游标仅来自目录页，因此不再存在“due账户超过limit拉回cursor”的旧路径；新实现增加目录生产租约。线上多实例负载下的完整遍历耗时仍需观测，不能将本地验证等同线上时延上界。

### More / Schedule 最终业务审查

- Schedule使用独立wake ZSET，只保留账户最早到期时间；claim每轮有界转移一个到期wake，并在running状态增加generation。与ACK为Redis原子操作，不存在两者交错清掉新唤醒的中间窗口。
- Success无新generation后，当前实现同时保留30秒ready保底轮询；idle是无立即工作状态，不代表永不再执行。未来计划另由Schedule精确唤醒，不再仅靠全目录重扫。
- Changed不会覆盖ready的既有退避时间；运行中Changed保留generation，但Failure仍按故障退避（上限60秒）。这是账户控制面语义，不是手工发布Job的优先级。不能声称Changed会立即抢占故障退避。
- More用于满页/阶段预算未完成，队尾10ms续调并保留Turn；Deferred用于业务租约busy或总预算到期，短延迟1秒；Failure用于业务错误，指数退避。阶段超时现已显式记录More，不再静默视为全部完成。
- 复审发现executeAccountScheduleClaim曾优先解释More，errors.Join(More, 实际数据库错误)会绕过故障退避。现已改为accountScheduleResult递归审查joined叶错误，阶段预算分支同样先检查真实错误；真实业务错误优先Failure，纯剩余工作才More。独立执行 `go test ./addons/youban_publish/logic/sys -run '^TestAccountScheduleResultRealErrorPrecedesMore$' -count=1` 通过（1.077s），覆盖wrapped/joined真实错误、总预算、阶段deadline与busy，原阻塞项已关闭。

目录与修复范围补充：小表归属可复用channelBaseModel的channel→tg_account连接，再用tg_account.account_id限定业务账户；account_collect_runtime.go的accountCollectTgAccount已明确区分Id与AccountId。显式dirty频道应由已知tenant/account/channel的写入端标记；修复cp查询仍需限定tenant/account，不能只按租户遍历后过滤。dirty version并发清理CAS由周期实现agent负责，本agent不修改周期文件。

### 历史计划恢复的明确限制

主协调agent已删除syncTelegramJobCycleSchedule成功路径的无条件markAccountCycleRepairChannel：每次成功都增加dirty version会让热频道反复从映射起点扫描，当前页还未完成就被重置，放大发布成功后的数据库压力。

正常成功只更新该Job对应计划/映射及Redis索引；历史空计划仅通过显式dirty恢复。本轮不会自动seed所有历史频道，不自动修复此前所有NULL next_cycle_at，也不把“新调度上线”解释为所有历史计划已回填。后续历史修复需独立preview、授权范围、有界分页与监控；dirty版本CAS保护执行期间新增修复请求，不能代替历史种子任务。

### 最终交接

- 本agent独立7项故障-race通过；结果分类focused测试通过；文档与独立测试文件diff检查通过。
- 主协调agent最终回传：真实Redis配置下队列全包-race通过（1.796s）；focused测试含结果分类通过（1.075s）；本地PostgreSQL隔离测试含dirty版本CAS竞态通过（1.11s）；`go build ./...` 通过。这些是本地结果，未声称完成生产部署或线上恢复验收。
- 已关闭本轮发现的More吞真实错误阻塞项。旧专用账户队列保持隔离、历史计划不自动seed、采集授权不自动恢复是明确边界，需独立处理，不能作为本次完成后已自动修复的内容。
- 本agent写集仅本文档与 `server/addons/youban_publish/library/accountschedule/queue_fault_integration_test.go`；未修改核心实现、周期实现或生产。
