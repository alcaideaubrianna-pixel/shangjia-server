# 采集来源统一与兼容改造 Review

日期：2026-10-20。状态：已在隔离分支实现后端采集来源档案、统一读取与回填工具；原方案及 review 证据保留如下。尚未执行生产迁移、回填或部署。

## 已核实样本

`[wang] M21763`：tenant=56、account=561、profile=654490，派发 226069 → event 4794056 → source 348。采集方式 account，tg_account_id=54，频道原始 ID 4439615649，规范 ID -1004439615649，用户名 lanrunrunff22，消息 1033，媒体组 14333524971101069。source 在 2026-10-12 17:51:49 软删除，事件及派发快照仍在。资料 source_note_id=1033 是消息 ID，不是事件 ID。

上一轮未查到来源属于排查错误：直接按规范频道 ID 查原始 ID，并将消息 ID 作为事件 ID 查询。不能据此认定数据库丢记录。

## Review 发现

1. **来源持久性不独立。** collect_event.go 创建 dispatch 时保存频道、消息和方式快照，但 collect_material_commit.go 保存 profile 与 collect_profile_dispatch.go 回写 dispatch.profile_id 分属不同事务。期间失败，资料已经存在却可能没有派发关联。collect_source_trigger.go 的 clearCollectEventWorkflow 会删除 dispatch，重新建关联前再次存在窗口；若重算失败，窗口不会自然关闭。
2. **当前接口缺少采集方式。** ProfileModel 只有 sourceType=youban_collect、频道和消息信息；内部 profileCollectionMetadataRow 有 account/bot/follow，却未返回。AI Ops 也未提供独立的采集方式字段。不可从 youban_collect 推断 Bot/协议号，更不可从 source_note_id 推断 event ID。
3. **历史与当前信息混用。** profile_query.go 优先读来源配置/缓存标题，dispatch 快照兜底；来源修改后，显示不再严格代表采集当时。AI Ops 独立拼装来源，并未完整复用 dispatch 的名称/用户名/方式快照，与资料列表逻辑不同。
4. **消息归属不等于采集方式。** collectPublishClientRequestId 有意忽略 rule 和 collector：同租户、账号、频道、媒体组的 Bot 与协议号采集可以落入同一资料。保存单个可覆盖的 collector 会损失真实历史，改身份键则会改变去重、产生重复资料。
5. **fallback 不覆盖所有断链。** applyProfileCollectionJobSourceFallback 只为已有 metadata 且缺消息 ID 的资料查 job；完全没有 dispatch 的资料不会进入 fallback。job 也不保存完整频道标题和采集方式。
6. **既有回填不能作为完整追溯保障。** collect_source_backfill.go 只补 source_note_id，并按 profile.source_key=event.source_unique_key 查询；新版 collect:v2 身份键与带 collector 的事件身份键语义不同，不能假设相等。不得用错误的字符串连接补采集方式。

采集源常规删除目前是软删除并取消未完成任务，不会直接删除已成功资料的来源快照；定时事件保留策略只清 failed/ignored。不能把本样本归因为常规定时清理。

## 最小兼容设计

### 永久来源档案

建议新增插件表 `hg_youban_publish_profile_origin`，保存资料的一到多条来源事实，与 event/dispatch/source 的运行状态分离。沿用现有插件 DAO/logic 分层，通过 GoFrame CLI 生成 DAO/entity/do，禁止手改生成文件。

建议字段分组（具体 DDL 在实现时评审）：

- 归属：id、tenant_id、account_id、profile_id。
- 幂等：origin_key，使用事件身份构造并包含租户/账号/资料作用域，不使用 dispatch_id 或 rule_id 作为唯一事实身份。同一个事件多规则、多次重试只产生一条来源；另一 collector 的事件允许追加一条。唯一键 `(tenant_id, account_id, profile_id, origin_key)`。
- 分类：origin_kind（collect/tg_import 等）；collect_method 使用现有 account/bot/follow 常量。导入不伪装为采集，未知方式明确 unknown。
- 坐标：chat_id_raw、chat_id（复用现有规范化方法，保留私聊/普通群语义，不能对所有正数盲目加 -100）、message_id、grouped_id。
- 快照：channel_title、channel_username、source_id、event_id、bot_id、tg_account_id、follow_account_id；不包含 token/session/私密邀请链接。
- 时间：source_message_at（有真实证据才填）、received_at、created_at；接收时间不得冒充消息发布时间。
- 证据：evidence_kind、completeness（complete/partial/unknown）、缺失原因。必要时关联 import_task_id/import_group_id。

名称和用户名是当时快照，永久频道身份以 chat_id 为准。后续频道改名可另返回当前名称；不能覆盖历史名称。私聊来源必须如实标记 chat_type，不能一律称为“频道”。

不得使用 `ON DELETE CASCADE` 从 source/event/dispatch 删除档案；原记录删除后 ID 仍可作为历史标识。档案保留期跟随资料，资料实际删除仍走现有授权清理流程。不要把来源表加入普通日志/失败事件清理。

同一资料多条来源时：保留首次入库来源、全部来源；材料补全来源作为额外事实，不能把首次来源改成最后一次重试。历史首次来源无法确定时标记“最早可证来源”，不能拿最小数据库 ID 冒充真实首次。

### 写入接入

集中实现 `saveProfileOriginTx`，只接收已构造的来源快照与当前 tx，不在事务内访问 Telegram、下载媒体或查询外部服务。

在 `commitCollectPreparedProfile` 现有事务内，在 profileId 确定后写来源，覆盖：新增资料、正常更新、材料不足而提前 return 的分支。所有经过同一入口的普通采集、审核通过、重试、验证视频补全使用同一规则。核对全部调用点后接入，不能只改首次入库。

profile 与来源必须一起提交；来源写失败返回带上下文错误，沿用现有任务重试，不吞错并报告成功。保留旧版兼容开关时，必须区分 legacy/new writer：只有启用新 writer 并提交完整来源的资料可计入新保证范围；不能一边允许 fail-open 一边宣称 100%。

不得修改 collect:v2 身份键、内容指纹去重、资料编号、权限、审核分支、原发布 operationNo、Outbox 和发送顺序。不得为追溯修复自动重发、重新上架或重跑历史采集。

TG 文件导入一期可以仅统一返回结构、继续走已有导入链路；需要接入永久档案时单独评审其多阶段 saveProfile 事务，不能为了“统一”顺便重构整个导入流程。collect 的保证与导入覆盖分别报告。

### 一个读取入口

新增内部批量来源查询，资料列表、详情和 AI Ops/MCP 共享。批量按 tenant/account/profile 查询，权限沿用各入口现有校验，禁止按 profile_no 跨租户取来源；字段不暴露 Bot Token、协议号手机号或 session。

优先级：永久档案 → 派发历史快照/对应事件 → 已软删除的原来源配置补缺 → 同归属的发布 job → 可验证的旧 source_key。每个 fallback 带证据与完整度。仅从 key 恢复频道/消息不能推断方式；仅有配置当前 collector 也不能覆盖事件当时方式。可恢复字段逐项合并，不能把不同频道或 collector 的记录拼成伪造的完整来源。

兼容保留 sourceType、isCollected、collectSourceId/Name/ChatId/MessageId/Url 等已有字段及意义。新增 collectMethod、originKind、collector 引用、originCompleteness 和 origins；首次历史来源通过新增字段表达，旧字段显示行为在迁移阶段不骤然改变。方式枚举由后端字典统一提供，不由前端硬编码。

来源筛选需同步考虑无 event 但有快照/档案的资料，目前按 event INNER JOIN 的筛选不能用作永久来源筛选。读档案表缺失时不能静默吞数据库错误：由已验证迁移与明确功能开关控制；普通 SQL 错误照常返回。

## 安全迁移与上线顺序

1. 在目标分支最新提交的隔离工作区实施，保留用户未提交改动。先加独立表/索引，显式迁移，设置锁和语句超时；不在每个 API/Worker 启动或请求中执行 DDL。
2. 新增写入与回归测试，读路径先维持原行为。混合版本期间旧 writer 仍可能写无档案资料，必须记录切换时间和版本边界；只有所有相关 writer 更新后才能宣称新资料强保证。
3. 单独实现 dry-run、按 profile ID 游标、有界批次、可恢复、幂等的历史回填。优先 dispatch→event 精确关联，再使用 job 和配置补缺。只写来源档案，不更新资料排序时间、状态、媒体、编号，不触发发送。冲突或证据不足单列，不猜填。
4. 按租户与方式比对新旧结果后，再切换统一 reader；兼容保留旧来源字段和旧快照。读开关可回退，新增表保留，不通过删表回滚。
5. 监控新 writer 已提交资料的档案缺失数、partial/unknown、同来源幂等冲突、查询耗时；对新增 complete 合同违规告警。历史未知单独统计，不能混在新增成功率里。

“100%”验收定义：迁移完成并启用新 writer 后，每条成功采集入库资料，必须至少有一条同事务写入的、可确认 chat 身份和采集方式的永久来源；配置删除、重试和事件清理不影响读取。频道未公开用户名时，保证 ID 可识别，不保证用户可以打开消息链接。历史资料按证据覆盖率报告，不能保证恢复已丢失的信息。

## 必须覆盖的回归

- Bot/协议号/follow，各自来源准确；同消息组跨 collector 仍仅一份资料、保存多条来源。
- 同事件并发、多规则、重试幂等；profile 与来源任一写失败时一起回滚，恢复不重复发布。
- 正常更新和材料不足提前返回、审核通过、验证视频补全均保留原始来源。
- 来源软删/改名、事件清理、工作流重置失败后来源仍可读；快照和当前名称区分。
- 正负频道 ID、私聊/普通群、无用户名、媒体组/单消息、长 grouped ID 精确性。
- 无 dispatch 有 job、仅 key、证据冲突：正确部分结果与 unknown，不编造方式或混合来源。
- 列表/详情/MCP 一致，跨租户/跨账号不可见；旧字段兼容，批量列表无 N+1。
- PostgreSQL 真实事务故障注入与幂等回填；迁移重复运行、回填中断恢复、旧镜像读路径回滚。
- 保持既有资料身份、内容去重、规则、审核、媒体和发布专项测试通过；不把已知范围外失败掩盖为全量测试通过。

实施状态及已执行检查见 `server/tools/profile_origin_backfill/README.md`。TG 文件导入事务和前端交互本期保持原样；其余全链路及生产验收要求仍需在受控上线时执行。

## 实施补充

- 来源身份使用同租户/账号/资料下的原始事件 ID；没有事件 ID 的历史 key/job 使用带证据类型的独立身份。不会依赖重建后的派发 ID。
- 回归确认 ORM 会对关联的软删除 source 默认加过滤，历史读取已显式 Unscoped，并保留 tenant/account 连接条件。
- 新表显式迁移，默认开关关闭；已提供 ENV 映射。配置缺失表不会被忽略为成功，关闭开关时不依赖该表。
- 新写入、回填和授权资料删除统一 parent→origin 锁顺序；回填时资料已删除的候选计入 skipped。
- permanent origins 不受来源配置/事件/派发清理影响；来源消息直接查询可通过永久档案恢复，歧义返回明确错误。
- 现有字段保留显示行为；新增 origins 提供历史快照、方法、执行者引用及完整度。不能将不同频道/事件的字段拼成完整来源。
