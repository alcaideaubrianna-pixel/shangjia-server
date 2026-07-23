package sys

import pdao "hotgo/addons/youban_publish/internal/dao"

var (
	publishTenantTable               = pdao.YoubanPublishTenant.Table()
	publishAccountTable              = pdao.YoubanPublishAccount.Table()
	publishTaskTable                 = pdao.YoubanPublishTask.Table()
	publishImportTaskTable           = pdao.YoubanPublishImportTask.Table()
	publishImportMatchRunTable       = pdao.YoubanPublishImportMatchRun.Table()
	publishImportMatchItemTable      = pdao.YoubanPublishImportMatchItem.Table()
	publishImportMatchCandidateTable = pdao.YoubanPublishImportMatchCandidate.Table()
	publishMediaTable                = pdao.YoubanPublishMedia.Table()
	publishMediaPHashBucketTable     = "hg_youban_publish_media_phash_bucket"
	publishTgJobTable                = pdao.YoubanPublishTgJob.Table()
	publishTgQueueStatTable          = pdao.YoubanPublishTgQueueStat.Table()
	publishTgChannelStatTable        = pdao.YoubanPublishTgChannelStat.Table()
	publishTgBotStatTable            = pdao.YoubanPublishTgBotStat.Table()
	publishBotTable                  = pdao.YoubanPublishBot.Table()
	publishTgLoginTable              = pdao.YoubanPublishTgLogin.Table()
	publishTgAccountTable            = "hg_youban_publish_tg_account"
	publishChannelTable              = "hg_youban_publish_channel"
	publishTgChannelTable            = "hg_youban_publish_tg_channel"
	publishTgChannelMemberTable      = "hg_youban_publish_tg_channel_member"
	publishTgChannelMemberTaskTable  = "hg_youban_publish_tg_channel_member_sync_task"
	publishTagTable                  = "hg_youban_publish_tag"
	publishTgMessageTable            = "hg_youban_publish_tg_message"
	publishTgMessageRepairRunTable   = pdao.YoubanPublishTgMessageRepairRun.Table()
	publishTgMessageCacheTable       = pdao.YoubanPublishTgMessageCache.Table()
	publishTgJobLogTable             = "hg_youban_publish_tg_job_log"
	publishCyclePlanTable            = pdao.YoubanPublishCyclePlan.Table()
	publishCycleRunTable             = pdao.YoubanPublishCycleRun.Table()
	publishCycleRunLogTable          = pdao.YoubanPublishCycleRunLog.Table()
	publishDailyStatTable            = "hg_youban_publish_daily_stat"
	publishAccountSettingTable       = "hg_youban_publish_account_setting"
	publishCollectContentTable       = pdao.YoubanPublishCollectContent.Table()
	publishCollectContentMediaTable  = pdao.YoubanPublishCollectContentMedia.Table()
	publishCollectHistoryTaskTable   = pdao.YoubanPublishCollectHistoryTask.Table()
	publishCollectHistoryLogTable    = pdao.YoubanPublishCollectHistoryLog.Table()
)
