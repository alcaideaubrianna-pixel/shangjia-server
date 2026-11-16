# 发布架构与可执行门禁

## 事实来源与边界

根目录AGENTS.md规定分层、缓存、日志和运行角色；本文把发布链路的部分约束转成可执行检查，不能替代业务评审。领域状态、调度状态、采集资格相互独立，账户调度不得恢复被明确禁用的授权。

发布链路保持：业务入口 → 持久Job/Outbox → 频道队列 → 受租约保护的发送与对账 → 终态投影。账户调度只合并唤醒，不删除真实发布任务。Telegram明确成功后本地投影恢复不能重新发送；未知结果先对账，不能直接重发。

## 本地与CI命令

从server目录执行：

```sh
go test ./tools/architecture_guard -count=1
go run ./tools/architecture_guard
go build ./...
go test ./addons/youban_publish/logic/sys -run '^(TestStrongTelegramJobStates|TestStrongTelegramJobStatePredicates|TestTelegramJobTransitionLongDispatchError|TestValidateTelegramJobTransition|TestTelegramJobTransitionUpdate|TestTelegramJobLifecyclePredicates|TestTelegramJobSourceClassification|TestAccountScheduleResultRealErrorPrecedesMore)$' -count=1
go test -race ./addons/youban_publish/library/accountschedule -count=1
```

最后一项默认使用隔离miniredis；设置YOUBAN_ACCOUNT_SCHEDULE_TEST_REDIS时，只允许显式127.0.0.1地址。PR工作流提供Redis7.4.2服务，常规队列测试使用该服务；独立故障测试自行创建miniredis并控制时钟。工作流不导入生产Secret、不运行部署脚本、不全量运行sys测试。上述命令不等于端到端TG发送测试。

## AST增量约束

工具位置：`server/tools/architecture_guard`。扫描整个`server/addons/youban_publish`中的非测试Go源码，包括新文件和新子包；不通过build tags隐藏检查对象。测试文件不纳入业务基线，guard自身使用专门违规样例自测。

| 规则 | 拦截对象 |
|---|---|
| redis-import/client/primitive | 新Redis依赖、g.Redis、cache.RedisClient、cache.ZSet调用、Group访问器、Eval/EvalSha、Do等底层入口 |
| state-map-write/index-write | 受保护Job上下文或明确生命周期值的status，以及dispatch_status、send_phase、publish_task_status写入；排除普通HTTP/filter/table状态 |
| state-selector-write | Job对象或受保护生命周期上下文的Status，以及DispatchStatus、SendPhase、PublishTaskStatus直接赋值 |
| state-sql-write | SQL字符串中直接UPDATE并SET状态字段的表达式 |
| state-cast | 对已登记JobStatus/DispatchStatus/SendPhase等领域状态的直接转换 |
| untyped-job-field | telegramJobRecord三个生命周期字段退化为string |
| controller-layer-import | 控制器新导入DAO或logic包 |
| dot-import | 无法可靠识别来源的点导入 |

这些是AST结构检查，不是完整数据流/SQL分析器。Do等方法可能属于HTTP客户端，存量同样精确登记；新合法调用需评审，而不是扩大目录豁免。动态字段名、间接函数值、改名后的自定义包装器，以及其他addon调用尚不能由本工具完整证明安全。新增这些模式必须人工评审；不能宣传为“任意绕过均可检测”。

## 精确存量清单

`baseline.json`每条包含相对文件、函数（含接收者）、规则、规范化AST表达式、SHA256、允许次数和原因。表达式改变、移动到新函数/文件、增加次数均会失败。通配文件/函数和重复条目会失败。相同函数中同表达式的移位无法区分，但不能超过允许次数。

存量清单是迁移债务，不是业务正确证明。清理存量后允许保留暂时未使用条目，后续应一并删除；工具不会把整目录视为可信。package级声明和import按精确表达式登记，不能以包名放行。

新增受控ownership helper也必须按具体函数审查。例如withTelegramUnknownJobLease的cache.RedisClient仅用于ownedlease；它的精确表达式可登记，不得忽略tg_reconcile_lease.go整个文件。锁负责排他执行，业务checkpoint负责对账进度，两者不能混用。

生成候选清单仅用于审查，不自动写入已批准基线：

```sh
go run ./tools/architecture_guard -inventory > /tmp/publish-architecture-candidate.json
```

禁止在CI中重新生成基线后直接通过。审查变更的具体表达式、函数职责、生命周期和幂等后，再人工修改对应条目及原因；修改guard、基线和workflow本身也须独立评审。本轮强类型迁移稳定后已定稿精确清单；不保留中间态豁免。

## 强类型边界

- JobStatus、DispatchStatus、SendPhase、Source代表不同状态域，不能互相赋值；域内使用统一转换入口和合法状态常量。
- Success/Failure/Deferred/More分别表示完成、业务错误、暂缓和有剩余工作；Joined错误中的真实错误优先故障退避。
- 业务AccountID与TelegramAccountID不能混用。DB/JSON/Redis适配层解析并验证领域类型，未知状态报错，不静默转换。
- Go命名类型仍允许字面量与显式转换，不是封闭enum。编译检查和AST转换限制必须同时存在。
- 不手改生成的DAO/entity/do；领域适配层承担类型转换。修复不能通过把领域字段改回string解除编译错误。

## CI与合并治理

新增工作流`publish-architecture.yml`支持pull_request、手动触发及workflow_call，包含guard自测、guard扫描、全包构建、选定纯状态测试和队列race测试。PR仅检查，不发布镜像、不部署。目标分支push由现有镜像工作流调用同一检查，镜像build通过`needs: checks`依赖检查成功；检查失败或取消时不会执行镜像构建与发布。检查工作流不再独立监听push，避免同一次push重复检查。镜像工作流路径过滤包含两个workflow文件。

仓库维护者应把PR的`publish-architecture`检查设为目标分支必须通过，并限制直接推送。镜像构建的检查依赖已写入workflow；本轮未修改远程分支保护。复用检查仅授予contents读取权限，不继承镜像仓库或生产Secret。

当前没有已确认的维护者用户名，本文不虚构CODEOWNERS。维护者确认后，应保护领域状态、队列/租约、运行角色、guard、baseline、workflow及本文；仅添加CODEOWNERS文件不等于启用强制审核。拥有绕过权限的人仍可同时修改工具和基线，必须通过仓库权限与独立评审补上这一层。

## 本轮门禁验收

本轮执行链路调整如下：

- Job记录与迁移参数使用强类型；迁移附加字段禁止绕过`status`、`dispatch_status`、`send_phase`。阶段写入增加条件更新，不能把已确认发送进度回退。
- 频道任务原子领取到processing凭证，记录token和deadline；续租、ACK、重新入队均验证归属。Worker崩溃后按过期凭证恢复，旧消费者迟到ACK不能删除新领取结果。队列判空和移除active标记在同一脚本执行。
- unknown对账按Job持有独立租约；数据库processing只是进度，不作为执行所有权。不同Job仍可并行对账。
- 恢复步骤轮转起点；实时恢复从Redis到期索引和持久SCAN游标推进，不再执行全局历史Job扫描。历史deadline回填要求显式租户、账户、主键范围和小批量事务，不在Watchdog自动执行。
- 单频道坏键记录错误后继续扫描并提交游标，避免阻塞同页及后续频道。到期索引前部有持续坏频道时，后续恢复可能等待SCAN轮转，不能承诺固定恢复时限。

新旧实例共存有协议边界：旧消费者不认识processing凭证；发布完成必须确认旧消费者已退出，不能仅凭新实例就绪认定队列协议已切换。此轮未部署或修改生产数据。

本地组合race测试已通过，覆盖真实隔离Redis、临时PostgreSQL阶段条件更新和显式历史回填，以及领取过期、迟到ACK、活跃标记竞态、恢复公平性和对账互斥；全包`go build ./...`通过。测试通过不代表生产积压已恢复。

- guard自测通过（0.658s），包含指针receiver、精确基线、违规状态写入、HTTP/filter元数据误报及新增子包扫描回归。
- `go run ./tools/architecture_guard`通过：281条精确表达式，无新增旁路。
- workflow YAML解析与复用依赖契约检查通过；镜像build依赖checks，检查不继承生产Secret。
- `.gitignore`仅开放guard的两个Go文件和baseline；其他tools内容仍忽略。

这些本地检查不代表GitHub托管执行已经完成，也不代表已启用远程分支保护。
