package sys

// Message push schema is managed by the addon install and upgrade SQL files.
// Request and worker paths must never run DDL because PostgreSQL ALTER TABLE
// takes an ACCESS EXCLUSIVE lock even when an IF NOT EXISTS clause is used.
const (
	messageTemplateTable = "hg_youban_publish_message_template"
	messageMediaTable    = "hg_youban_publish_message_media"
	messagePushPlanTable = "hg_youban_publish_message_push_plan"
	quickPushPlanTable   = "hg_youban_publish_quick_push_plan"
)
