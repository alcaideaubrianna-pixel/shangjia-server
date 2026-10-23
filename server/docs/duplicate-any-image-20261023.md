# 批量资料查重：任意展示图片相似

算法版本 8。保留 scan/batch 返回结构、筛选和账号范围、后台分页/断点机制。

- 只比较未删除的展示图片（image/photo；purpose 为 display 或历史空值）。不比较文案或视频。
- 任意图片 MD5 相同，或者有效 64 位 pHash 汉明距离不超过 12，即命中。图片数量、顺序可以不同；其他图片缺指纹不阻止已有指纹命中。
- 按图片建立 MD5/LSH 索引，分批读取候选资料后做最终距离校验。每个结果候选与该组 keep 直接匹配，不通过相似链推断新的匹配。
- keep 是较新的对比资料，是否删除任何资料由前端用户决定。scan/batch 不下架、不删除、不发送 Telegram 消息。
- 老的 duplicate/cleanup 只接受原先完整签名校验；相似候选不能作为自动删除依据，确认后走独立资料删除接口。
- 不兼容版本 7 扫描结果；会话校验会重新创建扫描任务。

## 用户样本

固定测试数据：`addons/youban_publish/logic/sys/testdata/duplicate_kkbaoyang_20261023.json`。

| 链接 | 样本来源 | 展示图片数 |
| --- | --- | --- |
| https://t.me/KKbaoyang01/557940 | 生产资料 410504 的展示图片 MD5/pHash 只读快照 | 5 |
| https://t.me/KKbaoyang01/552975 | 生产资料 670334 的展示图片 MD5/pHash 只读快照 | 1 |
| https://t.me/KKbaoyang01/3130 | 公开 embed 图片，goimagehash.PerceptionHash；未找到对应发布记录，不声称已定位生产资料 ID | 6 |

两两最小 pHash 距离分别为 0、6、6。前两组有一张图片 MD5 相同；旧整组签名规则三对均不相等。
测试只固化指纹，不依赖实时 Telegram 或生产数据库，也不提交原始图片。

## 验证

普通测试包含真实样本匹配、阈值边界、缺指纹/非法指纹、Redis 跨页和重试幂等、非传递相似链。
可选本地 PostgreSQL 回归运行真实扫描页读取和结果生成，校验 verify-only 图片被排除，扫描后原资料和媒体行数保持不变。

```sh
YOUBAN_LOCAL_DUPLICATE_PG=1 go test -race ./addons/youban_publish/logic/sys -run '^TestDuplicate|^TestValidateDuplicate' -count=1 -timeout=90s
```

本地 PG 固定为 127.0.0.1:35432 postgres，使用当前系统用户名；创建并销毁隔离 schema。
Redis 使用测试创建的 miniredis；不执行生产删除或生产扫描任务。
