package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	twowaymodel "hotgo/addons/youban_two_way_bot/model"
)

const publishBotTable = "hg_youban_publish_bot"

const publishTwoWayBotFields = `id,tenant_id,account_id,tg_account_id,
bot_name AS name,bot_token,bot_user_id,bot_username,supergroup_id,
supergroup_access_hash,supergroup_title,invite_link,setup_status,
webhook_status,status,error_message,last_setup_at,last_webhook_at,
created_at,updated_at,deleted_at,welcome_message`

func publishTwoWayBots(ctx context.Context) *gdb.Model {
	return g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields(publishTwoWayBotFields).
		Where("bot_type", "two_way")
}

func scanPublishTwoWayBot(ctx context.Context, id, tenantID int64) (*twowaymodel.BotRuntime, error) {
	var row *twowaymodel.BotRuntime
	err := publishTwoWayBots(ctx).
		Where("id", id).
		Where("tenant_id", tenantID).
		WhereNull("deleted_at").
		Scan(&row)
	if err != nil || row != nil && row.Id > 0 {
		return row, err
	}
	// Cooperation configurations created before the publish_bot migration
	// stored IDs from hg_youban_two_way_bot_bot. Resolve that legacy reference
	// to the sole current active publish bot owned by the same tenant/account.
	var legacy struct {
		TenantID  int64 `orm:"tenant_id"`
		AccountID int64 `orm:"account_id"`
	}
	if legacyErr := g.DB().Model("hg_youban_two_way_bot_bot").Safe().Ctx(ctx).
		Fields("tenant_id,account_id").Where("id", id).Where("tenant_id", tenantID).
		WhereNull("deleted_at").Scan(&legacy); legacyErr != nil || legacy.AccountID <= 0 {
		return row, err
	}
	var candidates []*twowaymodel.BotRuntime
	if legacyErr := publishTwoWayBots(ctx).
		Where("tenant_id", legacy.TenantID).
		Where("account_id", legacy.AccountID).
		Where("status", 1).WhereNull("deleted_at").Scan(&candidates); legacyErr != nil {
		return nil, legacyErr
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return row, err
}
