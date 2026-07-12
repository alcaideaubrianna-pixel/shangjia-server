package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishRecordRetentionDays = 7

func (s *sSysPublish) runPublishRecordRetentionCleaner(ctx context.Context) {
	if err := s.cleanupPublishRecordsOlderThan(ctx, publishRecordRetentionDays); err != nil {
		g.Log().Warningf(ctx, "清理发送记录失败：%+v", err)
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.cleanupPublishRecordsOlderThan(ctx, publishRecordRetentionDays); err != nil {
				g.Log().Warningf(ctx, "清理发送记录失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) cleanupPublishRecordsOlderThan(ctx context.Context, days int) error {
	if days <= 0 {
		days = publishRecordRetentionDays
	}
	cutoff := gtime.Now().Add(-time.Duration(days) * 24 * time.Hour)
	_, err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).
		WhereLT("created_at", cutoff).
		Delete()
	if err != nil {
		return gerror.Wrap(err, "删除过期发送记录失败")
	}
	return nil
}
