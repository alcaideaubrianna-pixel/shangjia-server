# 扫图批量近似检索改造及本地验证

## 实现

Bot 搜索仍先做整组身份与精确匹配；只对精确未命中的指纹批量召回。每批最多8个输入，共享主指纹和别名桶扫描。保留每个输入独立的桶命中数、真实汉明距离和50000条媒体候选上限，再按原有资料去重与排序函数组装。超过8个输入顺序分批，不增加请求内数据库并发；沿用调用方整组5秒预算。

共享候选的有效媒体、资料状态、资料索引检查只执行一次。租户和账户组合在桶扫描阶段过滤，不扩大可见范围。仍保留Go侧距离复核。单图、非PostgreSQL、pgvector启用、LSH未就绪或不支持的阈值继续使用原路径。

批量路径复用既有结果缓存键与版本失效逻辑，整组只解析一次范围版本；只缓存完整成功的批次。相同冷批次通过singleflight合并数据库查询，等待者可按自己的截止时间退出。后续批次失败时保留精确命中、缓存命中和已经完成的批次，并返回错误标记不完整。当前失败批次不能返回数据库未完成的部分结果。

新增日志阶段：`query.phash_batch.plan`、`query.phash_batch`，包含输入数量、待查询数量、批大小、查询策略、耗时和错误。关闭新路径可设置 `YOUBAN_PUBLISH_MEDIA_SEARCH_BATCH_APPROXIMATE_ENABLED=false`；默认启用。没有新增数据库字段或迁移。

## 本地验证

全部数据库写入仅在127.0.0.1:35432临时表，未启动业务消费者、未发Telegram消息、未部署。

1. 批量SQL与逐图SQL逐项比较完整媒体集合和桶命中数，覆盖阈值0/12/14/15、两种查询策略、重复输入、主指纹与别名、跨租户、同租户不同账户、已删除媒体、缺失资料状态或资料索引。
2. 1/10/30输入的服务层分批、缓存复用、排序一致性，以及资料删除后版本失效通过。
3. 批量精确优先、保留输入位置、部分失败保留已完成结果、拒绝不合法返回长度，以及共享查询等待者取消不取消首个调用者等测试通过。
4. 相关定向测试运行 `go test -race` 通过，无竞态报告。不是全仓所有测试通过。

参数上限测试发现初始批量方案可能超过PG65535个绑定参数，已将桶值改为integer数组绑定；503账户、2租户分区、8张图的真实SQL本地执行通过。

### 10万条本地噪声媒体，503账户范围，三轮

使用四组原始输入指纹，每张输入另外构造128条相近指纹，以避免只测空结果；候选数据为本地合成，不是线上候选索引快照。下表为阈值12的SQL生成、执行及结果扫描耗时范围，两种策略合并列出。逐图基线是同事务串行执行，不是线上并发2的总耗时。

| 原输入标识 | 张数 | 批量SQL | 逐图串行SQL | 每组候选媒体行数 |
| --- | ---: | ---: | ---: | ---: |
| ICFBUMHJ | 8 | 121—128ms | 207—282ms | 1024 |
| FPKMXDZB | 7 | 110—119ms | 183—254ms | 896 |
| YOEKMFUD | 6 | 102—119ms | 155—210ms | 768 |
| 7LYKUCBD | 5 | 89—153ms | 131—176ms | 640 |

这三轮完整结果一致，不以该样本声明统计显著收益或线上端到端低于2秒。此前较小权限范围的一轮5张图桶查询曾比逐图串行略慢，不能承诺所有输入都加速。

## 复现

```sh
YOUBAN_SCAN_STORAGE_PG=1 \
YOUBAN_SCAN_BATCH_NOISE_ROWS=100000 \
YOUBAN_SCAN_INCIDENT_FIXTURE="$PWD/.tmp/l19688-replay.json" \
go test ./addons/youban_publish/logic/sys \
  -run '^TestMediaPHashBatchLocalPostgres$' -count=3 -v

YOUBAN_SCAN_STORAGE_PG=1 \
YOUBAN_SCAN_INCIDENT_FIXTURE="$PWD/.tmp/l19688-replay.json" \
go test -race ./addons/youban_publish/logic/sys ./internal/bootstrap/envconfig \
  -run '^Test(BotMedia.*|LoadBotMedia.*|RankBotMedia.*|MediaPHashBatch.*|MediaPHashLsh.*|MediaPHashBucket.*|ScanIncidentIdentityLocalReplay|MediaFileCacheEnvironmentMappings|MediaSearchStorageLocalPostgres)$/(original_media_and_projection|lsh_distance_filter_preserves_threshold_and_scope|bot_batch_chunks_cache_and_ranking)$' \
  -count=1
```

## 限制和发布前检查

- 尚未用生产规模候选索引快照验证首次完整服务请求、真实冷页、并发2旧实现对照和数据库峰值内存。当前不能承诺线上首次稳定2秒。
- 批量候选中间集随范围和指纹分布增大；work_mem为事务局部64MB、每个执行节点的限制，不是整请求64MB硬上限。需通过真实计划检查临时文件及峰值资源。
- 每张图仍有50000候选上限；新路径为同桶命中数增加media_id/hash_value的确定性截断次序。达到上限时，可能与旧版未定义同分次序的集合不同；当前对照数据未触及该上限。
- 相同冷批次合并查询；仅部分重叠或输入顺序不同的相册仍可能重复扫描。尚需真实并发负载验证；首个调用者取消时，共享其查询的调用者会收到错误，不会自动无限重试。
- 扩大回归选择曾遇到现有存储测试失败：防扫图测试未插入有效媒体归属；组合运行时还出现版本数量与别名写入断言失败。未放宽生产保护或把这些测试计为通过。定向新链路竞态测试随后通过。
- 本次没有改变下载、回复、预览或频道发送流程，没有执行新的线上写入。
