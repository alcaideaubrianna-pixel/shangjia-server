package sysin

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"time"
)

// DailyReportSourceRow is the source-channel breakdown shown in the Bot
// report. It is based on distinct profiles, so retries do not inflate counts.
type DailyReportSourceRow struct {
	SourceID     int64  `json:"sourceId"`
	SourceChatID string `json:"sourceChatId"`
	SourceTitle  string `json:"sourceTitle"`
	Count        int    `json:"count"`
}

type DailyReportChannelRow struct {
	ChannelID  int64  `json:"channelId"`
	Title      string `json:"title"`
	Action     string `json:"action"`
	Success    int    `json:"success"`
	Failed     int    `json:"failed"`
	Processing int    `json:"processing"`
}

type DailyReportModel struct {
	Channels []*DailyReportChannelRow `json:"channels"`

	CycleSuccess    int `json:"cycleSuccess"`
	CycleFailed     int `json:"cycleFailed"`
	CycleProcessing int `json:"cycleProcessing"`

	TenantID          int64                   `json:"tenantId"`
	AccountID         int64                   `json:"accountId"`
	ReportDate        string                  `json:"reportDate"`
	CollectedTotal    int                     `json:"collectedTotal"`
	PublishSuccess    int                     `json:"publishSuccess"`
	PublishFailed     int                     `json:"publishFailed"`
	PublishProcessing int                     `json:"publishProcessing"`
	Sources           []*DailyReportSourceRow `json:"sources"`
	GeneratedAt       time.Time               `json:"generatedAt"`
}

type DailyReportListInp struct {
	TenantID  int64 `json:"tenantId"`
	AccountID int64 `json:"accountId"`
	Days      int   `json:"days"`
}

// AccountNotificationSettingInp intentionally excludes VIP expiry reminders.
// They cannot be disabled by an account preference.
type AccountNotificationSettingInp struct {
	AccountId int64  `json:"accountId"`
	Kind      string `json:"kind"`
	Enabled   int    `json:"enabled"`
}

func (in *AccountNotificationSettingInp) Filter(_ context.Context) error {
	if in.AccountId <= 0 {
		return gerror.New("账号ID不能为空")
	}
	if in.Enabled != 0 && in.Enabled != 1 {
		return gerror.New("推送开关参数不合法")
	}
	switch in.Kind {
	case "daily", "channel", "account", "task":
		return nil
	}
	return gerror.New("推送设置类型不合法")
}
