# 查看资料详情权限保存失效

## 线上修复

2026-10-20，通过 DBX 查询确认 `hg_youban_publish_account_setting.profile_detail_view_enabled`
已存在（smallint、NOT NULL、默认 0），无需再次运行完整插件更新。

用户明确指定 `@baihuage_dianzhu`：账户 616，租户 78，上架账户。
使用 tenant_id + account_id + username + account_type + deleted_at 条件更新，只将
`profile_detail_view_enabled` 从 0 改为 1，实际更新 1 行，并回查确认。
其他权限保持原值。刷新账户页面读取新权限；来源频道标签需要发布前端代码。

## 复现与代码修复

本地独立 PostgreSQL（127.0.0.1:35432）创建临时设置表，先调用 TableFields 缓存旧结构，
再 ALTER TABLE 新增权限列。原代码保存 1 后回读为 0，回归测试失败。
GoFrame 默认过滤未知字段，字段缓存不自动过期，导致权限字段被静默过滤。

保存设置前清理并重新读取本表字段缓存，缺少迁移时明确报错，保存后校验权限。
前端阻止设置未加载时保存，忽略旧账户加载响应，并校验返回权限，不再误报成功。

验证命令（仅本地临时表，不连接线上）：

```sh
YOUBAN_SCAN_STORAGE_PG=1 go test ./addons/youban_publish/logic/sys \
  -run 'TestAccountSettingSaveAfterOnlineMigration|TestProfileDetailRead|TestProfileFilterOptions|TestProfilePushChannelFilter' -count=1
```

覆盖在线迁移后的旧缓存、开关反复保存、首次插入、缺失迁移明确报错、跨租户隔离、只读权限不提权。
本修复发布后只需 Web 角色及前端，无需重启 Account、Publish、Media 或 Collector。

## 插件更新边界

完整插件更新会在同一事务运行多张表 DDL，失败信息不能仅靠文件名判断。
先查 SQLSTATE 和具体语句；本权限字段已经补齐，不要为启用一个账户重复执行整个升级文件。
本次没有重新运行线上完整迁移，也没有将本地修复代码部署到线上。

如需撤销此次授权，先核对该账户当前权限和后续管理员修改，再只撤销账户 616、租户 78 的此项权限。

## 来源频道重复选项

线上租户 78 存在同一个 chat_id 对应两条 Bot-all 来源规则的记录。旧接口以 source_id + chat_id 去重，
所以同频道仍会出现多个选项。现改为规范化 chat_id 去重（兼容 -100 前缀），不同 ID 的同名频道保留。
频道选项返回 SourceId=0，筛选按 chat_id 汇总多个规则下的资料，不只取第一条规则。
原有指定 SourceId 的规则筛选仍保留；资料查询与 note_index 查询复用同一筛选方法，账户/租户范围不变。

本地 PostgreSQL 测试覆盖重复规则、历史规则、Bot 缓存、两种 chat_id 形式、同名不同频道、
跨规则查询、单规则兼容、无结果、同资料多条派发不重复、跨租户隔离。
