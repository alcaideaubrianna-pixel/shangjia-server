package sys

import pdao "hotgo/addons/youban_publish/internal/dao"

var (
	publishTenantTable    = pdao.YoubanPublishTenant.Table()
	publishAccountTable   = pdao.YoubanPublishAccount.Table()
	publishTaskTable      = pdao.YoubanPublishTask.Table()
	publishMediaTable     = pdao.YoubanPublishMedia.Table()
	publishTgJobTable     = pdao.YoubanPublishTgJob.Table()
	publishBotTable       = pdao.YoubanPublishBot.Table()
	publishTgLoginTable   = pdao.YoubanPublishTgLogin.Table()
	publishTgAccountTable = "hg_youban_publish_tg_account"
	publishChannelTable   = "hg_youban_publish_channel"
)
