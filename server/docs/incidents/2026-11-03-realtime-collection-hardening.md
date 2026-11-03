# 实时采集链路修复与验证

2026-11-03。只修改实时采集相关逻辑；发布锁/Telegram 发送恢复由另一工作流处理。代码验证不等于已部署。

## 共因

链路具有多个确认边界：账号更新 → Collector Event → Delivery → 业务 Event → 处理 Outbox → 媒体/规则/审核 → 发布。旧实现把「进内存缓冲」当接收成功，部分入队错误只记录日志，历史标记未穿过插件适配边界，恢复分类又使用记录创建时间而非稳定来源标记。这些不一致会造成丢失、实时池被回补污染及超时任务遗漏。

此前4,371条样本回放只到模拟业务处理器，没有验证账号入口背压或业务Outbox，不能用其结果宣称完整流程已通过。本次扩大到真实业务转换与持久化交接，并单独测试失败边界。

## 本次修复

- 账号更新同步调用持久化入口。依赖暂时失败时指数退避200ms至5s，并保持相同事件身份重试；成功后才向更新dispatcher返回成功。移除可满后丢弃的4,096条进程内缓冲，取消时返回错误，不吞失败。此保证针对本应用返回成功的更新，不等于已经验证断电/Telegram更新游标的跨进程重放。
- Collector历史事件单独入history队列，由独立单并发消费者处理，实时消费者保留容量。
- `History` 通过Delivery适配器、CollectMessage、媒体组入队和持久化Outbox传递；过时Bot消息按来源时间进入历史业务池。
- 业务Outbox新增可空`history`字段：true历史、false明确实时、NULL旧记录。旧记录批量读取来源事件的时间分类；恢复补投不覆盖已持久化的分类。
- 每批为实时pending、历史pending、过期processing分别保留约60/20/20配额；过期processing不再受创建时间10分钟限制。任务、状态和队列均不批量重置。
- Outbox数据库时间通过`utility/dbtime`统一解释，避免UTC附着造成8小时年龄误差。
- Collector Worker新增2秒周期、每轮10秒预算、每批200条的轻量Outbox dispatcher；原分钟级恢复保留兜底。使用数据库条件领取协调多实例，无需新增全局锁。
- 媒体组入队失败向上返回，避免Delivery提前确认；缺失必要history字段不能被当作缺整张表静默降级。

## 验证

- 三个包全量测试通过：telegram_collector/logic/runtime、telegram_collector/logic/sys、youban_publish/logic/sys。未配置外部集成环境的既有用例仍跳过。
- 入口暂时失败重试成功、取消不确认，历史入口独立路由、运行组件停止测试通过。
- 实际临时PostgreSQL + miniredis：满负荷实时输入下仍选中历史和旧过期任务；20分钟前创建的过期任务也能正确进入恢复通道，覆盖原8小时偏差。
- 4,371条真实事件经过Collector重建、数据库领取、Asynq消费后，私有临时文件捕获成功交付对象；再调用真实业务转换和Outbox持久化，每条重复提交一次，持久化记录仍4,371条。业务层实时605条、历史/延迟3,766条。Bot业务账号使用隔离固定测试owner，此项不验证VIP、权限或规则匹配。
- 上述新增入口、路由、恢复和真实样本业务交接测试通过race检测。
- 新迁移在临时PostgreSQL中执行成功，原pending与processing两条哨兵记录的状态保持不变，history为NULL。原始样本和交付捕获文件均在仓库外，不提交正文或媒体地址。
- 本次未向生产发送Telegram消息，未执行线上DDL、部署、重置任务或清队列。实际网络下载、账号断线/断电重放、审核和最终频道发送仍需部署后独立验收；旧真实样本不能代表当前生产吞吐或SLA。

## 上线要求

先执行`addons/youban_publish/resource/sql/collect_process_history.pgsql.sql`。其中索引使用CONCURRENTLY，必须在显式事务外运行，并核对新索引indisvalid。迁移新增列和索引，不回填/重置历史任务。此前统计持久化修复还要求`collect_source_stats_work.pgsql.sql`；两项均需完成。

升级Collector Worker、Publish/Delivery Worker和账号入口相关角色，确认新旧队列都有消费者后再验收延迟。旧producer过渡期缺少标记的记录按NULL旧记录时间兼容处理；仅升级一端不能认定全链路隔离完成。回滚应用可以保留新增字段与索引，不删除任何队列；须保留新队列消费者直到存量任务完成。

指标至少分段观察：账号消息到event、event到Delivery、Delivery到业务event、业务Outbox最老pending/过期processing、history/realtime队列、媒体完成、审核等待及发布完成。区分正常等待审核/验证与运行故障。
