# 双向机器人租户账户配置迁移

全局 `two_way_relay` 插件只控制总开关，业务配置不再保存在 feature.config_json。
配置归属为 `(tenant_id, account_id)`，每个租户账户一份；`source_bot_id` 指向上架端用户在后台创建的 Bot。
内部 `bot_id` 是 Gateway/用户目录适配记录，不能作为认领他人 Bot 的依据。

## 升级顺序

1. 先升级 `youban_publish` 插件，为后台 Bot 增加 `account_id` 和 `bot_type`。
2. 再升级 `youban_bot` 插件，为双向配置增加 `tenant_id/account_id/source_bot_id` 及唯一索引。
3. 部署对应代码并刷新 Gateway。主 Bot 输入 `/relayadmin`，绑定自己的后台 Bot。

新建后台 Bot 的账户归属由已登录的上架端账号写入，不接受 Telegram 用户传入账户ID。
只有租户、所属账户、创建者同时匹配，bot_type=two_way，且账号和 Bot 均启用的 Bot 才能选择。
用户后台 Managed Bot 创建接口默认用途为 two_way；普通 Token Bot 保存接口默认用途为 normal，可显式传 botType。
系统后台代建、全局 Bot 和历史 `account_id=0` 的 Bot 不会自动认领。
没有自己的 Bot 时，主 Bot 提示先在后台创建；没有 Telegram 创建/输入 Token 入口。

## 历史数据

旧的无归属双向配置保留全部绑定、规则、审计记录，但升级时将 `enabled` 置为 0。
不要用旧 `owner_id` 自动推断上架端账户：旧值是后台管理员ID，与上架端账号不在同一身份空间。
旧后台 Bot 的 `created_by` 也可能属于系统管理员，不能仅凭数字相同自动认领。

需人工确认并记录：真实租户、真实上架端账户、用户自己创建的后台 Bot、原双向配置。
迁移前检查：

```sql
SELECT id, tenant_id, account_id, created_by, bot_type, bot_username, status
FROM hg_youban_publish_bot
WHERE deleted_at IS NULL;

SELECT id, bot_id, owner_id, tenant_id, account_id, source_bot_id, enabled
FROM hg_youban_bot_relay_config;
```

确认归属后，在事务中为已核实的后台 Bot 设置 account_id 和 bot_type=two_way，并为原双向配置设置
相同的 tenant_id/account_id/source_bot_id，owner_id 改为上架端账户ID。迁移时保持 enabled=0。
必须确认原适配 Bot Token 与后台源 Bot Token 相同，且适配 Bot 不是默认主机器人。
如果历史 created_by 属于系统后台代建，该 Bot 不能冒充用户自己创建，需用户在后台重新创建。
唯一索引会拒绝同一账户多份配置或同一后台 Bot 绑定多份配置，不要删除索引绕过冲突。
核对用户与管理员权限后，通过主 Bot 的管理面板启用。

## 回归检查

- A 租户、B 租户、同租户不同账户互不可见、不可配置。
- 猜测 Bot ID、旧按钮、旧全局表单均不能跨账户写入。
- 后台 Bot 被停用、删除、转移归属或更换 Token 后，双向投递停止。
- 主机器人 `/help` 保持主帮助，双向 Bot `/help` 仅管理员可用。
- 绑定用户首次有效私聊消息通知配置管理员，同一绑定生命周期内去重。

菜单 SQL 已将双向权限拆成独立隐藏权限项，兼容原 permissions varchar(512)，
不再需要依赖扩容；推送记录仅保留隐藏权限项，不产生二级目录入口。
