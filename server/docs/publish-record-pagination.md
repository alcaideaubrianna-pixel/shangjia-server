# 资料推送记录分页

`/api/youban_publish/publish/admin/record/list` 和用户发送记录接口沿用
HotGo `PageReq/PageRes`，默认 `page=1&pageSize=10`，支持翻页和调整每页数量。
`pageSize` 是标准分页参数；客户端不要只发送 `perPage`。

查询保留现有租户/账号权限、资料、任务、动作、状态和关键词过滤，按
`created_at DESC, id DESC` 排序。记录展示使用快照，状态及错误从同租户
TG Job 读取。总数与列表使用相同过滤条件，仍隐藏尚未开始的历史全量推送预建记录。

列表不再计算全量推送批次汇总及历史成功排名，兼容字段 `progress*` 保持零值。
业务 Job、发送与重试流程均不修改。

索引脚本为 `addons/youban_publish/resource/sql/manual/20261118_publish_record_profile_page.pgsql.sql`，
上线前显式在事务外执行，并确认两个索引 valid/ready；不在启动或接口中执行 DDL。
回滚索引可在事务外执行 `DROP INDEX CONCURRENTLY` 删除脚本内两个索引。

隔离回归使用 `YOUBAN_LOCAL_PUBLISH_RECORD=1`，仅连接本机 `127.0.0.1:35432`
临时 PostgreSQL，覆盖时间排序、同时间 ID 排序、默认每页十条、三页无重复、
租户/账号/资料隔离、Job 状态及错误、无批次表时正常读取。
