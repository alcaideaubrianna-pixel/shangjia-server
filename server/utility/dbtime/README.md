# 无时区数据库时间

旧业务表的 `timestamp without time zone` / `datetime` 保存 **Asia/Shanghai 钟面值**。
驱动可能将它表示为 UTC；不能直接比较，也不能用 `.In()` 修复这类值。

读取边界必须复用：

```go
deadline := dbtime.FromTimestamp(row.NextRetryAt) // *gtime.Time
leaseUntil := dbtime.FromTime(row.LeaseUntil)    // time.Time
if deadline != nil && deadline.After(gtime.Now()) {
    // 尚未到期。
}
```

当前时间、Telegram Unix 时间、RFC3339、`timestamptz`、缓存 Unix 时间戳均为绝对时间，
**禁止**调用 `FromTimestamp` / `FromTime`。业务钟点排程或显示使用：

```go
now := time.Now().In(dbtime.Location()) // 保持绝对时间不变。
```

转换保留纳秒、nil 和零值，不修改输入，可重复调用；时区数据内嵌，精简容器无需额外安装。
禁止复制钟面转换、硬减 8 小时、依赖 `time.Local` 或对任意结构体反射转换。
必须先确认字段类型和来源；不能从 Go 的 `Location` 推断数据库字段类型。

当前仅保留统一工具，业务入口尚未迁移。后续每次只迁移一个入口，确认字段来源、补齐行为测试并 review 后再继续。
生产镜像已有 `TZ=Asia/Shanghai`，仍须保留，旧 SQL 写入/查询代码依赖此配置；
本包不全局修改进程时区，也不更改历史字段类型、存储值或通知幂等键。

工具测试需分别在 `TZ=UTC` 和 `TZ=Asia/Shanghai` 下运行：`go test ./utility/dbtime -count=1`。
当前未增加镜像构建门禁，也未启用业务层语法检查。
