package sys

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

var accountUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

// Explicit creation does not bind the administrator's Telegram identity to
// the new uploader. Binding belongs to the recipient authorization flow.
func uploaderCreateInput(username string) *pi.AccountSaveInp {
	return &pi.AccountSaveInp{Username: username, Nickname: username, AccountType: pi.PublishAccountTypeUploader, Status: 1}
}

func (s *sSysBot) handleAccountManagementInput(ctx context.Context, botID int64, msg *models.Message) (bool, error) {
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return false, nil
	}
	session := s.activeProfileSession(ctx, botID, msg)
	if session == nil || (session.Scene != "account_create" && session.Scene != "account_setting") {
		return false, nil
	}
	chatID := fmt.Sprint(msg.Chat.ID)
	account, err := s.boundProfileAccountByUser(ctx, msg.From.ID)
	if err != nil || account == nil || account.AccountType != pi.PublishAccountTypeAdmin || account.AccountId != session.AccountId || account.TenantId != session.TenantId {
		return true, s.reply(ctx, botID, chatID, "账户绑定已改变，请重新打开账户管理。")
	}
	username := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(username, "/") {
		return true, s.cancelProfileSession(ctx, botID, msg)
	}
	if session.Scene == "account_setting" {
		if username == "-" {
			username = ""
		}
		patch := &pi.AccountSettingPatchInp{AccountId: session.ProfileId, Field: session.Step, Value: username}
		if err := patch.Filter(ctx); err != nil {
			return true, s.reply(ctx, botID, chatID, html.EscapeString(err.Error()))
		}
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("account_id", account.AccountId).Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Data(g.Map{"status": profileSessionStatusCompleted}).Update()
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			if err != nil || affected == 0 {
				return err
			}
			_, err = ps.SysPublish().PatchAccountSetting(quickPushMediaUploadContext(ctx, account.AccountId), patch)
			return err
		})
		if err != nil {
			return true, s.reply(ctx, botID, chatID, "保存设置失败，请重新检查权限后重试。")
		}
		return true, s.showEditableAccountSetting(ctx, botID, chatID, account, session.ProfileId)
	}
	if !accountUsernamePattern.MatchString(username) {
		return true, s.reply(ctx, botID, chatID, "用户名需为3～32位字母、数字或下划线，请重新输入。")
	}
	created, err := s.createUploaderFromSession(ctx, session, account, username)
	if err != nil {
		return true, s.reply(ctx, botID, chatID, "创建失败，请检查用户名是否已存在后重试。")
	}
	if created == nil {
		return true, nil
	}
	return true, s.reply(ctx, botID, chatID, fmt.Sprintf("上架账户创建成功\n账号：<code>%s</code>\n初始密码：<code>%s</code>\n账号 ID：%d", html.EscapeString(username), html.EscapeString(created.Password), created.Id))
}

// A session is consumed in the same transaction as creation, so retries after
// successful persistence cannot create another account.
func (s *sSysBot) createUploaderFromSession(ctx context.Context, session *profileSessionRow, account *botProfileAccount, username string) (*pi.AccountSaveModel, error) {
	var created *pi.AccountSaveModel
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("account_id", account.AccountId).Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Data(g.Map{"status": profileSessionStatusCompleted}).Update()
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		created, err = ps.SysPublish().AdminAccountSave(quickPushMediaUploadContext(ctx, account.AccountId), uploaderCreateInput(username))
		return err
	})
	return created, err
}

func (s *sSysBot) resetOwnAccountFromSession(ctx context.Context, session *profileSessionRow, account *botProfileAccount) (*pi.AccountSaveModel, error) {
	var reset *pi.AccountSaveModel
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("account_id", account.AccountId).Where("tenant_id", account.TenantId).Where("scene", "account_reset").Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Data(g.Map{"status": profileSessionStatusCompleted}).Update()
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil || affected == 0 {
			return err
		}
		reset, err = ps.SysPublish().MyAccountResetPassword(quickPushMediaUploadContext(ctx, account.AccountId))
		return err
	})
	return reset, err
}
