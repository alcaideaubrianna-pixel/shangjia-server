# 上架账户资料详情只读权限

先执行同目录 `profile_detail_read_permission_20261020.pgsql.sql`，再更新 web/API 角色。新增列默认 0，不授予历史账户权限；旧版程序忽略新增列，新版 capability 查询依赖该列，不能先部署代码。

复用账户设置接口授予 `profileDetailViewEnabled=1`。个人设置强制保留数据库中的权限值，无法自行授权。资料详情、列表和指定资料的发布/收录记录扩大至同租户；写入权限和原文件脱敏规则不变。无 profileId 的发布记录仍只读本人。

普通端新增 GET `/publish/note/filterOptions?accountId=N`，返回 accounts、sourceChannels、pushChannels。来源频道读取 compact 账户频道关系、历史 source 配置和 Bot 频道缓存，不扫描采集事件；已彻底清理配置和频道缓存的历史来源不会凭空恢复选项。推送频道筛选 `pushChannelId` 表示资料配置目的地，手动映射优先，否则使用租户默认频道，非历史发送目的地。

本地验证：

```sh
YOUBAN_SCAN_STORAGE_PG=1 go test -race ./addons/youban_publish/logic/sys -run 'TestProfile(DetailRead|PushChannelFilter|WriteAccountScope|AdminOwnerGroups)|TestSharedProfilePermission' -count=1
go build ./...
```

权限 PG 测试覆盖同租户授权、跨租户拒绝、账户筛选收窄、指定资料记录授权、无资料 ID 不扩大权限、即时撤销、手动频道优先于默认及软删映射回退。只读权限不会授予 CanEdit。

性能边界：本地 PostgreSQL 10 万条紧凑频道关系合成数据执行 EXPLAIN (ANALYZE, BUFFERS)，按 tenant/account 查询走关系联合索引及频道主键，返回 10 条 Execution Time 0.032 ms（Planning 0.264 ms）。这是核心 SQL 局部证据，不代表线上完整 HTTP 耗时，未写生产数据。

回滚：回滚代码即可，保留新增列，避免影响混合版本；默认关闭无需数据回滚。
