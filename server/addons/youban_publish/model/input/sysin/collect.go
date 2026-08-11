package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	CollectSourceTypeAccount = "account"
	CollectSourceTypeBot     = "bot"
	CollectSourceTypeFollow  = "follow"

	CollectHistoryModeAll        = "all"
	CollectHistoryModeRecentDays = "recent_days"

	CollectEventStatusPending      = "pending"
	CollectEventStatusGroupCollect = "group_collecting"
	CollectEventStatusWaitingOrder = "waiting_order"
	CollectEventStatusPrechecked   = "prechecked"
	CollectEventStatusMediaPending = "media_pending"
	CollectEventStatusMediaReady   = "media_ready"
	CollectEventStatusDispatched   = "dispatched"
	CollectEventStatusMatched      = CollectEventStatusDispatched
	CollectEventStatusProcessed    = "processed"
	CollectEventStatusIgnored      = "ignored"
	CollectEventStatusFailed       = "failed"

	CollectEventIgnoreTypeDedupe = "dedupe"
	CollectEventIgnoreTypeMatch  = "match"

	CollectReviewStatusPending  = "pending"
	CollectReviewStatusApproved = "approved"
	CollectReviewStatusRejected = "rejected"

	CollectDispatchStatusPending   = "pending"
	CollectDispatchStatusReviewing = "reviewing"
	CollectDispatchStatusSent      = "sent"
	CollectDispatchStatusSkipped   = "skipped"
	CollectDispatchStatusFailed    = "failed"

	CollectHistoryTaskStatusPending  = "pending"
	CollectHistoryTaskStatusRunning  = "running"
	CollectHistoryTaskStatusPaused   = "paused"
	CollectHistoryTaskStatusSuccess  = "success"
	CollectHistoryTaskStatusFailed   = "failed"
	CollectHistoryTaskStatusCanceled = "canceled"
)

type CollectSourceListInp struct {
	form.PageReq
	Keyword    string `json:"keyword" dc:"关键词"`
	SourceType string `json:"sourceType" dc:"来源类型"`
	Status     int    `json:"status" dc:"状态"`
}

type CollectConfigModel struct {
	Enabled              int `json:"enabled" dc:"采集总开关"`
	PushEnabled          int `json:"pushEnabled" dc:"采集推送总开关"`
	RealtimePushDelaySec int `json:"realtimePushDelaySec" dc:"实时采集推送延迟秒数"`
}

type CollectConfigSaveInp struct {
	Enabled              int  `json:"enabled" dc:"采集总开关"`
	PushEnabled          *int `json:"pushEnabled" dc:"采集推送总开关"`
	RealtimePushDelaySec int  `json:"realtimePushDelaySec" dc:"实时采集推送延迟秒数"`
}

func (in *CollectConfigSaveInp) Filter(ctx context.Context) error {
	if in.Enabled != 0 {
		in.Enabled = 1
	}
	if in.PushEnabled != nil && *in.PushEnabled != 0 {
		*in.PushEnabled = 1
	}
	if in.RealtimePushDelaySec < 0 {
		in.RealtimePushDelaySec = 0
	}
	if in.RealtimePushDelaySec > 0 && in.RealtimePushDelaySec < 600 {
		in.RealtimePushDelaySec = 600
	}
	if in.RealtimePushDelaySec > 600 {
		in.RealtimePushDelaySec = 600
	}
	return nil
}

type CollectStatsModel struct {
	BlockedCount        int `json:"blockedCount" dc:"已屏蔽数量"`
	CollectingCount     int `json:"collectingCount" dc:"采集中数量"`
	EventCount          int `json:"eventCount" dc:"采集事件数"`
	FailedDispatchCount int `json:"failedDispatchCount" dc:"失败分发数"`
	PendingReviewCount  int `json:"pendingReviewCount" dc:"待审核数"`
	PushSuccessRate     int `json:"pushSuccessRate" dc:"推送成功率"`
	RuleCount           int `json:"ruleCount" dc:"规则数"`
	TodayPushedCount    int `json:"todayPushedCount" dc:"今日成功推送"`
}

type CollectSourceModel struct {
	Id                    int64       `json:"id" dc:"ID"`
	TenantId              int64       `json:"tenantId" dc:"租户ID"`
	AccountId             int64       `json:"accountId" dc:"账号ID"`
	SourceType            string      `json:"sourceType" dc:"来源类型"`
	Title                 string      `json:"title" dc:"名称"`
	SourceChatId          string      `json:"sourceChatId" dc:"频道/群聊ID"`
	SourceUsername        string      `json:"sourceUsername" dc:"用户名"`
	TgAccountId           int64       `json:"tgAccountId" dc:"协议号ID"`
	BotId                 int64       `json:"botId" dc:"机器人ID"`
	BotCollectScope       string      `json:"botCollectScope" dc:"Bot采集范围"`
	FollowAccountId       int64       `json:"followAccountId" dc:"关注账号ID"`
	CollectEnabled        int         `json:"collectEnabled" dc:"采集开关"`
	HistoryCollectEnabled int         `json:"historyCollectEnabled" dc:"历史采集开关"`
	HistoryCollectMode    string      `json:"historyCollectMode" dc:"历史采集模式"`
	HistoryCollectDays    int         `json:"historyCollectDays" dc:"历史采集天数"`
	RuleIds               []int64     `json:"ruleIds" dc:"绑定规则ID"`
	Status                int         `json:"status" dc:"状态"`
	EventTotal            int64       `json:"eventTotal" dc:"事件总数"`
	SuccessTotal          int64       `json:"successTotal" dc:"成功数"`
	FailedTotal           int64       `json:"failedTotal" dc:"失败数"`
	LastEventAt           *gtime.Time `json:"lastEventAt" dc:"最后事件时间"`
	Remark                string      `json:"remark" dc:"备注"`
	CreatedAt             *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt             *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CollectSourceSaveInp struct {
	Id                    int64   `json:"id" dc:"ID"`
	SourceType            string  `json:"sourceType" dc:"来源类型"`
	Title                 string  `json:"title" dc:"名称"`
	SourceChatId          string  `json:"sourceChatId" dc:"频道/群聊ID"`
	SourceUsername        string  `json:"sourceUsername" dc:"用户名"`
	TgAccountId           int64   `json:"tgAccountId" dc:"协议号ID"`
	BotId                 int64   `json:"botId" dc:"机器人ID"`
	BotCollectScope       string  `json:"botCollectScope" dc:"Bot采集范围"`
	FollowAccountId       int64   `json:"followAccountId" dc:"关注账号ID"`
	CollectEnabled        int     `json:"collectEnabled" dc:"采集开关"`
	HistoryCollectEnabled int     `json:"historyCollectEnabled" dc:"历史采集开关"`
	HistoryCollectMode    string  `json:"historyCollectMode" dc:"历史采集模式"`
	HistoryCollectDays    int     `json:"historyCollectDays" dc:"历史采集天数"`
	RuleIds               []int64 `json:"ruleIds" dc:"绑定规则ID"`
	Remark                string  `json:"remark" dc:"备注"`
	Status                int     `json:"status" dc:"状态"`
}

type CollectSourceHistoryStartInp struct {
	Id int64 `json:"id" dc:"采集源ID"`
}

type CollectSourceTriggerModel struct {
	QueuedCount    int `json:"queuedCount" dc:"已投递数量"`
	ProcessedCount int `json:"processedCount" dc:"已处理数量"`
	FailedCount    int `json:"failedCount" dc:"失败数量"`
}

type CollectSourceResetInp struct {
	Id int64 `json:"id" v:"required|min:1#采集源ID不能为空|采集源ID不能为空" dc:"采集源ID"`
}

type CollectSourceResetModel struct {
	EventCount    int `json:"eventCount" dc:"重置事件数量"`
	DispatchCount int `json:"dispatchCount" dc:"清理分发数量"`
	ReviewCount   int `json:"reviewCount" dc:"清理审核数量"`
}

type CollectHistoryTaskListInp struct {
	form.PageReq
	SourceId int64  `json:"sourceId" dc:"采集源ID"`
	Status   string `json:"status" dc:"状态"`
}

type CollectHistoryLogListInp struct {
	form.PageReq
	TaskId int64 `json:"taskId" dc:"任务ID"`
}

type CollectHistoryTaskModel struct {
	Id             int64       `json:"id" dc:"ID"`
	TenantId       int64       `json:"tenantId" dc:"租户ID"`
	AccountId      int64       `json:"accountId" dc:"账号ID"`
	SourceId       int64       `json:"sourceId" dc:"采集源ID"`
	TgAccountId    int64       `json:"tgAccountId" dc:"协议号ID"`
	SourceChatId   string      `json:"sourceChatId" dc:"来源频道ID"`
	Mode           string      `json:"mode" dc:"采集模式"`
	Days           int         `json:"days" dc:"采集天数"`
	OffsetId       int         `json:"offsetId" dc:"历史游标"`
	ScannedCount   int         `json:"scannedCount" dc:"扫描数量"`
	EventCount     int         `json:"eventCount" dc:"入库事件数"`
	DuplicateCount int         `json:"duplicateCount" dc:"重复数量"`
	FailedCount    int         `json:"failedCount" dc:"失败数量"`
	Status         string      `json:"status" dc:"状态"`
	ErrorMessage   string      `json:"errorMessage" dc:"错误信息"`
	NextRunAt      *gtime.Time `json:"nextRunAt" dc:"下次执行时间"`
	StartedAt      *gtime.Time `json:"startedAt" dc:"开始时间"`
	FinishedAt     *gtime.Time `json:"finishedAt" dc:"完成时间"`
	CreatedAt      *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt      *gtime.Time `json:"updatedAt" dc:"更新时间"`
	SourceTitle    string      `json:"sourceTitle" dc:"采集源名称"`
	SourceUsername string      `json:"sourceUsername" dc:"采集源用户名"`
}

type CollectHistoryLogModel struct {
	Id        int64       `json:"id" dc:"ID"`
	TaskId    int64       `json:"taskId" dc:"任务ID"`
	TenantId  int64       `json:"tenantId" dc:"租户ID"`
	AccountId int64       `json:"accountId" dc:"账号ID"`
	Level     string      `json:"level" dc:"等级"`
	Stage     string      `json:"stage" dc:"阶段"`
	Message   string      `json:"message" dc:"消息"`
	MetaJson  string      `json:"metaJson" dc:"元数据"`
	CreatedAt *gtime.Time `json:"createdAt" dc:"创建时间"`
}

func (in *CollectSourceSaveInp) Filter(ctx context.Context) error {
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.Title = strings.TrimSpace(in.Title)
	in.SourceChatId = strings.TrimSpace(in.SourceChatId)
	in.SourceUsername = strings.TrimSpace(in.SourceUsername)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.SourceType == "" {
		in.SourceType = CollectSourceTypeBot
	}
	if in.SourceType != CollectSourceTypeAccount && in.SourceType != CollectSourceTypeBot && in.SourceType != CollectSourceTypeFollow {
		return gerror.New("采集源类型不合法")
	}
	if in.SourceType == CollectSourceTypeBot {
		if in.BotId <= 0 {
			return gerror.New("Bot采集必须选择机器人")
		}
		in.BotCollectScope = "all"
	} else {
		in.BotCollectScope = ""
	}
	if in.Title == "" {
		return gerror.New("采集源名称不能为空")
	}
	if in.CollectEnabled != 1 {
		in.CollectEnabled = 0
	}
	in.HistoryCollectEnabled, in.HistoryCollectMode, in.HistoryCollectDays = NormalizeCollectHistoryConfig(
		in.SourceType,
		in.HistoryCollectEnabled,
		in.HistoryCollectMode,
		in.HistoryCollectDays,
	)
	if in.Status == 0 {
		in.Status = 1
	}
	return nil
}

func NormalizeCollectHistoryConfig(sourceType string, enabled int, mode string, days int) (int, string, int) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = CollectHistoryModeRecentDays
	}
	if mode != CollectHistoryModeAll && mode != CollectHistoryModeRecentDays {
		mode = CollectHistoryModeRecentDays
	}
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if strings.TrimSpace(sourceType) != CollectSourceTypeAccount || enabled != 1 {
		return 0, mode, days
	}
	return 1, mode, days
}

type CollectRuleListInp struct {
	form.PageReq
	Keyword       string `json:"keyword" dc:"关键词"`
	GlobalEnabled int    `json:"globalEnabled" dc:"全局开关"`
	Status        int    `json:"status" dc:"状态"`
}

type CollectRuleModel struct {
	Id                      int64                     `json:"id" dc:"ID"`
	TenantId                int64                     `json:"tenantId" dc:"租户ID"`
	AccountId               int64                     `json:"accountId" dc:"账号ID"`
	Name                    string                    `json:"name" dc:"名称"`
	GlobalEnabled           int                       `json:"globalEnabled" dc:"全局应用"`
	TargetChannelIds        []int64                   `json:"targetChannelIds" dc:"目标频道ID"`
	ReviewEnabled           int                       `json:"reviewEnabled" dc:"审核开关"`
	DedupeEnabled           int                       `json:"dedupeEnabled" dc:"去重开关"`
	DedupeDays              int                       `json:"dedupeDays" dc:"去重天数"`
	FullMatchEnabled        int                       `json:"fullMatchEnabled" dc:"全量匹配"`
	Keywords                []string                  `json:"keywords" dc:"关键词"`
	Tags                    []string                  `json:"tags" dc:"标签"`
	Replacements            []CollectRuleReplaceModel `json:"replacements" dc:"文本替换"`
	DeleteLineTexts         []string                  `json:"deleteLineTexts" dc:"整行删除文本"`
	DeleteTexts             []string                  `json:"deleteTexts" dc:"删除文本"`
	TruncateIntroFeeEnabled int                       `json:"truncateIntroFeeEnabled" dc:"清理资料头部标识及介绍费后续文案"`
	BlockTexts              []string                  `json:"blockTexts" dc:"屏蔽文本"`
	BlockLink               int                       `json:"blockLink" dc:"屏蔽链接"`
	BlockUsername           int                       `json:"blockUsername" dc:"屏蔽用户名"`
	BlockPlainText          int                       `json:"blockPlainText" dc:"屏蔽纯文本"`
	HeaderEnabled           int                       `json:"headerEnabled" dc:"前置文案开关"`
	HeaderMarkdown          string                    `json:"headerMarkdown" dc:"前置文案"`
	FooterEnabled           int                       `json:"footerEnabled" dc:"后置文案开关"`
	FooterMarkdown          string                    `json:"footerMarkdown" dc:"后置文案"`
	Sort                    int                       `json:"sort" dc:"排序"`
	Status                  int                       `json:"status" dc:"状态"`
	CreatedAt               *gtime.Time               `json:"createdAt" dc:"创建时间"`
	UpdatedAt               *gtime.Time               `json:"updatedAt" dc:"更新时间"`
}

type CollectRuleSaveInp struct {
	Id                      int64                     `json:"id" dc:"ID"`
	Name                    string                    `json:"name" dc:"名称"`
	GlobalEnabled           int                       `json:"globalEnabled" dc:"全局应用"`
	TargetChannelIds        []int64                   `json:"targetChannelIds" dc:"目标频道ID"`
	ReviewEnabled           int                       `json:"reviewEnabled" dc:"审核开关"`
	DedupeEnabled           int                       `json:"dedupeEnabled" dc:"去重开关"`
	DedupeDays              int                       `json:"dedupeDays" dc:"去重天数"`
	FullMatchEnabled        int                       `json:"fullMatchEnabled" dc:"全量匹配"`
	Keywords                []string                  `json:"keywords" dc:"关键词"`
	Tags                    []string                  `json:"tags" dc:"标签"`
	Replacements            []CollectRuleReplaceModel `json:"replacements" dc:"文本替换"`
	DeleteLineTexts         []string                  `json:"deleteLineTexts" dc:"整行删除文本"`
	DeleteTexts             []string                  `json:"deleteTexts" dc:"删除文本"`
	TruncateIntroFeeEnabled int                       `json:"truncateIntroFeeEnabled" dc:"清理资料头部标识及介绍费后续文案"`
	BlockTexts              []string                  `json:"blockTexts" dc:"屏蔽文本"`
	BlockLink               int                       `json:"blockLink" dc:"屏蔽链接"`
	BlockUsername           int                       `json:"blockUsername" dc:"屏蔽用户名"`
	BlockPlainText          int                       `json:"blockPlainText" dc:"屏蔽纯文本"`
	HeaderEnabled           int                       `json:"headerEnabled" dc:"前置文案开关"`
	HeaderMarkdown          string                    `json:"headerMarkdown" dc:"前置文案"`
	FooterEnabled           int                       `json:"footerEnabled" dc:"后置文案开关"`
	FooterMarkdown          string                    `json:"footerMarkdown" dc:"后置文案"`
	Sort                    int                       `json:"sort" dc:"排序"`
	Status                  int                       `json:"status" dc:"状态"`
}

func (in *CollectRuleSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.TargetChannelIds = uniquePositiveInt64(in.TargetChannelIds)
	in.Keywords = trimCollectInputValues(in.Keywords)
	in.Tags = trimCollectInputValues(in.Tags)
	in.DeleteLineTexts = trimCollectInputValues(in.DeleteLineTexts)
	in.DeleteTexts = trimCollectInputValues(in.DeleteTexts)
	in.BlockTexts = trimCollectInputValues(in.BlockTexts)
	for index := range in.Replacements {
		in.Replacements[index].From = strings.TrimSpace(in.Replacements[index].From)
		in.Replacements[index].To = strings.TrimSpace(in.Replacements[index].To)
	}
	if in.Name == "" {
		return gerror.New("规则名称不能为空")
	}
	if len(in.TargetChannelIds) == 0 {
		return gerror.New("目标频道不能为空")
	}
	if in.DedupeDays < 0 {
		in.DedupeDays = 0
	}
	if in.DedupeDays > 3650 {
		in.DedupeDays = 3650
	}
	if in.Status == 0 {
		in.Status = 1
	}
	return nil
}

type CollectRuleReplaceModel struct {
	From string `json:"from" dc:"原文本"`
	To   string `json:"to" dc:"替换文本"`
}

func trimCollectInputValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

type IdsInp struct {
	Ids []int64 `json:"ids" v:"required#请选择数据" dc:"ID列表"`
}

type CollectStatusInp struct {
	Id      int64 `json:"id" v:"required|min:1#ID不能为空|ID不能为空" dc:"ID"`
	Enabled int   `json:"enabled" dc:"开关"`
	Status  int   `json:"status" dc:"状态"`
}

type CollectSourceDownInp struct {
	Id int64 `json:"id" v:"required|min:1#采集源ID不能为空|采集源ID不能为空" dc:"采集源ID"`
}

type CollectSourceDownModel struct {
	SourceId     int64 `json:"sourceId" dc:"采集源ID"`
	TaskCount    int   `json:"taskCount" dc:"关联任务数"`
	JobCount     int   `json:"jobCount" dc:"TG任务数"`
	MessageCount int   `json:"messageCount" dc:"目标频道消息数"`
	Queued       int   `json:"queued" dc:"是否已投递异步任务"`
}
