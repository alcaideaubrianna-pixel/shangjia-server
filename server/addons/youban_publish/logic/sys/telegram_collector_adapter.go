package sys

import (
	"context"
	"encoding/json"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

func (h *publishCollectorDeliveryHandler) HandleCollectorDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if h == nil || h.publish == nil || delivery == nil {
		return gerror.New("Telegram采集交付处理参数无效")
	}
	if delivery.SourceType != collectorin.SourceTypeBot {
		return gerror.Newf("暂不支持的Telegram采集来源类型：%s", delivery.SourceType)
	}
	var update models.Update
	if err := json.Unmarshal(delivery.RawUpdate, &update); err != nil {
		return gerror.Wrap(err, "解析Telegram采集交付原始消息失败")
	}
	message, _ := telegramUpdateMessage(&update)
	if message == nil {
		return nil
	}
	return h.publish.collectBotMessage(ctx, delivery.SourceID, message)
}
