package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/contexts"
)

func (s *sSysBot) LoginCodeStart(ctx context.Context, in *sysin.CodeStartInp) (res *sysin.CodeStartModel, err error) {
	if in == nil {
		in = &sysin.CodeStartInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	return s.createCode(ctx, sysin.BotCodeSceneLogin, in.App, 0)
}

func (s *sSysBot) LoginCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (*sysin.CodeStatusModel, error) {
	return s.codeStatus(ctx, strings.TrimSpace(in.Code), sysin.BotCodeSceneLogin, "", 0)
}

func (s *sSysBot) BindCodeStart(ctx context.Context) (res *sysin.CodeStartModel, err error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	if _, err = s.ensureTelegramBindingAccess(ctx, user.App, user.Id); err != nil {
		return nil, err
	}
	return s.createCode(ctx, sysin.BotCodeSceneBind, user.App, user.Id)
}

func (s *sSysBot) BindCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (*sysin.CodeStatusModel, error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	if _, err := s.ensureTelegramBindingAccess(ctx, user.App, user.Id); err != nil {
		return nil, err
	}
	return s.codeStatus(ctx, strings.TrimSpace(in.Code), sysin.BotCodeSceneBind, user.App, user.Id)
}

func (s *sSysBot) BindInfo(ctx context.Context) (res *sysin.BindInfoModel, err error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	if _, err = s.ensureTelegramBindingAccess(ctx, user.App, user.Id); err != nil {
		return nil, err
	}
	botUsername := ""
	if bot, botErr := s.officialBot(ctx); botErr == nil && bot != nil {
		botUsername = bot.BotUsername
	} else if botErr != nil {
		g.Log().Warning(ctx, "读取官方Bot失败，绑定信息暂不返回快捷入口", g.Map{"err": botErr.Error()})
	}
	var row *sysin.BindInfoModel
	err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Fields("telegram_user_id,telegram_username").Where("app", user.App).Where("account_id", user.Id).Where("status", 1).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram绑定信息失败")
	}
	if row == nil {
		return &sysin.BindInfoModel{Bound: false, BotUsername: botUsername}, nil
	}
	row.Bound = strings.TrimSpace(row.TelegramUserId) != ""
	row.BotUsername = botUsername
	return row, nil
}

func (s *sSysBot) ensureTelegramBindingAccess(ctx context.Context, app string, accountId int64) (*publishsysin.AccountCapabilityModel, error) {
	capability, err := publishService.SysPublish().AccountCapability(ctx, app, accountId)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(app) == sysin.BotAppApi && capability.AccountType == publishsysin.PublishAccountTypeUploader && capability.TelegramBindingEnabled != 1 {
		return nil, gerror.New("当前上架账号未开启TG绑定权限，请联系管理员")
	}
	return capability, nil
}

func (s *sSysBot) createCode(ctx context.Context, scene, app string, accountId int64) (*sysin.CodeStartModel, error) {
	app = sysin.NormalizeApp(app)
	if scene == sysin.BotCodeSceneBind && accountId <= 0 {
		return nil, gerror.New("账号ID不能为空")
	}
	bot, err := s.officialBot(ctx)
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	expiresAt := now.Add(5 * time.Minute)
	code, err := s.uniqueCode(ctx)
	if err != nil {
		return nil, err
	}
	_, _ = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("scene", scene).Where("app", app).Where("account_id", accountId).Where("status", sysin.BotCodeStatusPending).Data(g.Map{"status": sysin.BotCodeStatusExpired, "error_message": "已生成新的验证码", "updated_at": now}).Update()
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Data(g.Map{"code": code, "scene": scene, "app": app, "account_id": accountId, "bot_id": bot.Id, "status": sysin.BotCodeStatusPending, "expires_at": expiresAt, "created_at": now, "updated_at": now}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "创建Telegram验证码失败")
	}
	return &sysin.CodeStartModel{Code: code, Scene: scene, App: app, BotUsername: bot.BotUsername, ExpiresAt: expiresAt}, nil
}

func (s *sSysBot) codeStatus(ctx context.Context, code, scene, app string, accountId int64) (*sysin.CodeStatusModel, error) {
	if strings.TrimSpace(code) == "" {
		return nil, gerror.New("验证码不能为空")
	}
	var codeRow *authCodeRow
	mod := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("scene", scene)
	if app != "" {
		mod = mod.Where("app", app)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&codeRow); err != nil {
		return nil, gerror.Wrap(err, "读取验证码状态失败")
	}
	if codeRow == nil {
		return nil, gerror.New("验证码不存在")
	}
	if codeRow.ExpiresAt != nil && codeRow.ExpiresAt.Before(gtime.Now()) && codeRow.Status == sysin.BotCodeStatusPending {
		_, _ = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Data(g.Map{"status": sysin.BotCodeStatusExpired, "error_message": "验证码已过期", "updated_at": gtime.Now()}).Update()
		codeRow.Status = sysin.BotCodeStatusExpired
		codeRow.ErrorMessage = "验证码已过期"
	}
	row := codeRow.statusModel()
	if scene == sysin.BotCodeSceneLogin && row.Status == sysin.BotCodeStatusAuthorized {
		if strings.TrimSpace(row.Token) == "" {
			row.Status = sysin.BotCodeStatusFailed
			row.ErrorMessage = "登录凭证生成失败，请重新获取验证码"
			_, _ = s.markCodeFailed(ctx, row.Code, row.Status, row.ErrorMessage)
			row.Token = ""
			row.AccessToken = ""
			return row, nil
		}
		profile, profileErr := s.loginAccountProfile(ctx, row.App, row.AccountId)
		if profileErr != nil {
			row.Status = sysin.BotCodeStatusFailed
			row.ErrorMessage = profileErr.Error()
			_, _ = s.markCodeFailed(ctx, row.Code, row.Status, row.ErrorMessage)
			row.Token = ""
			row.AccessToken = ""
			return row, nil
		}
		row.TenantId = profile.TenantId
		row.AccountType = profile.AccountType
		row.Username = profile.Username
		row.Nickname = profile.Nickname
		row.AccessToken = row.Token
	} else {
		row.Token = ""
		row.AccessToken = ""
	}
	return row, nil
}

func (s *sSysBot) uniqueCode(ctx context.Context) (string, error) {
	for i := 0; i < 20; i++ {
		code := fmt.Sprintf("%06d", grand.N(100000, 999999))
		count, err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("status", sysin.BotCodeStatusPending).Count()
		if err != nil {
			return "", gerror.Wrap(err, "生成验证码失败")
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", gerror.New("生成验证码失败，请重试")
}
