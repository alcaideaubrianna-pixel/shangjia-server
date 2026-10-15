# 挂起采集事件恢复

通过 SG SSH 隧道连接线上 DB/Redis，使用现有 TG 账号下载任务、去重入库及推送流程。本地不启动 Telegram 账号 session 或常规采集 worker。

编译：`go build -o /tmp/collect-repair ./tools/collect_repair`。
认证信息通过受保护的环境注入，禁止写入报告或命令行。

```sh
/tmp/collect-repair -apply -all-suspended -limit 100000 -concurrency 8 \
  -report /tmp/collect-repair-all.json
```

`-all-suspended` 覆盖所有租户、账号和日期的 `suspended_realtime_relief`、`suspended_verify_repair`。不传此参数时仍使用 `-tenant-id`、`-account-id`、`-date` 的限定范围。

媒体下载与完整 ffmpeg 解码最多并行 8 组，可通过 `-concurrency` 调整为 1–32。按租户、账号、来源、原频道、原消息 ID 排序，依次解除完整组的挂起并重放。等待前一组所有派发记录进入 sent/skipped/reviewing 后才提交下一组；reviewing 保持现有审核流程，不代表已推送。推送失败或超时停止后续提交。媒体准备失败时，阻止同一来源后续组越过该组。

配对边界包括租户、资料归属账号、TG 账号、来源和原频道。沿用现有分类与验证窗口，验证组必须有可完整解码的视频，两组媒体数量和原事件记录一致、全部缓存 ready 才解除。无法匹配的事件保留挂起，列入报告 unresolved。消息顺序关联和媒体可读性不构成人物身份核验。

报告包括候选、媒体哈希、完整性及未匹配事件 ID，不包含媒体地址和原文。运行中断或失败可能已解除某组挂起；重新启动前必须核查该组派发状态，禁止直接重复提交。

`-audit` 为限定范围的只读扫描和解码；`-audit -download` 补缓存但不解除挂起。`-event-ids ... -dry-run=false` 为已绑定展示组的重放入口。
