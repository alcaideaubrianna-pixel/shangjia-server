package sys

import pdao "hotgo/addons/youban_publish/internal/dao"

var (
	publishTenantTable               = pdao.YoubanPublishTenant.Table()
	publishAccountTable              = pdao.YoubanPublishAccount.Table()
	publishProfileStateTable         = "hg_youban_publish_profile_state"
	publishProfileChannelTable       = "hg_youban_publish_profile_channel"
	publishImportTaskTable           = pdao.YoubanPublishImportTask.Table()
	publishImportMatchRunTable       = pdao.YoubanPublishImportMatchRun.Table()
	publishImportMatchItemTable      = pdao.YoubanPublishImportMatchItem.Table()
	publishImportMatchCandidateTable = pdao.YoubanPublishImportMatchCandidate.Table()
	publishMediaTable                = pdao.YoubanPublishMedia.Table()
	publishNoteIndexTable            = pdao.YoubanPublishNoteIndex.Table()
	publishMediaPHashBucketTable     = "hg_youban_publish_media_phash_bucket"
	publishMediaPHashLshTable        = "hg_youban_publish_media_phash_lsh"
	// Keep this table name explicit. The recovery query is also used during
	// startup, before all generated DAO metadata is guaranteed to be loaded.
	publishTgJobTable               = "hg_youban_publish_tg_job"
	publishTgQueueStatTable         = pdao.YoubanPublishTgQueueStat.Table()
	publishTgChannelStatTable       = pdao.YoubanPublishTgChannelStat.Table()
	publishTgBotStatTable           = pdao.YoubanPublishTgBotStat.Table()
	publishBotTable                 = pdao.YoubanPublishBot.Table()
	publishTgLoginTable             = pdao.YoubanPublishTgLogin.Table()
	publishTgAccountTable           = "hg_youban_publish_tg_account"
	publishChannelTable             = "hg_youban_publish_channel"
	publishTgChannelTable           = "hg_youban_publish_tg_channel"
	publishTgChannelMemberTable     = "hg_youban_publish_tg_channel_member"
	publishTgChannelMemberTaskTable = "hg_youban_publish_tg_channel_member_sync_task"
	publishTagTable                 = "hg_youban_publish_tag"
	publishTgMessageTable           = "hg_youban_publish_tg_message"
	publishTgMessageRepairRunTable  = pdao.YoubanPublishTgMessageRepairRun.Table()
	publishTgMessageCacheTable      = pdao.YoubanPublishTgMessageCache.Table()
	publishTgJobLogTable            = "hg_youban_publish_tg_job_log"
	publishSuccessRecordTable       = "hg_youban_publish_success_record"
	publishCycleRunTable            = pdao.YoubanPublishCycleRun.Table()
	publishCycleRunLogTable         = pdao.YoubanPublishCycleRunLog.Table()
	publishDailyStatTable           = "hg_youban_publish_daily_stat"
	publishAccountSettingTable      = "hg_youban_publish_account_setting"
	publishCollectContentTable      = pdao.YoubanPublishCollectContent.Table()
	publishCollectHistoryTaskTable  = pdao.YoubanPublishCollectHistoryTask.Table()
	publishCollectHistoryLogTable   = pdao.YoubanPublishCollectHistoryLog.Table()
	publishCollectSourceTable       = pdao.YoubanPublishCollectSource.Table()
	publishCollectDispatchTable     = pdao.YoubanPublishCollectDispatch.Table()
	publishCollectEventTable        = pdao.YoubanPublishCollectEvent.Table()
)
