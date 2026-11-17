package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/botnotice"
)

const collectorSystemNoticeReason = "BOT系统回执，原始消息已保存，无需采集交付"

func collectorSystemNotice(sourceType string, raw []byte) bool {
	return sourceType == sysin.SourceTypeBot && botnotice.PlainUpdate(raw)
}

func logCollectorSystemNotice(ctx context.Context, tenantID, sourceID, eventID int64) {
	g.Log().Info(ctx, "BOT系统回执已在原始事件层收口", g.Map{"event": "collector.event.system_notice_ignored", "tenantId": tenantID, "sourceId": sourceID, "eventId": eventID, "deliveryRequired": false, "rawPersisted": true})
}

func ignoreClaimedSystemNotice(ctx context.Context, row *entity.TgCollectorEvent) error {
	c := dao.TgCollectorEvent.Columns()
	result, err := dao.TgCollectorEvent.Ctx(ctx).WherePri(row.Id).
		Where(c.TenantId, row.TenantId).Where(c.SourceId, row.SourceId).
		Where(c.Status, sysin.EventStatusProcessing).Where(c.LeaseOwner, row.LeaseOwner).Where(c.AttemptCount, row.AttemptCount).
		Data(g.Map{c.Status: sysin.EventStatusIgnored, c.ProcessedAt: gtime.Now(), c.LeaseOwner: "", c.LeaseUntil: nil, c.NextRunAt: nil, c.ErrorMessage: collectorSystemNoticeReason}).Update()
	if err != nil {
		return gerror.Wrap(err, "收口BOT系统回执失败")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return gerror.New("BOT系统回执执行归属已变化，未确认忽略状态")
	}
	logCollectorSystemNotice(ctx, row.TenantId, row.SourceId, row.Id)
	return nil
}
