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

## 历史验证视频错绑修复

必须先生成只读报告：

```sh
/tmp/collect-repair \
  -misbinding-audit \
  -source-id 351 \
  -misbinding-since '2026-10-14 00:00:00' \
  -misbinding-until '2026-10-16 00:00:00' \
  -limit 500 \
  -report /tmp/verification-misbinding-audit.json
```

报告将候选分为 `safe`、`manual`、`skip`。只有同时满足以下条件的 `safe` 项才允许自动处理：

- 目标展示事件及目标资料都只有一个验证候选；
- 验证视频全部缓存完成；
- 目标资料尚无验证视频；
- 旧资料有另一条独立验证视频，且当前没有使用候选视频；
- 目标资料存在历史成功发布频道。

确认报告后执行修复、统一下架和原频道重新上架：

```sh
/tmp/collect-repair \
  -misbinding-apply \
  -misbinding-republish \
  -source-id 351 \
  -misbinding-since '2026-10-14 00:00:00' \
  -misbinding-until '2026-10-16 00:00:00' \
  -limit 500 \
  -cleanup-wait 30m \
  -report /tmp/verification-misbinding-apply.json
```

首次执行应通过 `-event-ids <验证事件ID>` 选择一个 `safe` 候选验证完整闭环，再扩大处理范围。

每条重绑在独立事务中完成并写入事件审计日志。工具先通过统一清理队列下架全部历史消息，确认该资料不再存在 `sent` 消息后，才按历史成功发布频道重新上架。重新上架使用稳定操作号 `verify-misbinding-repair:<event>:profile:<profile>`，中断后重复运行不会创建重复任务。清理超时的资料保持下架并记录错误，不会抢先重新发布。
