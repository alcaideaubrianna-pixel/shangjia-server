# 统一频道推送引擎

## 目标

发送和删除使用独立的 Asynq 队列；同一频道通过现有频道调度和 lease 保证串行，不同频道互不阻塞。循环和批次只负责生成标准发送任务，不直接调用 Telegram。

## 灰度

`hg_youban_publish_account_setting.push_engine` 控制账户路径：

- `legacy`：现有优先级分片队列，默认值；
- `unified`：统一发送队列 `youban_publish_push_send`；删除按来源进入
  `youban_publish_push_delete_manual`、`youban_publish_push_delete_keyword`、
  `youban_publish_push_delete_cycle`。旧的 `youban_publish_push_delete` 仍注册用于兼容历史任务。

账户开关读取后缓存 5 分钟，保存设置会主动清理缓存；缓存或新字段不可用时回退 `legacy`。

字段不存在、读取失败或值非法时必须回退 `legacy`。

## 任务路径

```text
业务入口 → tg_job/outbox → Asynq → Channel Dispatcher → Telegram
```

统一发送 lane 由持久化 outbox 按频道队首投递，频道 lease 保证同一频道串行；发送优先级仍由 outbox 的有效优先级决定：手动上架、实时采集、AI 修复、循环翻新、批次上架。删除任务独立于发送，并按手动上架删除、关键字删除、循环翻新删除处理。

## 迁移步骤

1. 执行 `channel_dispatch_engine.pgsql.sql` 或对应在线升级脚本。
2. 保持所有账户为 `legacy`，部署 worker，确认新队列已注册。
3. 为一个测试账户设置 `push_engine=unified`。
4. 验证发送、删除、循环、批次、重试、超时恢复和频道顺序。
5. 按账户逐步放量；异常时将账户改回 `legacy`。
6. 所有账户稳定运行后，再删除旧调度代码。

## 约束

- 不在业务层直接操作底层 Redis；
- 不为每个频道创建 goroutine 或进程；
- 不一次性生成整个循环队列；
- 已发送任务迁移时不得重复发送；
- 删除失败不回滚成功发送，进入独立重试链路。
