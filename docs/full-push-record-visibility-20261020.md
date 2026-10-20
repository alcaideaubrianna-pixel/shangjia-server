# 全量推送记录提前刷屏修复

## 问题与约定

按“每实际执行一条资料推送，创建或更新该任务的一条记录”处理；全量任务准备、入队阶段不提前展示整批待发送记录。正在发送、发送成功、失败及重试信息保留，同一 job 重试不新增重复记录。本文不将一次全量批次压缩成一条最终汇总。

代码审查确认：enqueuePreparedProfileJobs 对每个任务调用 pending 记录写入；旧修复 e1f2b446 只在一批展开完成或某个任务进入 sending 后删除多余活动记录。准备阶段到事后删除之间，列表会读到整批 pending 行；普通与关键词列表均没有执行可见性过滤。没有证据表明是本次 SQL 优化提交改回了这段行为。

## 修复

- 全量推送 pending 调用在统一记录写入入口直接跳过，不影响数据库任务、outbox、队列或发送状态机。
- sending、成功、失败等执行路径继续按唯一 job_id 写入/更新记录；重试回到 pending 时保留已存在的执行记录。
- 移除“写入后再批量删除其他任务记录”的逻辑，避免删除其他并发发送或重试任务的执行记录。
- 普通列表、关键词列表和计数统一过滤历史遗留的 full_push pending 行；已进入执行/结果状态的历史行继续可见。兼容 action 仍为 publish、但 operation_no 使用 full_push 前缀的旧记录。
- 过滤发生在计数和分页之前，不在前端过滤，不删历史数据，不需要 SQL 迁移。

## 验证

专用本地 PostgreSQL 127.0.0.1:55484，无 Telegram 外部发送或生产数据修改。

```sh
YOUBAN_SQL_REVIEW_LOCAL_PG=1 go test ./addons/youban_publish/logic/sys -run 'Test(FullPush|PublishRecord|PublishJobRecord|BoundPublishSuccessRecordMessage|MessagePushOperation|NormalizePublishRecordPage)' -count=1
```

通过：50 个 pending 全量任务不写入记录；sending→failed_retry→sending→success 始终一条；其他正在执行任务不被删除；pending 重试保留已存在执行记录；失败可见；旧 pending 行隐藏；旧成功行及普通非全量 pending 行保留；普通/关键词计数、分页及租户/账号条件保持一致。git diff --check 通过。

尚未做生产 HTTP 或实际 Telegram 发送端到端验收；源码合并、镜像构建和部署必须分别核对，不能将本地测试当作线上已修复。
