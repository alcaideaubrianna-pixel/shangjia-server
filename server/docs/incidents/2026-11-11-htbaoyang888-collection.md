# 汉庭 BOT 采集恢复记录

## 已证实的范围

北京时间 2026-11-11；生产 DBX PostgreSQL_Xiaohuiji/railway。
租户 78，业务账户 615（Baihuage），BOT 64，采集源 351，规则 338。
频道 -1004386299825，采集频道主键 272067，关系 51567，后缀配置 36。

消息 1937～1941 为展示媒体组，1942 为验证视频。原始消息已收到，
但入口 spool 为 received、attempt_count=0；当时没有 publish collect_event。
关系 51567 为 disabled/rejected/collect_enabled=0，disabled_at 为
2026-10-30 02:34:54.276819。后缀配置 status=1，采集源启用且绑定规则 338。
目前没有证据确认禁用操作的发起者。

## 代码根因和验证

频道候选查询把频道 keyword 同时用于筛选采集源，搜索“汉庭”排除了标题为
“百花阁 BOT 采集”、source_chat_id 为空的聚合源，最终用“未绑定”覆盖真实状态。
修复移除采集源查询的 keyword 条件；租户、账户、BOT 和规则条件保留。
另修复 saveIntroFeeChannelCollectionStatus 未填写 source_id 的缺陷：
原 upsert 的冲突键含 source_id，却以默认 0 写入，不能更新实际 source351 关系。
现在仅遍历本租户、账户、当前规则绑定且匹配频道/BOT 的来源，写入真实 source_id；
没有匹配源时报错并回滚事务。未认定历史禁用操作由此函数发起。

本机独立 PostgreSQL 35432 上使用上述线上元数据重现；无生产凭据或私有消息文本。
改前 TestIntroFeeChannelBindingOnlineFixturePostgres 失败，改后通过。
覆盖名称/用户名/ID/后缀搜索、空搜索、禁用、恢复、源暂停、不同租户绑定、
删除绑定和 120 个更新频道；同时运行 TestIntroFee*、TestCollectRuleLifecycleLocalPostgres。
没有启动本地生产消费者。临时 PostgreSQL 已停止。

## 精确线上变更

1. 条件更新关系 51567，恢复 enabled/approved/collect_enabled=1，清除 disabled_at/reason；1 行。
2. 提升 1937～1942 对应 6 条 spool 优先级至 1000；6 行。
3. 提升对应交付 28736203、28736205～28736209 优先级；6 行。
4. 近 72 小时该频道 86 条原始消息中，另外 80 条缺少 collector 原始事件。
   其中 70 条 spool 已标 completed、10 条为 received；不推断缺失记录的删除来源。
   以 tenant+完整 event_key+chat_id 逐条匹配，仅这 80 行置 received、priority=1000，
   attempt_count 和原 payload 保留，交由原消费者幂等处理。
5. 80 条均重新生成事件；对应 pending 交付限定同租户、来源、完整 delivery_key
   提升优先级，80 行。未删除数据，未更改其他频道的禁用状态。

86 条消息已全部完成交付；publish 事件 13378850 与 13378853 已配对，
生成资料 796619 / I02532，4 图+1 展示视频+1 验证视频均 ready，
列表索引归属 tenant78/account615 正确，dispatch 397632 已 sent。
共 11 份资料已入库：4 份 sent；另外 7 份 dispatch failed。
失败原因经生产表结构核对为 content_profile 不含 tenant_id/account_id，线上旧版本
profileMediaOperationPending 却查询这两个字段。本地已有提交 14123b5a 和
2a4be6a1 修正对应查询；本次未部署，也未反复重试这 7 份。
8 个非资料事件 ignored（消息不是资料组或验证组）。
其余入库及媒体结果需以最新 DB 状态为准，不能把入口 completed 当作资料成功。
Collector 日志确认正在运行的版本 a2150ee，出现了该展示/验证组绑定日志。

## 回滚和发布边界

代码尚未部署；修复的是 Web 列表查询。恢复状态为线上数据变更，已经生效。
若要撤销频道恢复，仅针对关系 51567/tenant78/account615/source351/channel272067
恢复 disabled/rejected/collect_enabled=0，disabled_at 恢复上述时间。
approved_at 在恢复时已写当前时间，变更前未保存该字段，不应声称能精确恢复旧值。
优先级提升不修改原消息内容；已完成交付不回退、不重复推送、不删除资料。
