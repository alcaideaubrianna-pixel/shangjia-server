# 协议号频道访问变化通知

2026-10-24，本地实现，尚未提交、构建或部署。本次没有发送真实测试通知，没有改动线上采集开关。

## 行为

- Account通过Self取得实际Telegram用户ID，以atomic保存，识别自身UpdateChannelParticipant及UpdateChannel随附的完整频道状态。
- 自身被移除、禁止查看消息、离开、ChannelForbidden分别形成原因。普通成员变化、改名、只禁止发言、缺失成员状态、min频道对象不触发。
- 匹配当前账号所属租户、协议号、频道的已启用account采集源；事件早于配置最后修改时间则忽略。使用原通知队列保存快照，稳定TaskID按来源和配置版本去重。
- Background消费后行锁核对归属及精确版本，只暂停该来源collect_enabled，不删除资料，不停用整个TG账号，不改备注。暂停版本随任务保存，失败重试可继续通知；用户手动编辑/恢复后的配置不被旧任务覆盖。
- 向已配置的采集超级管理员发送通知，并通过官方Bot/绑定Bot向来源所属用户通知。两路分别尝试，失败返回队列重试；沿用管理员收件人去重，所属用户送达后7天缓存去重。普通用户通知开关不拦截本告警。
- 文案含TG号名称及@用户名、TG账号编号、频道、频道ID、来源名称/编号、原因及恢复方法。ChannelForbidden不会声称确定被踢。
- 新事件识别失败/通知入队失败记录警告，但不阻断同容器内其他新消息。只做有界DB/Redis入队，不在更新回调执行Bot发送。

## 本地验证

固定本机临时PostgreSQL 127.0.0.1:35432，使用事务临时表，无生产连接、真实Telegram发送或消费者启动：

```sh
YOUBAN_LOCAL_COLLECT_PG=1 go test ./addons/telegram_collector/... ./addons/youban_publish/logic/sys \
  -run 'TestAccountChannelAccess|TestAccountRuntime|TestCollectChannelPurge|TestCollectSourceSuccessTotalsLocalPostgres|TestCollectRuleLifecycleLocalPostgres' -count=1
go test -race ./addons/telegram_collector/logic/runtime -run 'TestAccountChannelAccess|TestAccountRuntime' -count=1
```

均通过。覆盖自身与其他成员、离开/封禁/禁言/改名/partial entity、同批消息不被失败通知抑制、重复状态合并、真实SQL来源匹配、时间版本、租户/归属/协议号/频道隔离、通知失败重试和所属用户去重、保留备注与其他来源、手动重新启用后迟到任务不覆盖。管理员收件人解析/发送由原NotifyCollectEvent实现，测试使用替身而非真实Bot网络。

## 发布及限制

需部署Account（事件接收）和Background（通知/暂停消费者），先Background后Account；Account保持单实例。无DB字段迁移，不需仅为本功能部署Web/Publish/Media。

本次只补用户要求的收到自身变化后的即时通知路径；不包括更新缺口Manager、周期成员资格巡检或历史缺失消息回填。已有退群没有新事件不会凭空告警；更新回调发生在Self完成前、入队DB/Redis失败、部署/断线期间漏更新仍有漏告警可能，不能宣称覆盖所有历史情况。

管理员需已配置采集通知接收人并建立Bot会话；普通成员被移除时Telegram未必提供操作者，通知不虚构踢人者。队列为至少一次发送，Telegram响应超时或发送成功后写去重缓存失败仍可能重复提醒，不承诺恰好一次。
