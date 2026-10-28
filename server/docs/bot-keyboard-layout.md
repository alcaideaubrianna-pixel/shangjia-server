# 全局 Bot 底部菜单

入口：全局 Bot 管理 → 插件 → 开始使用 → 插件配置。

`start` 是默认启用的内置插件，实现在 `plugins_start.go`，固定命令 `/start`。保留账号授权、资料预览、合作推广深链；其他全局插件不能配置相同命令。双向机器人继续使用网关的独占菜单，不注册第二个默认入口。

配置项 `keyboardUnbound` / `keyboardBound` 分别用于未绑定、已绑定上架账号（BotAppApi）的用户。可视化编辑支持增删行、移动行、同一行按钮左右排序、选择插件和颜色、预览与 JSON 导入。修改后保存插件配置。

```json
{
  "keyboardUnbound": [
    [{"featureKey": "start", "style": "primary"}],
    [{"featureKey": "account_bind", "style": "success"}]
  ],
  "keyboardBound": [
    [{"featureKey": "profile_manage", "style": "primary"}],
    [{"featureKey": "info"}]
  ]
}
```

- `null`：使用系统默认布局，兼容旧配置。
- `[]`：隐藏全部底部按钮，并在用户下一次交互时移除已发送键盘。
- 二维数组：数组顺序即行顺序，每行数组顺序即按钮顺序；最多12行，每行1至4个按钮，插件不可重复。
- 颜色仅支持 Telegram 原生样式：省略/空为默认，`primary` 蓝色、`success` 绿色、`danger` 红色；不支持任意十六进制颜色。实际颜色受客户端主题影响。
- 配置只控制布局，不能开放权限。停用、隐藏或无权限插件在渲染时仍被过滤，空行自动移除。
- 绑定后使用对应布局；绑定成功路径已有键盘刷新。配置修改在用户下一次交互时通过键盘版本检测生效，颜色变更也计入版本。
- JSON 输入后点击“应用 JSON”，再保存；未应用的文本不参与保存。

验证：新增排序/权限过滤/空布局/颜色刷新/保留命令与重复命令回归测试通过；网关菜单所有权测试通过；前端 ESLint、Vite 构建通过。全包 Go 测试仍存在失败，包含测试数据库缺表、双向帮助断言和旧固定分组断言，尚未验证其基线。

## 保存与刷新修复

- 命令清空时按实际生效的默认命令检查冲突，并将默认命令显式保存；分发与校验复用同一解析函数。
- 元数据保存事务先锁住 start 插件行，再从数据库读取当前插件命令校验并写入；事务提交前保持锁，覆盖并发请求和多实例旧缓存。
- 空菜单在文本回复和欢迎图片回复中统一转成 `ReplyKeyboardRemove`。空指针不会被记录成键盘投递成功；发送失败不更新版本。没有传入 markup 的既有流程继续保留原菜单。
- 新增真实本地 PostgreSQL 并发、旧缓存、回滚、双向状态保存与模拟 Telegram 消息/版本回归测试。运行：`YOUBAN_LOCAL_COLLECT_PG=1 go test ./addons/youban_bot/logic/sys -run 'Test(FeatureCommandSaveLocalPostgres|EmptyKeyboardDeliveryLocalPostgres)' -count=1`，仅连接 127.0.0.1:35432，并在独立临时 schema 内测试。
