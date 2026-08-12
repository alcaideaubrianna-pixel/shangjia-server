package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

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
	if _, err = s.profileState(ctx, profileId, tenantId, accountId); err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err = mod.OrderAsc("sort_index").OrderAsc("id").Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "读取资料媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	normalizeMediaListFileURL(list)
	return list, nil
}

func (s *sSysPublish) mediaListByEditableProfile(ctx context.Context, profileId int64, tenantId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if _, err = s.profileState(ctx, profileId, tenantId, accountId); err != nil {
		return nil, err
	}
	return s.mediaListByProfile(ctx, profileId, tenantId, accountId)
}
