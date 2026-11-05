package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/tasklog"
)

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
		WhereGTE("l.created_at", gtime.Now().Add(-time.Duration(tasklog.RetentionDays())*24*time.Hour))
	if in.SourceId > 0 {
		mod = mod.Where("e.source_id", in.SourceId)
	}
	countKey := fmt.Sprintf("youban_publish:collect:event_log:count:%d:%d:%d", account.TenantId, account.Id, in.SourceId)
	if cached, cacheErr := cache.Instance().Get(ctx, countKey); cacheErr == nil && !cached.IsNil() {
		totalCount = cached.Int()
	} else {
		if totalCount, err = mod.Count(); err != nil {
			return nil, 0, gerror.Wrap(err, "统计采集事件日志失败")
		}
		_ = cache.Instance().Set(ctx, countKey, totalCount, 15*time.Second)
	}
	if err = mod.Fields("l.*").Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集事件日志失败")
	}
	return list, totalCount, nil
}
