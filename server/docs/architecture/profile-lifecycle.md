# 资料生命周期与异步任务契约

资料的 HTTP/Bot 请求只负责校验、写入主体数据和创建持久化任务，不在请求线程执行 Telegram、OSS 或媒体计算。

## 操作类型

```text
create / edit / publish / republish / unpublish / delete / unpublish_delete
```

同一资料使用 Redis 资料级锁：

```text
youban_publish:profile_operation:lock:{tenant_id}:{profile_id}
```

操作锁只防止重复点击，最终幂等依赖数据库 operation_no 和频道 job 唯一约束。

## 媒体处理

Bot 新建或编辑资料后写入 `hg_youban_publish_profile_media_operation`，然后投递：

```text
youban_publish:profile:media_process
```

任务由 `background` 角色消费，处理顺序为：

1. 按 Telegram file_unique_id/file_id 复用。
2. 按 asset hash/MD5 复用。
3. OSS HeadObject。
4. 未命中时下载 Telegram 并上传 OSS。
5. 写媒体关系。
6. 后台计算图片 pHash 和视频封面。

媒体完成后调用 `BotProfilePersistMedia`；资料处于 `waiting_media` 时自动继续已有上架流程。

## 发布与清理

发布、重新发布、下架和删除均拆成频道级 Telegram job。唯一键为：

```text
profile_id + operation_no + channel_id
```

Telegram 发送和删除必须由 `publish`/`cleanup` 队列完成。单个频道失败只影响该频道，不能阻塞其他频道。

删除资料时先保存 profile_no/title 快照并加入 Telegram 清理队列，再完成资料删除；清理完成后通过 `account_notice` 发送一次终态通知。

## 通知

完成通知统一写入 `hg_youban_publish_account_notice`，使用 operation_no 派生 event_key。通知发送、重试和 Telegram/API 故障与业务任务解耦。

## SAE 角色

| 角色 | 职责 |
| --- | --- |
| web | 接收 Bot/Web 请求，快速返回 |
| background | 媒体任务、资料恢复、后台任务 |
| media | 采集媒体和媒体计算 |
| publish | Telegram 发布、下架和删除 |
| scheduler | 超时任务恢复和周期调度 |

部署必须保证 `background` 和 `publish` 与 `web` 使用同一版本迁移；只部署 web 不会消费持久化媒体和 Telegram job。

## 恢复规则

background 启动时以及每分钟扫描一次媒体操作：

```text
pending
processing 且 updated_at 超过 2 分钟
```

恢复任务重新投递原 operation_id，不创建新的业务操作，避免重启后丢任务或重复发布。
