package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) profileImageSearchNotesByProfileIds(ctx context.Context, profileIds []int64, tenantId int64, filterAccountIds []int64, viewer *sysin.AccountModel, permission string) ([]*sysin.NoteModel, error) {
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, nil
	}
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, err
	}
	if len(filterAccountIds) > 0 {
		base = base.WhereIn("t.account_id", uniqueIds(filterAccountIds))
	}
	rows, err := base.Fields(profileListFields()).WhereIn("p.id", profileIds).All()
	if err != nil {
		return nil, gerror.Wrap(err, "获取图片搜索资料失败")
	}
	profiles := make([]*sysin.ProfileModel, 0, len(rows))
	for _, row := range rows {
		item := new(sysin.ProfileModel)
		if err = gconv.Struct(row, item); err != nil {
			return nil, gerror.Wrap(err, "解析图片搜索资料失败")
		}
		profiles = append(profiles, item)
	}
	if err = s.ensureProfileListUUID(ctx, profiles); err != nil {
		return nil, err
	}
	if err = s.applyProfileOwnerNames(ctx, profiles); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, profiles); err != nil {
		return nil, err
	}
	mediaBuckets, err := s.mediaListByProfileModels(ctx, profiles)
	if err != nil {
		return nil, err
	}
	profileMap := make(map[int64]*sysin.ProfileModel, len(profiles))
	for _, profile := range profiles {
		if profile == nil || profile.Id <= 0 {
			continue
		}
		if permission != "" {
			markProfilePermission(profile, permission)
		} else if viewer != nil {
			markProfilePermission(profile, profilePermissionForViewer(viewer, profile))
		}
		profileMap[profile.Id] = profile
	}
	list := make([]*sysin.NoteModel, 0, len(profileIds))
	for _, profileId := range profileIds {
		profile := profileMap[profileId]
		if profile == nil {
			continue
		}
		list = append(list, &sysin.NoteModel{ProfileModel: *profile, Media: mediaBuckets[profileId]})
	}
	return list, nil
}

func (s *sSysPublish) mediaListByProfileModels(ctx context.Context, profiles []*sysin.ProfileModel) (map[int64][]*sysin.MediaModel, error) {
	buckets := make(map[int64][]*sysin.MediaModel, len(profiles))
	profileIds := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		if profile == nil || profile.Id <= 0 {
			continue
		}
		profileIds = append(profileIds, profile.Id)
		buckets[profile.Id] = []*sysin.MediaModel{}
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return buckets, nil
	}
	var media []*sysin.MediaModel
	err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,task_id,profile_id,attachment_id,original_attachment_id,edited_attachment_id,media_type,purpose,name,file_url,original_file_url,edited_file_url,poster_url,storage_path,original_storage_path,edited_storage_path,poster_storage_path,mime_type,md5,perceptual_hash,edit_config_json,edit_status,tg_file_id,tg_thumb_file_id,tg_cache_asset_hash,tg_cache_status,size,sort_index,status,created_at,updated_at").
		WhereIn("profile_id", profileIds).
		WhereNull("deleted_at").
		OrderAsc("profile_id").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&media)
	if err != nil {
		return nil, gerror.Wrap(err, "获取图片搜索媒体失败")
	}
	normalizeMediaListFileURL(media)
	for _, item := range media {
		if item == nil || item.ProfileId <= 0 {
			continue
		}
		buckets[item.ProfileId] = append(buckets[item.ProfileId], item)
	}
	return buckets, nil
}
