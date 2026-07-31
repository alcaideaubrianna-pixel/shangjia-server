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

type CollectEventLogListInp struct {
	form.PageReq
	SourceId int64 `json:"sourceId" dc:"采集源ID"`
}

type CollectEventLogModel struct {
	Id         int64       `json:"id" dc:"日志ID"`
	TenantId   int64       `json:"tenantId" dc:"租户ID"`
	AccountId  int64       `json:"accountId" dc:"账号ID"`
	EventId    int64       `json:"eventId" dc:"事件ID"`
	DispatchId int64       `json:"dispatchId" dc:"分发ID"`
	Stage      string      `json:"stage" dc:"阶段"`
	Status     string      `json:"status" dc:"状态"`
	Message    string      `json:"message" dc:"消息"`
	MetaText   string      `json:"metaText" dc:"元数据"`
	CreatedAt  *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type CollectMaterialDiagnoseInp struct {
	SourceId int64 `json:"sourceId" dc:"采集源ID，0表示全部"`
	Limit    int   `json:"limit" dc:"诊断数量，默认30"`
}

type CollectMaterialDiagnoseItem struct {
	EventId         int64  `json:"eventId" dc:"事件ID"`
	SourceId        int64  `json:"sourceId" dc:"采集源ID"`
	SourceChatId    string `json:"sourceChatId" dc:"来源Chat ID"`
	SourceMessageId int64  `json:"sourceMessageId" dc:"来源消息ID"`
	SourceGroupedId string `json:"sourceGroupedId" dc:"媒体组ID"`
	Status          string `json:"status" dc:"事件状态"`
	MaterialRole    string `json:"materialRole" dc:"资料角色"`
	Classification  string `json:"classification" dc:"规则分类"`
	MediaCount      int    `json:"mediaCount" dc:"媒体数量"`
	DisplayMedia    int    `json:"displayMedia" dc:"展示媒体数量"`
	VerifyMedia     int    `json:"verifyMedia" dc:"验证媒体数量"`
	VerifyEventId   int64  `json:"verifyEventId" dc:"匹配验证事件ID"`
	VerifyMessageId int64  `json:"verifyMessageId" dc:"匹配验证消息ID"`
	ReviewId        int64  `json:"reviewId" dc:"审核ID"`
	ReviewStatus    string `json:"reviewStatus" dc:"审核状态"`
	ErrorMessage    string `json:"errorMessage" dc:"错误信息"`
}

type CollectMaterialDiagnoseModel struct {
	TotalEvents        int                            `json:"totalEvents" dc:"事件总数"`
	DisplayEvents      int                            `json:"displayEvents" dc:"展示事件数"`
	VerifyEvents       int                            `json:"verifyEvents" dc:"验证事件数"`
	PairedEvents       int                            `json:"pairedEvents" dc:"已匹配验证事件数"`
	UnmatchedVerify    int                            `json:"unmatchedVerify" dc:"未匹配验证事件数"`
	MissingVerify      int                            `json:"missingVerify" dc:"未找到验证事件的展示事件数"`
	ReviewEvents       int                            `json:"reviewEvents" dc:"已进入审核事件数"`
	MediaPendingEvents int                            `json:"mediaPendingEvents" dc:"媒体处理中事件数"`
	WaitingEvents      int                            `json:"waitingEvents" dc:"等待前序/分组事件数"`
	FailedEvents       int                            `json:"failedEvents" dc:"失败事件数"`
	Items              []*CollectMaterialDiagnoseItem `json:"items" dc:"诊断明细"`
}

type CollectMediaBenchmarkInp struct {
	AccountId     int64 `json:"accountId" dc:"上架账号ID，仅超级管理员可指定"`
	SourceId      int64 `json:"sourceId" dc:"采集源ID，0表示全部"`
	Limit         int   `json:"limit" dc:"压测媒体数"`
	Concurrency   int   `json:"concurrency" dc:"并发数"`
	IncludeCached bool  `json:"includeCached" dc:"是否包含已缓存媒体"`
}

type CollectMediaBenchmarkItem struct {
	MediaId      int64  `json:"mediaId" dc:"媒体ID"`
	EventId      int64  `json:"eventId" dc:"事件ID"`
	SourceId     int64  `json:"sourceId" dc:"采集源ID"`
	TgAccountId  int64  `json:"tgAccountId" dc:"TG账号ID"`
	MediaType    string `json:"mediaType" dc:"媒体类型"`
	ExpectedSize int64  `json:"expectedSize" dc:"预期字节数"`
	DurationMs   int64  `json:"durationMs" dc:"耗时毫秒"`
	Success      bool   `json:"success" dc:"是否成功"`
	Error        string `json:"error,omitempty" dc:"错误"`
}

type CollectMediaBenchmarkModel struct {
	Total           int                          `json:"total" dc:"总数"`
	Started         int                          `json:"started" dc:"已开始数"`
	Succeeded       int                          `json:"succeeded" dc:"成功数"`
	Failed          int                          `json:"failed" dc:"失败数"`
	Concurrency     int                          `json:"concurrency" dc:"实际并发"`
	TotalDurationMs int64                        `json:"totalDurationMs" dc:"总耗时毫秒"`
	P50DurationMs   int64                        `json:"p50DurationMs" dc:"P50耗时毫秒"`
	P95DurationMs   int64                        `json:"p95DurationMs" dc:"P95耗时毫秒"`
	Bytes           int64                        `json:"bytes" dc:"预期下载字节数"`
	ThroughputMbps  float64                      `json:"throughputMbps" dc:"吞吐Mbps"`
	Items           []*CollectMediaBenchmarkItem `json:"items" dc:"明细"`
}

type CollectEventModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	TenantId           int64       `json:"tenantId" dc:"租户ID"`
	AccountId          int64       `json:"accountId" dc:"账号ID"`
	SourceId           int64       `json:"sourceId" dc:"采集源ID"`
	SourceTitle        string      `json:"sourceTitle" dc:"采集源名称"`
	SourceType         string      `json:"sourceType" dc:"来源类型"`
	BotId              int64       `json:"botId" dc:"机器人ID"`
	TgAccountId        int64       `json:"tgAccountId" dc:"协议号ID"`
	SourceChatId       string      `json:"sourceChatId" dc:"来源Chat ID"`
	SourceMessageId    int64       `json:"sourceMessageId" dc:"来源消息ID"`
	SourceGroupedId    string      `json:"sourceGroupedId" dc:"媒体组ID"`
	SourceUniqueKey    string      `json:"sourceUniqueKey" dc:"来源唯一键"`
	RawText            string      `json:"rawText" dc:"原始文本"`
	MediaCount         int         `json:"mediaCount" dc:"媒体数"`
	MediaJson          string      `json:"mediaJson" dc:"媒体JSON"`
	MediaCacheStatus   string      `json:"mediaCacheStatus" dc:"媒体缓存状态"`
	MediaCacheMessage  string      `json:"mediaCacheMessage" dc:"媒体缓存说明"`
	TextHash           string      `json:"textHash" dc:"文本哈希"`
	DedupeKey          string      `json:"dedupeKey" dc:"去重键"`
	Status             string      `json:"status" dc:"状态"`
	IgnoreType         string      `json:"ignoreType" dc:"忽略类型"`
	ErrorMessage       string      `json:"errorMessage" dc:"错误信息"`
	TargetChannelIds   []int64     `json:"targetChannelIds" dc:"目标频道ID"`
	TargetChannelNames []string    `json:"targetChannelNames" dc:"目标频道名称"`
	ReviewStatus       string      `json:"reviewStatus" dc:"审核状态"`
	DispatchStatus     string      `json:"dispatchStatus" dc:"分发状态"`
	ReceivedAt         *gtime.Time `json:"receivedAt" dc:"接收时间"`
	ProcessedAt        *gtime.Time `json:"processedAt" dc:"处理时间"`
	CreatedAt          *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type CollectEventClearInp struct {
	SourceId int64 `json:"sourceId" v:"required|min:1#采集源ID不能为空|采集源ID不能为空" dc:"采集源ID"`
}

type CollectEventReprocessInp struct {
	EventId int64 `json:"eventId" v:"required|min:1#事件ID不能为空|事件ID不能为空" dc:"事件ID"`
}

type CollectEventReprocessModel struct {
	EventId int64 `json:"eventId" dc:"事件ID"`
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
	Cursor   string `json:"cursor" dc:"下一页游标"`
	Status   string `json:"status" dc:"状态"`
	SourceId int64  `json:"sourceId" dc:"采集源ID"`
	RuleId   int64  `json:"ruleId" dc:"规则ID"`
	Keyword  string `json:"keyword" dc:"关键词"`
}

type CollectReviewModel struct {
	Id                  int64                      `json:"id" dc:"ID"`
	TenantId            int64                      `json:"tenantId" dc:"租户ID"`
	AccountId           int64                      `json:"accountId" dc:"账号ID"`
	SourceId            int64                      `json:"sourceId" dc:"采集源ID"`
	SourceTitle         string                     `json:"sourceTitle" dc:"采集源名称"`
	SourceType          string                     `json:"sourceType" dc:"采集源类型"`
	SourceDisplayName   string                     `json:"sourceDisplayName" dc:"采集来源显示名称"`
	SourceUsername      string                     `json:"sourceUsername" dc:"采集来源用户名"`
	RuleId              int64                      `json:"ruleId" dc:"规则ID"`
	RuleName            string                     `json:"ruleName" dc:"规则名称"`
	EventId             int64                      `json:"eventId" dc:"事件ID"`
	DispatchId          int64                      `json:"dispatchId" dc:"分发ID"`
	RawText             string                     `json:"rawText" dc:"原始文本"`
	MediaCount          int                        `json:"mediaCount" dc:"媒体数"`
	MediaJson           string                     `json:"mediaJson" dc:"审核媒体快照"`
	Media               []*CollectReviewMediaModel `json:"media" dc:"审核媒体"`
	TargetChannelIdJson string                     `json:"targetChannelIdJson" dc:"目标频道JSON"`
	TargetChannelNames  []string                   `json:"targetChannelNames" dc:"目标频道名称"`
	BotIdJson           string                     `json:"botIdJson" dc:"BOT JSON"`
	Status              string                     `json:"status" dc:"审核状态"`
	ReviewReason        string                     `json:"reviewReason" dc:"审核原因"`
	ReviewedBy          int64                      `json:"reviewedBy" dc:"审核人"`
	ReviewedAt          *gtime.Time                `json:"reviewedAt" dc:"审核时间"`
	CreatedAt           *gtime.Time                `json:"createdAt" dc:"创建时间"`
	UpdatedAt           *gtime.Time                `json:"updatedAt" dc:"更新时间"`
}

type CollectReviewMediaModel struct {
	Type        string `json:"type" dc:"媒体类型"`
	Purpose     string `json:"purpose" dc:"用途"`
	FileId      string `json:"fileId" dc:"文件ID"`
	FileUrl     string `json:"fileUrl" dc:"文件地址"`
	StoragePath string `json:"storagePath" dc:"存储路径"`
	PosterUrl   string `json:"posterUrl" dc:"预览图地址"`
	MetaJson    string `json:"metaJson" dc:"媒体元数据"`
}

type CollectReviewPageModel struct {
	List       []*CollectReviewModel `json:"list" dc:"审核列表"`
	HasMore    bool                  `json:"hasMore" dc:"是否还有下一页"`
	NextCursor string                `json:"nextCursor" dc:"下一页游标"`
}

type CollectReviewEditInp struct {
	Id      int64  `json:"id" v:"required|min:1#审核ID不能为空|审核ID不能为空" dc:"审核ID"`
	RawText string `json:"rawText" dc:"审核文案"`
}

func (in *CollectReviewEditInp) Filter(ctx context.Context) error {
	in.RawText = strings.TrimSpace(in.RawText)
	if in.Id <= 0 {
		return gerror.New("审核ID不能为空")
	}
	if in.RawText == "" {
		return gerror.New("审核文案不能为空")
	}
	return nil
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
