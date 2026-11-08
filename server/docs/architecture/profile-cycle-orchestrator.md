# 资料级循环调度模型

循环翻新的调度单位改为“资料循环批次”，频道只负责执行顺序。

## 数据关系

```text
profile_cycle_run
  ├─ profile_id
  ├─ due_at / claimed_at / status
  └─ profile_cycle_run_channel
       ├─ channel_id
       ├─ job_id
       └─ status
```

同一资料的一轮循环只创建一个 `profile_cycle_run`。运行器先读取资料当前有效的全部频道配置，再为这些频道生成子任务。子任务共享同一个 `run_id`，因此可以追踪整轮是否完成。

## 调度流程

```text
PostgreSQL profile_cycle_run
  ↓ due_at
Redis ZSET profile-cycle-due
  ↓ 唤醒
Profile Cycle Orchestrator
  ↓ 一次读取一个资料
创建该资料全部频道子任务
  ↓
tg_job + outbox
  ↓
Redis Stream / Asynq
  ↓
频道独立串行发送
```

ZSET 只做到期索引，PostgreSQL 保存最终状态。ZSET 丢失时由恢复扫描重新建立，不作为唯一可靠来源。

## 同步保证

- 资料的所有有效频道在同一个 run 中生成；
- 频道不可用时记录该频道子任务失败，不阻塞其他频道；
- 资料批次只有在所有频道进入终态后才计算下一次 `due_at`；
- 任一频道重试不会重新创建整轮资料批次；
- 发送顺序仍由每个频道自己的 lease 保证；
- 同一资料不同频道允许并行发送，但属于同一个循环批次。

## 迁移方式

1. 保留当前 `tg_job.cycle_enabled/next_cycle_at` 作为兼容恢复来源。
2. 新建资料级 run 表和频道子任务表。
3. 先由旧计划生成资料级 run，验证完整频道覆盖后再关闭旧的逐频道扫描。
4. ZSET 只接收新 run 的 `due_at`，恢复扫描按 PostgreSQL 补建。
5. 稳定后删除旧的逐频道循环领取逻辑。

循环、批次和手动上架共用资料频道解析能力，但批次运行仍保留自己的批次状态和游标。
