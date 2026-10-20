// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagerState is the golang structure for table youban_bot_manager_state.
type YoubanBotManagerState struct {
	Id                 int64       `json:"id"                 orm:"id"                   ` //
	TenantId           int64       `json:"tenantId"           orm:"tenant_id"            ` //
	AccountId          int64       `json:"accountId"          orm:"account_id"           ` //
	EntryBotId         int64       `json:"entryBotId"         orm:"entry_bot_id"         ` //
	TelegramUserId     int64       `json:"telegramUserId"     orm:"telegram_user_id"     ` //
	SelectedChannelIds string      `json:"selectedChannelIds" orm:"selected_channel_ids" ` //
	SelectionSaved     int         `json:"selectionSaved"     orm:"selection_saved"      ` //
	InputMode          string      `json:"inputMode"          orm:"input_mode"           ` //
	Keyword            string      `json:"keyword"            orm:"keyword"              ` //
	DraftBotId         int64       `json:"draftBotId"         orm:"draft_bot_id"         ` //
	Revision           int64       `json:"revision"           orm:"revision"             ` //
	SyncedAt           *gtime.Time `json:"syncedAt"           orm:"synced_at"            ` //
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"           ` //
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"           ` //
	DraftRightsJson    string      `json:"draftRightsJson"    orm:"draft_rights_json"    ` //
}
