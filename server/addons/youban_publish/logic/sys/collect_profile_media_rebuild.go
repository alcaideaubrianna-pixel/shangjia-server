package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type CollectProfileMediaRebuildOptions struct {
	ProfileIDs []int64
	Limit      int
	DryRun     bool
}

// RebuildCollectProfileMedia reopens completed collection events whose source
// media is complete but the derived profile media is incomplete.
func RebuildCollectProfileMedia(ctx context.Context, options CollectProfileMediaRebuildOptions) (*sysin.CollectProfileMediaRebuildResult, error) {
	if options.Limit <= 0 || options.Limit > 1000 {
		options.Limit = 100
	}
	rows, err := collectProfileMediaRebuildCandidates(ctx, options)
	if err != nil {
		return nil, err
	}
	result := &sysin.CollectProfileMediaRebuildResult{ProfileIDs: make([]int64, 0, len(rows))}
	service := NewSysPublish()
	for _, row := range rows {
		result.Candidates++
		profileID := row["profile_id"].Int64()
		result.ProfileIDs = append(result.ProfileIDs, profileID)
		if !collectProfileMediaRebuildRecoverable(row["recoverable"].Int()) {
			continue
		}
		result.Recoverable++
		if options.DryRun {
			continue
		}
		if err = service.resetCollectProfileMediaRebuild(ctx, row); err != nil {
			return result, err
		}
		result.Requeued++
	}
	return result, nil
}

func collectProfileMediaRebuildCandidates(ctx context.Context, options CollectProfileMediaRebuildOptions) (gdb.Result, error) {
	model := g.DB().Model("hg_content_profile p").Safe().Ctx(ctx).
		Fields("p.id AS profile_id,d.event_id,e.tenant_id,e.account_id,e.source_id,e.tg_account_id,e.media_count,e.material_role,e.material_group_status,COUNT(DISTINCT m.id) AS profile_media_count,COUNT(DISTINCT CASE WHEN COALESCE(em.source_file_id,'') <> '' OR COALESCE(em.source_message_ref,'') <> '' OR em.backup_message_id > 0 THEN em.id END) AS recoverable").
		InnerJoin("hg_youban_publish_collect_dispatch d", "d.profile_id=p.id").
		InnerJoin("hg_youban_publish_collect_event e", "e.id=d.event_id").
		LeftJoin("hg_youban_publish_media m", "m.profile_id=p.id AND m.deleted_at IS NULL").
		LeftJoin("hg_youban_publish_collect_event_media em", "em.event_id=e.id").
		Where("p.source_type", "youban_collect").
		WhereNull("p.deleted_at").
		Where("(e.status IN (?, ?, ?, ?, ?) OR (e.status = ? AND e.error_message = ?))",
			"processed", "ignored", "failed", "prechecked", "media_ready",
			"media_pending", "资料媒体重建：重新下载").
		Where("e.media_count > 0")
	if len(options.ProfileIDs) > 0 {
		model = model.WhereIn("p.id", options.ProfileIDs)
	}
	rows, err := model.Group("p.id,d.event_id,e.tenant_id,e.account_id,e.source_id,e.tg_account_id,e.media_count,e.material_role,e.material_group_status").
		Having("e.media_count > COUNT(DISTINCT m.id)").
		OrderAsc("p.id").Limit(options.Limit).All()
	return rows, gerror.Wrap(err, "读取资料媒体重建候选失败")
}

func collectProfileMediaRebuildRecoverable(count int) bool { return count > 0 }

func (s *sSysPublish) resetCollectProfileMediaRebuild(ctx context.Context, row gdb.Record) error {
	now := gtime.Now().Add(-10 * time.Minute)
	materialRole := row["material_role"].String()
	if materialRole != collectMaterialRoleDisplay && materialRole != collectMaterialRoleVerify {
		materialRole = collectMaterialRoleDisplay
	}
	materialGroupStatus := row["material_group_status"].String()
	if materialGroupStatus == "" || materialGroupStatus == collectMaterialGroupCollecting {
		materialGroupStatus = "complete"
	}
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
			Where("event_id", row["event_id"].Int64()).
			Where("COALESCE(source_file_id,'') <> '' OR COALESCE(source_message_ref,'') <> '' OR backup_message_id > 0").
			Data(g.Map{"cache_status": "pending", "file_url": "", "storage_path": "", "poster_url": "", "error_message": "资料媒体重建：重新下载", "download_attempts": 0, "next_retry_at": nil, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "重置资料采集媒体失败")
		}
		if _, err := tx.Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
			Where("id", row["event_id"].Int64()).
			Data(g.Map{"status": "media_pending", "material_role": materialRole, "material_group_status": materialGroupStatus, "processed_at": nil, "error_message": "资料媒体重建：重新下载", "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "重置资料采集事件失败")
		}
		return nil
	}); err != nil {
		return err
	}
	payload := collectMediaQueuePayload{
		EventId: row["event_id"].Int64(), TenantId: row["tenant_id"].Int64(),
		AccountId: row["account_id"].Int64(), SourceId: row["source_id"].Int64(),
		TgAccountId: row["tg_account_id"].Int64(),
	}
	enqueued, err := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
	if err == nil {
		g.Log().Infof(ctx, "资料媒体重建任务已投递 eventId:%d queue:%s redis:%s enqueued:%t", payload.EventId, collectMediaQueueName(ctx, payload), g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(), enqueued)
	}
	return err
}
