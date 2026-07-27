package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const profileImageSearchResultTTL = 30 * time.Second

func (s *sSysPublish) profileImageSearchNotesByProfileIds(ctx context.Context, profileIds []int64, tenantId int64, filterAccountIds []int64, viewer *sysin.AccountModel, permission string) ([]*sysin.NoteModel, error) {
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return []*sysin.NoteModel{}, nil
	}
	cacheKey := profileImageSearchResultCacheKey(ctx, profileIds, tenantId, filterAccountIds, viewer, permission)
	if cached, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cached.IsNil() {
		var list []*sysin.NoteModel
		if scanErr := cached.Scan(&list); scanErr == nil && list != nil {
			return list, nil
		}
	}
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, err
	}
	if len(filterAccountIds) > 0 {
		base = base.WhereIn("ps.account_id", uniqueIds(filterAccountIds))
	}
	rows, err := base.Fields(profileImageSearchListFields()).WhereIn("p.id", profileIds).All()
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
	_ = cache.Instance().Set(ctx, cacheKey, list, profileImageSearchResultTTL)
	return list, nil
}

func profileImageSearchListFields() string {
	return "p.id,p.source_note_uuid AS uuid,p.profile_no,p.title,p.province,p.city," + profileTagFieldExpr() + " AS tag,p.visibility,p.review_status,p.status,p.image_count,p.video_count,p.published_at,p.created_at,p.updated_at,ps.tenant_id,ps.account_id,a.nickname AS account_name,a.nickname,a.username,'' AS task_status"
}

func profileImageSearchResultCacheKey(ctx context.Context, profileIds []int64, tenantId int64, filterAccountIds []int64, viewer *sysin.AccountModel, permission string) string {
	ids := append([]int64(nil), filterAccountIds...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := []string{
		"youban_publish:profile_image_search:result:v1",
		fmt.Sprintf("tenant=%d", tenantId),
		fmt.Sprintf("permission=%s", strings.TrimSpace(permission)),
		fmt.Sprintf("version=%s", mediaPHashBucketVersion(ctx, tenantId, ids)),
		fmt.Sprintf("profiles=%v", profileIds),
		fmt.Sprintf("accounts=%v", uniqueIds(ids)),
	}
	if viewer != nil {
		parts = append(parts, fmt.Sprintf("viewer=%d", viewer.Id))
	}
	return mediaPHashHashKey(strings.Join(parts, "|"))
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
