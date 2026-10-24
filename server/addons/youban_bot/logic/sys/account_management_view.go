package sys

import (
	"context"
	"fmt"
	"html"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

// Resolve the label only after the existing service has checked management
// permission. Names are presentation data; callback IDs remain authoritative.
func (s *sSysBot) accountManagementSetting(ctx context.Context, account *botProfileAccount, id int64) (*pi.AccountSettingModel, string, error) {
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	setting, err := ps.SysPublish().AdminAccountSettingView(callCtx, &pi.AccountSettingViewInp{AccountId: id})
	if err != nil {
		return nil, "", err
	}
	if setting == nil {
		return nil, "", gerror.New("账户已不可用，请返回账户列表重新选择。")
	}
	if id == account.AccountId {
		current, err := ps.SysPublish().CurrentAccount(callCtx)
		if err != nil {
			return nil, "", err
		}
		if current == nil {
			return nil, "", gerror.New("当前账户已不可用，请重新绑定后再试。")
		}
		return setting, firstNonEmpty(current.Nickname, current.Username, "未命名账户"), nil
	}
	options, err := ps.SysPublish().AdminAccountOptions(callCtx, &pi.AccountOptionsInp{Scope: pi.AccountOptionsScopeMine})
	if err != nil {
		return nil, "", err
	}
	for _, item := range options {
		if item != nil && item.Value == id {
			name := firstNonEmpty(item.Label, "未命名账户")
			if name == fmt.Sprintf("账号 %d", id) {
				name = "未命名账户"
			}
			return setting, name, nil
		}
	}
	return nil, "", gerror.New("账户已不可用，请返回账户列表重新选择。")
}

func (s *sSysBot) accountManagementName(ctx context.Context, account *botProfileAccount, id int64) (string, error) {
	_, name, err := s.accountManagementSetting(ctx, account, id)
	return name, err
}

func (s *sSysBot) showManagedAccount(ctx context.Context, botID int64, chatID string, account *botProfileAccount, id int64, page int) error {
	if id == account.AccountId {
		return gerror.New("请在我的账户中管理当前账户。")
	}
	name, err := s.accountManagementName(ctx, account, id)
	if err != nil {
		return err
	}
	return s.replyWithMarkup(ctx, botID, chatID, fmt.Sprintf("<b>上架账户</b>\n\n账户：<code>%s</code>\n\n请选择操作：", html.EscapeString(name)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "账号设置", CallbackData: fmt.Sprintf("acct:edit:%d:view", id)}},
		{{Text: "重置密码", CallbackData: fmt.Sprintf("acct:accounts:reset:%d", id)}},
		{{Text: "删除账户", CallbackData: fmt.Sprintf("acct:accounts:delete:%d", id)}},
		{{Text: "返回账户列表", CallbackData: fmt.Sprintf("acct:accounts:page:%d", page)}},
	}})
}

func accountSettingSectionForField(field string) string {
	switch field {
	case "enableSuffix", "suffixContent":
		return "suffix"
	case "enableTitleMark", "markMode", "numberSource", "customMarkText", "markPosition":
		return "title"
	case "profileDetailViewEnabled", "sharedResourceEnabled", "telegramBindingEnabled", "groupPushEnabled":
		return "permissions"
	default:
		return "menu"
	}
}

func (s *sSysBot) showAccountSettingSection(ctx context.Context, botID int64, chatID string, account *botProfileAccount, id int64, section string) error {
	setting, name, err := s.accountManagementSetting(ctx, account, id)
	if err != nil {
		return err
	}
	var (
		title = "账号设置"
		body  = "请选择要修改的设置："
		rows  [][]models.InlineKeyboardButton
	)
	button := func(label, field, value string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: label, CallbackData: fmt.Sprintf("acct:set:%d:%s:%s", id, field, value)}
	}
	edit := func(label, field string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: label, CallbackData: fmt.Sprintf("acct:edit:%d:%s", id, field)}
	}
	switch section {
	case "menu":
		rows = append(rows,
			[]models.InlineKeyboardButton{{Text: "发送后缀", CallbackData: fmt.Sprintf("acct:section:%d:suffix", id)}},
			[]models.InlineKeyboardButton{{Text: "标题标识", CallbackData: fmt.Sprintf("acct:section:%d:title", id)}},
		)
		back := "acct:mine"
		if id != account.AccountId {
			rows = append(rows, []models.InlineKeyboardButton{{Text: "账户权限", CallbackData: fmt.Sprintf("acct:section:%d:permissions", id)}})
			back = fmt.Sprintf("acct:accounts:view:%d:1", id)
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账户", CallbackData: back}})
	case "suffix":
		title = "发送后缀"
		body = fmt.Sprintf("状态：%s\n\n后缀内容：\n%s\n\n开启后，推送资料时会带上这段后缀。", boolText(setting.EnableSuffix), html.EscapeString(firstNonEmpty(setting.SuffixContent, "尚未设置")))
		rows = [][]models.InlineKeyboardButton{
			{button("发送后缀："+boolText(setting.EnableSuffix), "enableSuffix", fmt.Sprint(1-setting.EnableSuffix))},
			{edit("修改后缀内容", "suffixContent")},
		}
	case "title":
		title = "标题标识"
		body = fmt.Sprintf("状态：%s\n自定义标识：%s\n\n带 ✅ 的按钮是当前选项，点击其他选项即可切换。", boolText(setting.EnableTitleMark), html.EscapeString(firstNonEmpty(setting.CustomMarkText, "尚未设置")))
		selected := func(label, current, value, fallback string) string {
			if firstNonEmpty(current, fallback) == value {
				return "✅ " + label
			}
			return label
		}
		rows = [][]models.InlineKeyboardButton{
			{button("标题标识："+boolText(setting.EnableTitleMark), "enableTitleMark", fmt.Sprint(1-setting.EnableTitleMark))},
			{button(selected("昵称标识", setting.MarkMode, "nickname", "nickname"), "markMode", "nickname"), button(selected("自定义标识", setting.MarkMode, "custom", "nickname"), "markMode", "custom")},
			{edit("修改自定义标识", "customMarkText")},
			{button(selected("顺序编号", setting.NumberSource, "sequence", "sequence"), "numberSource", "sequence"), button(selected("随机编号", setting.NumberSource, "random", "sequence"), "numberSource", "random")},
			{button(selected("顶部", setting.MarkPosition, "top", "top"), "markPosition", "top"), button(selected("底部", setting.MarkPosition, "bottom", "top"), "markPosition", "bottom"), button(selected("介绍费行", setting.MarkPosition, "feeLine", "top"), "markPosition", "feeLine")},
		}
	case "permissions":
		if id == account.AccountId {
			return gerror.New("管理员账户不需要配置上架账户权限。")
		}
		title = "账户权限"
		body = "点击按钮开启或关闭权限，只影响当前选中的上架账户。"
		for _, permission := range []struct {
			field, label string
			value        int
		}{
			{"profileDetailViewEnabled", "查看租户资料详情", setting.ProfileDetailViewEnabled},
			{"sharedResourceEnabled", "管理共享资料", setting.SharedResourceEnabled},
			{"telegramBindingEnabled", "绑定并使用 Telegram", setting.TelegramBindingEnabled},
			{"groupPushEnabled", "群聊推送", setting.GroupPushEnabled},
		} {
			rows = append(rows, []models.InlineKeyboardButton{button(permission.label+"："+boolText(permission.value), permission.field, fmt.Sprint(1-permission.value))})
		}
	default:
		return gerror.New("设置页面已失效，请重新打开账号设置。")
	}
	if section != "menu" {
		rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账号设置", CallbackData: fmt.Sprintf("acct:edit:%d:view", id)}})
	}
	text := fmt.Sprintf("<b>%s</b>\n\n账户：<code>%s</code>\n\n%s", title, html.EscapeString(name), body)
	return s.replyWithMarkup(ctx, botID, chatID, text, &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}
