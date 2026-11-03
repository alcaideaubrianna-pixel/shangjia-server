# Delivery 隔离修复与线上样本回放

时间：2026-11-02，基线提交 `b2177e13`。本记录对应未提交的后续修复，不代表已上线。

## 范围与数据

只读取得生产采集事件的有界样本，在本机临时 PostgreSQL 与 miniredis 中回放。共 4,371 条，覆盖 6 个账号、83 个 source；按当前兼容分类规则，实时 2,601 条、历史 1,770 条，含媒体的事件 2,642 条、带媒体组标识的事件 2,163 条。原始样本不入 Git，不记录消息正文、访问凭据或媒体地址。

未启动连接生产的消费者，未清理或重置线上任务，未发送 Telegram 消息、写入生产资料、部署或执行生产迁移。

## 已修复

- 旧 Delivery 队列使用独立分类消费者，不再与历史交付共用一个消费者；历史阻塞不会阻止旧队列重新分类。
- 旧事件分类原先将 RFC3339 消息时间与 PostgreSQL `timestamp without time zone` 扫描值直接比较。真实回放暴露账号实时事件被误判。改用同一行的数据库 received_at 与 created_at 比较，保持同一种时间存储语义。显式 history 标记优先，不受兼容推断影响。旧数据无标记仍采用 5 分钟年龄阈值，并非对历史任务来源的精确还原。
- 确定损坏的原事件按 Delivery 主键转为 quarantined，保留记录和原文；恢复批次继续处理正常记录。数据库暂时不可用仍返回错误重试，不隔离。
- Asynq 使用 Start，由应用统一 Shutdown；移除 Run 的额外信号等待，避免 StopRuntime 等待组不退出。启动与停止的注册过程由 queueMu 串行化。
- 源统计请求改为数据库持久化工作集，一源一行，以 generation 防止刷新期间的新请求被清除；Redis 入队失败可由恢复任务补发。同源 Redis 任务合并，保留全局扫描互斥锁。

## 验证及边界

- 真实原始事件 → PostgreSQL event → 实际 Delivery 入库/Redis 入队 → Asynq 消费 → 模拟业务处理器：4,371 条完整回放。
- 人为阻塞历史处理器，实时交付仍完成；重复入口、旧队列任务与新队列共存时，业务成功回调无重复；注入一次下游暂时失败后，同 ID 再处理成功。
- 对每条重建 Delivery 做完整序列化摘要比较（排除运行生成 ID、DeliveryKey、原始 JSON），校验账号、source、媒体 ID/下载元数据、分组及正文等信息未变。
- 余下历史事件在停止 Worker 后直接调用 processDelivery 完成元数据与幂等校验，未按生产限速跑完全部历史队列。不能据此宣称历史队列吞吐达标。
- 非竞态回放的队列启动至模拟处理器 P95 约 791ms；竞态检测回放约 4.89s。仅为本机样本结果，不是生产 SLA 或 Telegram 到上架延迟。
- Delivery 全部定向测试（含真实样本）通过 race 检测；采集 sys 包全量测试通过。统计的聚合、并发合并、1,000 次同源触发、入队失败恢复、刷新中及完成后的新请求保留测试通过，含 race 检测。
- 本机 Redis 为 miniredis，未测真实 Redis 网络故障、主从切换或 Lua 性能。未执行真实 Telegram 接收、媒体下载、审核、资料提交及目标频道发布端到端测试。

## 发布模块失败项

当前工作区完整 youban_publish sys 测试未通过，不能标记为全绿：

- 原提交的独立临时副本同样失败：TestClassifyProfileMessage（2 个子用例）、TestParseProfilePublishMark、TestAntiScanPortraitUnderlayMaskKeepsThreeDistinctLayers。
- 原提交通过、当前工作区失败：TestBuildCollectRuleDecisionPreservesOtherLinesWhenSuffixEnabled、TestBuildCollectRuleDecisionReplacesJoinedIntroFeeSuffix、TestNormalizeCollectMaterialTextPreservesModifiedIntroFee。

上述三个新增失败涉及工作区已有的费用文案修改，不在本次队列修改中；保留其他工作的文件，未覆盖。部署前必须处理这些失败或明确隔离相应改动。

## 迁移与上线前提

必须先显式执行 `addons/youban_publish/resource/sql/collect_source_stats_work.pgsql.sql`，再部署调用该工作表的角色；代码不做启动 DDL。该迁移只新增统计工作表及索引，不改已有任务。回滚应用时可保留工作表。

保留旧 Delivery 队列消费者直到旧队列排空。新旧 Worker 滚动共存期间旧版本仍可能消费旧队列，因此实时隔离效果需以旧版本退出后验收；任务幂等依赖数据库条件领取，不能仅依赖 Redis Unique。

生产验收仍需观察各队列 backlog/最老任务时间、账号收取时间、event 到 Delivery 到业务处理各段延迟、审核状态、媒体和发布结果。已有业务任务不清队列、不批量重置。

## 2026-11-03 修复更新

上述失败已修复，未通过修改断言规避：

- 首三行标题清理跳过费用行和明确的「编号」字段，防止费用后缀重建丢失；保留已有编码标题清理。
- 通用消息分类仅将纯视频的非资料消息判为验证。带文案的图文混合资料保持展示；无文案混合相册仅在已有展示上下文内配对验证，跨业务边界不配对。
- 搜索优先识别发布标记；保留大写旧资料编号（含 FNLZ146897）和常规小写资料编号的精确匹配。小写多字母前缀如 xxy6400 识别为发布标记。两种标识本质存在字面歧义，此处大小写约定避免将原有大写编号改走发布序号查询。
- 人像底图蒙版包含人物本身，周边仍保留渐变，不扩大远处背景。

两个 sys 包全量测试通过（启用临时 PostgreSQL 和 4,371 条真实样本回放）；修复相关定向 race 测试通过。未提供外部集成凭据的测试仍按既有 gate 跳过，不代表生产下载、审核及发布端到端已验收。代码尚未部署，统计表迁移仍需在上线前显式执行。
