# 介绍费频道禁用修复（2026-11-18）

## 故障证据

生产 Web 请求 08:18:08（Asia/Shanghai）：
`POST /api/youban_publish/publish/collect/intro-fee/channel-rule/blacklist`，
SLS trace `5a19ff9d961cd7192a69e697f3b3623e`，HTTP 200、business_code=0。
账号 567、租户 62、规则 1435、配置行 56、频道 3291507707。

该频道没有独立 collect_source：账号的来源为 1516（共享 BOT）和 421，
其 source_chat_id 均不是目标频道。旧接口对 source.channel_blacklisted
更新匹配 0 行仍报告成功；通用 source_id=0 频道关系被禁用，实际 source_id=1516
关系却仍 enabled/approved。列表的 blacklist 回读也只查独立来源，
前端因此一直得到 false，重复提交 blacklisted=1。

前端按 blacklisted 决定按钮，成功后重新加载列表；本轮不通过本地翻转按钮
掩盖后端写入或回读失败。

## 精确线上止血

通过用户授权的 PostgreSQL SSH，在 railway 主库事务内设置
lock_timeout=3s、statement_timeout=5s。锁定并验证：

| relation id | channel id | source id | 修改前 |
|---|---|---|---|
| 1749783 | 13925664 | 0 | disabled/rejected/collect_enabled=0，reason 空 |
| 2047413 | 13925664 | 1516 | enabled/approved/collect_enabled=1，reason 空 |

WHERE 同时约束租户、账号、频道、关系 ID、source_id、原状态和原 updated_at；
断言实际影响恰好 2 行才提交。第一步将两行设为 disabled/rejected、
collect_enabled=0、disabled_reason=intro_fee_channel_blacklist。
随后按同一精确范围及 marker 检查，仅将关系 2047413 的 approval_status 恢复为
原先的 approved（断言 1 行），保留 collect_enabled=0/status=disabled，
使黑名单不抹掉该来源原先的审批资格。最终通用关系为 rejected，
来源 1516 的关系为 approved，两者均处于禁用状态。
未修改共享 collect_source 1516，未影响其他频道、账号或租户。
未删除、下架或重新发布历史资料，已经完成的发送不属于此次回滚范围。

不要全局清理 Redis。已有采集授权按来源关系回查数据库；新代码的黑名单解除
应使用现有采集配置版本失效机制，使旧拒绝缓存失效。

## 应用修复边界

黑名单采用账号/实际频道维度的 source_id=0 持久拒绝标记；
只关闭目标频道的实际启用关系，恢复时只恢复黑名单拥有的关系，
不覆盖其他禁用、审批和清理限制。保留独立来源的旧黑名单兼容读取。
用户端和管理端必须共用写入逻辑并透传错误，不能依赖更新 0 行的成功返回。

运行时检查覆盖 BOT 首次发现、绑定缓存命中、协议号关系及已入队事件；
新来源或启用别名不能绕过标记。明确拒绝的结构化事件为
`collect.channel.blacklist_rejected`，明细放日志，不新增高基数指标。

不新增数据库字段或索引。Web 更新接口与列表读模型，Collector 更新运行时入口；
其他启用了同一采集入口的角色按实际配置同步更新。
遵守禁止编译和运行测试的要求；新增和调整的回归测试源码尚未执行。

## 验收与回退

部署后检查禁用列表 blacklisted=true、再次点击提交 0、实际关联来源停止/恢复，
共享来源的其他频道应继续采集；同时验证已有独立禁用/待审批关系不被启用。
不要把事务提交或静态审查当作已部署的前端端到端验收。

线上止血的状态可经新的解除黑名单接口恢复；不得直接批量启用所有频道关系。
若必须人工回退，仅按上述两个关系 ID、租户、账号、频道以及当前 marker
做事务内检查，保留历史状态审计；本轮未执行回退。
