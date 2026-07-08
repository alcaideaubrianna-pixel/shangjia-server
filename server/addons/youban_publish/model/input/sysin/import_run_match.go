package sysin

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	ImportMatchRunStatusPending = "pending"
	ImportMatchRunStatusRunning = "running"
	ImportMatchRunStatusSuccess = "success"
	ImportMatchRunStatusFailed  = "failed"

	ImportMatchItemStatusAutoSelected  = "auto_selected"
	ImportMatchItemStatusManualPending = "manual_pending"
	ImportMatchItemStatusConfirmed     = "confirmed"
	ImportMatchItemStatusSkipped       = "skipped"
)

type ImportRunMatchConfigInp struct {
	ImportRunId int64 `json:"importRunId" v:"required#导入记录ID不能为空" dc:"导入记录ID"`
}

type ImportRunMatchStartInp struct {
	ImportRunId int64   `json:"importRunId" v:"required#导入记录ID不能为空" dc:"导入记录ID"`
	ChannelIds  []int64 `json:"channelIds" v:"required#请选择匹配频道" dc:"频道ID"`
	Threshold   int     `json:"threshold" dc:"自动匹配阈值"`
}

func (in *ImportRunMatchStartInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("匹配参数不能为空")
	}
	if in.ImportRunId <= 0 {
		return gerror.New("导入记录ID不能为空")
	}
	if len(in.ChannelIds) == 0 {
		return gerror.New("请选择匹配频道")
	}
	if in.Threshold <= 0 {
		in.Threshold = 80
	}
	if in.Threshold < 80 {
		in.Threshold = 80
	}
	if in.Threshold > 100 {
		in.Threshold = 100
	}
	return nil
}

type ImportRunTgSyncStartInp struct {
	ImportRunId int64   `json:"importRunId" v:"required#导入记录ID不能为空" dc:"导入记录ID"`
	ChannelIds  []int64 `json:"channelIds" v:"required#请选择同步频道" dc:"频道ID"`
	ScanDays    int     `json:"scanDays" dc:"拉取天数"`
}

func (in *ImportRunTgSyncStartInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("同步参数不能为空")
	}
	if in.ImportRunId <= 0 {
		return gerror.New("导入记录ID不能为空")
	}
	if len(in.ChannelIds) == 0 {
		return gerror.New("请选择同步频道")
	}
	if in.ScanDays <= 0 {
		in.ScanDays = DefaultImportTgMatchDays
	}
	if in.ScanDays > 365 {
		in.ScanDays = 365
	}
	return nil
}

type ImportRunMatchViewInp struct {
	Id          int64 `json:"id" dc:"匹配运行ID"`
	ImportRunId int64 `json:"importRunId" dc:"导入记录ID"`
}

type ImportRunMatchItemListInp struct {
	form.PageReq
	MatchRunId int64  `json:"matchRunId" v:"required#匹配运行ID不能为空" dc:"匹配运行ID"`
	ChannelId  int64  `json:"channelId" dc:"频道ID"`
	Status     string `json:"status" dc:"匹配状态"`
	Keyword    string `json:"keyword" dc:"关键词"`
}

type ImportRunMatchCandidateListInp struct {
	ItemId  int64  `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
	Purpose string `json:"purpose" dc:"用途：display/verify"`
}

type ImportRunMatchCandidateSearchInp struct {
	form.PageReq
	ItemId  int64  `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
	Keyword string `json:"keyword" dc:"关键词"`
}

type ImportRunMatchSaveDraftInp struct {
	ItemId          int64  `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
	DisplayGroupKey string `json:"displayGroupKey" dc:"展示资料消息组"`
	VerifyGroupKey  string `json:"verifyGroupKey" dc:"验证资料消息组"`
}

type ImportRunMatchConfirmInp struct {
	ItemId int64 `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
}

type ImportRunMatchBatchConfirmInp struct {
	MatchRunId int64 `json:"matchRunId" v:"required#匹配运行ID不能为空" dc:"匹配运行ID"`
}

type ImportRunMatchSkipInp struct {
	ItemId int64 `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
}

type ImportRunMatchUnbindInp struct {
	ItemId int64 `json:"itemId" v:"required#匹配项ID不能为空" dc:"匹配项ID"`
}

type ImportRunMatchChannelModel struct {
	Id           int64  `json:"id" dc:"频道ID"`
	Name         string `json:"name" dc:"频道名称"`
	TargetChatId string `json:"targetChatId" dc:"TG频道"`
	TgAccountId  int64  `json:"tgAccountId" dc:"协议号ID"`
}

type ImportRunMatchConfigModel struct {
	ImportRunId int64                         `json:"importRunId" dc:"导入记录ID"`
	LatestRun   *ImportRunMatchRunModel       `json:"latestRun" dc:"最近匹配运行"`
	Channels    []*ImportRunMatchChannelModel `json:"channels" dc:"可选频道"`
}

type ImportRunMatchRunModel struct {
	Id             int64       `json:"id" dc:"匹配运行ID"`
	ImportRunId    int64       `json:"importRunId" dc:"导入记录ID"`
	TenantId       int64       `json:"tenantId" dc:"租户ID"`
	AccountId      int64       `json:"accountId" dc:"账号ID"`
	Status         string      `json:"status" dc:"状态"`
	Stage          string      `json:"stage" dc:"阶段"`
	ChannelIdJson  string      `json:"channelIdJson" dc:"频道JSON"`
	ChannelIds     []int64     `json:"channelIds" dc:"频道ID"`
	ScanDays       int         `json:"scanDays" dc:"扫描天数"`
	Threshold      int         `json:"threshold" dc:"阈值"`
	ProfileTotal   int         `json:"profileTotal" dc:"资料总数"`
	ProfileDone    int         `json:"profileDone" dc:"已处理资料数"`
	CandidateTotal int         `json:"candidateTotal" dc:"候选数"`
	AutoMatched    int         `json:"autoMatched" dc:"自动匹配数"`
	ManualPending  int         `json:"manualPending" dc:"待人工数"`
	Confirmed      int         `json:"confirmed" dc:"已确认数"`
	Skipped        int         `json:"skipped" dc:"已跳过数"`
	ErrorMessage   string      `json:"errorMessage" dc:"错误信息"`
	CreatedAt      *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt      *gtime.Time `json:"updatedAt" dc:"更新时间"`
	FinishedAt     *gtime.Time `json:"finishedAt" dc:"完成时间"`
}

type ImportRunMatchItemModel struct {
	Id              int64       `json:"id" dc:"匹配项ID"`
	MatchRunId      int64       `json:"matchRunId" dc:"匹配运行ID"`
	ImportRunId     int64       `json:"importRunId" dc:"导入记录ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	AccountId       int64       `json:"accountId" dc:"账号ID"`
	ProfileId       int64       `json:"profileId" dc:"资料ID"`
	TaskId          int64       `json:"taskId" dc:"任务ID"`
	ChannelId       int64       `json:"channelId" dc:"频道ID"`
	ChannelName     string      `json:"channelName" dc:"频道名称"`
	Title           string      `json:"title" dc:"标题"`
	ProfileNo       string      `json:"profileNo" dc:"编号"`
	SourceKey       string      `json:"sourceKey" dc:"来源键"`
	PlainText       string      `json:"plainText" dc:"资料文本"`
	DisplayGroupKey string      `json:"displayGroupKey" dc:"展示资料消息组"`
	VerifyGroupKey  string      `json:"verifyGroupKey" dc:"验证资料消息组"`
	DisplayScore    int         `json:"displayScore" dc:"展示匹配分"`
	VerifyScore     int         `json:"verifyScore" dc:"验证匹配分"`
	TotalScore      int         `json:"totalScore" dc:"总分"`
	MatchStatus     string      `json:"matchStatus" dc:"匹配状态"`
	MatchMode       string      `json:"matchMode" dc:"匹配方式"`
	ReasonJson      string      `json:"reasonJson" dc:"原因JSON"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ImportRunMatchCandidateModel struct {
	Id             int64       `json:"id" dc:"候选ID"`
	MatchRunId     int64       `json:"matchRunId" dc:"匹配运行ID"`
	TenantId       int64       `json:"tenantId" dc:"租户ID"`
	ChannelId      int64       `json:"channelId" dc:"频道ID"`
	GroupKey       string      `json:"groupKey" dc:"消息组键"`
	MediaGroupId   string      `json:"mediaGroupId" dc:"TG媒体组ID"`
	FirstMessageId int64       `json:"firstMessageId" dc:"首消息ID"`
	LastMessageId  int64       `json:"lastMessageId" dc:"末消息ID"`
	MessageDate    *gtime.Time `json:"messageDate" dc:"消息时间"`
	CaptionText    string      `json:"captionText" dc:"文案"`
	MediaCount     int         `json:"mediaCount" dc:"媒体数"`
	MediaTypes     string      `json:"mediaTypes" dc:"媒体类型"`
	PreviewJson    string      `json:"previewJson" dc:"预览JSON"`
	Score          int         `json:"score" dc:"匹配分"`
	CreatedAt      *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt      *gtime.Time `json:"updatedAt" dc:"更新时间"`
}
