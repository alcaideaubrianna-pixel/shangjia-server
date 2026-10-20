# 蓝润润来源下架恢复与来源档案审查

## 线上操作范围与结果

- 租户 56，账户 561 / wang。
- 源频道 lanrunrunff22，Telegram chat 4439615649 / -1004439615649。
- 旧采集源 348 已删除，新采集源 355 指向相同频道。
- M21763 / profile 654490 属于旧源 348，缺少 compact channel/profile 索引。
- 通过 MCP 现有 offline、delete 能力处理 43 条尚存资料，两个计划均完成且无失败。
- offline plan: sbg3gl01000dm9h3d1ls1a3uc0lbm04b。
- delete plan: 1i2v3cn1000dm9h3luz87r8yi0fugy2o。
- 43 条资料关联的 5626 条 TG 消息状态均为 deleted。
- 扩展到该来源全部 44 条历史资料（其中一条此前已删除），关联 5670 条消息均为 deleted。
- 删除通过现有业务队列执行，未直接 SQL 删除资料、消息或绕过清理队列。

## 根因

采集源下架原来仅按当前 source_id 选择 dispatch。来源删除重建后，旧 source_id 的资料被遗漏。频道下架又仅依赖 compact 索引；索引无记录时把源 Telegram chat ID 当作内部发布 channel_id 查询，两个 ID 命名空间不同，既可能漏查，也存在碰撞误选风险。索引部分缺失时旧 fallback 根本不会运行。

## 29c43e6 审查与补修

永久来源档案在入库事务中保存频道、消息、采集方式及操作者标识，避免仅依赖可清理的事件。该提交本身没有更改历史下架选择器，不能单独修复本次事故。

本次补修：

- 始终合并 compact 索引、dispatch 频道快照、事件频道、TG job 源频道快照。
- 永久来源开关开启时合并 origin；关闭时不访问未迁移表。
- 所有证据都限制 tenant/account；频道比较兼容正数和 -100 格式。
- 规则下架保留 exact rule 证据。Origin 没有 rule_id，不能凭当前配置猜历史规则；没有规则证据的 origin 仅用于整个源频道下架。
- 采集源下架按相同频道找到旧配置资料，停止选中资料的历史 dispatch 重试；重试/重建采集仍保留原来的 source_id 范围，避免扩大重采集。
- 沿用现有通知、下架、Telegram 清理、后台删除流程。

## 发布约束

线上检查时永久来源表尚不存在，profileOriginEnabled 默认 false。先执行 profile_origin.pgsql.sql 显式迁移，再启用 YOUBAN_PUBLISH_PROFILE_ORIGIN_ENABLED，才能实际留存后续来源。历史来源通过现有有界 backfill 工具补齐；不得凭标题猜测缺失证据。

Origin 没有 rule_id。本次将现有 compact 规则关联与资料入库放进同一事务，并保留到资料删除；删除采集配置不再删除该关联。历史上已经丢失的规则证据不能凭当前配置补猜，不能把整个频道来源当成某一个规则。

规则级下架保留禁用标记，事件分发和最终提交前均检查；不关闭其他规则共用的账户/频道关联。发现资料被其他规则共用时拒绝规则级整条删除，必须明确选择全部规则范围。重新启用沿用现有配置保存入口清除禁用标记。

部署需要覆盖处理相关链路的 Web/API、采集、媒体处理和发布角色；应按实际角色启动配置确认消费者所在应用。旧消费者不识别新增规则级禁用标记，因此不能在混合版本期间使用规则级清理。全部相关消费者切换完成后再执行清理。此补修未增加启动建表操作，不要求为 compact 关联新增迁移；永久 origin 功能仍遵循上述独立迁移和开关。

来源下架保留原 exact source_id 历史任务集合，以便继续清理已删除资料残留的发布任务；页面资料数仅统计尚存且属于当前账户的资料，两者不应解释为完全相同的历史任务数量。

## 验证

使用本机 PostgreSQL 127.0.0.1:35432 私有事务临时表，不连接生产、不启动 TG worker：

```sh
YOUBAN_SOURCE_DOWN_LOCAL_PG=1 YOUBAN_SOURCE_DOWN_LOAD_PG=1 go test ./addons/youban_publish/logic/sys -run 'Test(CollectSourceDownHistoricalLocalPostgres|CollectChannelPurge)' -count=1 -v
```

覆盖：源 348 删除、355 重建；compact 部分覆盖和零覆盖；事件已清理但快照存在；job-only 证据；正负 chat ID；tenant/account/rule 隔离；发布频道 ID 碰撞；来源表未迁移且关闭开关；开启后 origin 合并；无规则证据不扩大规则下架；历史 dispatch 停止重试。

新增覆盖：跨规则共用保护、规则禁用隔离、资料事务回滚、分页计数与删除范围一致、600 条消息状态分批统计、10 万历史资料聚合和分批发布任务读取。查询和更新按每批 200 个资料 ID 执行，Telegram 删除沿用已有异步清理消费者，不在清理调度中逐条同步调用 Telegram。

本机一次测量：10 万条历史资料聚合约 117 ms，10 万个资料 ID 分批读取发布任务约 132 ms。这仅说明临时表测试结果，不代表生产耗时；生产索引、数据分布、积压和远端限流仍会影响完成时间。普通页面计数设置 3 秒查询上下文上限，事务内沿用原事务上下文，避免取消查询影响同一事务后续写入。

相关 ProfileOrigin / CollectChannel / CollectSourceDown / IntroFeeChannelPurge 测试和插件全部包编译通过。全量 sys 测试未通过，存在 NormalizeAntiScanMattingConcurrency、DuplicatePHashScanBucketsBoundLookupCost、ClassifyProfileMessage、ParseProfilePublishMark 等其他测试失败；已在未修改的 d17e27ae 基线上复现相同失败，不能声称全库通过。

本次本地验证不连接生产，不向 Telegram 发消息，不验证生产清理吞吐，也不意味着混合版本或正在进行的外部发送已具备强事务隔离。
