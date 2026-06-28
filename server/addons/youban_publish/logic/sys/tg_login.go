package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const publishTgLoginTable = "hg_youban_publish_tg_login"

func (s *sSysPublish) TelegramLoginStart(ctx context.Context, in *sysin.TelegramLoginStartInp) (res *sysin.TelegramLoginModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	if conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先在插件配置中填写Telegram App ID和App Hash")
	}
	now := gtime.Now()
	token := grand.S(32)
	data := g.Map{
		"merchant_id":   account.MerchantId,
		"account_id":    account.Id,
		"login_token":   token,
		"status":        "unsupported",
		"error_message": "扫码登录需要接入Telegram MTProto客户端，当前仅完成配置和会话骨架",
		"expires_at":    now.Add(5 * time.Minute),
		"created_at":    now,
		"updated_at":    now,
	}
	id, err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建Telegram扫码登录会话失败")
	}
	return s.telegramLoginById(ctx, id, account.Id)
}

func (s *sSysPublish) TelegramLoginStatus(ctx context.Context, in *sysin.TelegramLoginStatusInp) (res *sysin.TelegramLoginModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(in.LoginToken)
	if token == "" {
		return nil, gerror.New("登录令牌不能为空")
	}
	var item *sysin.TelegramLoginModel
	if err = g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
		Where("login_token", token).
		Where("account_id", account.Id).
		Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram扫码登录状态失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("扫码登录会话不存在")
	}
	return item, nil
}

func (s *sSysPublish) telegramLoginById(ctx context.Context, id int64, accountId int64) (*sysin.TelegramLoginModel, error) {
	var item *sysin.TelegramLoginModel
	err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("account_id", accountId).
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram扫码登录会话失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("扫码登录会话不存在")
	}
	return item, nil
}
