# 采集频道游标本地回测

回测输入为频道范围内已落库的采集消息事实，包含消息顺序、相册 ID、文案、媒体类型和已有配对。导出只读 PostgreSQL，回测从本地文件向 Go 匹配器注入数据。

## 导出一个频道的样本

预先配置只读 libpq service。历史及实时样本各用一个明确消息范围，分别保存。在命令行中使用精确的存储账户和频道 ID。

```sh
python3 tools/collect_cursor_replay.py export \
  --service youban_readonly --tenant 78 --account 615 --source 351 \
  --chat=-1004424159439 --first 7878 --last 7903 --output /tmp/collect-history.jsonl
```

单次导出默认限 1,000 行。范围内行数超过限额时命令报错。输出包含生产文案，应存放在受控目录，脚本以仅当前用户可读写的权限创建文件。

## 导入已有归档

```sh
python3 tools/collect_cursor_replay.py import \
  --events /path/to/business-pending.jsonl \
  --media /path/to/business-media.jsonl \
  --output /tmp/collect-history.jsonl
```

## 本地注入

传入已构建、包含频道游标回测的 Go 测试二进制。脚本运行时不调用 Go 编译器。

```sh
python3 tools/collect_cursor_replay.py run --binary /path/to/publish-replay.test --input /tmp/collect-history.jsonl --batch-size 100
python3 tools/collect_cursor_replay.py run --binary /path/to/publish-replay.test --input /tmp/collect-realtime.jsonl --batch-size 1
```

批大小为 1 可以强制展示和验证跨批。运行器比较分批推进和整段匹配的输出，并报告匹配数量及与已存配对的差异。样本必须至少包含一组可匹配资料以避免空跑。默认要求上下文校验通过。排查历史快照差异时可加 `--report-only`，分批和整段配对结果仍必须一致。该回测覆盖消息事实到配对结果，完整 Webhook、媒体下载、规则分发和资料写入需在本地角色集成环境中验证。
