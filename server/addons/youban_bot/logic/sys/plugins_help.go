package sys

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// Main Bot help is independent of the optional relay plugin.
type helpFeature struct{}

func (helpFeature) Key() string         { return "main_help" }
func (helpFeature) Command() string     { return "help" }
func (helpFeature) Description() string { return "主机器人帮助" }
func (helpFeature) Handle(ctx context.Context, bot *sSysBot, fc *botFeatureContext) (bool, error) {
	lines := []string{"<b>主机器人 · 使用帮助</b>", "选择下方菜单，或复制命令使用："}
	for _, feature := range botFeatures {
		if feature == nil || feature.Key() == (helpFeature{}).Key() || !bot.featureMenuVisible(ctx, feature.Key()) {
			continue
		}
		if !bot.featureVisibleForTelegramUser(ctx, feature, telegramUserIdFromCtx(ctx)) {
			continue
		}
		command := bot.featureCommand(ctx, feature)
		if command == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("<code>/%s</code> — %s", html.EscapeString(command), html.EscapeString(bot.featureDescription(ctx, feature))))
	}
	lines = append(lines, "", "双向机器人管理：<code>/relayadmin</code> 或输入「双向机器人管理」。", "双向聊天的管理命令请在对应双向机器人中发送 <code>/help</code>，仅配置的管理员可使用。")
	return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), strings.Join(lines, "\n"))
}
