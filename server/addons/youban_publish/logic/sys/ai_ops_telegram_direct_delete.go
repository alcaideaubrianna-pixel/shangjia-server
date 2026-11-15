package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AIOpsTelegramDirectDelete(ctx context.Context, in *sysin.AIOpsTelegramDirectDeleteInp) (*sysin.AIOpsTelegramDirectDeleteResult, error) {
	if in == nil || len(in.Messages) == 0 || len(in.Messages) > 100 {
		return nil, gerror.New("单次只能处理1到100条TG消息")
	}
	result := &sysin.AIOpsTelegramDirectDeleteResult{Total: len(in.Messages), Items: make([]*sysin.AIOpsTelegramDirectDeleteItem, 0, len(in.Messages))}
	for _, message := range in.Messages {
		target := fmt.Sprintf("%s/%d", strings.TrimSpace(message.ChatId), message.MessageId)
		item := &sysin.AIOpsTelegramDirectDeleteItem{Target: target}
		if strings.TrimSpace(message.ChatId) == "" || message.MessageId <= 0 {
			item.Status, item.Message = "failed", "Chat ID和消息ID不能为空"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		var channel struct {
			Id        int64  `json:"id"`
			TenantId  int64  `json:"tenant_id"`
			BotIdJson string `json:"bot_id_json"`
		}
		chatID := normalizeTelegramChannelChatID(message.ChatId)
		err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Fields("id,tenant_id,bot_id_json").
			Where("(target_chat_id = ? OR target_chat_id = ?)", chatID, strings.TrimSpace(message.ChatId)).
			Where("status", 1).WhereNull("deleted_at").Scan(&channel)
		if err != nil {
			item.Status, item.Message = "failed", gerror.Wrap(err, "读取频道配置失败").Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if channel.Id <= 0 {
			item.Status, item.Message = "failed", "未找到启用的目标频道"
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		botID, botErr := telegramChannelSenderBotId(telegramJobChannel{BotIdJson: channel.BotIdJson})
		if botErr != nil {
			item.Status, item.Message = "failed", botErr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		token, tokenErr := s.telegramCleanupJobBotToken(ctx, botID, channel.TenantId)
		if tokenErr != nil {
			item.Status, item.Message = "failed", tokenErr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		if deleteErr := s.deleteMatchedTelegramMessageByChat(ctx, token, chatID, int(message.MessageId)); deleteErr != nil && !isTelegramMessageAlreadyDeletedError(deleteErr) {
			item.Status, item.Message = "failed", deleteErr.Error()
			result.Failed++
			result.Items = append(result.Items, item)
			continue
		}
		item.Status, item.Message = "deleted", "TG消息已直接删除"
		result.Deleted++
		result.Items = append(result.Items, item)
	}
	return result, nil
}
