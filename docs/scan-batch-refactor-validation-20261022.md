# 扫图批量查询重构与验证（2026-10-22）

## 本次实现

- Bot 收到相册成员时只记录媒体标识、合适尺寸的图片/视频封面 ID，不逐张进行权限展开和 getFile。
- 相册收齐后，缓存命中直接复用；其余成员只做一次批量发送记录预查询。未命中成员解析预览 URL，并发最多 4、单项最多 5 秒、解析阶段总预算 12 秒。视频仅请求封面，不下载视频正文。
- Publish 对整组先执行最多三条批量精确查询：MD5、主 pHash、防扫图别名。保留 MD5 优先、租户/账户权限、资料和状态软删除过滤。
- 精确阶段全部完成后才查询剩余成员的近似候选，不重复执行已完成的精确阶段；候选阶段仍保留既有整组预算和并发 2。
- 近似查询失败时保留已确认匹配，并显式标记部分完成。未完成不能显示为“没有匹配”。
- 扫图预览准备和展示/验证发送共用继承请求上下文的 35 秒预算。相册引用被明确拒绝且不能完整转换为 URL 时，直接准备完整二进制相册，不再重试夹杂已拒绝引用的相册。
- 保留网络结果未知时不重发、相册不拆散的规则。删除被替代的旧逐条解析函数，并将测试改为验证实际新入口。
- 新批量查询不执行 DDL。没有新增数据库字段，不需要本次新增字段迁移；既有媒体指纹、别名表及索引需已完成部署。

## 已执行验证

```sh
YOUBAN_SCAN_INCIDENT_FIXTURE="$PWD/.tmp/l19688-replay.json" \
go test -race ./addons/youban_publish/logic/sys ./addons/youban_bot/logic/sys ./utility/scantrace \
  -run 'Test(BotMedia|LoadBotMedia|Scan|EnsureScan|NormalizeScan|ContextIsolation|SensitiveError|FailureLog|ProfilePreview|CachedScan)' -count=1
```

上述相关测试通过，包含真实输入回放、超时保留其他成员命中、部分失败传播、图片/封面元数据、整组预览降级、未知发送结果不重试、取消时不启动下载。

固定本地 PostgreSQL `127.0.0.1:35432` 上，实际批量 SQL 验证通过：MD5 优先、主 pHash、别名去重、跨租户隔离、已删除资料/状态和缺失状态排除。1/10/30 输入在竞态检测下约 4–13 ms；数据为小型受控测试表，不代表生产规模性能。

真实 L19688 输入回放为 40 批、156 条；22 条可直接匹配发送记录。一次普通回放最慢标识查询约 7.2 ms，仅测量标识阶段；其余 134 条仍需要指纹链路。

## 全量检查限制

两个业务包的全量测试未全部通过。在独立 HEAD `af7dbf84` 工作区复现了相同失败：防扫图并发默认值、重复图片探测数量、消息分类、资料编号解析、relay 帮助审计按钮等。Publish 基线使用相同本地配置目录，避免缺配置导致误判。

`go vet` 还报告未改动的 `tg_message_repair.go:564` unreachable code。不能宣称全仓检查通过。

额外启用 `TestMediaSearchStorageLocalPostgres` 时，单独运行仍有 `generation_failure_falls_back_to_source_upload` 失败；未在本次修改采集/发布存储逻辑，也未将该集成测试计为通过。

## 未验证与上线边界

- 未对线上执行数据更新、索引操作、部署或真实 Telegram 发送。
- 尚未证明生产规模、冷缓存、503 账户权限范围下，所有近似查询均能低于 3 秒。已有约 2.9 秒磁盘读取的真实案例仍需专项验证；本地精确查询快不能代替该验证。
- 未切换 pgvector，未修改频道发布任务顺序、Attempt、Account 单实例逻辑。
- 未新增 Bot 归属媒体映射迁移；旧媒体引用仍依赖明确拒绝后的安全降级，不宣称已补齐全部跨 Bot 缓存治理。
- 新旧代码滚动共存时，旧入口可能产生不含 PreviewFileId 的待处理相册；保留其已有 FileUrl/指纹缓存路径。旧实例读取新 metadata-only 相册可能无法补取 URL，扫图入口应按角色协调切换，并观察最长 2 分钟相册 TTL；不影响频道发布队列。

构建部署前需确认实际承载 Bot 扫图事件消费的角色。此次不能仅部署 Publish/Account 就认为扫图入口已更新。

## 后续如何凭日志定位

统一结构化日志标识为 `log_type=bot_scan_stage`，通过用户看到的八位 `traceId` 搜索日志正文；不能直接用八位业务码查询存放框架 requestId 的数据库日志列。

同一请求还记录框架 `requestId`、Bot/账户信息、实例名。阶段开始、结束使用同一个 `stageId`，记录 `durationMs`、剩余预算和超时/取消状态。超过五秒的阶段有单次 slow 提示；只有 start 没有 end 时，应结合实例停止/重启日志判断是否进程被终止。

建议先看以下 summary，再追具体 start/end：

| stage | 排查信息 |
|---|---|
| `telegram.identity_batch.summary` | 输入数、实际预查询数、缓存/标识可复用数 |
| `telegram.resolve_batch.summary` | 成功解析数、失败数、解析耗时、是否部分失败 |
| `query.exact_batch.summary` | 去重指纹数、精确命中成员数、剩余近似查询成员数 |
| `query.approx_batch.summary` | 近似查询输入数、命中资料数、是否未完成及原因 |
| `search.summary` | 权限账户/分区数、标识命中数、指纹缓存命中数、冷处理数与失败数、返回数、总耗时、complete/partial/failed |

数据库详情加载单独记录 `result.hydrate`；结果列表回复记录 `reply.results`。用户点击预览后的链路为 `result.view`、`result.card`、`result.media.*`。搜索 complete 但预览失败不能归因为数据库搜索超时。

summary 及阶段错误统一记录可读、脱敏的错误文本，不把 Go error 对象直接序列化成 `{}`；不会记录 Token、签名 URL 或图片正文。没有新增 DB 日志表。
