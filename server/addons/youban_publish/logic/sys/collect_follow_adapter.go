package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
)

func (s *sSysPublish) collectFollowProfilePublished(ctx context.Context, task gdb.Record) error {
	if !s.collectGlobalEnabled(ctx) {
		return nil
	}
	if task.IsEmpty() || task["profile_id"].Int64() <= 0 {
		return nil
	}
	authorTenantId := task["tenant_id"].Int64()
	authorAccountId := task["account_id"].Int64()
	profileId := task["profile_id"].Int64()
	if authorTenantId <= 0 || authorAccountId <= 0 || profileId <= 0 {
		return nil
	}
	profile, err := s.collectFollowProfile(ctx, authorTenantId, authorAccountId, profileId)
	if err != nil {
		return err
	}
	if profile.IsEmpty() {
		return nil
	}
	sources, err := s.collectFollowSources(ctx, authorAccountId)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	media, err := s.collectFollowProfileMedia(ctx, authorTenantId, authorAccountId, profileId)
	if err != nil {
		return err
	}
	for _, source := range sources {
		message := &CollectMessage{
			TenantId:        source["tenant_id"].Int64(),
			AccountId:       source["account_id"].Int64(),
			SourceId:        source["id"].Int64(),
			SourceType:      sysin.CollectSourceTypeFollow,
			SourceChatId:    fmt.Sprintf("account:%d", authorAccountId),
			SourceMessageId: profileId,
			SourceUniqueKey: collectFollowProfileUniqueKey(source["id"].Int64(), authorAccountId, profile),
			RawText:         collectFollowProfileText(profile),
			Media:           media,
			ReceivedAt:      gtime.Now(),
		}
		eventId, err := s.ingestCollectMessage(ctx, message)
		if err != nil {
			g.Log().Warningf(ctx, "保存关注采集事件失败 source:%d profile:%d err:%+v", source["id"].Int64(), profileId, err)
			continue
		}
		if err = s.processCollectEvent(ctx, eventId, source["tenant_id"].Int64(), source["account_id"].Int64()); err != nil {
			g.Log().Warningf(ctx, "处理关注采集事件失败 event:%d profile:%d err:%+v", eventId, profileId, err)
		}
	}
	return nil
}

func (s *sSysPublish) collectFollowSources(ctx context.Context, authorAccountId int64) ([]gdb.Record, error) {
	sourceDao := pdao.YoubanPublishCollectSource
	followDao := pdao.YoubanPublishAccountFollow
	sourceCols := sourceDao.Columns()
	followCols := followDao.Columns()
	rows, err := sourceDao.DB().Model(sourceDao.Table()+" s").Safe().Ctx(ctx).
		InnerJoin(followDao.Table()+" f", "f."+followCols.TenantId+"=s."+sourceCols.TenantId+
			" AND f."+followCols.FollowerAccountId+"=s."+sourceCols.AccountId+
			" AND f."+followCols.FollowingAccountId+"=s."+sourceCols.FollowAccountId+
			" AND f."+followCols.Status+"=? AND f."+followCols.DeletedAt+" IS NULL", sysin.AccountFollowStatusApproved).
		Where("s."+sourceCols.SourceType, sysin.CollectSourceTypeFollow).
		Where("s."+sourceCols.FollowAccountId, authorAccountId).
		Where("s."+sourceCols.CollectEnabled, 1).
		Where("s."+sourceCols.Status, 1).
		WhereNull("s." + sourceCols.DeletedAt).
		Fields("s.*").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取关注采集源失败")
	}
	return rows, nil
}

func (s *sSysPublish) collectFollowProfile(ctx context.Context, tenantId int64, accountId int64, profileId int64) (gdb.Record, error) {
	columns := dao.ContentProfile.Columns()
	row, err := dao.ContentProfile.Ctx(ctx).
		Where(columns.Id, profileId).
		Where(columns.Status, 1).
		Where(columns.Visibility, consts.ContentVisibilityPublic).
		WhereNull(columns.DeletedAt).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取关注资料失败")
	}
	if row.IsEmpty() {
		return row, nil
	}
	taskCount, err := pdao.YoubanPublishTask.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "检查关注资料归属失败")
	}
	if taskCount == 0 {
		return gdb.Record{}, nil
	}
	return row, nil
}

func (s *sSysPublish) collectFollowProfileMedia(ctx context.Context, tenantId int64, accountId int64, profileId int64) ([]collectMediaItem, error) {
	var rows []gdb.Record
	err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		Where("purpose", "display").
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取关注资料媒体失败")
	}
	items := make([]collectMediaItem, 0, len(rows))
	for _, row := range rows {
		mediaType := "photo"
		if row["media_type"].String() == "video" {
			mediaType = "video"
		}
		items = append(items, collectMediaItem{
			Type:        mediaType,
			FileId:      row["tg_file_id"].String(),
			FileUrl:     row["file_url"].String(),
			StoragePath: row["storage_path"].String(),
			PosterUrl:   row["poster_url"].String(),
		})
	}
	return items, nil
}

func collectFollowProfileText(profile gdb.Record) string {
	parts := make([]string, 0, 3)
	if title := strings.TrimSpace(profile["title"].String()); title != "" {
		parts = append(parts, title)
	}
	if text := strings.TrimSpace(profile["plain_text"].String()); text != "" {
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func collectFollowProfileUniqueKey(sourceId int64, authorAccountId int64, profile gdb.Record) string {
	version := profile["updated_at"].String()
	if updatedAt := profile["updated_at"].GTime(); updatedAt != nil {
		version = updatedAt.Format("U")
	}
	return fmt.Sprintf("follow:source:%d:author:%d:profile:%d:%s", sourceId, authorAccountId, profile["id"].Int64(), version)
}
