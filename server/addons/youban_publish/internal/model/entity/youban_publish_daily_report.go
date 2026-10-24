// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishDailyReport is the golang structure for table youban_publish_daily_report.
type YoubanPublishDailyReport struct {
	Id                  int64       `json:"id"                  orm:"id"                    description:""`
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"             description:""`
	AccountId           int64       `json:"accountId"           orm:"account_id"            description:""`
	ReportDate          *gtime.Time `json:"reportDate"          orm:"report_date"           description:""`
	CollectedTotal      int         `json:"collectedTotal"      orm:"collected_total"       description:""`
	PublishSuccess      int         `json:"publishSuccess"      orm:"publish_success"       description:""`
	PublishFailed       int         `json:"publishFailed"       orm:"publish_failed"        description:""`
	PublishProcessing   int         `json:"publishProcessing"   orm:"publish_processing"    description:""`
	SourceBreakdownJson string      `json:"sourceBreakdownJson" orm:"source_breakdown_json" description:""`
	GeneratedAt         *gtime.Time `json:"generatedAt"         orm:"generated_at"          description:""`
	NotifyStatus        string      `json:"notifyStatus"        orm:"notify_status"         description:""`
	NotifyRetryCount    int         `json:"notifyRetryCount"    orm:"notify_retry_count"    description:""`
	NotifyNextRetryAt   *gtime.Time `json:"notifyNextRetryAt"   orm:"notify_next_retry_at"  description:""`
	NotifyLeaseUntil    *gtime.Time `json:"notifyLeaseUntil"    orm:"notify_lease_until"    description:""`
	NotifyToken         string      `json:"notifyToken"         orm:"notify_token"          description:""`
	NotifyCursor        int         `json:"notifyCursor"        orm:"notify_cursor"         description:""`
	NotifyError         string      `json:"notifyError"         orm:"notify_error"          description:""`
}
