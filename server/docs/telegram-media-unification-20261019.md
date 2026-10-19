# Telegram 媒体下载统一（2026-10-19）

## 问题

PF-930 展示图片解析使用了统一配置的 MediaClient，普通图片也进入自建 Bot API 的本地文件服务；公网文件下载连接被重置，资料未保存，因此没有上架按钮。原实现分别存在于 Bot、采集和客服中，路由与重试策略不一致。

## 实现

- `internal/library/telegrammedia`：统一文件类型/大小分流、getFile 重试、文件地址转换、HTTP 下载、大小限制、临时文件清理、错误脱敏。仅返回地址、字节或本地文件，不负责租户权限和 COS 入库。
- Gateway `ResolveMedia` / `DownloadMedia`：统一提供官方/自建客户端和文件服务配置；普通媒体与大视频分别限流，自建 getFile 连续失败触发熔断。
- Bot 资料创建、快捷推送、扫图预览、自定义 Emoji、采集和客服附件均接入；采集缓存、MD5/pHash、存储复用和 COS 保存仍由业务层负责。
- 同组 Bot 转存复用 HTTP 客户端和连接池。
- 删除 Bot 私有下载器、getFile 重试器、官方/本地地址转换器，以及采集私有的 getFile 回退与重试实现。HTTP 缓存下载底层也调用公共传输实现，已有缓存重试调度使用单次传输，避免重试叠加。

## 路由规则

| 媒体 | 路由 |
| --- | --- |
| 图片、视频预览图、小视频 | 官方 Bot API |
| 视频大小超过 20 MiB | 自建 Bot API |
| 大小未知的视频 | 先官方；仅明确 `file is too big` 或返回大小超过限制时转自建 |
| 普通网络错误、超时 | 原路有界重试，不因超时转自建 |
| 超过限制的非视频文件 | 返回明确错误，不转自建 |

父级 deadline 始终优先。官方 getFile 单次最多 3 秒、最多 3 次；自建大视频准备单次最多 2 分钟。HTTP 重试共用 2 分钟预算，响应超限、空文件、永久 HTTP 错误会失败。内存下载有大小上限，流式下载通过私有临时文件原子替换目标文件。

Gateway 限流为单进程配额：官方全局 8、自建全局 4；每个 Bot 在两个端点各 2 个，避免慢视频阻塞官方图片请求。自建准备连续临时失败 3 次后熔断 5 分钟，普通官方图片仍可解析。

## 验证及边界

- 公共层与 Gateway：真实 SDK + 本地 HTTP 服务测试，覆盖图片、小视频、20 MiB 边界、大视频、未知大小、缩略图独立路由、超时、重试、错误脱敏及断流后的临时文件清理。
- Gateway 集成：5 张展示照片 + 大验证视频 + 封面，校验官方/自建请求数、下载字节及 JPEG 解码；自建熔断不影响官方图片，慢视频配额不阻塞图片配额。
- `go build ./...` 通过。
- 全量相关模块回归有 6 个既存失败，已在修改前 `92c637b2` 隔离工作树复现：`TestRelayAuditControlsStayOutOfDedicatedHelp`、`TestRelayHelpAndSettingsValidation`、`TestNormalizeAntiScanMattingConcurrency`、`TestDuplicatePHashScanBucketsBoundLookupCost`、`TestClassifyProfileMessage`、`TestParseProfilePublishMark`。新实现未修改这些业务。
- 公共下载、Gateway、Bot 资料/扫图、采集缓存及客服相关用例的 `go test -race` 通过。扩大到整包 race 时，还触发了既存 `message_push_plan_test.go` 改写 `time.Local` 与 GoFrame 文件监视协程之间的数据竞争；该测试不属于下载改动，未宣称整包 race 全绿。
- 全仓 `go test ./... -run '^$'` 另受既存 queue 格式化 vet 错误与 hook 测试数据库配置缺失影响，不等同于全仓测试通过。

## PF-930 真实只读下载验证

从生产数据库只读提取 PF-930 的 5 张展示照片和验证视频 file_id，使用本地新公共能力访问官方 Telegram API 下载。Token 仅保存在权限 0600 的临时输入中，不进入日志，验证后删除输入。

| 输入 | 实际路由 | 获取地址及下载耗时 |
| --- | --- | --- |
| 展示图片 75412 | official | 2377 ms |
| 展示图片 75413 | official | 1299 ms |
| 展示图片 75414 | official | 1431 ms |
| 展示图片 75415 | official | 1248 ms |
| 展示图片 75416 | official | 1240 ms |
| 验证视频 75417（2,493,532 字节） | official | 6081 ms |
| 视频预览图 | official | 1092 ms |

7 个下载全部成功，照片/预览图可解码；验证视频本身没有超过官方限制，不应进入自建服务。测试从本地经代理发起，未上传 COS、未创建或修改资料、未发送 Telegram 消息。可通过 `YOUBAN_TELEGRAM_MEDIA_TEST_FIXTURE` 与可选 `YOUBAN_TELEGRAM_MEDIA_TEST_PROXY` 显式启用公共库中的只读真实下载测试；默认跳过。

这些结果不代表已经完成 SAE/COS 全链路性能验证。尚未部署、未更换泄露的 Token、未恢复或重发 PF-930。
