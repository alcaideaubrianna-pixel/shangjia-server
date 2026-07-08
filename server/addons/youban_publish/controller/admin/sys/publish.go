package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublishServer{}
var PublishAdmin = cPublishAdmin{}

type cPublishServer struct{}
type cPublishAdmin struct{}

func (c *cPublishServer) TenantList(ctx context.Context, req *publish.TenantListReq) (res *publish.TenantListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTenantList(ctx, &req.TenantListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TenantModel{}
	}
	res = new(publish.TenantListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) TenantSave(ctx context.Context, req *publish.TenantSaveReq) (res *publish.TenantSaveRes, err error) {
	saveRes, err := service.SysPublish().AdminTenantSave(ctx, &req.TenantSaveInp)
	if err != nil {
		return
	}
	res = &publish.TenantSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishServer) TenantDelete(ctx context.Context, req *publish.TenantDeleteReq) (res *publish.TenantDeleteRes, err error) {
	err = service.SysPublish().AdminTenantDelete(ctx, &req.TenantDeleteInp)
	if err != nil {
		return
	}
	res = &publish.TenantDeleteRes{}
	return
}

func (c *cPublishServer) AccountList(ctx context.Context, req *publish.AccountListReq) (res *publish.AccountListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerAccountList(ctx, &req.AccountListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AccountModel{}
	}
	res = new(publish.AccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) AccountSave(ctx context.Context, req *publish.AccountSaveReq) (res *publish.AccountSaveRes, err error) {
	saveRes, err := service.SysPublish().ServerAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return
	}
	res = &publish.AccountSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishServer) AccountResetPassword(ctx context.Context, req *publish.AccountResetPasswordReq) (res *publish.AccountResetPasswordRes, err error) {
	saveRes, err := service.SysPublish().ServerAccountResetPassword(ctx, &req.AccountResetPasswordInp)
	if err != nil {
		return
	}
	res = &publish.AccountResetPasswordRes{Password: saveRes.Password}
	return
}

func (c *cPublishServer) AccountDelete(ctx context.Context, req *publish.AccountDeleteReq) (res *publish.AccountDeleteRes, err error) {
	err = service.SysPublish().ServerAccountDelete(ctx, &req.AccountDeleteInp)
	if err != nil {
		return
	}
	res = &publish.AccountDeleteRes{}
	return
}

func (c *cPublishServer) TaskList(ctx context.Context, req *publish.TaskListReq) (res *publish.TaskListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.TaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) TaskSave(ctx context.Context, req *publish.TaskSaveReq) (res *publish.TaskSaveRes, err error) {
	id, err := service.SysPublish().ServerTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.TaskSaveRes{Id: id}
	return
}

func (c *cPublishServer) TaskSubmit(ctx context.Context, req *publish.TaskSubmitReq) (res *publish.TaskSubmitRes, err error) {
	err = service.SysPublish().ServerTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.TaskSubmitRes{}
	return
}

func (c *cPublishServer) TaskCancel(ctx context.Context, req *publish.TaskCancelReq) (res *publish.TaskCancelRes, err error) {
	err = service.SysPublish().ServerTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.TaskCancelRes{}
	return
}

func (c *cPublishServer) ProfileList(ctx context.Context, req *publish.ProfileListReq) (res *publish.ProfileListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerProfileList(ctx, &req.ProfileListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	res = new(publish.ProfileListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ProfileView(ctx context.Context, req *publish.ProfileViewReq) (res *publish.ProfileViewRes, err error) {
	data, err := service.SysPublish().ServerProfileView(ctx, &req.ProfileViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.ProfileViewRes{ProfileViewModel: data}
	return
}

func (c *cPublishServer) ProfileSave(ctx context.Context, req *publish.ProfileSaveReq) (res *publish.ProfileSaveRes, err error) {
	data, err := service.SysPublish().ServerProfileSave(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.ProfileSaveRes{}
	if data != nil {
		res.Id = data.Id
		res.Uuid = data.Uuid
		res.TaskId = data.TaskId
	}
	return
}

func (c *cPublishServer) ProfileDelete(ctx context.Context, req *publish.ProfileDeleteReq) (res *publish.ProfileDeleteRes, err error) {
	if err = service.SysPublish().ServerProfileDelete(ctx, &req.ProfileDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.ProfileDeleteRes{}
	return
}

func (c *cPublishServer) ProfileReview(ctx context.Context, req *publish.ProfileReviewReq) (res *publish.ProfileReviewRes, err error) {
	if err = service.SysPublish().ServerProfileReview(ctx, &req.ProfileReviewInp); err != nil {
		return nil, err
	}
	res = &publish.ProfileReviewRes{}
	return
}

func (c *cPublishServer) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().ServerMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	res = &publish.MediaListRes{List: list}
	return
}

func (c *cPublishServer) MediaDelete(ctx context.Context, req *publish.MediaDeleteReq) (res *publish.MediaDeleteRes, err error) {
	if err = service.SysPublish().ServerMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MediaDeleteRes{}
	return
}

func (c *cPublishServer) TagList(ctx context.Context, req *publish.TagListReq) (res *publish.TagListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTagList(ctx, &req.TagListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TagModel{}
	}
	res = new(publish.TagListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) TagSave(ctx context.Context, req *publish.TagSaveReq) (res *publish.TagSaveRes, err error) {
	if err = service.SysPublish().ServerTagSave(ctx, &req.TagSaveInp); err != nil {
		return nil, err
	}
	res = &publish.TagSaveRes{}
	return
}

func (c *cPublishServer) TagDelete(ctx context.Context, req *publish.TagDeleteReq) (res *publish.TagDeleteRes, err error) {
	if err = service.SysPublish().ServerTagDelete(ctx, &req.TagDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.TagDeleteRes{}
	return
}

func (c *cPublishServer) BotList(ctx context.Context, req *publish.BotListReq) (res *publish.BotListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	res = new(publish.BotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) BotSave(ctx context.Context, req *publish.BotSaveReq) (res *publish.BotSaveRes, err error) {
	if err = service.SysPublish().ServerBotSave(ctx, &req.BotSaveInp); err != nil {
		return nil, err
	}
	res = &publish.BotSaveRes{}
	return
}

func (c *cPublishServer) BotDelete(ctx context.Context, req *publish.BotDeleteReq) (res *publish.BotDeleteRes, err error) {
	if err = service.SysPublish().ServerBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.BotDeleteRes{}
	return
}

func (c *cPublishServer) BotRefresh(ctx context.Context, req *publish.BotRefreshReq) (res *publish.BotRefreshRes, err error) {
	list, err := service.SysPublish().ServerBotRefresh(ctx, &req.BotRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &publish.BotRefreshRes{List: list}
	return
}

func (c *cPublishServer) TgAccountList(ctx context.Context, req *publish.TgAccountListReq) (res *publish.TgAccountListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerTgAccountList(ctx, &req.TgAccountListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	res = new(publish.TgAccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) TgAccountStartLogin(ctx context.Context, req *publish.TgAccountStartLoginReq) (res *publish.TgAccountStartLoginRes, err error) {
	return nil, gerror.New("请在上架端管理员账号中绑定TG账号")
}

func (c *cPublishServer) TgAccountLoginStatus(ctx context.Context, req *publish.TgAccountLoginStatusReq) (res *publish.TgAccountLoginStatusRes, err error) {
	return nil, gerror.New("请在上架端管理员账号中查看TG账号登录状态")
}

func (c *cPublishServer) TgAccountPassword(ctx context.Context, req *publish.TgAccountPasswordReq) (res *publish.TgAccountPasswordRes, err error) {
	return nil, gerror.New("请在上架端管理员账号中提交TG账号二次验证")
}

func (c *cPublishServer) TgAccountDelete(ctx context.Context, req *publish.TgAccountDeleteReq) (res *publish.TgAccountDeleteRes, err error) {
	if err = service.SysPublish().ServerTgAccountDelete(ctx, &req.TgAccountDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.TgAccountDeleteRes{}
	return
}

func (c *cPublishServer) TgAccountRefresh(ctx context.Context, req *publish.TgAccountRefreshReq) (res *publish.TgAccountRefreshRes, err error) {
	list, err := service.SysPublish().ServerTgAccountRefresh(ctx, &req.TgAccountRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountRefreshModel{}
	}
	res = &publish.TgAccountRefreshRes{List: list}
	return
}

func (c *cPublishServer) ChannelList(ctx context.Context, req *publish.ChannelListReq) (res *publish.ChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	res = new(publish.ChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ChannelSave(ctx context.Context, req *publish.ChannelSaveReq) (res *publish.ChannelSaveRes, err error) {
	return nil, gerror.New("请在上架端管理员账号中配置频道")
}

func (c *cPublishServer) ChannelDelete(ctx context.Context, req *publish.ChannelDeleteReq) (res *publish.ChannelDeleteRes, err error) {
	if err = service.SysPublish().ServerChannelDelete(ctx, &req.ChannelDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelDeleteRes{}
	return
}

func (c *cPublishServer) ChannelBatchBots(ctx context.Context, req *publish.ChannelBatchBotsReq) (res *publish.ChannelBatchBotsRes, err error) {
	return nil, gerror.New("请在上架端管理员账号中批量编辑频道Bot")
}

func (c *cPublishServer) ChannelRefresh(ctx context.Context, req *publish.ChannelRefreshReq) (res *publish.ChannelRefreshRes, err error) {
	list, err := service.SysPublish().ServerChannelRefresh(ctx, &req.ChannelRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelRefreshModel{}
	}
	res = &publish.ChannelRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) AccountList(ctx context.Context, req *publish.AccountListReq) (res *publish.AccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminAccountList(ctx, &req.AccountListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AccountModel{}
	}
	res = new(publish.AccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) AccountSave(ctx context.Context, req *publish.AccountSaveReq) (res *publish.AccountSaveRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return
	}
	res = &publish.AccountSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishAdmin) AccountResetPassword(ctx context.Context, req *publish.AccountResetPasswordReq) (res *publish.AccountResetPasswordRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountResetPassword(ctx, &req.AccountResetPasswordInp)
	if err != nil {
		return
	}
	res = &publish.AccountResetPasswordRes{Password: saveRes.Password}
	return
}

func (c *cPublishAdmin) AccountDelete(ctx context.Context, req *publish.AccountDeleteReq) (res *publish.AccountDeleteRes, err error) {
	err = service.SysPublish().AdminAccountDelete(ctx, &req.AccountDeleteInp)
	if err != nil {
		return
	}
	res = &publish.AccountDeleteRes{}
	return
}

func (c *cPublishAdmin) TaskList(ctx context.Context, req *publish.TaskListReq) (res *publish.TaskListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.TaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TaskSave(ctx context.Context, req *publish.TaskSaveReq) (res *publish.TaskSaveRes, err error) {
	id, err := service.SysPublish().AdminTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.TaskSaveRes{Id: id}
	return
}

func (c *cPublishAdmin) TaskSubmit(ctx context.Context, req *publish.TaskSubmitReq) (res *publish.TaskSubmitRes, err error) {
	err = service.SysPublish().AdminTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.TaskSubmitRes{}
	return
}

func (c *cPublishAdmin) TaskCancel(ctx context.Context, req *publish.TaskCancelReq) (res *publish.TaskCancelRes, err error) {
	err = service.SysPublish().AdminTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.TaskCancelRes{}
	return
}

func (c *cPublishServer) ImportTaskList(ctx context.Context, req *publish.ImportTaskListReq) (res *publish.ImportTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerImportTaskList(ctx, &req.ImportTaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportTaskModel{}
	}
	res = new(publish.ImportTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ImportTaskCreate(ctx context.Context, req *publish.ImportTaskCreateReq) (res *publish.ImportTaskCreateRes, err error) {
	id, err := service.SysPublish().ServerImportTaskCreate(ctx, &req.ImportTaskCreateInp)
	if err != nil {
		return
	}
	res = &publish.ImportTaskCreateRes{Id: id}
	return
}

func (c *cPublishServer) ImportTaskView(ctx context.Context, req *publish.ImportTaskViewReq) (res *publish.ImportTaskViewRes, err error) {
	data, err := service.SysPublish().ServerImportTaskView(ctx, &req.ImportTaskViewInp)
	if err != nil {
		return
	}
	res = &publish.ImportTaskViewRes{ImportTaskModel: data}
	return
}

func (c *cPublishServer) ImportTaskStart(ctx context.Context, req *publish.ImportTaskStartReq) (res *publish.ImportTaskStartRes, err error) {
	if err = service.SysPublish().ServerImportTaskStart(ctx, &req.ImportTaskActionInp); err != nil {
		return
	}
	res = &publish.ImportTaskStartRes{}
	return
}

func (c *cPublishServer) ImportTaskCancel(ctx context.Context, req *publish.ImportTaskCancelReq) (res *publish.ImportTaskCancelRes, err error) {
	if err = service.SysPublish().ServerImportTaskCancel(ctx, &req.ImportTaskActionInp); err != nil {
		return
	}
	res = &publish.ImportTaskCancelRes{}
	return
}

func (c *cPublishServer) ImportTaskRetry(ctx context.Context, req *publish.ImportTaskRetryReq) (res *publish.ImportTaskRetryRes, err error) {
	if err = service.SysPublish().ServerImportTaskRetry(ctx, &req.ImportTaskActionInp); err != nil {
		return
	}
	res = &publish.ImportTaskRetryRes{}
	return
}

func (c *cPublishServer) ImportTaskScan(ctx context.Context, req *publish.ImportTaskScanReq) (res *publish.ImportTaskScanRes, err error) {
	data, err := service.SysPublish().ServerImportTaskScan(ctx, &req.ImportTaskScanInp)
	if err != nil {
		return
	}
	res = &publish.ImportTaskScanRes{ImportTaskScanModel: data}
	return
}

func (c *cPublishServer) ImportTaskRepair(ctx context.Context, req *publish.ImportTaskRepairReq) (res *publish.ImportTaskRepairRes, err error) {
	if err = service.SysPublish().ServerImportTaskRepair(ctx, &req.ImportTaskRepairInp); err != nil {
		return
	}
	res = &publish.ImportTaskRepairRes{}
	return
}

func (c *cPublishServer) ImportRunList(ctx context.Context, req *publish.ImportRunListReq) (res *publish.ImportRunListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerImportRunList(ctx, &req.ImportRunListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunModel{}
	}
	res = new(publish.ImportRunListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ImportRunCreate(ctx context.Context, req *publish.ImportRunCreateReq) (res *publish.ImportRunCreateRes, err error) {
	id, err := service.SysPublish().ServerImportRunCreate(ctx, &req.ImportRunCreateInp)
	if err != nil {
		return
	}
	res = &publish.ImportRunCreateRes{Id: id}
	return
}

func (c *cPublishServer) ImportRunDelete(ctx context.Context, req *publish.ImportRunDeleteReq) (res *publish.ImportRunDeleteRes, err error) {
	if err = service.SysPublish().ServerImportRunDelete(ctx, &req.ImportRunActionInp); err != nil {
		return
	}
	res = &publish.ImportRunDeleteRes{}
	return
}

func (c *cPublishServer) ImportRunCancel(ctx context.Context, req *publish.ImportRunCancelReq) (res *publish.ImportRunCancelRes, err error) {
	if err = service.SysPublish().ServerImportRunCancel(ctx, &req.ImportRunActionInp); err != nil {
		return
	}
	res = &publish.ImportRunCancelRes{}
	return
}

func (c *cPublishServer) ImportRunLogList(ctx context.Context, req *publish.ImportRunLogListReq) (res *publish.ImportRunLogListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerImportRunLogList(ctx, &req.ImportRunLogListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunLogModel{}
	}
	res = new(publish.ImportRunLogListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ImportRunLogClear(ctx context.Context, req *publish.ImportRunLogClearReq) (res *publish.ImportRunLogClearRes, err error) {
	if err = service.SysPublish().ServerImportRunLogClear(ctx, &req.ImportRunActionInp); err != nil {
		return
	}
	res = &publish.ImportRunLogClearRes{}
	return
}

func (c *cPublishServer) ImportRunMatchConfig(ctx context.Context, req *publish.ImportRunMatchConfigReq) (res *publish.ImportRunMatchConfigRes, err error) {
	data, err := service.SysPublish().ServerImportRunMatchConfig(ctx, &req.ImportRunMatchConfigInp)
	if err != nil {
		return
	}
	res = &publish.ImportRunMatchConfigRes{ImportRunMatchConfigModel: data}
	return
}

func (c *cPublishServer) ImportRunMatchStart(ctx context.Context, req *publish.ImportRunMatchStartReq) (res *publish.ImportRunMatchStartRes, err error) {
	data, err := service.SysPublish().ServerImportRunMatchStart(ctx, &req.ImportRunMatchStartInp)
	if err != nil {
		return
	}
	res = &publish.ImportRunMatchStartRes{ImportRunMatchRunModel: data}
	return
}

func (c *cPublishServer) ImportRunTgSyncStart(ctx context.Context, req *publish.ImportRunTgSyncStartReq) (res *publish.ImportRunTgSyncStartRes, err error) {
	data, err := service.SysPublish().ServerImportRunTgSyncStart(ctx, &req.ImportRunTgSyncStartInp)
	if err != nil {
		return
	}
	res = &publish.ImportRunTgSyncStartRes{ImportRunMatchRunModel: data}
	return
}

func (c *cPublishServer) ImportRunMatchView(ctx context.Context, req *publish.ImportRunMatchViewReq) (res *publish.ImportRunMatchViewRes, err error) {
	data, err := service.SysPublish().ServerImportRunMatchView(ctx, &req.ImportRunMatchViewInp)
	if err != nil {
		return
	}
	res = &publish.ImportRunMatchViewRes{ImportRunMatchRunModel: data}
	return
}

func (c *cPublishServer) ImportRunMatchItemList(ctx context.Context, req *publish.ImportRunMatchItemListReq) (res *publish.ImportRunMatchItemListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerImportRunMatchItemList(ctx, &req.ImportRunMatchItemListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunMatchItemModel{}
	}
	res = new(publish.ImportRunMatchItemListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ImportRunMatchCandidateList(ctx context.Context, req *publish.ImportRunMatchCandidateListReq) (res *publish.ImportRunMatchCandidateListRes, err error) {
	list, err := service.SysPublish().ServerImportRunMatchCandidateList(ctx, &req.ImportRunMatchCandidateListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunMatchCandidateModel{}
	}
	res = &publish.ImportRunMatchCandidateListRes{List: list}
	return
}

func (c *cPublishServer) ImportRunMatchCandidateSearch(ctx context.Context, req *publish.ImportRunMatchCandidateSearchReq) (res *publish.ImportRunMatchCandidateSearchRes, err error) {
	list, totalCount, err := service.SysPublish().ServerImportRunMatchCandidateSearch(ctx, &req.ImportRunMatchCandidateSearchInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunMatchCandidateModel{}
	}
	res = new(publish.ImportRunMatchCandidateSearchRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ImportRunMatchSaveDraft(ctx context.Context, req *publish.ImportRunMatchSaveDraftReq) (res *publish.ImportRunMatchSaveDraftRes, err error) {
	if err = service.SysPublish().ServerImportRunMatchSaveDraft(ctx, &req.ImportRunMatchSaveDraftInp); err != nil {
		return
	}
	res = &publish.ImportRunMatchSaveDraftRes{}
	return
}

func (c *cPublishServer) ImportRunMatchConfirm(ctx context.Context, req *publish.ImportRunMatchConfirmReq) (res *publish.ImportRunMatchConfirmRes, err error) {
	if err = service.SysPublish().ServerImportRunMatchConfirm(ctx, &req.ImportRunMatchConfirmInp); err != nil {
		return
	}
	res = &publish.ImportRunMatchConfirmRes{}
	return
}

func (c *cPublishServer) ImportRunMatchBatchConfirm(ctx context.Context, req *publish.ImportRunMatchBatchConfirmReq) (res *publish.ImportRunMatchBatchConfirmRes, err error) {
	if err = service.SysPublish().ServerImportRunMatchBatchConfirm(ctx, &req.ImportRunMatchBatchConfirmInp); err != nil {
		return
	}
	res = &publish.ImportRunMatchBatchConfirmRes{}
	return
}

func (c *cPublishServer) ImportRunMatchSkip(ctx context.Context, req *publish.ImportRunMatchSkipReq) (res *publish.ImportRunMatchSkipRes, err error) {
	if err = service.SysPublish().ServerImportRunMatchSkip(ctx, &req.ImportRunMatchSkipInp); err != nil {
		return
	}
	res = &publish.ImportRunMatchSkipRes{}
	return
}

func (c *cPublishAdmin) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().AdminMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	res = &publish.MediaListRes{List: list}
	return
}

func (c *cPublishAdmin) MediaDelete(ctx context.Context, req *publish.MediaDeleteReq) (res *publish.MediaDeleteRes, err error) {
	if err = service.SysPublish().AdminMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MediaDeleteRes{}
	return
}

func (c *cPublishAdmin) TagList(ctx context.Context, req *publish.TagListReq) (res *publish.TagListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTagList(ctx, &req.TagListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TagModel{}
	}
	res = new(publish.TagListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TagSave(ctx context.Context, req *publish.TagSaveReq) (res *publish.TagSaveRes, err error) {
	if err = service.SysPublish().AdminTagSave(ctx, &req.TagSaveInp); err != nil {
		return nil, err
	}
	res = &publish.TagSaveRes{}
	return
}

func (c *cPublishAdmin) TagDelete(ctx context.Context, req *publish.TagDeleteReq) (res *publish.TagDeleteRes, err error) {
	if err = service.SysPublish().AdminTagDelete(ctx, &req.TagDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.TagDeleteRes{}
	return
}

func (c *cPublishAdmin) BotList(ctx context.Context, req *publish.BotListReq) (res *publish.BotListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	res = new(publish.BotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) BotSave(ctx context.Context, req *publish.BotSaveReq) (res *publish.BotSaveRes, err error) {
	if err = service.SysPublish().AdminBotSave(ctx, &req.BotSaveInp); err != nil {
		return nil, err
	}
	res = &publish.BotSaveRes{}
	return
}

func (c *cPublishAdmin) BotDelete(ctx context.Context, req *publish.BotDeleteReq) (res *publish.BotDeleteRes, err error) {
	if err = service.SysPublish().AdminBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.BotDeleteRes{}
	return
}

func (c *cPublishAdmin) BotRefresh(ctx context.Context, req *publish.BotRefreshReq) (res *publish.BotRefreshRes, err error) {
	list, err := service.SysPublish().AdminBotRefresh(ctx, &req.BotRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &publish.BotRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) TgAccountList(ctx context.Context, req *publish.TgAccountListReq) (res *publish.TgAccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgAccountList(ctx, &req.TgAccountListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	res = new(publish.TgAccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgAccountStartLogin(ctx context.Context, req *publish.TgAccountStartLoginReq) (res *publish.TgAccountStartLoginRes, err error) {
	item, err := service.SysPublish().AdminTgAccountStartLogin(ctx, &req.TgAccountStartLoginInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountStartLoginRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountLoginStatus(ctx context.Context, req *publish.TgAccountLoginStatusReq) (res *publish.TgAccountLoginStatusRes, err error) {
	item, err := service.SysPublish().AdminTgAccountLoginStatus(ctx, &req.TgAccountLoginStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountLoginStatusRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountPassword(ctx context.Context, req *publish.TgAccountPasswordReq) (res *publish.TgAccountPasswordRes, err error) {
	item, err := service.SysPublish().AdminTgAccountPassword(ctx, &req.TgAccountPasswordInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountPasswordRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountDelete(ctx context.Context, req *publish.TgAccountDeleteReq) (res *publish.TgAccountDeleteRes, err error) {
	if err = service.SysPublish().AdminTgAccountDelete(ctx, &req.TgAccountDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.TgAccountDeleteRes{}
	return
}

func (c *cPublishAdmin) TgAccountRefresh(ctx context.Context, req *publish.TgAccountRefreshReq) (res *publish.TgAccountRefreshRes, err error) {
	list, err := service.SysPublish().AdminTgAccountRefresh(ctx, &req.TgAccountRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountRefreshModel{}
	}
	res = &publish.TgAccountRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) ChannelList(ctx context.Context, req *publish.ChannelListReq) (res *publish.ChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	res = new(publish.ChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) ChannelSave(ctx context.Context, req *publish.ChannelSaveReq) (res *publish.ChannelSaveRes, err error) {
	if err = service.SysPublish().AdminChannelSave(ctx, &req.ChannelSaveInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelSaveRes{}
	return
}

func (c *cPublishAdmin) ChannelDelete(ctx context.Context, req *publish.ChannelDeleteReq) (res *publish.ChannelDeleteRes, err error) {
	if err = service.SysPublish().AdminChannelDelete(ctx, &req.ChannelDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelDeleteRes{}
	return
}

func (c *cPublishAdmin) ChannelBatchBots(ctx context.Context, req *publish.ChannelBatchBotsReq) (res *publish.ChannelBatchBotsRes, err error) {
	if err = service.SysPublish().AdminChannelBatchBots(ctx, &req.ChannelBatchBotsInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelBatchBotsRes{}
	return
}

func (c *cPublishAdmin) ChannelRefresh(ctx context.Context, req *publish.ChannelRefreshReq) (res *publish.ChannelRefreshRes, err error) {
	list, err := service.SysPublish().AdminChannelRefresh(ctx, &req.ChannelRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelRefreshModel{}
	}
	res = &publish.ChannelRefreshRes{List: list}
	return
}
