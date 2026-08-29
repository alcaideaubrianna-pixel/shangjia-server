package api

import (
	"context"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

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

func (c *cPublish) AccountSettingSave(ctx context.Context, req *publish.AccountSettingSaveReq) (res *publish.AccountSettingSaveRes, err error) {
	in := sysin.AccountSettingSaveInp{
		EnableSuffix:    req.EnableSuffix,
		SuffixContent:   req.SuffixContent,
		EnableTitleMark: req.EnableTitleMark,
		MarkMode:        req.MarkMode,
		NumberSource:    req.NumberSource,
		CustomMarkText:  req.CustomMarkText,
		MarkPosition:    req.MarkPosition,
	}
	fillAccountSettingSaveInpFromRequest(ctx, &in)
	data, err := service.SysPublish().MyAccountSettingSave(ctx, &in)
	if err != nil {
		return nil, err
	}
	res = &publish.AccountSettingSaveRes{AccountSettingModel: data}
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

func (c *cPublish) InviteInfo(ctx context.Context, req *publish.InviteInfoReq) (res *publish.InviteInfoRes, err error) {
	data, err := service.SysPublish().InviteInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.InviteInfoRes{InviteInfoModel: data}, nil
}

func (c *cPublish) InviteList(ctx context.Context, req *publish.InviteListReq) (res *publish.InviteListRes, err error) {
	list, totalCount, err := service.SysPublish().InviteList(ctx, &req.InviteListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.InviteModel{}
	}
	res = new(publish.InviteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) InviteGenerate(ctx context.Context, req *publish.InviteGenerateReq) (res *publish.InviteGenerateRes, err error) {
	data, err := service.SysPublish().CreateInviteCode(ctx, &req.InviteCreateInp)
	if err != nil {
		return nil, err
	}
	return &publish.InviteGenerateRes{InviteCreateModel: data}, nil
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

func (c *cPublishAdmin) AccountTransferPreview(ctx context.Context, req *publish.AdminAccountTransferPreviewReq) (res *publish.AdminAccountTransferPreviewRes, err error) {
	preview, err := service.SysPublish().AdminAccountTransferPreview(ctx, &req.AccountTransferPreviewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountTransferPreviewRes{AccountTransferPreviewModel: preview}
	return
}

func (c *cPublishAdmin) AccountTransferProfiles(ctx context.Context, req *publish.AdminAccountTransferProfilesReq) (res *publish.AdminAccountTransferProfilesRes, err error) {
	transfer, err := service.SysPublish().AdminAccountTransferProfiles(ctx, &req.AccountTransferProfilesInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountTransferProfilesRes{AccountTransferProfilesModel: transfer}
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
	in := req.AccountSettingSaveInp
	fillAccountSettingSaveInpFromRequest(ctx, &in)
	data, err := service.SysPublish().AdminAccountSettingSave(ctx, &in)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAccountSettingSaveRes{AccountSettingModel: data}
	return
}

func fillAccountSettingSaveInpFromRequest(ctx context.Context, in *sysin.AccountSettingSaveInp) {
	if in == nil {
		return
	}
	r := ghttp.RequestFromCtx(ctx)
	if r == nil || len(r.GetBody()) == 0 {
		return
	}
	body := gjson.New(r.GetBody())
	if body == nil {
		return
	}
	in.AccountId = body.Get("accountId", in.AccountId).Int64()
	in.EnableSuffix = body.Get("enableSuffix", in.EnableSuffix).Int()
	in.SuffixContent = body.Get("suffixContent", in.SuffixContent).String()
	in.EnableTitleMark = body.Get("enableTitleMark", in.EnableTitleMark).Int()
	in.MarkMode = body.Get("markMode", in.MarkMode).String()
	in.NumberSource = body.Get("numberSource", in.NumberSource).String()
	in.CustomMarkText = body.Get("customMarkText", in.CustomMarkText).String()
	in.MarkPosition = body.Get("markPosition", in.MarkPosition).String()
	in.SharedResourceEnabled = body.Get("sharedResourceEnabled", in.SharedResourceEnabled).Int()
	in.TelegramBindingEnabled = body.Get("telegramBindingEnabled", in.TelegramBindingEnabled).Int()
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

func (c *cPublishAdmin) BotChannelCacheList(ctx context.Context, req *publish.AdminBotChannelCacheListReq) (res *publish.AdminBotChannelCacheListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminBotChannelCacheList(ctx, &req.BotChannelCacheListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotChannelCacheModel{}
	}
	res = new(publish.AdminBotChannelCacheListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) BotCreate(ctx context.Context, req *publish.AdminBotCreateReq) (res *publish.AdminBotCreateRes, err error) {
	data, err := service.SysPublish().AdminBotCreate(ctx, &req.BotCreateInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminBotCreateRes{BotModel: data}, nil
}

func (c *cPublishAdmin) BotUsernameCheck(ctx context.Context, req *publish.AdminBotUsernameCheckReq) (res *publish.AdminBotUsernameCheckRes, err error) {
	data, err := service.SysPublish().AdminBotUsernameCheck(ctx, &req.BotUsernameCheckInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminBotUsernameCheckRes{BotUsernameCheckModel: data}, nil
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

func (c *cPublishAdmin) TgAccountPhoneStart(ctx context.Context, req *publish.AdminTgAccountPhoneStartReq) (res *publish.AdminTgAccountPhoneStartRes, err error) {
	item, err := service.SysPublish().AdminTgAccountPhoneStart(ctx, &req.TgAccountPhoneStartInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountPhoneStartRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountCode(ctx context.Context, req *publish.AdminTgAccountCodeReq) (res *publish.AdminTgAccountCodeRes, err error) {
	item, err := service.SysPublish().AdminTgAccountCode(ctx, &req.TgAccountCodeInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgAccountCodeRes{TgAccountModel: item}
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

func (c *cPublishAdmin) ChannelCacheResolve(ctx context.Context, req *publish.AdminChannelCacheResolveReq) (res *publish.AdminChannelCacheResolveRes, err error) {
	list, err := service.SysPublish().AdminChannelCacheResolve(ctx, &req.ChannelCacheResolveInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelCacheResolveModel{}
	}
	res = &publish.AdminChannelCacheResolveRes{List: list}
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

func (c *cPublishAdmin) ChannelCacheRefreshStatus(ctx context.Context, req *publish.AdminChannelCacheRefreshStatusReq) (res *publish.AdminChannelCacheRefreshStatusRes, err error) {
	item, err := service.SysPublish().AdminChannelCacheRefreshStatus(ctx, &req.ChannelCacheRefreshStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminChannelCacheRefreshStatusRes{ChannelCacheRefreshModel: item}
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

func (c *cPublishAdmin) ChannelFullPush(ctx context.Context, req *publish.AdminChannelFullPushReq) (res *publish.AdminChannelFullPushRes, err error) {
	item, err := service.SysPublish().AdminChannelFullPush(ctx, &req.ChannelFullPushInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminChannelFullPushRes{ChannelFullPushModel: item}
	return
}

func (c *cPublishAdmin) ChannelCycleRun(ctx context.Context, req *publish.AdminChannelCycleRunReq) (res *publish.AdminChannelCycleRunRes, err error) {
	item, err := service.SysPublish().AdminChannelCycleRun(ctx, &req.ChannelCycleRunInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminChannelCycleRunRes{ChannelFullPushModel: item}, nil
}

func (c *cPublishAdmin) MessageTemplateList(ctx context.Context, req *publish.AdminMessageTemplateListReq) (res *publish.AdminMessageTemplateListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminMessageTemplateList(ctx, &req.MessageTemplateListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MessageTemplateModel{}
	}
	res = new(publish.AdminMessageTemplateListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) MessageTemplateSave(ctx context.Context, req *publish.AdminMessageTemplateSaveReq) (res *publish.AdminMessageTemplateSaveRes, err error) {
	item, err := service.SysPublish().AdminMessageTemplateSave(ctx, &req.MessageTemplateSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMessageTemplateSaveRes{MessageTemplateSaveModel: item}
	return
}

func (c *cPublishAdmin) MessageTemplateDelete(ctx context.Context, req *publish.AdminMessageTemplateDeleteReq) (res *publish.AdminMessageTemplateDeleteRes, err error) {
	if err = service.SysPublish().AdminMessageTemplateDelete(ctx, &req.MessageTemplateDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMessageTemplateDeleteRes{}
	return
}

func (c *cPublishAdmin) MessageTemplatePush(ctx context.Context, req *publish.AdminMessageTemplatePushReq) (res *publish.AdminMessageTemplatePushRes, err error) {
	item, err := service.SysPublish().AdminMessageTemplatePush(ctx, &req.MessageTemplatePushInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMessageTemplatePushRes{MessageTemplatePushModel: item}
	return
}

func (c *cPublishAdmin) MessageTemplateMediaUpload(ctx context.Context, req *publish.AdminMessageTemplateMediaUploadReq) (res *publish.AdminMessageTemplateMediaUploadRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	poster := g.RequestFromCtx(ctx).GetUploadFile("poster")
	data, err := service.SysPublish().AdminMessageTemplateMediaUpload(ctx, &req.MessageTemplateMediaUploadInp, file, poster)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMessageTemplateMediaUploadRes{MessageTemplateMediaModel: data}
	return
}

func (c *cPublishAdmin) MessagePushPlanList(ctx context.Context, req *publish.AdminMessagePushPlanListReq) (res *publish.AdminMessagePushPlanListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminMessagePushPlanList(ctx, &req.MessagePushPlanListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MessagePushPlanModel{}
	}
	res = new(publish.AdminMessagePushPlanListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) MessagePushPlanSave(ctx context.Context, req *publish.AdminMessagePushPlanSaveReq) (res *publish.AdminMessagePushPlanSaveRes, err error) {
	item, err := service.SysPublish().AdminMessagePushPlanSave(ctx, &req.MessagePushPlanSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMessagePushPlanSaveRes{MessagePushPlanSaveModel: item}
	return
}

func (c *cPublishAdmin) MessagePushPlanDelete(ctx context.Context, req *publish.AdminMessagePushPlanDeleteReq) (res *publish.AdminMessagePushPlanDeleteRes, err error) {
	if err = service.SysPublish().AdminMessagePushPlanDelete(ctx, &req.MessagePushPlanDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMessagePushPlanDeleteRes{}
	return
}

func (c *cPublishAdmin) MessagePushPlanStatus(ctx context.Context, req *publish.AdminMessagePushPlanStatusReq) (res *publish.AdminMessagePushPlanStatusRes, err error) {
	if err = service.SysPublish().AdminMessagePushPlanStatus(ctx, &req.MessagePushPlanStatusInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMessagePushPlanStatusRes{}
	return
}

func (c *cPublishAdmin) QuickPushPlanList(ctx context.Context, req *publish.AdminQuickPushPlanListReq) (res *publish.AdminQuickPushPlanListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminQuickPushPlanList(ctx, &req.QuickPushPlanListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.QuickPushPlanModel{}
	}
	res = new(publish.AdminQuickPushPlanListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) QuickPushPlanSave(ctx context.Context, req *publish.AdminQuickPushPlanSaveReq) (res *publish.AdminQuickPushPlanSaveRes, err error) {
	item, err := service.SysPublish().AdminQuickPushPlanSave(ctx, &req.QuickPushPlanSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminQuickPushPlanSaveRes{QuickPushPlanSaveModel: item}
	return
}

func (c *cPublishAdmin) QuickPushPlanDelete(ctx context.Context, req *publish.AdminQuickPushPlanDeleteReq) (res *publish.AdminQuickPushPlanDeleteRes, err error) {
	if err = service.SysPublish().AdminQuickPushPlanDelete(ctx, &req.QuickPushPlanDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminQuickPushPlanDeleteRes{}
	return
}

func (c *cPublishAdmin) QuickPushPlanStatus(ctx context.Context, req *publish.AdminQuickPushPlanStatusReq) (res *publish.AdminQuickPushPlanStatusRes, err error) {
	if err = service.SysPublish().AdminQuickPushPlanStatus(ctx, &req.QuickPushPlanStatusInp); err != nil {
		return nil, err
	}
	res = &publish.AdminQuickPushPlanStatusRes{}
	return
}

func (c *cPublishAdmin) UploadMedia(ctx context.Context, req *publish.AdminUploadMediaReq) (res *publish.AdminUploadMediaRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	poster := g.RequestFromCtx(ctx).GetUploadFile("poster")
	originalFile := g.RequestFromCtx(ctx).GetUploadFile("originalFile")
	data, err := service.SysPublish().AdminMediaUpload(ctx, &req.MediaUploadInp, file, poster, originalFile)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminUploadMediaRes{MediaModel: data}
	return
}

func (c *cPublishAdmin) MediaMultipartCheck(ctx context.Context, req *publish.AdminMediaMultipartCheckReq) (res *publish.AdminMediaMultipartCheckRes, err error) {
	data, err := service.SysPublish().MediaMultipartCheck(ctx, req.CheckMultipartParams)
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaMultipartCheckRes{CheckMultipartModel: data}, nil
}

func (c *cPublishAdmin) MediaMultipartPart(ctx context.Context, req *publish.AdminMediaMultipartPartReq) (res *publish.AdminMediaMultipartPartRes, err error) {
	if req.UploadPartParams == nil {
		return nil, gerror.New("分片上传参数不能为空")
	}
	req.UploadPartParams.File = g.RequestFromCtx(ctx).GetUploadFile("file")
	if req.UploadPartParams.File == nil {
		return nil, gerror.New("没有找到上传的分片文件")
	}
	data, err := service.SysPublish().MediaMultipartPart(ctx, req.UploadPartParams)
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaMultipartPartRes{UploadPartModel: data}, nil
}

func (c *cPublishAdmin) MediaMultipartAttach(ctx context.Context, req *publish.AdminMediaMultipartAttachReq) (res *publish.AdminMediaMultipartAttachRes, err error) {
	data, err := service.SysPublish().AdminMediaMultipartAttach(ctx, &req.MediaMultipartAttachInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaMultipartAttachRes{MediaModel: data}, nil
}

func (c *cPublishAdmin) MediaDirectUploadCreate(ctx context.Context, req *publish.AdminMediaDirectUploadCreateReq) (*publish.AdminMediaDirectUploadCreateRes, error) {
	data, err := service.SysPublish().AdminMediaDirectUploadCreate(ctx, &req.MediaDirectUploadCreateInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaDirectUploadCreateRes{MediaDirectUploadCreateModel: data}, nil
}

func (c *cPublishAdmin) MediaDirectUploadSign(ctx context.Context, req *publish.AdminMediaDirectUploadSignReq) (*publish.AdminMediaDirectUploadSignRes, error) {
	data, err := service.SysPublish().AdminMediaDirectUploadSign(ctx, &req.MediaDirectUploadSignInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaDirectUploadSignRes{MediaDirectUploadSignModel: data}, nil
}

func (c *cPublishAdmin) MediaDirectUploadComplete(ctx context.Context, req *publish.AdminMediaDirectUploadCompleteReq) (*publish.AdminMediaDirectUploadCompleteRes, error) {
	data, err := service.SysPublish().AdminMediaDirectUploadComplete(ctx, &req.MediaDirectUploadCompleteInp, g.RequestFromCtx(ctx).GetUploadFile("poster"))
	if err != nil {
		return nil, err
	}
	return &publish.AdminMediaDirectUploadCompleteRes{MediaModel: data}, nil
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

func (c *cPublishAdmin) ProfileEdit(ctx context.Context, req *publish.AdminProfileEditReq) (res *publish.AdminProfileEditRes, err error) {
	data, err := service.SysPublish().AdminProfileEdit(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileEditRes{}
	if data != nil {
		res.Id = data.Id
		res.Uuid = data.Uuid
	}
	return
}

func (c *cPublishAdmin) ProfileCreate(ctx context.Context, req *publish.AdminProfileCreateReq) (res *publish.AdminProfileCreateRes, err error) {
	data, err := service.SysPublish().AdminProfileCreate(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileCreateRes{}
	if data != nil {
		res.Id, res.Uuid = data.Id, data.Uuid
	}
	return
}

func (c *cPublishAdmin) ProfilePublish(ctx context.Context, req *publish.AdminProfilePublishReq) (res *publish.AdminProfilePublishRes, err error) {
	if err = service.SysPublish().AdminProfilePublish(ctx, &req.AdminProfilePublishInp); err != nil {
		return nil, err
	}
	return &publish.AdminProfilePublishRes{}, nil
}

func (c *cPublishAdmin) ProfileBatchCancel(ctx context.Context, req *publish.AdminProfileBatchCancelReq) (res *publish.AdminProfileBatchCancelRes, err error) {
	data, err := service.SysPublish().AdminProfileBatchCancel(ctx, &req.AdminProfileBatchCancelInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminProfileBatchCancelRes{AdminProfileBatchCancelModel: data}, nil
}

func (c *cPublishAdmin) ProfileDelete(ctx context.Context, req *publish.AdminProfileDeleteReq) (res *publish.AdminProfileDeleteRes, err error) {
	if err = service.SysPublish().AdminProfileDelete(ctx, &req.ProfileDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminProfileDeleteRes{}
	return
}

func (c *cPublishAdmin) ProfileStatus(ctx context.Context, req *publish.AdminProfileStatusReq) (res *publish.AdminProfileStatusRes, err error) {
	data, err := service.SysPublish().AdminProfileStatus(ctx, &req.ProfileStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminProfileStatusRes{ProfileStatusModel: data}
	return
}

func (c *cPublishAdmin) ProfileImageSearch(ctx context.Context, req *publish.AdminProfileImageSearchReq) (res *publish.AdminProfileImageSearchRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("image")
	if file == nil {
		return nil, gerror.New("请先上传要搜索的图片")
	}
	list, totalCount, err := service.SysPublish().AdminProfileImageSearch(ctx, &req.ProfileImageSearchInp, file)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.AdminProfileImageSearchRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgMessageRepairStart(ctx context.Context, req *publish.AdminTgMessageRepairStartReq) (res *publish.AdminTgMessageRepairStartRes, err error) {
	data, err := service.SysPublish().AdminTgMessageRepairStart(ctx, &req.TgMessageRepairStartInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgMessageRepairStartRes{TgMessageRepairModel: data}
	return
}

func (c *cPublishAdmin) TgMessageRepairView(ctx context.Context, req *publish.AdminTgMessageRepairViewReq) (res *publish.AdminTgMessageRepairViewRes, err error) {
	data, err := service.SysPublish().AdminTgMessageRepairView(ctx, &req.TgMessageRepairViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminTgMessageRepairViewRes{TgMessageRepairModel: data}
	return
}

func (c *cPublishAdmin) NoteList(ctx context.Context, req *publish.AdminNoteListReq) (res *publish.AdminNoteListRes, err error) {
	data, err := service.SysPublish().AdminNoteList(ctx, &req.NoteListInp)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &sysin.AdminNotePageModel{List: []*sysin.AdminNoteListModel{}}
	} else if data.List == nil {
		data.List = []*sysin.AdminNoteListModel{}
	}
	return &publish.AdminNoteListRes{AdminNotePageModel: data}, nil
}

func (c *cPublishAdmin) NoteBatchIds(ctx context.Context, req *publish.AdminNoteBatchIdsReq) (res *publish.AdminNoteBatchIdsRes, err error) {
	data, err := service.SysPublish().AdminNoteBatchIds(ctx, &req.NoteListInp)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &sysin.AdminNoteBatchIdsModel{Ids: []int64{}}
	} else if data.Ids == nil {
		data.Ids = []int64{}
	}
	return &publish.AdminNoteBatchIdsRes{AdminNoteBatchIdsModel: data}, nil
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

func (c *cPublish) MyImportRunList(ctx context.Context, req *publish.MyImportRunListReq) (res *publish.MyImportRunListRes, err error) {
	list, totalCount, err := service.SysPublish().MyImportRunList(ctx, &req.ImportRunListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunModel{}
	}
	res = new(publish.MyImportRunListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyImportRunLogList(ctx context.Context, req *publish.MyImportRunLogListReq) (res *publish.MyImportRunLogListRes, err error) {
	list, totalCount, err := service.SysPublish().MyImportRunLogList(ctx, &req.ImportRunLogListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ImportRunLogModel{}
	}
	res = new(publish.MyImportRunLogListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) UploadMedia(ctx context.Context, req *publish.UploadMediaReq) (res *publish.UploadMediaRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	poster := g.RequestFromCtx(ctx).GetUploadFile("poster")
	originalFile := g.RequestFromCtx(ctx).GetUploadFile("originalFile")
	data, err := service.SysPublish().MyMediaUpload(ctx, &req.MediaUploadInp, file, poster, originalFile)
	if err != nil {
		return nil, err
	}
	res = &publish.UploadMediaRes{MediaModel: data}
	return
}

func (c *cPublish) MediaMultipartCheck(ctx context.Context, req *publish.MediaMultipartCheckReq) (res *publish.MediaMultipartCheckRes, err error) {
	data, err := service.SysPublish().MediaMultipartCheck(ctx, req.CheckMultipartParams)
	if err != nil {
		return nil, err
	}
	return &publish.MediaMultipartCheckRes{CheckMultipartModel: data}, nil
}

func (c *cPublish) MediaMultipartPart(ctx context.Context, req *publish.MediaMultipartPartReq) (res *publish.MediaMultipartPartRes, err error) {
	if req.UploadPartParams == nil {
		return nil, gerror.New("分片上传参数不能为空")
	}
	req.UploadPartParams.File = g.RequestFromCtx(ctx).GetUploadFile("file")
	if req.UploadPartParams.File == nil {
		return nil, gerror.New("没有找到上传的分片文件")
	}
	data, err := service.SysPublish().MediaMultipartPart(ctx, req.UploadPartParams)
	if err != nil {
		return nil, err
	}
	return &publish.MediaMultipartPartRes{UploadPartModel: data}, nil
}

func (c *cPublish) MediaMultipartAttach(ctx context.Context, req *publish.MediaMultipartAttachReq) (res *publish.MediaMultipartAttachRes, err error) {
	data, err := service.SysPublish().MyMediaMultipartAttach(ctx, &req.MediaMultipartAttachInp)
	if err != nil {
		return nil, err
	}
	return &publish.MediaMultipartAttachRes{MediaModel: data}, nil
}

func (c *cPublish) MediaDirectUploadCreate(ctx context.Context, req *publish.MediaDirectUploadCreateReq) (*publish.MediaDirectUploadCreateRes, error) {
	data, err := service.SysPublish().MyMediaDirectUploadCreate(ctx, &req.MediaDirectUploadCreateInp)
	if err != nil {
		return nil, err
	}
	return &publish.MediaDirectUploadCreateRes{MediaDirectUploadCreateModel: data}, nil
}

func (c *cPublish) MediaDirectUploadSign(ctx context.Context, req *publish.MediaDirectUploadSignReq) (*publish.MediaDirectUploadSignRes, error) {
	data, err := service.SysPublish().MyMediaDirectUploadSign(ctx, &req.MediaDirectUploadSignInp)
	if err != nil {
		return nil, err
	}
	return &publish.MediaDirectUploadSignRes{MediaDirectUploadSignModel: data}, nil
}

func (c *cPublish) MediaDirectUploadComplete(ctx context.Context, req *publish.MediaDirectUploadCompleteReq) (*publish.MediaDirectUploadCompleteRes, error) {
	data, err := service.SysPublish().MyMediaDirectUploadComplete(ctx, &req.MediaDirectUploadCompleteInp, g.RequestFromCtx(ctx).GetUploadFile("poster"))
	if err != nil {
		return nil, err
	}
	return &publish.MediaDirectUploadCompleteRes{MediaModel: data}, nil
}

func (c *cPublish) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().MyMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MediaListRes{List: list}
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

func (c *cPublish) MyProfileEdit(ctx context.Context, req *publish.MyProfileEditReq) (res *publish.MyProfileEditRes, err error) {
	data, err := service.SysPublish().MyProfileEdit(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileEditRes{}
	if data != nil {
		res.Id = data.Id
		res.Uuid = data.Uuid
	}
	return
}

func (c *cPublish) MyProfileCreate(ctx context.Context, req *publish.MyProfileCreateReq) (res *publish.MyProfileCreateRes, err error) {
	data, err := service.SysPublish().MyProfileCreate(ctx, &req.ProfileSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileCreateRes{}
	if data != nil {
		res.Id, res.Uuid = data.Id, data.Uuid
	}
	return
}

func (c *cPublish) MyProfilePublish(ctx context.Context, req *publish.MyProfilePublishReq) (res *publish.MyProfilePublishRes, err error) {
	if err = service.SysPublish().MyProfilePublish(ctx, &req.ProfileViewInp); err != nil {
		return nil, err
	}
	return &publish.MyProfilePublishRes{}, nil
}

func (c *cPublish) MyProfileDelete(ctx context.Context, req *publish.MyProfileDeleteReq) (res *publish.MyProfileDeleteRes, err error) {
	if err = service.SysPublish().MyProfileDelete(ctx, &req.ProfileDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MyProfileDeleteRes{}
	return
}

func (c *cPublish) MyProfileStatus(ctx context.Context, req *publish.MyProfileStatusReq) (res *publish.MyProfileStatusRes, err error) {
	data, err := service.SysPublish().MyProfileStatus(ctx, &req.ProfileStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyProfileStatusRes{ProfileStatusModel: data}
	return
}

func (c *cPublish) MyTgMessageRepairStart(ctx context.Context, req *publish.MyTgMessageRepairStartReq) (res *publish.MyTgMessageRepairStartRes, err error) {
	data, err := service.SysPublish().MyTgMessageRepairStart(ctx, &req.TgMessageRepairStartInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyTgMessageRepairStartRes{TgMessageRepairModel: data}
	return
}

func (c *cPublish) MyTgMessageRepairView(ctx context.Context, req *publish.MyTgMessageRepairViewReq) (res *publish.MyTgMessageRepairViewRes, err error) {
	data, err := service.SysPublish().MyTgMessageRepairView(ctx, &req.TgMessageRepairViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyTgMessageRepairViewRes{TgMessageRepairModel: data}
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
