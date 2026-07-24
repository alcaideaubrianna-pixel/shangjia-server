package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

func (s *sSysPublish) profileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, 0, err
	}
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, 0, err
	}
	return s.profileListByModel(ctx, base, in)
}

func (s *sSysPublish) profileListByAccountIds(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, accountIds []int64) (list []*sysin.ProfileModel, totalCount int, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, 0, err
	}
	accountIds = uniqueIds(accountIds)
	if len(accountIds) == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	base, err := s.profileBaseModel(ctx, tenantId, 0)
	if err != nil {
		return nil, 0, err
	}
	base = base.WhereIn("t.account_id", accountIds)
	return s.profileListByModel(ctx, base, in)
}

func (s *sSysPublish) profileListByModel(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	list, totalCount, err = s.searchProfilePage(ctx, base, in, profileListFields(), "统计资料失败", "获取资料列表失败")
	if err != nil {
		return nil, 0, err
	}
	if err = s.ensureProfileListUUID(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyProfileTagNames(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) profileView(ctx context.Context, profileId int64, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, err
	}
	if profileId <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = base.Where("p.id", profileId).Fields(profileListFields()).Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, res); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, []*sysin.ProfileModel{res}); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) profileViewBySelector(ctx context.Context, in *sysin.ProfileViewInp, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
	if err = s.ensureLegacyProfileNosOnce(ctx); err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if in.Id > 0 {
		base = base.Where("p.id", in.Id)
	} else {
		base = base.Where("p.source_note_uuid", normalizeProfileUUID(in.Uuid))
	}
	if err = base.Fields(profileListFields()).Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取资料详情失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureProfileModelUUID(ctx, res); err != nil {
		return nil, err
	}
	if err = s.applyProfileTagNames(ctx, []*sysin.ProfileModel{res}); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) noteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	profiles, totalCount, err := s.profileList(ctx, &in.ProfileListInp)
	if err != nil {
		return nil, 0, err
	}
	list = make([]*sysin.NoteModel, 0, len(profiles))
	for _, item := range profiles {
		note := &sysin.NoteModel{ProfileModel: *item}
		note.Media, err = s.mediaListByProfile(ctx, item.Id, item.TenantId, item.AccountId)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, note)
	}
	return
}

func (s *sSysPublish) adminNoteList(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, tenantIds []int64, accountIds []int64, viewer *sysin.AccountModel) ([]*sysin.AdminNoteListModel, int, error) {
	base := s.adminNoteBaseModel(ctx, tenantId, tenantIds)
	if len(accountIds) > 0 {
		base = base.WhereIn("t.account_id", accountIds)
	} else if in != nil && in.AccountId > 0 {
		base = base.Where("t.account_id", in.AccountId)
	}
	profiles, totalCount, err := s.searchDistinctProfilePage(ctx, base, in, adminNoteListFields(), "统计笔记失败", "获取笔记列表失败")
	if err != nil {
		return nil, 0, err
	}
	if err = s.ensureProfileListUUID(ctx, profiles); err != nil {
		return nil, 0, err
	}
	mediaBuckets, err := s.adminNoteCoverMediaByProfiles(ctx, profiles)
	if err != nil {
		return nil, 0, err
	}
	list := make([]*sysin.AdminNoteListModel, 0, len(profiles))
	for _, item := range profiles {
		if item == nil {
			continue
		}
		permission := sysin.ProfilePermissionAdmin
		if len(accountIds) > 0 {
			permission = profilePermissionForViewer(viewer, item)
		}
		markProfilePermission(item, permission)
		list = append(list, adminNoteListFromProfile(item, mediaBuckets[item.Id]))
	}
	return list, totalCount, nil
}

func (s *sSysPublish) adminNoteBaseModel(ctx context.Context, tenantId int64, tenantIds []int64) *gdb.Model {
	mod := dao.ContentProfile.Ctx(ctx).As("p").
		InnerJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id AND a.deleted_at IS NULL").
		WhereNull("p.deleted_at")
	if len(tenantIds) > 0 {
		mod = mod.WhereIn("t.tenant_id", tenantIds)
	} else if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	return mod
}

func adminNoteListFields() string {
	return "p.id,p.source_note_uuid AS uuid,p.profile_no,p.title,p.province,p.city," + profileTagFieldExpr() + " AS tag,p.status,p.created_at,p.updated_at,t.id AS task_id,t.tenant_id,t.account_id,a.nickname AS account_name,a.nickname,a.username,t.status AS task_status"
}

func adminNoteListFromProfile(profile *sysin.ProfileModel, media []*sysin.AdminNoteMediaModel) *sysin.AdminNoteListModel {
	if profile == nil {
		return nil
	}
	if media == nil {
		media = []*sysin.AdminNoteMediaModel{}
	}
	return &sysin.AdminNoteListModel{
		Id:          profile.Id,
		Uuid:        profile.Uuid,
		TaskId:      profile.TaskId,
		AccountId:   profile.AccountId,
		AccountName: profile.AccountName,
		Nickname:    profile.Nickname,
		Username:    profile.Username,
		ProfileNo:   profile.ProfileNo,
		Title:       profile.Title,
		Province:    profile.Province,
		City:        profile.City,
		Tag:         profile.Tag,
		Status:      profile.Status,
		TaskStatus:  profile.TaskStatus,
		CanEdit:     profile.CanEdit,
		Permission:  profile.Permission,
		CreatedAt:   profile.CreatedAt,
		UpdatedAt:   profile.UpdatedAt,
		Media:       media,
	}
}

func (s *sSysPublish) adminNoteCoverMediaByProfiles(ctx context.Context, profiles []*sysin.ProfileModel) (map[int64][]*sysin.AdminNoteMediaModel, error) {
	buckets := make(map[int64][]*sysin.AdminNoteMediaModel, len(profiles))
	profileIds := make([]int64, 0, len(profiles))
	for _, profile := range profiles {
		if profile == nil || profile.Id <= 0 {
			continue
		}
		profileIds = append(profileIds, profile.Id)
		buckets[profile.Id] = []*sysin.AdminNoteMediaModel{}
	}
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return buckets, nil
	}
	media, err := firstProfileCoverMedia(ctx, profileIds)
	if err != nil {
		return nil, gerror.Wrap(err, "获取笔记封面失败")
	}
	normalizeMediaListFileURL(media)
	for _, item := range media {
		if item == nil || item.ProfileId <= 0 || len(buckets[item.ProfileId]) > 0 {
			continue
		}
		buckets[item.ProfileId] = append(buckets[item.ProfileId], &sysin.AdminNoteMediaModel{
			Id:        item.Id,
			ProfileId: item.ProfileId,
			MediaType: item.MediaType,
			FileUrl:   item.FileUrl,
			SortIndex: item.SortIndex,
		})
	}
	return buckets, nil
}

func firstProfileCoverMedia(ctx context.Context, profileIds []int64) ([]*sysin.MediaModel, error) {
	ids := uniqueIds(profileIds)
	if len(ids) == 0 {
		return []*sysin.MediaModel{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, "video")
	var media []*sysin.MediaModel
	sql := fmt.Sprintf(`
SELECT id, profile_id, media_type, file_url, storage_path, sort_index
FROM (
    SELECT id, profile_id, media_type, file_url, storage_path, sort_index,
           ROW_NUMBER() OVER (PARTITION BY profile_id ORDER BY sort_index ASC, id ASC) AS row_number
    FROM %s
    WHERE profile_id IN (%s)
      AND deleted_at IS NULL
      AND (media_type IS NULL OR media_type = '' OR media_type <> ?)
) AS profile_cover
WHERE row_number = 1
ORDER BY profile_id ASC`, publishMediaTable, placeholders)
	if err := g.DB().Raw(sql, args...).Scan(&media); err != nil {
		return nil, gerror.Wrap(err, "查询资料封面失败")
	}
	return media, nil
}

func (s *sSysPublish) profileBaseModel(ctx context.Context, tenantId int64, accountId int64) (*gdb.Model, error) {
	mod := dao.ContentProfile.Ctx(ctx).As("p").
		LeftJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id AND a.deleted_at IS NULL").
		Where("(p.source_type = ? OR t.id IS NOT NULL)", publishProfileSourceType).
		WhereNull("p.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("t.account_id", accountId)
	}
	return mod, nil
}

func profileListFields() string {
	return "p.id,p.source_note_uuid AS uuid,p.profile_no,p.title,p.summary,p.plain_text,p.province,p.city," + profileTagFieldExpr() + " AS tag,p.visibility,p.review_status,p.status,p.image_count,p.video_count,p.admin_remark AS customer_remark,p.published_at,p.created_at,p.updated_at,t.id AS task_id,t.tenant_id,t.account_id,tenant.name AS tenant_name,a.nickname AS account_name,a.nickname,a.username,t.channel_id_json,t.anti_scan_enabled,t.status AS task_status,t.tg_status,t.tg_push_enabled"
}

func (s *sSysPublish) applyProfileFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	mod = s.applyProfileNonKeywordFilters(ctx, mod, in)
	if in == nil {
		return mod
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		if profileNo, ok := normalizeProfileNoSearchKeyword(keyword); ok {
			mod = mod.Where("p.profile_no", profileNo)
		} else {
			terms := splitProfileSearchTerms(keyword)
			if len(terms) > 0 {
				condition, args := segmentedLikeCondition([]string{"p.title", "t.title", "p.plain_text", "t.plain_text"}, terms)
				mod = mod.Where(condition, args...)
			}
		}
	}
	return mod
}

func (s *sSysPublish) applyProfileNonKeywordFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	_ = ctx
	if in == nil {
		return mod
	}
	if in.Province != "" {
		mod = mod.Where("p.province", strings.TrimSpace(in.Province))
	}
	if in.City != "" {
		mod = mod.Where("p.city", strings.TrimSpace(in.City))
	}
	if in.Tag != "" {
		tags := splitProfileTagValues(in.Tag)
		if len(tags) == 1 {
			tag := strings.TrimSpace(tags[0])
			tagField := profileTagFieldExpr()
			mod = mod.Where("("+tagField+" = ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ?)", tag, tag+",%", "%,"+tag, "%,"+tag+",%")
		} else if len(tags) > 1 {
			conditions := make([]string, 0, len(tags)*4)
			args := make([]interface{}, 0, len(tags)*4)
			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				tagField := profileTagFieldExpr()
				conditions = append(conditions, "("+tagField+" = ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ? OR "+tagField+" LIKE ?)")
				args = append(args, tag, tag+",%", "%,"+tag, "%,"+tag+",%")
			}
			if len(conditions) > 0 {
				mod = mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
			}
		}
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("p.review_status", strings.TrimSpace(in.ReviewStatus))
	}
	if in.Visibility != "" {
		mod = mod.Where("p.visibility", strings.TrimSpace(in.Visibility))
	}
	if in.Status > 0 {
		mod = mod.Where("p.status", in.Status)
	}
	return mod
}

func splitProfileTagValues(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '|' || r == ';'
	})
	if len(parts) == 0 {
		return []string{strings.TrimSpace(value)}
	}
	return parts
}

func (s *sSysPublish) applyProfileOwnerNames(ctx context.Context, list []*sysin.ProfileModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.TenantName) == "" {
			item.TenantName = names[item.TenantId]
		}
		if item.TenantName == "" && item.TenantId > 0 {
			item.TenantName = fmt.Sprintf("账号归属#%d", item.TenantId)
		}
	}
	return nil
}
