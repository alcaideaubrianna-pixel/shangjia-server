package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/form"
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
	list, totalCount, err = s.profileList(ctx, in)
	markProfilesPermission(list, sysin.ProfilePermissionCreator)
	return
}

func (s *sSysPublish) MyProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	markProfilePermission(profile, sysin.ProfilePermissionCreator)
	media, err := s.mediaListByProfile(ctx, profile.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) MyProfileOptions(ctx context.Context) (res *sysin.ProfileOptionsModel, err error) {
	channels, _, err := s.MyChannelList(ctx, &sysin.ChannelListInp{
		PageReq: form.PageReq{
			Page:    1,
			PerPage: 200,
		},
	})
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileOptionsModel{Channels: filterProfileOptionChannels(channels)}, nil
}

func filterProfileOptionChannels(channels []*sysin.ChannelModel) []*sysin.ChannelModel {
	list := make([]*sysin.ChannelModel, 0, len(channels))
	for _, channel := range channels {
		if channel == nil || channel.PublishVisible == 2 {
			continue
		}
		list = append(list, channel)
	}
	return list
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

func (s *sSysPublish) MyProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
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
	list, totalCount, err = s.noteList(ctx, in)
	markNotesPermission(list, sysin.ProfilePermissionCreator)
	return
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
	scope, err := s.adminProfileVisibleScope(ctx, account, in)
	if err != nil {
		return nil, 0, err
	}
	if scope.Strict && len(scope.AccountIds) == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	list, totalCount, err = s.profileListByAccountIds(ctx, in, 0, scope.AccountIds)
	for _, item := range list {
		markProfilePermission(item, profilePermissionForViewer(account, item))
	}
	return
}

func (s *sSysPublish) AdminProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料UUID不能为空")
	}
	scope, err := s.adminProfileVisibleScope(ctx, account, &sysin.ProfileListInp{AccountScope: "all"})
	if err != nil {
		return nil, err
	}
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, err
	}
	profile, err := s.profileViewBySelector(ctx, in, 0, 0)
	if err != nil {
		return nil, err
	}
	if !containsInt64(scope.AccountIds, profile.AccountId) {
		return nil, gerror.New("资料不存在或无权操作")
	}
	markProfilePermission(profile, profilePermissionForViewer(account, profile))
	media, err := s.mediaListByProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) AdminProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in != nil && in.Id <= 0 && normalizeProfileUUID(in.Uuid) != "" {
		if in.Id, err = s.resolveProfileId(ctx, 0, in.Uuid, account.TenantId, 0); err != nil {
			return nil, err
		}
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

func (s *sSysPublish) AdminProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.updateProfileStatus(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) AdminNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.AdminNoteListModel, totalCount int, err error) {
	startedAt := time.Now()
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	scope, err := s.adminProfileVisibleScope(ctx, account, &in.ProfileListInp)
	if err != nil {
		return nil, 0, err
	}
	logSlowAdminNoteListStage(ctx, "scope", startedAt, len(scope.AccountIds), len(scope.TenantIds))
	stageStartedAt := time.Now()
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, 0, err
	}
	logSlowAdminNoteListStage(ctx, "scope_tenants", stageStartedAt, len(scope.AccountIds), len(scope.TenantIds))
	if scope.Strict && len(scope.AccountIds) == 0 {
		return []*sysin.AdminNoteListModel{}, 0, nil
	}
	list, totalCount, err = s.adminNoteList(ctx, &in.ProfileListInp, scope.TenantId, scope.TenantIds, scope.AccountIds, account)
	logSlowAdminNoteListStage(ctx, "total", startedAt, len(list), totalCount)
	return list, totalCount, err
}

func logSlowAdminNoteListStage(ctx context.Context, stage string, startedAt time.Time, first int, second int) {
	duration := time.Since(startedAt)
	if duration < 100*time.Millisecond {
		return
	}
	g.Log().Infof(ctx, "上架笔记列表慢阶段 stage:%s duration_ms:%d first:%d second:%d", stage, duration.Milliseconds(), first, second)
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

func (s *sSysPublish) ServerProfileList(ctx context.Context, in *sysin.ProfileListInp) (list []*sysin.ProfileModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	return s.profileList(ctx, in)
}

func (s *sSysPublish) ServerProfileView(ctx context.Context, in *sysin.ProfileViewInp) (res *sysin.ProfileViewModel, err error) {
	if in == nil || !hasProfileSelector(in.Id, in.Uuid) {
		return nil, gerror.New("资料ID不能为空")
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, 0, 0)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, 0, 0)
	if err != nil {
		return nil, err
	}
	media, err := s.mediaListByProfile(ctx, profile.Id, 0, 0)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media}, nil
}

func (s *sSysPublish) ServerProfileSave(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	task, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("profile_id", in.Id).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料任务失败")
	}
	if task.IsEmpty() {
		return nil, gerror.New("资料不属于上架端")
	}
	return s.saveProfile(ctx, in, task["tenant_id"].Int64(), task["account_id"].Int64())
}

func (s *sSysPublish) ServerProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	return s.deleteProfiles(ctx, in, 0, 0)
}

func (s *sSysPublish) ServerProfileReview(ctx context.Context, in *sysin.ProfileReviewInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return err
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, 0, 0)
	if err != nil {
		return err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, 0, 0)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).WhereIn(columns.Id, ids).Data(g.Map{
		columns.ReviewStatus: in.ReviewStatus,
	}).Update(); err != nil {
		return gerror.Wrap(err, "审核资料失败")
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}
