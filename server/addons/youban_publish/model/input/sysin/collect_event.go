package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

type CollectEventListInp struct {
	form.PageReq
	SourceId int64  `json:"sourceId" dc:"采集源ID"`
	Status   string `json:"status" dc:"状态"`
	Keyword  string `json:"keyword" dc:"关键词"`
}

type CollectEventModel struct {
	Id                int64       `json:"id" dc:"ID"`
	TenantId          int64       `json:"tenantId" dc:"租户ID"`
	AccountId         int64       `json:"accountId" dc:"账号ID"`
	SourceId          int64       `json:"sourceId" dc:"采集源ID"`
	SourceTitle       string      `json:"sourceTitle" dc:"采集源名称"`
	SourceType        string      `json:"sourceType" dc:"来源类型"`
	BotId             int64       `json:"botId" dc:"机器人ID"`
	TgAccountId       int64       `json:"tgAccountId" dc:"协议号ID"`
	SourceChatId      string      `json:"sourceChatId" dc:"来源Chat ID"`
	SourceMessageId   int64       `json:"sourceMessageId" dc:"来源消息ID"`
	SourceGroupedId   string      `json:"sourceGroupedId" dc:"媒体组ID"`
	SourceUniqueKey   string      `json:"sourceUniqueKey" dc:"来源唯一键"`
	RawText           string      `json:"rawText" dc:"原始文本"`
	MediaCount        int         `json:"mediaCount" dc:"媒体数"`
	MediaJson         string      `json:"mediaJson" dc:"媒体JSON"`
	MediaCacheStatus  string      `json:"mediaCacheStatus" dc:"媒体缓存状态"`
	MediaCacheMessage string      `json:"mediaCacheMessage" dc:"媒体缓存说明"`
	TextHash          string      `json:"textHash" dc:"文本哈希"`
	DedupeKey         string      `json:"dedupeKey" dc:"去重键"`
	Status            string      `json:"status" dc:"状态"`
	ErrorMessage      string      `json:"errorMessage" dc:"错误信息"`
	ReceivedAt        *gtime.Time `json:"receivedAt" dc:"接收时间"`
	ProcessedAt       *gtime.Time `json:"processedAt" dc:"处理时间"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type CollectEventClearInp struct {
	SourceId int64 `json:"sourceId" v:"required|min:1#采集源ID不能为空|采集源ID不能为空" dc:"采集源ID"`
}

type CollectEventProcessInp struct {
	Id int64 `json:"id" v:"required|min:1#事件ID不能为空|事件ID不能为空" dc:"事件ID"`
}

type CollectContentListInp struct {
	form.PageReq
	Keyword       string `json:"keyword" dc:"关键词"`
	Status        string `json:"status" dc:"状态"`
	Duplicated    int    `json:"duplicated" dc:"是否重复：1重复 2不重复"`
	MinMediaCount int    `json:"minMediaCount" dc:"最小媒体数"`
}

type CollectContentViewInp struct {
	Id int64 `json:"id" v:"required|min:1#内容ID不能为空|内容ID不能为空" dc:"内容ID"`
}

type CollectContentModel struct {
	Id             int64                       `json:"id" dc:"ID"`
	TenantId       int64                       `json:"tenantId" dc:"租户ID"`
	AccountId      int64                       `json:"accountId" dc:"账号ID"`
	FirstEventId   int64                       `json:"firstEventId" dc:"首次事件ID"`
	LastEventId    int64                       `json:"lastEventId" dc:"最近事件ID"`
	SourceType     string                      `json:"sourceType" dc:"来源类型"`
	RawText        string                      `json:"rawText" dc:"原始文本"`
	NormalizedText string                      `json:"normalizedText" dc:"归一化文本"`
	MediaCount     int                         `json:"mediaCount" dc:"媒体数"`
	MediaSignature string                      `json:"mediaSignature" dc:"媒体签名"`
	MediaJson      string                      `json:"mediaJson" dc:"媒体JSON"`
	TextHash       string                      `json:"textHash" dc:"文本哈希"`
	DedupeKey      string                      `json:"dedupeKey" dc:"去重键"`
	DuplicateTotal int                         `json:"duplicateTotal" dc:"重复次数"`
	Status         string                      `json:"status" dc:"状态"`
	FirstSeenAt    *gtime.Time                 `json:"firstSeenAt" dc:"首次出现时间"`
	PreviousSeenAt *gtime.Time                 `json:"previousSeenAt" dc:"上次出现时间"`
	LastSeenAt     *gtime.Time                 `json:"lastSeenAt" dc:"最近出现时间"`
	CreatedAt      *gtime.Time                 `json:"createdAt" dc:"创建时间"`
	UpdatedAt      *gtime.Time                 `json:"updatedAt" dc:"更新时间"`
	MediaList      []*CollectContentMediaModel `json:"mediaList,omitempty" dc:"媒体列表"`
}

type CollectContentMediaModel struct {
	Id              int64       `json:"id" dc:"ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	AccountId       int64       `json:"accountId" dc:"账号ID"`
	ContentId       int64       `json:"contentId" dc:"内容ID"`
	MediaType       string      `json:"mediaType" dc:"媒体类型"`
	SourceFileId    string      `json:"sourceFileId" dc:"来源文件ID"`
	SourceUniqueKey string      `json:"sourceUniqueKey" dc:"来源唯一键"`
	FileMd5         string      `json:"fileMd5" dc:"文件MD5"`
	FilePhash       string      `json:"filePhash" dc:"图片感知哈希"`
	SortIndex       int         `json:"sortIndex" dc:"排序"`
	Status          string      `json:"status" dc:"状态"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CollectReviewListInp struct {
	form.PageReq
	Status   string `json:"status" dc:"状态"`
	SourceId int64  `json:"sourceId" dc:"采集源ID"`
	RuleId   int64  `json:"ruleId" dc:"规则ID"`
	Keyword  string `json:"keyword" dc:"关键词"`
}

type CollectReviewModel struct {
	Id                  int64       `json:"id" dc:"ID"`
	TenantId            int64       `json:"tenantId" dc:"租户ID"`
	AccountId           int64       `json:"accountId" dc:"账号ID"`
	SourceId            int64       `json:"sourceId" dc:"采集源ID"`
	SourceTitle         string      `json:"sourceTitle" dc:"采集源名称"`
	RuleId              int64       `json:"ruleId" dc:"规则ID"`
	RuleName            string      `json:"ruleName" dc:"规则名称"`
	EventId             int64       `json:"eventId" dc:"事件ID"`
	DispatchId          int64       `json:"dispatchId" dc:"分发ID"`
	RawText             string      `json:"rawText" dc:"原始文本"`
	MediaCount          int         `json:"mediaCount" dc:"媒体数"`
	MediaJson           string      `json:"mediaJson" dc:"媒体JSON"`
	TargetChannelIdJson string      `json:"targetChannelIdJson" dc:"目标频道JSON"`
	BotIdJson           string      `json:"botIdJson" dc:"BOT JSON"`
	Status              string      `json:"status" dc:"审核状态"`
	ReviewReason        string      `json:"reviewReason" dc:"审核原因"`
	ReviewedBy          int64       `json:"reviewedBy" dc:"审核人"`
	ReviewedAt          *gtime.Time `json:"reviewedAt" dc:"审核时间"`
	CreatedAt           *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CollectReviewActionInp struct {
	Ids    []int64 `json:"ids" v:"required#请选择审核资料" dc:"审核ID列表"`
	Status string  `json:"status" v:"required#审核状态不能为空" dc:"审核状态"`
	Reason string  `json:"reason" dc:"原因"`
}

func (in *CollectReviewActionInp) Filter(ctx context.Context) error {
	in.Status = strings.TrimSpace(in.Status)
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Ids) == 0 {
		return gerror.New("请选择审核资料")
	}
	if in.Status != CollectReviewStatusApproved && in.Status != CollectReviewStatusRejected {
		return gerror.New("审核状态不合法")
	}
	return nil
}

type CollectDispatchModel struct {
	Id           int64       `json:"id" dc:"ID"`
	SourceId     int64       `json:"sourceId" dc:"采集源ID"`
	RuleId       int64       `json:"ruleId" dc:"规则ID"`
	EventId      int64       `json:"eventId" dc:"事件ID"`
	ReviewId     int64       `json:"reviewId" dc:"审核ID"`
	ProfileId    int64       `json:"profileId" dc:"资料ID"`
	TaskId       int64       `json:"taskId" dc:"任务ID"`
	Status       string      `json:"status" dc:"状态"`
	ErrorMessage string      `json:"errorMessage" dc:"错误信息"`
	CreatedAt    *gtime.Time `json:"createdAt" dc:"创建时间"`
}
