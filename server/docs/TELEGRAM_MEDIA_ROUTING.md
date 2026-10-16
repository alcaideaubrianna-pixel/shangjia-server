# Telegram 媒体发送与下载

## 入口

- `Gateway.SendClient`：发布、验证视频、监听通知和消息模板发送。跟随 `YOUBAN_TELEGRAM_RUNTIME_API_URL`；为空时走官方 API。HTTP 请求期限 5 分钟，外层任务取消仍然有效。
- `Gateway.MediaClient`：仅用于 `getFile` 与下载。继续读取 `YOUBAN_TELEGRAM_BOT_API_URL`（为空时读取后台 `botApiServerUrl`）。下载客户端期限保留 10 分钟，现有逐次 getFile 超时和重试保持不变。
- Bot 扫图/资料导入保留现有下载入口；协议号下载、COS 缓存、图片处理和防扫图流程保持不变。

`BOT_API_URL` 不再控制发布发送。需要回退到自建发送时，必须连同 Bot runtime 的归属一起安排切换，不能在某次发送超时后临时切换入口重发。

## 发送选择

1. 优先复用当前 Bot 的有效 `file_id`，禁止跨 Bot 复用。
2. 未开启防扫图且 URL 可用：图片不超过 5,000,000 字节、视频不超过 20,000,000 字节优先 CDN URL。
3. 已知超过 URL 限制：复用现有下载缓存和处理流程，改用 multipart。上传前检查实际处理后文件，图片不超过 10,000,000 字节、视频不超过 50,000,000 字节。视频仍调用 `sendVideo`，不转 `sendDocument`。
4. 历史记录大小未知时保留 URL 优先；只有 Telegram 明确拒绝 URL 才回退 multipart，不增加热路径 HEAD/数据库查询。处理过的资产大小不沿用原图大小。
5. 超时、EOF、取消等发送结果不明的错误继续交由现有 Attempt 状态机处理，不自动切换端点或再次上传。API 明确成功继续使用返回的消息记录确认。

以上采用保守的十进制大小阈值。项目约定最大文件 50MB；超过阈值需要压缩，不能通过 `sendDocument` 绕过视频展示要求。

## 上线检查

- 本提交只修改代码，不执行 `logOut`、`close`、webhook 更换或线上配置变更。
- 确认所有角色的 runtime URL 一致、Bot webhook 正常。runtime URL 为空/官方时，新发送请求会从原自建媒体入口迁至官方。
- 同一个 Bot 的官方/自建会话归属需要先验证。不要因下载超过 20MB 就自动对生产 Bot 执行 `logOut`；大文件协议号下载复用现有能力。保留自建下载的 Bot 应先在测试 Bot 上验证完整收发，必要时独立划分采集 Bot 与发布 Bot。
- 先灰度一个测试 Bot，验证小视频 URL、20～50MB multipart、file_id、防扫图、媒体组和大文件下载，再更新 Publish、Background、Account/Web 等实际发送角色。
- 观察日志中的 `operation:send/download`、`usesRuntimeEndpoint`、关联 jobId、API 耗时和 Attempt 结果；不得记录 Token 或签名媒体 URL。
