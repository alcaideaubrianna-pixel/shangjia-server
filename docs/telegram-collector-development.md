# Telegram Collector 插件开发与上线手册

## 当前实现状态（2026-08-11）

第一阶段已完成并保持生产默认关闭：

- 已创建 `addons/telegram_collector` HotGo 插件、安装 SQL 和 GoFrame 生成 DAO；
- 已复用 `youban_tg_bot_gateway`，未新增第二套 Bot Webhook；
- Bot Update 先幂等写入 Event，再按 urgent/realtime/history 权重进入 Collector Worker；
- Event 和 Delivery 均使用 DB 条件领取、租约、有限重试、超时恢复和死信状态；
- Delivery 由 Publish Worker 独立消费，通过注册式 Adapter 复用 `youban_publish` 现有采集业务；
- 媒体指纹使用 Redis 加速、PostgreSQL 兜底，相同指纹复用对象存储路径、预览图和 PHash；
- Account Runtime 已接入插件统一租约，并复用 HotGo `hgrds/lock` 看门狗；
- 新增 Collector、Media、Publish Worker 运行角色和 Railway 服务定义；
- 已接入 OTel Event/Delivery 数量和耗时指标；
- 已补 PostgreSQL 集成测试，覆盖 Event/Delivery 幂等、Delivery 消费和媒体租约复用。

生产开关仍为：

```text
YOUBAN_TELEGRAM_COLLECTOR_ENABLED=false
```

因此本次部署只增加代码、表结构和独立服务能力，不会自动切换现有采集流量。完成插件安装 SQL、Shadow 观测和积压核对后，再按租户逐步开启。

## 1. 开发范围

本手册用于实现 `addons/telegram_collector`，并将现有 `addons/youban_publish` 中的 Telegram 采集能力逐步迁移出来。

实现必须遵守：

- `server/AGENTS.md` 和仓库根目录 `AGENTS.md`；
- HotGo 插件分层；
- API → Controller → Service → Logic → DAO；
- GoFrame CLI 生成 entity、DO、DAO；
- 缓存统一使用 `cache.Instance()`；
- 队列复用项目现有 Asynq/Redis 基础设施和运行角色；
- HTTP 入口不执行耗时采集和媒体处理；
- 不新增第二套 Bot Webhook；
- 不让普通 Worker 创建协议号 Telegram Client。

## 2. 代码复用清单

### 2.1 必须复用

| 现有能力 | 当前位置 | 新插件用途 |
|---|---|---|
| Bot Webhook Gateway | `addons/youban_tg_bot_gateway` | 注册 Collector Feature，接收 Bot Update |
| Bot 绑定与生命周期 | `addons/youban_tg_bot_gateway` | Bot Token 管理、刷新和发送适配 |
| 协议号运行时 | `addons/youban_publish/logic/sys/account_collect_runtime.go` | 复制后改为 Account Runtime 专属实现 |
| 账号租约/并发槽 | `addons/youban_publish/logic/sys/collect_media_account_lease.go` | 迁移并集中到插件 |
| 实时事件处理 | `addons/youban_publish/logic/sys/collect_ingest.go` | 复制、拆成事件入库和 Collector Worker |
| 历史采集 | `addons/youban_publish/logic/sys/collect_history_*.go` | 迁移为 Account Task + History Worker |
| 媒体队列 | `addons/youban_publish/logic/sys/collect_media_queue.go` | 迁移并拆 Bot/Account/Process 队列 |
| 媒体缓存 | `addons/youban_publish/logic/sys/collect_media_cache.go` | 改造成指纹缓存 + DB 兜底 |
| 对象存储 | `addons/youban_publish/logic/sys/collect_media_storage.go` | 迁移为插件媒体存储 Service |
| 观测指标 | `addons/youban_publish/logic/sys/asynq_observe_metrics.go` | 复用 Telemetry Provider 和指标模式 |
| 全局 OTel | `internal/global/telemetry.go` | 直接使用，不重复初始化 Provider |
| HotGo Queue | `internal/library/queue` | 直接注册插件消费者 |
| HotGo Cache | `internal/library/cache` | 直接使用 `cache.Instance()` |

### 2.2 禁止复制后继续双维护

以下代码迁移稳定后必须从 `youban_publish` 删除：

- 旧采集事件消费者；
- 旧历史展开和处理循环；
- 旧媒体下载消费者；
- 旧账号监听 Goroutine；
- 旧 Bot 采集 Webhook 分支；
- 旧采集任务恢复器；
- 旧采集媒体缓存实现；
- 旧采集相关的重复状态枚举和队列 Topic。

## 3. 插件目录和文件责任

```text
server/addons/telegram_collector/
├── main.go
├── global/
│   ├── global.go
│   └── init.go
├── consts/
│   ├── cache.go
│   ├── queue.go
│   └── status.go
├── api/
│   ├── admin/source/source.go
│   ├── admin/history/history.go
│   ├── admin/account/account.go
│   └── api/diagnose/diagnose.go
├── controller/
│   ├── admin/sys/source.go
│   ├── admin/sys/history.go
│   └── api/diagnose.go
├── service/
│   └── sys.go
├── logic/
│   ├── logic.go
│   ├── sys/source.go
│   ├── sys/event.go
│   ├── sys/media.go
│   ├── sys/history.go
│   ├── sys/delivery.go
│   ├── runtime/account_owner.go
│   ├── runtime/account_client.go
│   ├── worker/event_worker.go
│   ├── worker/media_worker.go
│   ├── worker/delivery_worker.go
│   └── worker/recovery_worker.go
├── queues/
│   ├── event_realtime.go
│   ├── event_urgent.go
│   ├── event_history.go
│   ├── media_bot.go
│   ├── media_account.go
│   ├── media_process.go
│   ├── delivery_ready.go
│   └── recovery.go
├── crons/
│   ├── history_expand.go
│   ├── stale_task_recovery.go
│   ├── dead_letter_recovery.go
│   └── retention.go
├── model/input/sysin/
│   ├── source.go
│   ├── event.go
│   ├── media.go
│   ├── history.go
│   └── delivery.go
├── internal/model/entity/
├── internal/model/do/
├── internal/dao/internal/
└── internal/dao/
```

### 3.1 代码生成顺序

1. 编写安装/升级 SQL；
2. 使用 GoFrame CLI 生成 entity；
3. 生成 DO；
4. 生成 DAO 和 DAO internal；
5. 编写 `sysin`、service、logic、controller、API；
6. 添加插件路由；
7. 添加队列和 Cron 注册。

禁止手写或修改生成目录：

```text
internal/model/entity/
internal/model/do/
internal/dao/internal/
```

## 4. 数据库实施

### 4.1 第一阶段表

先实现以下最小闭环表：

```text
hg_tg_collector_source
hg_tg_collector_event
hg_tg_collector_event_media
hg_tg_collector_media
hg_tg_collector_history_task
hg_tg_collector_account_task
hg_tg_collector_delivery
hg_tg_collector_dead_letter
hg_tg_collector_runtime_instance
```

账号和 Bot 资料如果已有稳定主表，第一阶段通过关联 ID 复用，不重复创建账号凭证表。

### 4.2 必要索引

```text
source: (tenant_id, status, source_type, id)
event unique: (tenant_id, source_id, chat_id, message_id)
event task: (status, priority, next_run_at, lease_until, id)
event chat fairness: (source_id, chat_id, status, next_run_at, id)
media fingerprint unique: (tenant_id, fingerprint, pipeline_version)
media task: (status, priority, next_run_at, lease_until, id)
history due: (status, next_run_at, lease_until, id)
account task: (account_id, status, next_run_at, lease_until, id)
delivery task: (status, priority, next_run_at, lease_until, id)
dead letter: (task_type, source_id, created_at)
```

MySQL 线上环境重点补充：

```text
(tenant_id, source_id, status, priority, next_run_at, id)
(tenant_id, channel_id, status, id)
```

具体索引上线前必须用线上表规模和 `EXPLAIN` 验证，不能只凭字段数量添加索引。

### 4.3 迁移原则

- DDL 只通过显式安装/升级 Job 执行；
- 不在 API 请求中执行 `ensureColumns` 或建索引；
- 不让每个 Railway 副本启动时并发执行 DDL；
- 大表索引采用在线或低峰迁移；
- 迁移后执行只读校验和索引命中校验。

## 5. 队列消费者实现

每个 Consumer 必须：

1. 使用 typed model 解码消息；
2. 通过 DB 条件更新领取任务；
3. 写入 `lease_owner`、`lease_until`、`attempt_count`；
4. 处理成功后先更新 DB 状态，再确认队列消息；
5. 处理失败分类；
6. 可重试错误写入 `next_run_at`；
7. 达到次数进入 dead letter；
8. 日志带 `trace_id`、`task_id` 和 `attempt`。

不允许使用无 ACK 的 `LPOP` 模式承载采集、媒体和发布任务。

### 5.1 Collector Worker

处理步骤：

```text
claim event
    ↓
检查 source 是否仍有效
    ↓
过滤本租户发布频道，避免回流
    ↓
解析文本和 Forward 元数据
    ↓
聚合 media group
    ↓
创建/复用 media task
    ↓
等待 media ready
    ↓
创建 delivery
```

不要在 Collector Worker 内执行：

- 文件下载；
- FFmpeg；
- PHash；
- TG 发送。

### 5.2 Media Worker

处理步骤：

```text
claim media task
    ↓
fingerprint cache lookup
    ├── ready：写入引用并完成
    ├── processing：等待原任务
    └── miss：获取处理锁
              ↓
          下载媒体
              ↓
          处理/封面/PHash
              ↓
          上传对象存储
              ↓
          写 DB + Redis ready
```

Bot 下载直接执行；协议号下载必须投递到对应 Account Runtime。

### 5.3 Publish Worker

发布任务必须独立于资料创建接口：

```text
资料创建 HTTP 请求
    ↓ 快速写入笔记和 publish_intent
立即返回
    ↓
Publish Worker 异步发送
```

发布任务按频道领取头任务，不在频道锁内执行 Telegram 网络调用。

## 6. 用户测试优先级

### 6.1 触发条件

以下操作进入 urgent：

- 用户手动测试采集；
- 用户手动快速推送；
- 新资料单频道上架；
- 用户手动重试；
- 采集事件实时到达。

### 6.2 防止历史任务霸占

每个 Worker 实例配置独立并发池：

```text
urgent_concurrency = 6
normal_concurrency = 12
history_concurrency = 2
```

历史任务不能占满所有并发槽。若 urgent 为空，空闲槽位才可借给普通队列。

### 6.3 公平调度

- 每轮每个 source/chat 最多展开一页；
- 使用 `updated_at + id` 或持久化游标；
- 频道候选按最老任务和更新时间公平轮询；
- 单个频道不得一次性把几千个 Job 全部塞入 Redis；
- 只激活每个频道当前头任务，其余保持 DB pending；
- 新 urgent Job 主动失效候选缓存并尝试立即调度。

## 7. Railway 部署实施

### 7.1 服务和启动命令

```text
API：             /app/hotgo web
Collector Worker：/app/hotgo worker（第一阶段组件开关）
Media Worker：    /app/hotgo media-worker（第二阶段独立角色）
Publish Worker：  /app/hotgo publish-worker（第二阶段独立角色）
Account：         /app/hotgo account
Scheduler：       /app/hotgo scheduler
```

所有服务：

- 使用同一不可变镜像 SHA 标签；
- 共享 PostgreSQL、Redis、对象存储和配置；
- 使用 `YOUBAN_RUNTIME_ROLES` 或专用组件变量控制启动角色；
- 不使用 `latest`、`main` 等可变生产标签；
- 提供 `/healthz` 和 `/readyz`；
- 处理 SIGTERM，停止领取任务并释放租约。

### 7.2 推荐初始副本

```text
API：1～2
Collector Worker：2
Media Worker：2
Publish Worker：2
Account：1～2（账号租约分配）
Scheduler：1
```

不建议一开始盲目增加 Account 副本。Account 副本数量应根据账号数量、连接稳定性和租约压测结果调整。

### 7.3 扩容指标

| 服务 | 主要指标 |
|---|---|
| Collector Worker | realtime oldest age、event queue depth、处理耗时 |
| Media Worker | media queue depth、cache hit ratio、CPU、下载耗时 |
| Publish Worker | publish oldest age、成功率、SlowMode、发送耗时 |
| Account | lease owner、重连次数、账号任务积压 |
| Scheduler | 每轮耗时、恢复数量、cursor 推进时间 |

## 8. 测试和压测

### 8.1 单元测试

- 媒体指纹计算和缓存命中；
- 相同 MD5 并发只生成一个任务；
- Redis miss 后 DB 回填；
- 事件唯一键；
- 媒体组聚合；
- retry/backoff/dead-letter；
- 频道和账号隔离；
- 账号 lease epoch；
- urgent 优先级；
- 历史高低水位背压。

### 8.2 集成测试

至少覆盖：

1. 两个 Worker 同时领取同一个事件；
2. 两个 Account Runtime 竞争同一个账号；
3. Redis 重启后媒体通过 DB 恢复；
4. 对象存储上传成功但 Worker 进程被终止；
5. Webhook 重复投递；
6. Telegram FloodWait；
7. 一个频道卡死时其他频道继续发送；
8. 历史采集期间实时消息优先；
9. Worker 重启后 pending/processing 任务恢复；
10. 发布失败达到上限进入死信。

### 8.3 压测场景

#### 场景 A：实时消息

```text
10 个来源
100 个并发聊天
每秒 20 条消息
媒体组和无媒体消息混合
```

验收：实时队列最老任务不超过 60 秒。

#### 场景 B：历史采集

```text
100 个来源
每个来源 10 万条历史消息
实时消息持续进入
```

验收：实时消息不被历史任务阻塞，历史 cursor 持续推进。

#### 场景 C：媒体重复

```text
50 个来源重复发送相同媒体
10 个 Worker 并发处理
```

验收：相同指纹只发生一次上传和一次 PHash。

#### 场景 D：发布积压

```text
100 个频道
每频道 1000 条资料
其中 5 个频道触发 SlowMode
```

验收：SlowMode 只影响对应频道；其他频道继续发送；urgent 任务保持优先。

## 9. 上线顺序

### 第一步：基础设施和插件骨架

- 创建插件入口、global、router、service；
- 创建表和索引；
- 创建队列 Topic 和状态枚举；
- 接入统一 OTel；
- 不切生产流量。

### 第二步：事件接收

- 接入统一 Bot Gateway；
- 迁移协议号实时事件；
- 写入原始事件；
- 开启影子模式；
- 校验重复率和事件延迟。

### 第三步：媒体链路

- 迁移 Bot 下载；
- 迁移协议号下载 RPC；
- 接入 Redis 指纹缓存；
- 接入对象存储复用；
- 验证 PHash 和视频预览图复用。

### 第四步：Delivery 和 Youban 适配

- 让 `youban_publish` 消费标准 Delivery；
- 保留原有笔记和频道规则；
- 通过 source owner 控制 legacy/v2；
- 单个测试源切换。

### 第五步：Publish Worker

- 从资料创建接口中剥离发送；
- 创建 urgent/normal publish intent；
- 实现频道独立锁和头任务调度；
- 清理旧卡死 Job；
- 开启小范围灰度。

### 第六步：Railway 独立服务

- Collector Worker 独立扩容；
- Media Worker 独立扩容；
- Publish Worker 独立扩容；
- Account Runtime 开启租约压测；
- Scheduler 保持单副本。

### 第七步：清理旧链路

- 停止 legacy source 的新任务领取；
- 迁移或取消旧 pending/failed_retry；
- 验证无旧消费者；
- 删除 `youban_publish` 重复采集代码；
- 删除重复 Topic、恢复器和本地缓存逻辑。

## 10. 发布前检查清单

- [ ] 数据库迁移已显式执行并完成索引校验
- [ ] Redis 使用统一 `cache.Instance()` 且生产适配器为 Redis
- [ ] Bot Webhook 仍只有统一 Gateway 入口
- [ ] 普通 Worker 不会创建协议号 Client
- [ ] 每个账号存在唯一租约
- [ ] 媒体缓存命中时不会重复下载、上传或 PHash
- [ ] 所有媒体路径指向对象存储
- [ ] urgent 队列有保留并发
- [ ] 历史队列有高低水位背压
- [ ] Publish Worker 与 Collector Worker 分离
- [ ] 单频道卡死不会影响其他频道
- [ ] retry 有上限，dead letter 可查询和重试
- [ ] processing 任务有 lease_until 和恢复器
- [ ] OpenTelemetry Trace 能串起 event/media/delivery/publish
- [ ] OpenObserve 有队列、租约、媒体和发送看板
- [ ] SIGTERM 会停止领取任务并释放账号租约
- [ ] Worker/Account/Publish 至少完成双副本重启测试
- [ ] 旧采集链路尚未删除前已具备按 source 回滚能力

## 11. 最终验收标准

系统达到以下条件后，才允许删除旧采集链路：

1. 用户测试采集在有容量时 5 秒内进入 Collector Worker。
2. 已缓存媒体不会再次下载、上传或计算 PHash。
3. 新资料媒体 ready 后立即创建高优先级发布意图。
4. 正常情况下用户测试推送不等待历史采集批次。
5. 一个频道异常不会阻塞其他频道。
6. 一个账号断线不会导致其他账号任务失败。
7. Worker 重启后任务可以自动恢复。
8. Account Runtime 重启后账号可以通过租约自动接管。
9. 任意失败任务可以通过 `event_id/media_id/delivery_id/publish_job_id` 在 OpenObserve 完整追踪。
10. 历史采集有明确进度，超过重试上限进入死信，不再无限循环。
