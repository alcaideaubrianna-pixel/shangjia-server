package sys

import (
	"context"
	"encoding/json"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/model/entity"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysBot) relayDeliveryNotice(ctx context.Context, row *entity.YoubanBotRelayDelivery, status string) {
	if row.Kind != consts.RelayKindChat || status != consts.RelayFailed {
		return
	}
	var msg models.Message
	if err := json.Unmarshal([]byte(row.RawJson), &msg); err != nil {
		return
	}
	// Report a terminal unsupported/deleted message or recipient error once, after
	// storing its terminal state. A failure to notify must never resend the chat.
	if err := s.relayReply(ctx, row.BotId, &msg, "此消息未能传递，对方可能无法接收，或消息类型不支持。请联系管理员查看失败记录。"); err != nil {
		g.Log().Warningf(ctx, "双向消息发送失败提示未送达 botId:%d deliveryId:%d err:%+v", row.BotId, row.Id, err)
	}
}
