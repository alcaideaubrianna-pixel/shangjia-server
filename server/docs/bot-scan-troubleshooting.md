# BOT 扫图追踪日志

用户看到的 8 位 `traceId` 是业务追踪号，与 GoFrame 请求 span ID 不同。
跨媒体聚合和发布搜索服务必须传递同一业务号，禁止在中间步骤重新生成。

搜索 `Bot扫图链路` 或完整追踪号，日志为单行 JSON，类型 `bot_scan_stage`。
每个阶段有独立 `stageId`，包含 `start/end`、耗时、实例名、剩余预算，超过 5 秒最多追加一次 `slow`。
有截止时间的阶段结束时记录 `remainingMs`；失败记录脱敏 `error`、`timeout` 和 `contextError`。
诊断代码不修改超时、重试、查询范围或结果。

关键阶段：

| stage | 排查方向 |
|---|---|
| telegram.resolve | Telegram getFile / 媒体地址解析 |
| request.total / search.total | 回复生命周期 / 搜索调用；二者状态不同，错误提示发送成功不表示搜索成功 |
| access.check / scope.accounts / scope.partitions | 权限和搜索范围查询 |
| fingerprint.cache / fingerprint.cold | 指纹缓存 / 冷媒体处理 |
| media.download / media.read | 下载及缓存等待 / 读取本地缓存 |
| image.decode / image.phash | 解码 / 感知哈希计算 |
| fingerprint.search / query.cache_get / query.cache_set | 每个指纹搜索 / 结果缓存 |
| query.md5 / query.phash_exact / query.phash_approx | MD5 / 精确感知哈希 / 相似匹配 |
| profiles.query / profiles.uuid / profiles.owners / profiles.tags | 资料查询和字段补充 |
| profiles.sources / profiles.covers | 来源关联 / 媒体封面加载 |
| result.cache / reply.results / reply.empty / reply.error | 结果缓存 / 向 Telegram 回复 |

先按 `traceId` 筛选，再按 `stageId` 对应开始结束记录。最后一个没有结束的阶段是优先调查对象，
但进程重启、日志缺失也会造成这种情况，需核对实例名、运行版本和重启时间。
`search.total` 的错误用来区分超时与正常无匹配；不可用“没有结果”解释超时。

## SAE 采集要求

代码日志必须同时接入运行扫图的应用 stdout 采集。不能只搜索 Web 的 SLS 项目。
检查 Web、Background 和实际承接 BOT 更新的角色是否启用 stdout 日志采集，保留至少 3 天。
上线后发送一次扫图请求，用回复中的追踪号验证集中日志确实出现阶段记录；
验收前不能仅凭代码存在日志就声称线上可查询。
本次排查发现 Background 没有配置 SLS；需在平台补齐采集并验证，代码修改不能恢复未采集的历史日志。
