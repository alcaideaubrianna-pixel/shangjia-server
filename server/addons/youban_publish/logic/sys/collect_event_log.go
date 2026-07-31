package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectEventLogRetentionDays = 3

func (s *sSysPublish) CollectEventLogList(ctx context.Context, in *sysin.CollectEventLogListInp) (list []*sysin.CollectEventLogModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectEventLogListInp{}
	}
	logTable := pdao.YoubanPublishCollectEventLog.Table()
	eventTable := pdao.YoubanPublishCollectEvent.Table()
	mod := pdao.YoubanPublishCollectEventLog.DB().Model(logTable+" l").Safe().Ctx(ctx).
		LeftJoin(eventTable+" e", "e.id=l.event_id").
		Where("l.tenant_id", account.TenantId).
		Where("l.account_id", account.Id).
		WhereGTE("l.created_at", gtime.Now().Add(-collectEventLogRetentionDays*24*time.Hour))
	if in.SourceId > 0 {
		mod = mod.Where("e.source_id", in.SourceId)
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集事件日志失败")
	}
	if err = mod.Fields("l.*").Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集事件日志失败")
	}
	return list, totalCount, nil
}
