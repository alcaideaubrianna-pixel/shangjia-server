package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
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
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, 0, err
	}
	accountIds, err := s.sharedProfileAccountIds(ctx, capability)
	if err != nil {
		return nil, 0, err
	}
	in.TenantId, in.AccountId = account.TenantId, 0
	list, totalCount, err = s.profileListByAccountIds(ctx, in, account.TenantId, accountIds)
	for _, item := range list {
		markProfilePermission(item, sharedProfilePermission(capability, item))
	}
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
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	ownerScope := account.Id
	if capability.SharedResourceEnabled == 1 {
		ownerScope = 0
	}
	profileId, err := s.resolveProfileId(ctx, in.Id, in.Uuid, account.TenantId, ownerScope)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileView(ctx, profileId, account.TenantId, ownerScope)
	if err != nil {
		return nil, err
	}
	markProfilePermission(profile, sharedProfilePermission(capability, profile))
	media, err := s.mediaListByEditableProfile(ctx, profile.Id, account.TenantId, profile.AccountId)
	if err != nil {
		return nil, err
	}
	pushChannels, err := s.profilePushChannels(ctx, profile)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media, PushChannels: pushChannels}, nil
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

func (s *sSysPublish) MyProfileCreate(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	in.Id, in.Uuid = 0, ""
	in.Status = 2
	in.Visibility = consts.ContentVisibilityPublic
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileEdit(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || (in.Id <= 0 && normalizeProfileUUID(in.Uuid) == "") {
		return nil, gerror.New("资料UUID不能为空")
	}
	in.KeepPublishState = true
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) MyProfileDelete(ctx context.Context, in *sysin.ProfileDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return err
	}
	return s.deleteProfilesByCapability(ctx, in, capability)
}

func (s *sSysPublish) MyProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return s.updateProfileStatusByCapability(ctx, in, capability)
}

func (s *sSysPublish) MyNoteList(ctx context.Context, in *sysin.NoteListInp) (list []*sysin.NoteModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, 0, err
	}
	accountIds, err := s.sharedProfileAccountIds(ctx, capability)
	if err != nil {
		return nil, 0, err
	}
	in.TenantId, in.AccountId = account.TenantId, 0
	list, totalCount, err = s.noteListByAccountIds(ctx, in, accountIds)
	for _, item := range list {
		if item != nil {
			markProfilePermission(&item.ProfileModel, sharedProfilePermission(capability, &item.ProfileModel))
		}
	}
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
	media, err := s.mediaListByEditableProfile(ctx, profile.Id, profile.TenantId, profile.AccountId)
	if err != nil {
		return nil, err
	}
	pushChannels, err := s.profilePushChannels(ctx, profile)
	if err != nil {
		return nil, err
	}
	return &sysin.ProfileViewModel{Profile: profile, Media: media, PushChannels: pushChannels}, nil
}

func (s *sSysPublish) AdminProfileCreate(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	in.Id, in.Uuid = 0, ""
	in.Status = 2
	in.Visibility = consts.ContentVisibilityPublic
	return s.saveProfile(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminProfileEdit(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || (in.Id <= 0 && normalizeProfileUUID(in.Uuid) == "") {
		return nil, gerror.New("资料UUID不能为空")
	}
	if in != nil && in.Id <= 0 && normalizeProfileUUID(in.Uuid) != "" {
		if in.Id, err = s.resolveProfileId(ctx, 0, in.Uuid, account.TenantId, 0); err != nil {
			return nil, err
		}
	}
	state, err := s.profileState(ctx, in.Id, account.TenantId, 0)
	if err != nil {
		return nil, err
	}
	ownerAccountId := state["account_id"].Int64()
	if ownerAccountId <= 0 {
		return nil, gerror.New("资料归属账号不存在")
	}
	in.KeepPublishState = true
	return s.saveProfile(ctx, in, account.TenantId, ownerAccountId)
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

func (s *sSysPublish) AdminGlobalProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp) (res *sysin.ProfileStatusModel, err error) {
	if err = s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, err
	}
	return s.updateProfileStatus(ctx, in, 0, 0)
}

func (s *sSysPublish) AdminNoteList(ctx context.Context, in *sysin.NoteListInp) (res *sysin.AdminNotePageModel, err error) {
	startedAt := time.Now()
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	scope, err := s.adminProfileVisibleScope(ctx, account, &in.ProfileListInp)
	if err != nil {
		return nil, err
	}
	logSlowAdminNoteListStage(ctx, "scope", startedAt, len(scope.AccountIds), len(scope.TenantIds))
	stageStartedAt := time.Now()
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, err
	}
	logSlowAdminNoteListStage(ctx, "scope_tenants", stageStartedAt, len(scope.AccountIds), len(scope.TenantIds))
	if scope.Strict && len(scope.AccountIds) == 0 {
		return &sysin.AdminNotePageModel{List: []*sysin.AdminNoteListModel{}}, nil
	}
	res, err = s.adminNoteList(ctx, in, scope.TenantId, scope.TenantIds, scope.AccountIds, account)
	if res != nil {
		logSlowAdminNoteListStage(ctx, "total", startedAt, len(res.List), adminNoteBoolValue(res.HasMore))
	}
	return res, err
}

func (s *sSysPublish) AdminNoteBatchIds(ctx context.Context, in *sysin.NoteListInp) (res *sysin.AdminNoteBatchIdsModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	scope, err := s.adminProfileVisibleScope(ctx, account, &in.ProfileListInp)
	if err != nil {
		return nil, err
	}
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, err
	}
	if scope.Strict && len(scope.AccountIds) == 0 {
		return &sysin.AdminNoteBatchIdsModel{Ids: []int64{}}, nil
	}
	ids, err := s.adminNoteIndexProfileIds(ctx, in, scope.TenantId, scope.TenantIds, scope.AccountIds)
	if err != nil {
		return nil, err
	}
	return &sysin.AdminNoteBatchIdsModel{Ids: ids, Total: len(ids)}, nil
}

func adminNoteBoolValue(value bool) int {
	if value {
		return 1
	}
	return 0
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

func (s *sSysPublish) ServerProfileEdit(ctx context.Context, in *sysin.ProfileSaveInp) (res *sysin.ProfileSaveModel, err error) {
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("资料ID不能为空")
	}
	state, err := s.profileState(ctx, in.Id, 0, 0)
	if err != nil {
		return nil, err
	}
	return s.saveProfile(ctx, in, state["tenant_id"].Int64(), state["account_id"].Int64())
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
	for _, id := range ids {
		if err = s.syncProfileNoteIndex(ctx, id); err != nil {
			return err
		}
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}
