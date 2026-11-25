package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	twowaymodel "hotgo/addons/youban_two_way_bot/model"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

const publishBotTable = "hg_youban_publish_bot"

const publishTwoWayBotFields = `id,tenant_id,account_id,tg_account_id,bot_name AS name,bot_token,bot_user_id,bot_username,supergroup_id,supergroup_access_hash,supergroup_title,invite_link,setup_status,webhook_status,status,error_message,last_setup_at,last_webhook_at,created_at,updated_at,deleted_at,welcome_message`

const legacyTwoWayBotFields = `id,tenant_id,account_id,tg_account_id,name,bot_token,bot_user_id,bot_username,supergroup_id,supergroup_access_hash,supergroup_title,invite_link,setup_status,webhook_status,status,error_message,last_setup_at,last_webhook_at,created_at,updated_at,deleted_at,welcome_message`

func publishTwoWayBots(ctx context.Context) *gdb.Model {
	return g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields(publishTwoWayBotFields).
		Where("bot_type", "two_way")
}

func scanLegacyTwoWayBot(ctx context.Context, id, tenantID int64) (*twowaymodel.BotRuntime, error) {
	var row *twowaymodel.BotRuntime
	err := g.DB().Model("hg_youban_two_way_bot_bot").Safe().Ctx(ctx).
		Fields(legacyTwoWayBotFields).
		Where("id", id).Where("tenant_id", tenantID).
		Where("status", sysin.TwoWayBotStatusEnabled).WhereNull("deleted_at").
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史双向机器人失败")
	}
	return row, nil
}

func legacyEnabledTwoWayBots(ctx context.Context) ([]*twowaymodel.BotRuntime, error) {
	var rows []*twowaymodel.BotRuntime
	err := g.DB().Model("hg_youban_two_way_bot_bot").Safe().Ctx(ctx).
		Fields(legacyTwoWayBotFields).
		Where("status", sysin.TwoWayBotStatusEnabled).WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史双向机器人失败")
	}
	return rows, nil
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
	legacy, legacyErr := scanLegacyTwoWayBot(ctx, id, tenantID)
	if legacyErr != nil || legacy == nil || legacy.Id <= 0 {
		return row, err
	}
	var candidates []*twowaymodel.BotRuntime
	if legacyErr := publishTwoWayBots(ctx).
		Where("tenant_id", legacy.TenantId).
		Where("account_id", legacy.AccountId).
		Where("status", sysin.TwoWayBotStatusEnabled).
		WhereNull("deleted_at").Scan(&candidates); legacyErr != nil {
		return nil, legacyErr
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return legacy, nil
}
