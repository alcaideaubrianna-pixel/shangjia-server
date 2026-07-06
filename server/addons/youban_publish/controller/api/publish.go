package api

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublish{}
var PublishAuth = cPublishAuth{}
var PublishAdmin = cPublishAdmin{}

type cPublish struct{}
type cPublishAuth struct{}
type cPublishAdmin struct{}

func (c *cPublish) CurrentAccount(ctx context.Context, req *publish.CurrentAccountReq) (res *publish.CurrentAccountRes, err error) {
	data, err := service.SysPublish().CurrentAccount(ctx)
	if err != nil {
		return
	}
	res = &publish.CurrentAccountRes{CurrentAccountModel: data}
	return
}

func (c *cPublish) AccountSettingView(ctx context.Context, req *publish.AccountSettingViewReq) (res *publish.AccountSettingViewRes, err error) {
	data, err := service.SysPublish().MyAccountSettingView(ctx)
	if err != nil {
		return nil, err
	}
	res = &publish.AccountSettingViewRes{AccountSettingModel: data}
	return
}

func (c *cPublish) UpdateAccountPassword(ctx context.Context, req *publish.UpdateAccountPasswordReq) (res *publish.UpdateAccountPasswordRes, err error) {
	if err = service.SysPublish().UpdateAccountPassword(ctx, &req.UpdateAccountPasswordInp); err != nil {
		return nil, err
	}
	res = &publish.UpdateAccountPasswordRes{}
	return
}

func (c *cPublish) UpdateAccountProfile(ctx context.Context, req *publish.UpdateAccountProfileReq) (res *publish.UpdateAccountProfileRes, err error) {
	data, err := service.SysPublish().UpdateAccountProfile(ctx, &req.UpdateAccountProfileInp)
	if err != nil {
		return nil, err
	}
	res = &publish.UpdateAccountProfileRes{CurrentAccountModel: data}
	return
}

func (c *cPublishAuth) AccountLogin(ctx context.Context, req *publish.AccountLoginReq) (res *publish.AccountLoginRes, err error) {
	data, err := service.SysPublish().AccountLogin(ctx, &req.AccountLoginInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AccountLoginRes{AccountLoginModel: data}
	return
}

func (c *cPublishAuth) AccountRegister(ctx context.Context, req *publish.AccountRegisterReq) (res *publish.AccountRegisterRes, err error) {
	data, err := service.SysPublish().AccountRegister(ctx, &req.AccountRegisterInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AccountRegisterRes{AccountRegisterModel: data}
	return
}

func (c *cPublishAdmin) AccountList(ctx context.Context, req *publish.AdminAccountListReq) (res *publish.AdminAccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminAccountList(ctx, &req.AccountListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.AccountModel{}
	}
	res = new(publish.AdminAccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) AccountSave(ctx context.Context, req *publish.AdminAccountSaveReq) (res *publish.AdminAccountSaveRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountSaveRes{}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishAdmin) AccountResetPassword(ctx context.Context, req *publish.AdminAccountResetPasswordReq) (res *publish.AdminAccountResetPasswordRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountResetPassword(ctx, &req.AccountResetPasswordInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountResetPasswordRes{Password: saveRes.Password}
	return
}

func (c *cPublishAdmin) AccountDelete(ctx context.Context, req *publish.AdminAccountDeleteReq) (res *publish.AdminAccountDeleteRes, err error) {
	if err = service.SysPublish().AdminAccountDelete(ctx, &req.AccountDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminAccountDeleteRes{}
	return
}

func (c *cPublishAdmin) AccountSettingView(ctx context.Context, req *publish.AdminAccountSettingViewReq) (res *publish.AdminAccountSettingViewRes, err error) {
	data, err := service.SysPublish().AdminAccountSettingView(ctx, &req.AccountSettingViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountSettingViewRes{AccountSettingModel: data}
	return
}

func (c *cPublishAdmin) AccountSettingSave(ctx context.Context, req *publish.AdminAccountSettingSaveReq) (res *publish.AdminAccountSettingSaveRes, err error) {
	data, err := service.SysPublish().AdminAccountSettingSave(ctx, &req.AccountSettingSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountSettingSaveRes{AccountSettingModel: data}
	return
}

func (c *cPublishAdmin) BotList(ctx context.Context, req *publish.AdminBotListReq) (res *publish.AdminBotListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	res = new(publish.AdminBotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) BotSave(ctx context.Context, req *publish.AdminBotSaveReq) (res *publish.AdminBotSaveRes, err error) {
	if err = service.SysPublish().AdminBotSave(ctx, &req.BotSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminBotSaveRes{}
	return
}

func (c *cPublishAdmin) BotDelete(ctx context.Context, req *publish.AdminBotDeleteReq) (res *publish.AdminBotDeleteRes, err error) {
	if err = service.SysPublish().AdminBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminBotDeleteRes{}
	return
}

func (c *cPublishAdmin) BotRefresh(ctx context.Context, req *publish.AdminBotRefreshReq) (res *publish.AdminBotRefreshRes, err error) {
	list, err := service.SysPublish().AdminBotRefresh(ctx, &req.BotRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &publish.AdminBotRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) TgAccountList(ctx context.Context, req *publish.AdminTgAccountListReq) (res *publish.AdminTgAccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgAccountList(ctx, &req.TgAccountListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	res = new(publish.AdminTgAccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgAccountStartLogin(ctx context.Context, req *publish.AdminTgAccountStartLoginReq) (res *publish.AdminTgAccountStartLoginRes, err error) {
	item, err := service.SysPublish().AdminTgAccountStartLogin(ctx, &req.TgAccountStartLoginInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountStartLoginRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountLoginStatus(ctx context.Context, req *publish.AdminTgAccountLoginStatusReq) (res *publish.AdminTgAccountLoginStatusRes, err error) {
	item, err := service.SysPublish().AdminTgAccountLoginStatus(ctx, &req.TgAccountLoginStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountLoginStatusRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountPassword(ctx context.Context, req *publish.AdminTgAccountPasswordReq) (res *publish.AdminTgAccountPasswordRes, err error) {
	item, err := service.SysPublish().AdminTgAccountPassword(ctx, &req.TgAccountPasswordInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountPasswordRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountDelete(ctx context.Context, req *publish.AdminTgAccountDeleteReq) (res *publish.AdminTgAccountDeleteRes, err error) {
	if err = service.SysPublish().AdminTgAccountDelete(ctx, &req.TgAccountDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountDeleteRes{}
	return
}

func (c *cPublishAdmin) TgAccountRefresh(ctx context.Context, req *publish.AdminTgAccountRefreshReq) (res *publish.AdminTgAccountRefreshRes, err error) {
	list, err := service.SysPublish().AdminTgAccountRefresh(ctx, &req.TgAccountRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountRefreshModel{}
	}
	res = &publish.AdminTgAccountRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) ChannelList(ctx context.Context, req *publish.AdminChannelListReq) (res *publish.AdminChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	res = new(publish.AdminChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) ChannelSave(ctx context.Context, req *publish.AdminChannelSaveReq) (res *publish.AdminChannelSaveRes, err error) {
	if err = service.SysPublish().AdminChannelSave(ctx, &req.ChannelSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminChannelSaveRes{}
	return
}

func (c *cPublishAdmin) ChannelCacheList(ctx context.Context, req *publish.AdminChannelCacheListReq) (res *publish.AdminChannelCacheListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminChannelCacheList(ctx, &req.ChannelCacheListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelCacheModel{}
	}
	res = new(publish.AdminChannelCacheListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) ChannelCacheRefresh(ctx context.Context, req *publish.AdminChannelCacheRefreshReq) (res *publish.AdminChannelCacheRefreshRes, err error) {
	item, err := service.SysPublish().AdminChannelCacheRefresh(ctx, &req.ChannelCacheRefreshInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminChannelCacheRefreshRes{ChannelCacheRefreshModel: item}
	return
}

func (c *cPublishAdmin) ChannelCheck(ctx context.Context, req *publish.AdminChannelCheckReq) (res *publish.AdminChannelCheckRes, err error) {
	item, err := service.SysPublish().AdminChannelCheck(ctx, &req.ChannelCheckInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminChannelCheckRes{ChannelCheckModel: item}
	return
}

func (c *cPublishAdmin) ChannelDelete(ctx context.Context, req *publish.AdminChannelDeleteReq) (res *publish.AdminChannelDeleteRes, err error) {
	if err = service.SysPublish().AdminChannelDelete(ctx, &req.ChannelDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminChannelDeleteRes{}
	return
}

func (c *cPublishAdmin) ChannelBatchBots(ctx context.Context, req *publish.AdminChannelBatchBotsReq) (res *publish.AdminChannelBatchBotsRes, err error) {
	if err = service.SysPublish().AdminChannelBatchBots(ctx, &req.ChannelBatchBotsInp); err != nil {
		return nil, err
	}
	res = &publish.AdminChannelBatchBotsRes{}
	return
}

func (c *cPublishAdmin) ChannelRefresh(ctx context.Context, req *publish.AdminChannelRefreshReq) (res *publish.AdminChannelRefreshRes, err error) {
	list, err := service.SysPublish().AdminChannelRefresh(ctx, &req.ChannelRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelRefreshModel{}
	}
	res = &publish.AdminChannelRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) UploadMedia(ctx context.Context, req *publish.AdminUploadMediaReq) (res *publish.AdminUploadMediaRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	poster := g.RequestFromCtx(ctx).GetUploadFile("poster")
	data, err := service.SysPublish().AdminMediaUpload(ctx, &req.MediaUploadInp, file, poster)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminUploadMediaRes{MediaModel: data}
	return
}

func (c *cPublishAdmin) SortMedia(ctx context.Context, req *publish.AdminSortMediaReq) (res *publish.AdminSortMediaRes, err error) {
	if err = service.SysPublish().AdminMediaSort(ctx, &req.MediaSortInp); err != nil {
		return nil, err
	}
	res = &publish.AdminSortMediaRes{}
	return
}

func (c *cPublishAdmin) DeleteMedia(ctx context.Context, req *publish.AdminDeleteMediaReq) (res *publish.AdminDeleteMediaRes, err error) {
	if err = service.SysPublish().AdminMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminDeleteMediaRes{}
	return
}

func (c *cPublishAdmin) ProfileList(ctx context.Context, req *publish.AdminProfileListReq) (res *publish.AdminProfileListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminProfileList(ctx, &req.ProfileListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	res = new(publish.AdminProfileListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) ProfileView(ctx context.Context, req *publish.AdminProfileViewReq) (res *publish.AdminProfileViewRes, err error) {
	data, err := service.SysPublish().AdminProfileView(ctx, &req.ProfileViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileViewRes{ProfileViewModel: data}
	return
}

func (c *cPublishAdmin) ProfileSave(ctx context.Context, req *publish.AdminProfileSaveReq) (res *publish.AdminProfileSaveRes, err error) {
	data, err := service.SysPublish().AdminProfileSave(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileSaveRes{}
	if data != nil {
		res.Id = data.Id
		res.Uuid = data.Uuid
		res.TaskId = data.TaskId
	}
	return
}

func (c *cPublishAdmin) ProfileDelete(ctx context.Context, req *publish.AdminProfileDeleteReq) (res *publish.AdminProfileDeleteRes, err error) {
	if err = service.SysPublish().AdminProfileDelete(ctx, &req.ProfileDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminProfileDeleteRes{}
	return
}

func (c *cPublishAdmin) ProfileStatus(ctx context.Context, req *publish.AdminProfileStatusReq) (res *publish.AdminProfileStatusRes, err error) {
	if err = service.SysPublish().AdminProfileStatus(ctx, &req.ProfileStatusInp); err != nil {
		return nil, err
	}
	res = &publish.AdminProfileStatusRes{}
	return
}

func (c *cPublishAdmin) NoteList(ctx context.Context, req *publish.AdminNoteListReq) (res *publish.AdminNoteListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminNoteList(ctx, &req.NoteListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.AdminNoteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TagList(ctx context.Context, req *publish.AdminTagListReq) (res *publish.AdminTagListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTagList(ctx, &req.TagListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TagModel{}
	}
	res = new(publish.AdminTagListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TagSave(ctx context.Context, req *publish.AdminTagSaveReq) (res *publish.AdminTagSaveRes, err error) {
	if err = service.SysPublish().AdminTagSave(ctx, &req.TagSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminTagSaveRes{}
	return
}

func (c *cPublishAdmin) TagDelete(ctx context.Context, req *publish.AdminTagDeleteReq) (res *publish.AdminTagDeleteRes, err error) {
	if err = service.SysPublish().AdminTagDelete(ctx, &req.TagDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminTagDeleteRes{}
	return
}

func (c *cPublishAdmin) CityForward(ctx context.Context, req *publish.AdminCityForwardReq) (res *publish.AdminCityForwardRes, err error) {
	data, err := service.SysPublish().AdminCityForward(ctx, &req.CityForwardInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminCityForwardRes{CityForwardModel: data}
	return
}

func (c *cPublishAdmin) ProfileStats(ctx context.Context, req *publish.AdminProfileStatsReq) (res *publish.AdminProfileStatsRes, err error) {
	data, err := service.SysPublish().AdminProfileStats(ctx, &req.TrendInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileStatsRes{ProfileStatsModel: data}
	return
}

func (c *cPublishAdmin) PublishConfigView(ctx context.Context, req *publish.AdminPublishConfigViewReq) (res *publish.AdminPublishConfigViewRes, err error) {
	data, err := service.SysConfig().PublishConfigView(ctx, &req.PublishConfigViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminPublishConfigViewRes{PublishConfigViewModel: data}
	return
}

func (c *cPublishAdmin) PublishConfigSave(ctx context.Context, req *publish.AdminPublishConfigSaveReq) (res *publish.AdminPublishConfigSaveRes, err error) {
	if err = service.SysConfig().PublishConfigSave(ctx, &req.PublishConfigSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminPublishConfigSaveRes{}
	return
}

func (c *cPublishAdmin) AutoDeleteConfigView(ctx context.Context, req *publish.AdminAutoDeleteConfigViewReq) (res *publish.AdminAutoDeleteConfigViewRes, err error) {
	data, err := service.SysConfig().AutoDeleteConfigView(ctx, &req.AutoDeleteConfigViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAutoDeleteConfigViewRes{AutoDeleteConfigViewModel: data}
	return
}

func (c *cPublishAdmin) AutoDeleteConfigSave(ctx context.Context, req *publish.AdminAutoDeleteConfigSaveReq) (res *publish.AdminAutoDeleteConfigSaveRes, err error) {
	if err = service.SysConfig().AutoDeleteConfigSave(ctx, &req.AutoDeleteConfigSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminAutoDeleteConfigSaveRes{}
	return
}

func (c *cPublishAdmin) AntiScanConfigView(ctx context.Context, req *publish.AdminAntiScanConfigViewReq) (res *publish.AdminAntiScanConfigViewRes, err error) {
	data, err := service.SysConfig().AntiScanConfigView(ctx, &req.AntiScanConfigViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanConfigViewRes{AntiScanConfigViewModel: data}
	return
}

func (c *cPublishAdmin) AntiScanConfigSave(ctx context.Context, req *publish.AdminAntiScanConfigSaveReq) (res *publish.AdminAntiScanConfigSaveRes, err error) {
	if err = service.SysConfig().AntiScanConfigSave(ctx, &req.AntiScanConfigSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanConfigSaveRes{}
	return
}

func (c *cPublish) MyTaskList(ctx context.Context, req *publish.MyTaskListReq) (res *publish.MyTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().MyTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.MyTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) SaveTask(ctx context.Context, req *publish.SaveTaskReq) (res *publish.SaveTaskRes, err error) {
	id, err := service.SysPublish().MyTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.SaveTaskRes{Id: id}
	return
}

func (c *cPublish) SubmitTask(ctx context.Context, req *publish.SubmitTaskReq) (res *publish.SubmitTaskRes, err error) {
	err = service.SysPublish().MyTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.SubmitTaskRes{}
	return
}

func (c *cPublish) CancelTask(ctx context.Context, req *publish.CancelTaskReq) (res *publish.CancelTaskRes, err error) {
	err = service.SysPublish().MyTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.CancelTaskRes{}
	return
}

func (c *cPublish) MyImportTaskList(ctx context.Context, req *publish.MyImportTaskListReq) (res *publish.MyImportTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().MyImportTaskList(ctx, &req.ImportTaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportTaskModel{}
	}
	res = new(publish.MyImportTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyImportTaskView(ctx context.Context, req *publish.MyImportTaskViewReq) (res *publish.MyImportTaskViewRes, err error) {
	data, err := service.SysPublish().MyImportTaskView(ctx, &req.ImportTaskViewInp)
	if err != nil {
		return
	}
	res = &publish.MyImportTaskViewRes{ImportTaskModel: data}
	return
}

func (c *cPublish) UploadMedia(ctx context.Context, req *publish.UploadMediaReq) (res *publish.UploadMediaRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	poster := g.RequestFromCtx(ctx).GetUploadFile("poster")
	data, err := service.SysPublish().MyMediaUpload(ctx, &req.MediaUploadInp, file, poster)
	if err != nil {
		return nil, err
	}
	res = &publish.UploadMediaRes{MediaModel: data}
	return
}

func (c *cPublish) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().MyMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MediaListRes{List: list}
	return
}

func (c *cPublish) DeleteMedia(ctx context.Context, req *publish.DeleteMediaReq) (res *publish.DeleteMediaRes, err error) {
	if err = service.SysPublish().MyMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.DeleteMediaRes{}
	return
}

func (c *cPublish) SortMedia(ctx context.Context, req *publish.SortMediaReq) (res *publish.SortMediaRes, err error) {
	if err = service.SysPublish().MyMediaSort(ctx, &req.MediaSortInp); err != nil {
		return nil, err
	}
	res = &publish.SortMediaRes{}
	return
}

func (c *cPublish) MyProfileList(ctx context.Context, req *publish.MyProfileListReq) (res *publish.MyProfileListRes, err error) {
	list, totalCount, err := service.SysPublish().MyProfileList(ctx, &req.ProfileListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	res = new(publish.MyProfileListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyChannelList(ctx context.Context, req *publish.MyChannelListReq) (res *publish.MyChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().MyChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	res = new(publish.MyChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyProfileView(ctx context.Context, req *publish.MyProfileViewReq) (res *publish.MyProfileViewRes, err error) {
	data, err := service.SysPublish().MyProfileView(ctx, &req.ProfileViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileViewRes{ProfileViewModel: data}
	return
}

func (c *cPublish) MyProfileOptions(ctx context.Context, req *publish.MyProfileOptionsReq) (res *publish.MyProfileOptionsRes, err error) {
	data, err := service.SysPublish().MyProfileOptions(ctx)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileOptionsRes{ProfileOptionsModel: data}
	return
}

func (c *cPublish) MyProfileSave(ctx context.Context, req *publish.MyProfileSaveReq) (res *publish.MyProfileSaveRes, err error) {
	data, err := service.SysPublish().MyProfileSave(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileSaveRes{}
	if data != nil {
		res.Id = data.Id
		res.Uuid = data.Uuid
		res.TaskId = data.TaskId
	}
	return
}

func (c *cPublish) MyProfileDelete(ctx context.Context, req *publish.MyProfileDeleteReq) (res *publish.MyProfileDeleteRes, err error) {
	if err = service.SysPublish().MyProfileDelete(ctx, &req.ProfileDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MyProfileDeleteRes{}
	return
}

func (c *cPublish) MyProfileStatus(ctx context.Context, req *publish.MyProfileStatusReq) (res *publish.MyProfileStatusRes, err error) {
	if err = service.SysPublish().MyProfileStatus(ctx, &req.ProfileStatusInp); err != nil {
		return nil, err
	}
	res = &publish.MyProfileStatusRes{}
	return
}

func (c *cPublish) MyProfileImageSearch(ctx context.Context, req *publish.MyProfileImageSearchReq) (res *publish.MyProfileImageSearchRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("image")
	if file == nil {
		return nil, gerror.New("请先上传要搜索的图片")
	}
	list, totalCount, err := service.SysPublish().MyProfileImageSearch(ctx, &req.ProfileImageSearchInp, file)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.MyProfileImageSearchRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyNoteList(ctx context.Context, req *publish.MyNoteListReq) (res *publish.MyNoteListRes, err error) {
	list, totalCount, err := service.SysPublish().MyNoteList(ctx, &req.NoteListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.MyNoteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyTagList(ctx context.Context, req *publish.MyTagListReq) (res *publish.MyTagListRes, err error) {
	list, totalCount, err := service.SysPublish().MyTagList(ctx, &req.TagListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TagModel{}
	}
	res = new(publish.MyTagListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyTagSave(ctx context.Context, req *publish.MyTagSaveReq) (res *publish.MyTagSaveRes, err error) {
	if err = service.SysPublish().MyTagSave(ctx, &req.TagSaveInp); err != nil {
		return nil, err
	}
	res = &publish.MyTagSaveRes{}
	return
}

func (c *cPublish) MyTagDelete(ctx context.Context, req *publish.MyTagDeleteReq) (res *publish.MyTagDeleteRes, err error) {
	if err = service.SysPublish().MyTagDelete(ctx, &req.TagDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MyTagDeleteRes{}
	return
}

func (c *cPublish) MyCityForward(ctx context.Context, req *publish.MyCityForwardReq) (res *publish.MyCityForwardRes, err error) {
	data, err := service.SysPublish().MyCityForward(ctx, &req.CityForwardInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyCityForwardRes{CityForwardModel: data}
	return
}

func (c *cPublish) MyProfileStats(ctx context.Context, req *publish.MyProfileStatsReq) (res *publish.MyProfileStatsRes, err error) {
	data, err := service.SysPublish().MyProfileStats(ctx, &req.TrendInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileStatsRes{ProfileStatsModel: data}
	return
}

func (c *cPublish) TelegramLoginStart(ctx context.Context, req *publish.TelegramLoginStartReq) (res *publish.TelegramLoginStartRes, err error) {
	data, err := service.SysPublish().TelegramLoginStart(ctx, &req.TelegramLoginStartInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TelegramLoginStartRes{TelegramLoginModel: data}
	return
}

func (c *cPublish) TelegramLoginStatus(ctx context.Context, req *publish.TelegramLoginStatusReq) (res *publish.TelegramLoginStatusRes, err error) {
	data, err := service.SysPublish().TelegramLoginStatus(ctx, &req.TelegramLoginStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TelegramLoginStatusRes{TelegramLoginModel: data}
	return
}

func (c *cPublish) TelegramLoginPassword(ctx context.Context, req *publish.TelegramLoginPasswordReq) (res *publish.TelegramLoginPasswordRes, err error) {
	data, err := service.SysPublish().TelegramLoginPassword(ctx, &req.TelegramLoginPasswordInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TelegramLoginPasswordRes{TelegramLoginModel: data}
	return
}
