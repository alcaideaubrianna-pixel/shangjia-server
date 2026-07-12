package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
)

func (s *sSysBot) AdminUserList(ctx context.Context, in *sysin.UserListInp) (list []*sysin.UserModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.UserListInp{}
	}
	mod := g.DB().Model(userTable+" u").Safe().Ctx(ctx).LeftJoin(botTable+" b", "b.id=u.bot_id").Fields("u.*,b.bot_username")
	if in.BotId > 0 {
		mod = mod.Where("u.bot_id", in.BotId)
	}
	if len(in.BotIds) > 0 {
		mod = mod.WhereIn("u.bot_id", in.BotIds)
	}
	if in.Status > 0 {
		mod = mod.Where("u.status", in.Status)
	}
	if in.IsBound == 1 {
		mod = mod.Where("EXISTS (SELECT 1 FROM " + accountBindTbl + " ab WHERE ab.telegram_user_id=u.telegram_user_id AND ab.status=1 AND ab.deleted_at IS NULL)")
	}
	if in.IsBound == 2 {
		mod = mod.Where("NOT EXISTS (SELECT 1 FROM " + accountBindTbl + " ab WHERE ab.telegram_user_id=u.telegram_user_id AND ab.status=1 AND ab.deleted_at IS NULL)")
	}
	if bindApp := strings.TrimSpace(in.BindApp); bindApp != "" {
		mod = mod.Where("EXISTS (SELECT 1 FROM "+accountBindTbl+" ab WHERE ab.telegram_user_id=u.telegram_user_id AND ab.app=? AND ab.status=1 AND ab.deleted_at IS NULL)", bindApp)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(u.telegram_user_id LIKE ? OR u.telegram_username LIKE ? OR u.telegram_first_name LIKE ? OR u.telegram_last_name LIKE ? OR u.chat_id LIKE ? OR EXISTS (SELECT 1 FROM "+accountBindTbl+" ab LEFT JOIN "+publishAccountTable+" pa ON pa.id=ab.account_id AND ab.app='api' LEFT JOIN "+dao.AdminMember.Table()+" am ON am.id=ab.account_id AND ab.app='admin' WHERE ab.telegram_user_id=u.telegram_user_id AND ab.status=1 AND ab.deleted_at IS NULL AND (pa.username LIKE ? OR pa.nickname LIKE ? OR am.username LIKE ? OR am.real_name LIKE ?)))", like, like, like, like, like, like, like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("u.last_message_at").OrderDesc("u.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot用户失败")
	}
	for _, item := range list {
		if item != nil {
			_ = s.packUserBinding(ctx, item, strings.TrimSpace(in.BindApp))
		}
	}
	return
}

func (s *sSysBot) AdminMessageList(ctx context.Context, in *sysin.MessageListInp) (list []*sysin.MessageModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.MessageListInp{}
	}
	mod := g.DB().Model(messageTable+" m").Safe().Ctx(ctx).LeftJoin(botTable+" b", "b.id=m.bot_id").Fields("m.*,b.bot_username")
	if in.BotId > 0 {
		mod = mod.Where("m.bot_id", in.BotId)
	}
	if telegramUserId := strings.TrimSpace(in.TelegramUserId); telegramUserId != "" {
		mod = mod.Where("m.telegram_user_id", telegramUserId)
	}
	if messageType := strings.TrimSpace(in.MessageType); messageType != "" {
		mod = mod.Where("m.message_type", messageType)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(m.telegram_user_id LIKE ? OR m.telegram_username LIKE ? OR m.text LIKE ? OR m.chat_id LIKE ?)", like, like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("m.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot消息日志失败")
	}
	return
}

func (s *sSysBot) packUserBinding(ctx context.Context, item *sysin.UserModel, preferApp string) error {
	var bind *botBindRow
	mod := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Where("telegram_user_id", item.TelegramUserId).Where("status", 1).WhereNull("deleted_at")
	if strings.TrimSpace(preferApp) != "" {
		mod = mod.Where("app", strings.TrimSpace(preferApp))
	}
	if err := mod.OrderDesc("id").Scan(&bind); err != nil {
		return gerror.Wrap(err, "读取用户绑定失败")
	}
	if bind == nil || bind.AccountId <= 0 {
		return nil
	}
	item.IsBound = true
	item.BindApp = bind.App
	item.BindAccountId = bind.AccountId
	if bind.App == sysin.BotAppAdmin {
		var mb *entity.AdminMember
		if err := dao.AdminMember.Ctx(ctx).WherePri(bind.AccountId).Scan(&mb); err == nil && mb != nil {
			item.BindAccountName = firstNonEmpty(mb.RealName, mb.Username)
		}
		return nil
	}
	var account struct {
		TenantId int64  `json:"tenant_id"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
	}
	if err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Fields("tenant_id,username,nickname").Where("id", bind.AccountId).WhereNull("deleted_at").Scan(&account); err != nil {
		return err
	}
	item.BindTenantId = account.TenantId
	item.BindAccountName = firstNonEmpty(account.Nickname, account.Username)
	if item.BindApp == "" {
		item.BindApp = consts.AppApi
	}
	return nil
}

func (s *sSysBot) AdminUserSwitchSuperAdmin(ctx context.Context, in *sysin.UserSwitchSuperAdminInp) (err error) {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择用户")
	}
	_, err = g.DB().Model(userTable).Safe().Ctx(ctx).Where("id", in.Id).Data(g.Map{"is_super_admin": normalizeSwitch(in.IsSuperAdmin), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "设置超级管理员失败")
	}
	return nil
}

func (s *sSysBot) AdminSendMessage(ctx context.Context, in *sysin.SendMessageInp) (err error) {
	if in == nil {
		return gerror.New("消息内容不能为空")
	}
	if strings.TrimSpace(in.ChatId) == "" {
		return gerror.New("Chat ID不能为空")
	}
	if strings.TrimSpace(in.Text) == "" {
		return gerror.New("消息内容不能为空")
	}
	return s.Notify(ctx, &sysin.NotifyInp{BotId: in.BotId, ChatId: in.ChatId, Text: in.Text, ParseMode: "HTML", DisableNotice: in.DisableNotice})
}
