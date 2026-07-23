package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
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
	mediaBuckets, err := s.mediaCoverListByProfileModels(ctx, profiles)
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

func (s *sSysPublish) mediaCoverListByProfileModels(ctx context.Context, profiles []*sysin.ProfileModel) (map[int64][]*sysin.MediaModel, error) {
	buckets := make(map[int64][]*sysin.MediaModel, len(profiles))
	profileIds := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		if profile == nil || profile.Id <= 0 {
			continue
		}
		profileIds = append(profileIds, profile.Id)
		buckets[profile.Id] = []*sysin.MediaModel{}
	}
	media, err := firstProfileCoverMedia(ctx, profileIds)
	if err != nil {
		return nil, gerror.Wrap(err, "获取图片搜索封面失败")
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
