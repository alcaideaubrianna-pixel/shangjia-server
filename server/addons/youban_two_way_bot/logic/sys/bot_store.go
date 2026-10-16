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
	return row, err
}
