package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) cloneProfileTaskMedia(ctx context.Context, tx gdb.TX, sourceTaskId int64, targetTaskId int64, profileId int64, tenantId int64, accountId int64, operatorId int64) error {
	var rows []gdb.Record
	if err := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", sourceTaskId).Where("profile_id", profileId).WhereNull("deleted_at").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取原任务媒体失败")
	}
	now := gtime.Now()
	for _, row := range rows {
		data := g.Map{
			"tenant_id": tenantId, "merchant_id": tenantId, "account_id": accountId,
			"task_id": targetTaskId, "profile_id": profileId,
			"attachment_id": row["attachment_id"].Int64(), "original_attachment_id": row["original_attachment_id"].Int64(),
			"edited_attachment_id": row["edited_attachment_id"].Int64(), "media_type": row["media_type"].String(),
			"purpose": row["purpose"].String(), "name": row["name"].String(), "file_url": row["file_url"].String(),
			"original_file_url": row["original_file_url"].String(), "edited_file_url": row["edited_file_url"].String(),
			"poster_url": row["poster_url"].String(), "poster_storage_path": row["poster_storage_path"].String(),
			"storage_path": row["storage_path"].String(), "original_storage_path": row["original_storage_path"].String(),
			"edited_storage_path": row["edited_storage_path"].String(), "mime_type": row["mime_type"].String(),
			"md5": row["md5"].String(), "perceptual_hash": row["perceptual_hash"].String(),
			"edit_config_json": row["edit_config_json"].String(), "edit_status": row["edit_status"].String(),
			"tg_file_id": "", "tg_thumb_file_id": "", "tg_cache_asset_hash": "", "tg_cache_status": tgCacheStatusInvalid,
			"size": row["size"].Int64(), "sort_index": row["sort_index"].Int(), "status": row["status"].Int(),
			"created_by": operatorId, "updated_by": operatorId, "created_at": now, "updated_at": now,
		}
		if _, err := tx.Model(publishMediaTable).Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "创建发布媒体快照失败")
		}
	}
	return nil
}

func (s *sSysPublish) allowedProfileIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	mod := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		WhereIn("profile_id", ids).
		WhereGT("profile_id", 0).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	if err := mod.Fields("profile_id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "检查资料权限失败")
	}
	res := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ProfileId > 0 {
			res = append(res, row.ProfileId)
		}
	}
	return res, nil
}

func (s *sSysPublish) ensureProfileChannels(ctx context.Context, ids []int64, tenantId int64) error {
	if err := ensurePublishChannelColumns(ctx); err != nil {
		return err
	}
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return nil
	}
	count, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("publish_visible", 1).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查推送频道失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作或不可用的推送频道")
	}
	return nil
}

func (s *sSysPublish) availableProfileChannelIds(ctx context.Context, ids []int64, tenantId int64) ([]int64, error) {
	if err := ensurePublishChannelColumns(ctx); err != nil {
		return nil, err
	}
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("publish_visible", 1).
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "检查推送频道失败")
	}
	available := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			available[row.Id] = struct{}{}
		}
	}
	list := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := available[id]; ok {
			list = append(list, id)
		}
	}
	return list, nil
}

func (s *sSysPublish) mediaListByProfile(ctx context.Context, profileId int64, tenantId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if profileId <= 0 {
		return []*sysin.MediaModel{}, nil
	}
	taskMod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		taskMod = taskMod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		taskMod = taskMod.Where("account_id", accountId)
	}
	task, err := taskMod.Clone().
		Where("status", sysin.PublishTaskStatusPublished).
		OrderDesc("id").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料当前版本失败")
	}
	if task.IsEmpty() {
		task, err = taskMod.OrderDesc("id").One()
		if err != nil {
			return nil, gerror.Wrap(err, "读取资料当前版本失败")
		}
	}
	if task.IsEmpty() {
		return []*sysin.MediaModel{}, nil
	}
	return s.mediaListByTenant(ctx, task["id"].Int64(), tenantId)
}

func (s *sSysPublish) mediaListByEditableProfile(ctx context.Context, profileId int64, tenantId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if _, err = s.profileState(ctx, profileId, tenantId, accountId); err != nil {
		return nil, err
	}
	err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		Where("task_id", 0).
		WhereNull("deleted_at").
		OrderAsc("sort_index").OrderAsc("id").Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料媒体失败")
	}
	normalizeMediaListFileURL(list)
	return list, nil
}
