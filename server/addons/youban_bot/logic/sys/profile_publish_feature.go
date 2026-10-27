package sys

import (
	"context"
	"fmt"

	"hotgo/addons/youban_bot/model/input/sysin"
)

// profilePublishFeature is the dedicated batch intake entry point. It keeps
// the existing profile session implementation as the single source of truth
// for display/verification matching and profile creation.
type profilePublishFeature struct{}

func (profilePublishFeature) Key() string         { return "profile_publish" }
func (profilePublishFeature) Command() string     { return "publish" }
func (profilePublishFeature) Description() string { return "上架资料" }
func (profilePublishFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("上架资料文案", "请依次发送多组展示资料和验证资料，系统会逐条创建并返回上架按钮。")
}

func (profilePublishFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil || featureCtx.Msg.From == nil {
		return true, nil
	}
	_, err := bot.boundProfileAccount(ctx, featureCtx.Msg)
	if err != nil {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), err.Error())
	}
	chatID := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	if err = bot.startProfileSession(ctx, featureCtx.BotId, featureCtx.Msg, "batch_create", "waiting_display", 0, "", nil, ""); err != nil {
		return true, err
	}
	return true, bot.sendProfileCreateStepPrompt(ctx, featureCtx.BotId, chatID, "waiting_display")
}

var _ botFeature = profilePublishFeature{}
