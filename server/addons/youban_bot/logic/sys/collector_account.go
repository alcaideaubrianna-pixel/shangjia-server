package sys

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
)

func (s *sSysBot) resolveCollectorAccount(ctx context.Context, bot *collectorBotRow, user *models.User, allowProvision bool) (*botProfileAccount, error) {
	review, err := uploaderReviewByUser(ctx, bot, user.ID)
	if err != nil {
		return nil, err
	}
	if review != nil && (review.Status == uploaderApproved || review.Status == uploaderDirect) {
		if review.OwnerAccountId != bot.AccountId {
			return nil, gerror.New("上架机器人归属已变更")
		}
		return scopedUploaderAccount(ctx, bot.TenantId, bot.AccountId, review.AccountId)
	}
	if !allowProvision {
		return s.existingScopedUploaderAccount(ctx, bot, user.ID)
	}
	if err = uploaderReviewOwner(ctx, bot.TenantId, bot.AccountId); err != nil {
		return nil, err
	}
	var account *botProfileAccount
	err = g.DB().TransactionWithOptions(ctx, gdb.TxOptions{Propagation: gdb.PropagationNested, Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx gdb.TX) error {
		if err := lockPersonalUploader(tx, bot.TenantId, user.ID); err != nil {
			return err
		}
		review, err := uploaderReviewByUser(ctx, bot, user.ID)
		if err != nil {
			return err
		}
		if review != nil && (review.Status == uploaderApproved || review.Status == uploaderDirect) {
			if review.OwnerAccountId != bot.AccountId {
				return gerror.New("上架机器人归属已变更")
			}
			account, err = scopedUploaderAccount(ctx, bot.TenantId, bot.AccountId, review.AccountId)
			return err
		}
		accountID, err := s.findOrCreatePersonalUploader(ctx, tx, &uploaderReview{
			TenantId: bot.TenantId, BotId: bot.Id, OwnerAccountId: bot.AccountId,
			TelegramUserId: user.ID, TelegramUsername: user.Username,
		})
		if err != nil {
			return err
		}
		if review == nil {
			_, err = tx.Model(uploaderReviewTable).Data(g.Map{
				"tenant_id": bot.TenantId, "bot_id": bot.Id, "owner_account_id": bot.AccountId,
				"telegram_user_id": user.ID, "telegram_username": user.Username,
				"status": uploaderDirect, "account_id": accountID,
			}).Insert()
		} else if review.OwnerAccountId == bot.AccountId {
			_, err = tx.Model(uploaderReviewTable).Where("id", review.Id).
				Data(g.Map{"status": uploaderDirect, "account_id": accountID}).Update()
		} else {
			return gerror.New("上架机器人归属已变更")
		}
		if err != nil {
			return err
		}
		account, err = scopedUploaderAccount(ctx, bot.TenantId, bot.AccountId, accountID)
		return err
	})
	return account, err
}

// Both approval and first-use provisioning hold this lock until commit.
func lockPersonalUploader(tx gdb.TX, tenantID, userID int64) error {
	if g.DB().GetConfig().Type == consts.DBPgsql {
		_, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?),hashtext(?))",
			"youban:uploader:identity", fmt.Sprintf("%d:%d", tenantID, userID))
		return err
	}
	row, err := tx.Model("hg_youban_publish_tenant").Where("id", tenantID).Fields("id").LockUpdate().One()
	if err != nil {
		return err
	}
	if row.IsEmpty() {
		return gerror.New("上架机器人租户不存在")
	}
	return nil
}

func (s *sSysBot) findPersonalUploader(ctx context.Context, review *uploaderReview) (int64, error) {
	var candidates []struct {
		AccountId, OwnerAccountId int64
	}
	err := g.DB().Model(uploaderReviewTable).Ctx(ctx).Where("tenant_id", review.TenantId).
		Where("telegram_user_id", review.TelegramUserId).WhereIn("status", []string{uploaderApproved, uploaderDirect}).
		WhereGT("account_id", 0).Fields("account_id", "owner_account_id").Distinct().Limit(2).Scan(&candidates)
	if err != nil {
		return 0, err
	}
	if len(candidates) > 1 || len(candidates) == 1 && candidates[0].OwnerAccountId != review.OwnerAccountId {
		return 0, gerror.New("当前用户已有其他上架账户归属，请联系管理员")
	}
	accountID := int64(0)
	if len(candidates) == 1 {
		accountID = candidates[0].AccountId
	}
	legacy, err := s.bindingByTelegram(ctx, "api", fmt.Sprint(review.TelegramUserId))
	if err != nil {
		return 0, err
	}
	if legacy != nil && (accountID == 0 || legacy.AccountId != accountID) {
		// Reuse the legacy binding only if it points to a personal account
		// in the current tenant and under the same managing account.
		legacyAccount, err := g.DB().Model("hg_youban_publish_account").Ctx(ctx).Where("id", legacy.AccountId).
			Where("tenant_id", review.TenantId).Where("account_type", pi.PublishAccountTypeUploader).
			WhereNull("deleted_at").Fields("id,parent_id").One()
		if err != nil {
			return 0, err
		}
		if !legacyAccount.IsEmpty() {
			if accountID != 0 || legacyAccount["parent_id"].Int64() != review.OwnerAccountId {
				return 0, gerror.New("当前用户已有其他上架账户归属，请联系管理员")
			}
			accountID = legacy.AccountId
		}
	}
	if accountID > 0 {
		if _, err = scopedUploaderAccount(ctx, review.TenantId, review.OwnerAccountId, accountID); err != nil {
			return 0, err
		}
	}
	return accountID, nil
}

func (s *sSysBot) findOrCreatePersonalUploader(ctx context.Context, tx gdb.TX, review *uploaderReview) (int64, error) {
	accountID, err := s.findPersonalUploader(ctx, review)
	if err != nil || accountID > 0 {
		return accountID, err
	}
	if username := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(review.TelegramUsername), "@")); username != "" && g.DB().GetConfig().Type == consts.DBPgsql {
		if _, err = tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?),hashtext(?))", "youban:publish:username", username); err != nil {
			return 0, gerror.Wrap(err, "锁定上架账号用户名失败")
		}
	}
	return createReviewedUploader(ctx, review)
}

// The caller supplies an authorized administrator and owns the transaction.
func (s *sSysBot) createAndBindUploader(ctx context.Context, botID, ownerAccountID int64, username string, user models.User) (*pi.AccountSaveModel, error) {
	authCtx := quickPushMediaUploadContext(ctx, ownerAccountID)
	in := uploaderCreateInput(username)
	in.TelegramUserId = fmt.Sprint(user.ID)
	in.TelegramUsername = user.Username
	created, err := ps.SysPublish().AdminAccountSave(authCtx, in)
	if err != nil {
		return nil, err
	}
	if created == nil || created.Id <= 0 {
		return nil, gerror.New("未返回创建结果")
	}
	_, err = ps.SysPublish().PatchAccountSetting(authCtx, &pi.AccountSettingPatchInp{AccountId: created.Id, Field: "telegramBindingEnabled", Value: "1"})
	if err != nil {
		return nil, err
	}
	return created, bindLegacyUploader(ctx, botID, created.Id, user)
}

func bindLegacyUploader(ctx context.Context, botID, accountID int64, user models.User) error {
	_, err := g.DB().Model(accountBindTbl).Ctx(ctx).Data(g.Map{
		"app": "api", "account_id": accountID, "telegram_user_id": fmt.Sprint(user.ID),
		"telegram_username": user.Username, "telegram_first_name": user.FirstName,
		"telegram_last_name": user.LastName, "bot_id": botID, "status": 1,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "绑定TG失败（可能已绑定其他账户）")
	}
	return nil
}
