package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	basesysin "hotgo/internal/model/input/sysin"
	iservice "hotgo/internal/service"
)

func (s *sSysPublish) MyProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.profileList(ctx, in)
}

func (s *sSysPublish) MyProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	profile, err := s.profileView(ctx, in.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	media, err := s.mediaListByProfile(ctx, profile.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) MyProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteProfiles(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.updateProfileStatus(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.noteList(ctx, in)
}

func (s *sSysPublish) MyTagList(ctx context.Context, in *sysin.TagListInp) (list []*sysin.TagModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.tagList(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.saveTag(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteTags(ctx, in, account.Id, false)
}

func (s *sSysPublish) MyCityForward(ctx context.Context, in *sysin.CityForwardInp) (res *sysin.CityForwardModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.cityForward(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileStats(ctx context.Context, in *sysin.TrendInp) (res *sysin.ProfileStatsModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.profileStats(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	in.TenantId = account.TenantId
	return s.profileList(ctx, in)
}

func (s *sSysPublish) AdminProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteProfiles(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.updateProfileStatus(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	in.TenantId = account.TenantId
	return s.noteList(ctx, in)
}

func (s *sSysPublish) AdminTagList(ctx context.Context, in *sysin.TagListInp) (list []*sysin.TagModel, totalCount int, err error) {
	return s.tagList(ctx, in, 0, true)
}

func (s *sSysPublish) ServerTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	return s.saveTag(ctx, in, 0, true)
}

func (s *sSysPublish) ServerTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	return s.deleteTags(ctx, in, contexts.GetUserId(ctx), true)
}

func (s *sSysPublish) AdminTagSave(ctx context.Context, in *sysin.TagSaveInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.saveTag(ctx, in, account.Id, true)
}

func (s *sSysPublish) AdminTagDelete(ctx context.Context, in *sysin.TagDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteTags(ctx, in, account.Id, true)
}

func (s *sSysPublish) AdminCityForward(ctx context.Context, in *sysin.CityForwardInp) (res *sysin.CityForwardModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.cityForward(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminProfileStats(ctx context.Context, in *sysin.TrendInp) (res *sysin.ProfileStatsModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.profileStats(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) profileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	base, err := s.profileBaseModel(ctx, in.TenantId, in.AccountId)
	if err != nil {
		return nil, 0, err
	}
	base = s.applyProfileFilters(ctx, base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计资料失败")
	}
	if totalCount == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	if err = base.Fields(profileListFields()).Page(in.Page, in.PerPage).OrderDesc("p.updated_at").OrderDesc("p.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取资料列表失败")
	}
	return
}

func (s *sSysPublish) profileView(ctx context.Context, profileId int64, tenantId int64, accountId int64) (res *sysin.ProfileModel, err error) {
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

func (s *sSysPublish) saveProfile(ctx context.Context, in *sysin.ProfileSaveInp, tenantId int64, accountId int64) (res *sysin.ProfileSaveModel, err error) {
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	now := gtime.Now()
	var profileId int64
	var taskId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if in.Id > 0 {
			task, taskErr := s.profileTask(ctx, tx, in.Id, tenantId, 0)
			if taskErr != nil {
				return taskErr
			}
			profileId = in.Id
			taskId = task["id"].Int64()
			if taskId <= 0 {
				return gerror.New("资料不属于上架端")
			}
		} else if in.TaskId > 0 {
			task, taskErr := s.profileTaskByTaskId(ctx, tx, in.TaskId, tenantId, accountId)
			if taskErr != nil {
				return taskErr
			}
			taskId = task["id"].Int64()
			profileId = task["profile_id"].Int64()
		}
		if taskId == 0 {
			newTaskId, taskErr := tx.Model(publishTaskTable).Ctx(ctx).Data(g.Map{
				"tenant_id":       tenantId,
				"merchant_id":     tenantId,
				"account_id":      accountId,
				"title":           in.Title,
				"province":        in.Province,
				"city":            in.City,
				"plain_text":      in.PlainText,
				"media_count":     0,
				"tg_push_enabled": 0,
				"tg_status":       "skipped",
				"status":          sysin.PublishTaskStatusDraft,
				"created_by":      contexts.GetUserId(ctx),
				"updated_by":      contexts.GetUserId(ctx),
				"created_at":      now,
				"updated_at":      now,
			}).InsertAndGetId()
			if taskErr != nil {
				return gerror.Wrap(taskErr, "创建资料任务失败")
			}
			taskId = newTaskId
		}
		if profileId == 0 {
			profileId, err = s.createProfileFromInput(ctx, tx, in, tenantId, accountId, taskId)
			if err != nil {
				return err
			}
			if _, err = tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).Data(g.Map{"profile_id": profileId, "updated_at": now}).Update(); err != nil {
				return gerror.Wrap(err, "回写资料任务失败")
			}
		} else {
			if err = s.updateProfileFromInput(ctx, tx, in, profileId, taskId, tenantId, accountId); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileSaveModel{Id: profileId, TaskId: taskId}, nil
}

func (s *sSysPublish) deleteProfiles(ctx context.Context, in *sysin.ProfileDeleteInp, tenantId int64, accountId int64) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的资料")
	}
	ids, err := s.allowedProfileIds(ctx, in.Ids, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(g.Map{columns.DeletedAt: gtime.Now()}).Unscoped().Update(); err != nil {
		return gerror.Wrap(err, "删除资料失败")
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(g.Map{"deleted_by": contexts.GetUserId(ctx), "deleted_at": gtime.Now()}).Update()
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) updateProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp, tenantId int64, accountId int64) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要处理的资料")
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("资料状态不合法")
	}
	ids, err := s.allowedProfileIds(ctx, in.Ids, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	columns := dao.ContentProfile.Columns()
	data := g.Map{columns.Status: in.Status, columns.UpdatedAt: gtime.Now()}
	if in.Status == 1 {
		data[columns.Visibility] = consts.ContentVisibilityPublic
		data[columns.PublishedAt] = gtime.Now()
	} else {
		data[columns.Visibility] = consts.ContentVisibilityPrivate
	}
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料状态失败")
	}
	taskStatus := sysin.PublishTaskStatusPublished
	if in.Status == 2 {
		taskStatus = sysin.PublishTaskStatusCanceled
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).Data(g.Map{"status": taskStatus, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now()}).Update()
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) tagList(ctx context.Context, in *sysin.TagListInp, operatorId int64, isAdmin bool) (list []*sysin.TagModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TagListInp{}
	}
	mod := g.DB().Model(publishTagTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" a", "a.id=t.created_by AND a.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" tenant", "tenant.id=a.tenant_id AND tenant.deleted_at IS NULL").
		WhereNull("t.deleted_at")
	if !isAdmin {
		mod = mod.Where("(t.review_status = ? OR t.created_by = ?)", sysin.PublishTagReviewApproved, operatorId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("t.name", "%"+keyword+"%")
	}
	if in.ReviewStatus != "" {
		mod = mod.Where("t.review_status", in.ReviewStatus)
	}
	if in.Status > 0 {
		mod = mod.Where("t.status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计标签失败")
	}
	if totalCount == 0 {
		return []*sysin.TagModel{}, 0, nil
	}
	fields := "t.id,t.name,t.review_status,t.status,t.use_count,t.created_by,t.created_at,t.updated_at,a.username AS creator_username,a.tenant_id AS creator_tenant_id,tenant.name AS creator_tenant_name"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("t.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取标签列表失败")
	}
	return
}

func (s *sSysPublish) saveTag(ctx context.Context, in *sysin.TagSaveInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil {
		return gerror.New("标签信息不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	if !isAdmin {
		in.ReviewStatus = sysin.PublishTagReviewPending
		in.Status = 1
	}
	now := gtime.Now()
	names := splitTagNames(in.Name)
	if len(names) == 0 {
		return gerror.New("标签名称不能为空")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, name := range names {
			if err := s.saveOneTag(ctx, tx, name, in.ReviewStatus, in.Status, operatorId, isAdmin, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sSysPublish) saveOneTag(ctx context.Context, tx gdb.TX, name string, reviewStatus string, status int, operatorId int64, isAdmin bool, now *gtime.Time) error {
	existing, err := tx.Model(publishTagTable).Safe().Ctx(ctx).Where("name", name).WhereNull("deleted_at").Fields("id,created_by,review_status").One()
	if err != nil {
		return gerror.Wrap(err, "检查标签失败")
	}
	if !existing.IsEmpty() && !isAdmin && existing["created_by"].Int64() != operatorId {
		return gerror.New("标签已存在，请等待审核或选择已有标签")
	}
	data := g.Map{
		"name":          name,
		"review_status": reviewStatus,
		"status":        status,
		"updated_by":    operatorId,
		"updated_at":    now,
	}
	if !existing.IsEmpty() {
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(data).Update()
	} else {
		data["created_by"] = operatorId
		data["created_at"] = now
		_, err = tx.Model(publishTagTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存标签失败")
	}
	return nil
}

func splitTagNames(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == ',' || r == '，'
	})
	list := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func (s *sSysPublish) deleteTags(ctx context.Context, in *sysin.TagDeleteInp, operatorId int64, isAdmin bool) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的标签")
	}
	mod := g.DB().Model(publishTagTable).Safe().Ctx(ctx).WhereIn("id", in.Ids).WhereNull("deleted_at")
	if !isAdmin {
		mod = mod.Where("created_by", operatorId)
	}
	result, err := mod.Data(g.Map{
		"deleted_by": operatorId,
		"deleted_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除标签失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认标签删除结果失败")
	}
	if affected == 0 {
		return gerror.New("标签不存在或无权删除")
	}
	return nil
}

func (s *sSysPublish) cityForward(ctx context.Context, in *sysin.CityForwardInp, tenantId int64, accountId int64) (res *sysin.CityForwardModel, err error) {
	if in == nil {
		in = &sysin.CityForwardInp{}
	}
	data, err := iservice.SysProvinces().Select(ctx, &basesysin.ProvincesSelectInp{
		DataType: "pc",
		Value:    in.ParentId,
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &sysin.CityForwardModel{List: []*sysin.CityOptionModel{}}, nil
	}
	res = &sysin.CityForwardModel{List: make([]*sysin.CityOptionModel, 0, len(data.List))}
	for _, item := range data.List {
		res.List = append(res.List, &sysin.CityOptionModel{
			IsLeaf: item.IsLeaf,
			Label:  item.Label,
			Level:  item.Level,
			Value:  item.Value,
		})
	}
	return res, nil
}

func (s *sSysPublish) profileStats(ctx context.Context, in *sysin.TrendInp, tenantId int64, accountId int64) (res *sysin.ProfileStatsModel, err error) {
	if in == nil {
		in = &sysin.TrendInp{}
	}
	days := in.Days
	if days <= 0 {
		days = 7
	}
	if days > 90 {
		days = 90
	}
	res = &sysin.ProfileStatsModel{Trend: make([]*sysin.TrendPointModel, 0, days)}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if res.Total, err = base.Clone().Count(); err != nil {
		return nil, gerror.Wrap(err, "统计资料总数失败")
	}
	if res.UpCount, err = base.Clone().Where("p.status", 1).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计上架资料失败")
	}
	if res.DownCount, err = base.Clone().Where("p.status", 2).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计下架资料失败")
	}
	if res.Pending, err = base.Clone().Where("p.review_status", consts.ContentReviewPending).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计待审核资料失败")
	}
	if res.Approved, err = base.Clone().Where("p.review_status", consts.ContentReviewApproved).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计审核通过资料失败")
	}
	if res.Rejected, err = base.Clone().Where("p.review_status", consts.ContentReviewRejected).Count(); err != nil {
		return nil, gerror.Wrap(err, "统计审核拒绝资料失败")
	}
	var rows []struct {
		Date   string `json:"date"`
		Count  int    `json:"count"`
		Status int    `json:"status"`
	}
	start := time.Now().AddDate(0, 0, -days+1).Format("2006-01-02")
	trendMod, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = trendMod.Fields("DATE(p.created_at) AS date,p.status,COUNT(*) AS count").
		WhereGTE("p.created_at", start+" 00:00:00").
		Group("DATE(p.created_at),p.status").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取资料趋势失败")
	}
	index := make(map[string]*sysin.TrendPointModel, days)
	for i := days - 1; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		point := &sysin.TrendPointModel{Date: date}
		index[date] = point
		res.Trend = append(res.Trend, point)
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		point.ProfileCount += row.Count
		if row.Status == 1 {
			point.UpCount += row.Count
		}
		if row.Status == 2 {
			point.DownCount += row.Count
		}
	}
	return res, nil
}

func (s *sSysPublish) profileBaseModel(ctx context.Context, tenantId int64, accountId int64) (*gdb.Model, error) {
	mod := dao.ContentProfile.Ctx(ctx).As("p").
		LeftJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		Where("p.source_type", publishProfileSourceType).
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
	return "p.id,p.profile_no,p.title,p.summary,p.plain_text,p.province,p.city,p.cup_size AS tag,p.visibility,p.review_status,p.status,p.image_count,p.video_count,p.published_at,p.created_at,p.updated_at,t.id AS task_id,t.tenant_id,t.account_id"
}

func (s *sSysPublish) applyProfileFilters(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	if in == nil {
		return mod
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tagIds := s.tagIdsByKeyword(ctx, keyword)
		if len(tagIds) > 0 {
			args := []interface{}{like, like, like, like, like}
			conditions := []string{"p.profile_no LIKE ?", "p.title LIKE ?", "p.summary LIKE ?", "p.plain_text LIKE ?", "p.cup_size LIKE ?"}
			for _, tagId := range tagIds {
				conditions = append(conditions, "p.cup_size = ?", "p.cup_size LIKE ?", "p.cup_size LIKE ?", "p.cup_size LIKE ?")
				args = append(args, tagId, tagId+",%", "%,"+tagId, "%,"+tagId+",%")
			}
			mod = mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
		} else {
			mod = mod.Where("(p.profile_no LIKE ? OR p.title LIKE ? OR p.summary LIKE ? OR p.plain_text LIKE ? OR p.cup_size LIKE ?)", like, like, like, like, like)
		}
	}
	if in.Province != "" {
		mod = mod.Where("p.province", strings.TrimSpace(in.Province))
	}
	if in.City != "" {
		mod = mod.Where("p.city", strings.TrimSpace(in.City))
	}
	if in.Tag != "" {
		mod = mod.Where("p.cup_size", strings.TrimSpace(in.Tag))
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

func (s *sSysPublish) tagIdsByKeyword(ctx context.Context, keyword string) []string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []string{}
	}
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishTagTable).Safe().Ctx(ctx).
		Fields("id").
		WhereLike("name", "%"+keyword+"%").
		Where("status", 1).
		Where("review_status", sysin.PublishTagReviewApproved).
		WhereNull("deleted_at").
		Limit(50).
		Scan(&rows)
	if err != nil || len(rows) == 0 {
		return []string{}
	}
	res := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			res = append(res, strconv.FormatInt(row.Id, 10))
		}
	}
	return res
}

func (s *sSysPublish) createProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, tenantId int64, accountId int64, taskId int64) (int64, error) {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	data := g.Map{
		columns.ProfileNo:       fmt.Sprintf("YBP%d", taskId),
		columns.SourceType:      publishProfileSourceType,
		columns.SourceKey:       fmt.Sprintf("youban_publish:profile:%d", taskId),
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.CupSize:         in.Tag,
		columns.Visibility:      in.Visibility,
		columns.ReviewStatus:    consts.ContentReviewPending,
		columns.ImportStatus:    "manual",
		columns.SourceCreateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceCreatedAt: now,
		columns.SourceUpdatedAt: now,
		columns.Status:          in.Status,
		columns.CreatedAt:       now,
		columns.UpdatedAt:       now,
	}
	if in.Status == 1 {
		data[columns.PublishedAt] = now
	}
	id, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建资料失败")
	}
	return id, nil
}

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId int64, taskId int64, tenantId int64, accountId int64) error {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	data := g.Map{
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.CupSize:         in.Tag,
		columns.Visibility:      in.Visibility,
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdatedAt: now,
		columns.Status:          in.Status,
		columns.UpdatedAt:       now,
	}
	if in.Status == 1 {
		data[columns.PublishedAt] = now
	}
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料失败")
	}
	if _, err := tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).Data(g.Map{
		"title":      in.Title,
		"province":   in.Province,
		"city":       in.City,
		"plain_text": in.PlainText,
		"status":     sysin.PublishTaskStatusDraft,
		"updated_by": contexts.GetUserId(ctx),
		"updated_at": now,
	}).Update(); err != nil {
		return gerror.Wrap(err, "更新资料任务失败")
	}
	return nil
}

func (s *sSysPublish) profileTask(ctx context.Context, tx gdb.TX, profileId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := tx.Model(publishTaskTable).Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料不存在或无权操作")
	}
	return row, nil
}

func (s *sSysPublish) profileTaskByTaskId(ctx context.Context, tx gdb.TX, taskId int64, tenantId int64, accountId int64) (gdb.Record, error) {
	mod := tx.Model(publishTaskTable).Ctx(ctx).Where("id", taskId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料任务不存在或无权操作")
	}
	return row, nil
}

func (s *sSysPublish) allowedProfileIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
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
		res = append(res, row.ProfileId)
	}
	return res, nil
}

func (s *sSysPublish) mediaListByProfile(ctx context.Context, profileId int64, tenantId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err = mod.OrderAsc("sort_index").OrderAsc("id").Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "获取笔记媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	return list, nil
}
