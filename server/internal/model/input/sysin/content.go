package sysin

import (
	"context"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"
)

type ContentProfileListInp struct {
	form.PageReq
	Feed            string `json:"feed" dc:"首页流：nearby/latest/hot"`
	Keyword         string `json:"keyword" dc:"关键词"`
	Province        string `json:"province" dc:"省份"`
	City            string `json:"city" dc:"城市"`
	AgeMin          int    `json:"ageMin" dc:"最小年龄"`
	AgeMax          int    `json:"ageMax" dc:"最大年龄"`
	AgeRanges       string `json:"ageRanges" dc:"年龄范围，逗号分隔"`
	HeightMin       int    `json:"heightMin" dc:"最小身高"`
	HeightMax       int    `json:"heightMax" dc:"最大身高"`
	HeightRanges    string `json:"heightRanges" dc:"身高范围，逗号分隔"`
	WeightMin       int    `json:"weightMin" dc:"最小体重"`
	WeightMax       int    `json:"weightMax" dc:"最大体重"`
	WeightRanges    string `json:"weightRanges" dc:"体重范围，逗号分隔"`
	Cup             string `json:"cup" dc:"资料标签"`
	Cups            string `json:"cups" dc:"资料标签，逗号分隔"`
	HasVideo        int    `json:"hasVideo" dc:"是否有视频"`
	HasVerification int    `json:"hasVerification" dc:"是否有验证视频"`
	CanFly          int    `json:"canFly" dc:"可飞外省"`
	CanGoAbroad     int    `json:"canGoAbroad" dc:"可出国"`
	CanOvernight    int    `json:"canOvernight" dc:"可过夜"`
	CanCohabitate   int    `json:"canCohabitate" dc:"可同居"`
	HasHealthCheck  int    `json:"hasHealthCheck" dc:"有体检"`
	IsFullMonth     int    `json:"isFullMonth" dc:"满月"`
	IsVirgin        int    `json:"isVirgin" dc:"是否处"`
	AcceptSm        int    `json:"acceptSm" dc:"接受SM"`
	NoCondom        int    `json:"noCondom" dc:"体检后无套"`
	AllowCreampie   int    `json:"allowCreampie" dc:"可内射"`
	HasTattoo       int    `json:"hasTattoo" dc:"有纹身"`
	Sort            string `json:"sort" dc:"排序"`
	WithTotal       int    `json:"withTotal" dc:"是否返回总数"`
	ExcludeActions  []string
}

type HomeProfileCardsInp struct {
	ContentProfileListInp
}

type ContentProfileListModel struct {
	Id          int64                `json:"id" dc:"ID"`
	ProfileNo   string               `json:"profileNo" dc:"资料编号"`
	Name        string               `json:"name" dc:"昵称"`
	Title       string               `json:"title" dc:"标题"`
	Summary     string               `json:"summary" dc:"摘要"`
	Province    string               `json:"province" dc:"省份"`
	City        string               `json:"city" dc:"城市"`
	Age         int                  `json:"age" dc:"年龄"`
	Height      int                  `json:"height" dc:"身高"`
	Weight      int                  `json:"weight" dc:"体重"`
	Cup         string               `json:"cup" dc:"资料标签"`
	Avatar      string               `json:"avatar" dc:"主图"`
	CoverUrl    string               `json:"coverUrl" dc:"封面"`
	HasVideo    bool                 `json:"hasVideo" dc:"是否有视频"`
	VideoLocked bool                 `json:"videoLocked" dc:"视频是否锁定"`
	Verified    bool                 `json:"verified" dc:"是否认证"`
	ImageCount  int                  `json:"imageCount" dc:"图片数"`
	VideoCount  int                  `json:"videoCount" dc:"视频数"`
	Media       []*ContentMediaModel `json:"media" dc:"媒体列表"`
	Photos      []string             `json:"photos" dc:"图片展示地址"`
	PublishedAt *gtime.Time          `json:"publishedAt" dc:"发布时间"`
	ActionAt    *gtime.Time          `json:"actionAt" dc:"用户动作时间"`
}

type ContentProfileFilterOptionsModel struct {
	Regions    []*ContentProfileRegionOption    `json:"regions" dc:"地区选项"`
	Cups       []*ContentProfileFilterOption    `json:"cups" dc:"资料标签选项"`
	Attributes []*ContentProfileAttributeOption `json:"attributes" dc:"属性选项"`
}

type ContentProfileRegionsModel struct {
	Regions []*ContentProfileRegionOption `json:"regions" dc:"地区选项"`
}

type ContentProfileRegionOption struct {
	Label    string                        `json:"label" dc:"显示名称"`
	Value    string                        `json:"value" dc:"筛选值"`
	Province string                        `json:"province" dc:"省份/国家"`
	City     string                        `json:"city" dc:"城市"`
	Count    int                           `json:"count" dc:"资料数量"`
	Children []*ContentProfileRegionOption `json:"children,omitempty" dc:"城市列表"`
}

type ContentProfileFilterOption struct {
	Label string `json:"label" dc:"显示名称"`
	Value string `json:"value" dc:"筛选值"`
	Count int    `json:"count" dc:"资料数量"`
}

type ContentProfileAttributeOption struct {
	ContentProfileFilterOption
	Key string `json:"key" dc:"属性键"`
}

type ContentProfileViewInp struct {
	Id int64 `json:"id" v:"required#资料ID不能为空" dc:"资料ID"`
}

type ContentMediaModel struct {
	Id          int64  `json:"id" dc:"ID"`
	Type        string `json:"type" dc:"媒体类型"`
	DisplayUrl  string `json:"displayUrl" dc:"展示地址"`
	PreviewUrl  string `json:"previewUrl" dc:"预览地址"`
	Width       int    `json:"width" dc:"宽度"`
	Height      int    `json:"height" dc:"高度"`
	Duration    int    `json:"duration" dc:"时长"`
	Locked      bool   `json:"locked" dc:"是否锁定"`
	Placeholder bool   `json:"placeholder" dc:"是否占位媒体"`
	ProcessDone bool   `json:"processDone" dc:"是否处理完成"`
}

type ContentProfileViewModel struct {
	ContentProfileListModel
	Intro      string                         `json:"intro" dc:"简介"`
	PlainText  string                         `json:"plainText" dc:"正文"`
	Photos     []string                       `json:"photos" dc:"图片展示地址"`
	Media      []*ContentMediaModel           `json:"media" dc:"媒体列表"`
	Attributes []*ContentProfileAttributeItem `json:"attributes" dc:"资料属性"`
	ImageCount int                            `json:"imageCount" dc:"图片数"`
	VideoCount int                            `json:"videoCount" dc:"视频数"`
	MemberOnly bool                           `json:"memberOnly" dc:"会员可见"`
}

type ContentProfileAttributeItem struct {
	Label string `json:"label" dc:"属性名"`
	Value string `json:"value" dc:"属性值"`
}

type ContentImportFeiNiuInp struct {
	BatchSize   int    `json:"batchSize" dc:"批量数量"`
	TriggerType string `json:"triggerType" dc:"触发方式"`
}

type ContentImportFeiNiuModel struct {
	Scanned        int   `json:"scanned" dc:"扫描数量"`
	Imported       int   `json:"imported" dc:"导入数量"`
	Duplicate      int   `json:"duplicate" dc:"重复数量"`
	MediaImported  int   `json:"mediaImported" dc:"媒体导入数量"`
	LastSourceNote int64 `json:"lastSourceNote" dc:"最后来源笔记ID"`
}

type ContentDedupePHashInp struct {
	StartId int64 `json:"startId" dc:"起始资料ID"`
	Limit   int   `json:"limit" dc:"处理上限"`
}

type ContentDedupePHashModel struct {
	Scanned int   `json:"scanned" dc:"扫描资料数"`
	Frozen  int   `json:"frozen" dc:"停用重复资料数"`
	LastId  int64 `json:"lastId" dc:"最后扫描资料ID"`
}

type ContentImportOverviewInp struct {
	SourceName string `json:"sourceName" dc:"来源名称"`
}

type ContentImportOverviewModel struct {
	SourceName       string      `json:"sourceName" dc:"来源名称"`
	ProfileTotal     int         `json:"profileTotal" dc:"资料总数"`
	PublicTotal      int         `json:"publicTotal" dc:"公开资料数"`
	PendingTotal     int         `json:"pendingTotal" dc:"待审核资料数"`
	DuplicateTotal   int         `json:"duplicateTotal" dc:"重复资料数"`
	MediaTotal       int         `json:"mediaTotal" dc:"媒体总数"`
	DuplicateMedia   int         `json:"duplicateMedia" dc:"重复媒体数"`
	LastSourceNoteId int64       `json:"lastSourceNoteId" dc:"最后来源笔记ID"`
	LastSuccessAt    *gtime.Time `json:"lastSuccessAt" dc:"最后成功时间"`
	LastError        string      `json:"lastError" dc:"最后错误"`
	LastRunStatus    string      `json:"lastRunStatus" dc:"最近运行状态"`
	LastRunCostMs    int         `json:"lastRunCostMs" dc:"最近运行耗时"`
	AutoSyncCronId   int64       `json:"autoSyncCronId" dc:"自动同步任务ID"`
	AutoSyncEnabled  bool        `json:"autoSyncEnabled" dc:"自动同步是否启用"`
	AutoSyncStatus   string      `json:"autoSyncStatus" dc:"自动同步状态"`
	AutoSyncPattern  string      `json:"autoSyncPattern" dc:"自动同步表达式"`
}

type ContentImportAutoSyncInp struct {
	SourceName string `json:"sourceName" dc:"来源名称"`
	Enabled    bool   `json:"enabled" dc:"是否启用"`
}

type ContentImportAutoSyncModel struct {
	SourceName      string `json:"sourceName" dc:"来源名称"`
	AutoSyncCronId  int64  `json:"autoSyncCronId" dc:"自动同步任务ID"`
	AutoSyncEnabled bool   `json:"autoSyncEnabled" dc:"自动同步是否启用"`
	AutoSyncStatus  string `json:"autoSyncStatus" dc:"自动同步状态"`
	AutoSyncPattern string `json:"autoSyncPattern" dc:"自动同步表达式"`
}

type ContentImportReviewConfigInp struct {
	SourceName string `json:"sourceName" dc:"来源名称"`
}

type ContentImportReviewConfigModel struct {
	SourceName            string `json:"sourceName" dc:"来源名称"`
	ReviewBatchSize       int    `json:"reviewBatchSize" dc:"审核数量"`
	ReviewIntervalMinutes int    `json:"reviewIntervalMinutes" dc:"审核间隔分钟"`
	AutoApproveImported   int    `json:"autoApproveImported" dc:"导入后自动通过"`
	FreezeDuplicate       int    `json:"freezeDuplicate" dc:"重复资料自动冻结"`
	DefaultReviewStatus   string `json:"defaultReviewStatus" dc:"默认审核状态"`
	ReviewRemark          string `json:"reviewRemark" dc:"审核备注"`
}

type ContentImportReviewConfigEditInp struct {
	ContentImportReviewConfigModel
}

func (in *ContentImportReviewConfigEditInp) Filter(ctx context.Context) (err error) {
	if in.SourceName == "" {
		in.SourceName = "feiniu"
	}
	if in.ReviewBatchSize <= 0 {
		in.ReviewBatchSize = 200
	}
	if in.ReviewIntervalMinutes <= 0 {
		in.ReviewIntervalMinutes = 30
	}
	if in.DefaultReviewStatus == "" {
		in.DefaultReviewStatus = "approved"
	}
	return
}

type ContentImportRunListInp struct {
	form.PageReq
	SourceName string `json:"sourceName" dc:"来源名称"`
	Status     string `json:"status" dc:"运行状态"`
}

type ContentImportRunListModel struct {
	Id               int64       `json:"id" dc:"ID"`
	SourceName       string      `json:"sourceName" dc:"来源名称"`
	TriggerType      string      `json:"triggerType" dc:"触发方式"`
	BatchSize        int         `json:"batchSize" dc:"批量数量"`
	Scanned          int         `json:"scanned" dc:"扫描数量"`
	Imported         int         `json:"imported" dc:"导入数量"`
	Duplicate        int         `json:"duplicate" dc:"重复数量"`
	MediaImported    int         `json:"mediaImported" dc:"媒体导入数量"`
	LastSourceNoteId int64       `json:"lastSourceNoteId" dc:"最后来源笔记ID"`
	Status           string      `json:"status" dc:"运行状态"`
	ErrorMessage     string      `json:"errorMessage" dc:"错误信息"`
	StartedAt        *gtime.Time `json:"startedAt" dc:"开始时间"`
	FinishedAt       *gtime.Time `json:"finishedAt" dc:"结束时间"`
	CostMs           int         `json:"costMs" dc:"耗时毫秒"`
}

type ContentNoteListInp struct {
	form.PageReq
	Id              int64         `json:"id" dc:"ID"`
	ProfileNo       string        `json:"profileNo" dc:"资料编号"`
	Keyword         string        `json:"keyword" dc:"关键词"`
	SourceNoteId    int64         `json:"sourceNoteId" dc:"来源笔记ID"`
	SourceChannelId int64         `json:"sourceChannelId" dc:"来源频道ID"`
	ChannelKeyword  string        `json:"channelKeyword" dc:"频道关键词"`
	Province        string        `json:"province" dc:"省份"`
	City            string        `json:"city" dc:"城市"`
	Visibility      string        `json:"visibility" dc:"可见性"`
	ReviewStatus    string        `json:"reviewStatus" dc:"审核状态"`
	ImportStatus    string        `json:"importStatus" dc:"导入状态"`
	CupSize         string        `json:"cupSize" dc:"资料标签"`
	AgeMin          int           `json:"ageMin" dc:"最小年龄"`
	AgeMax          int           `json:"ageMax" dc:"最大年龄"`
	HeightMin       int           `json:"heightMin" dc:"最小身高"`
	HeightMax       int           `json:"heightMax" dc:"最大身高"`
	WeightMin       int           `json:"weightMin" dc:"最小体重"`
	WeightMax       int           `json:"weightMax" dc:"最大体重"`
	DaysMin         int           `json:"daysMin" dc:"最小陪伴天数"`
	DaysMax         int           `json:"daysMax" dc:"最大陪伴天数"`
	CostMin         int           `json:"costMin" dc:"最小期望生活费"`
	CostMax         int           `json:"costMax" dc:"最大期望生活费"`
	CanFly          int           `json:"canFly" dc:"可飞外省"`
	CanGoAbroad     int           `json:"canGoAbroad" dc:"可出国"`
	CanOvernight    int           `json:"canOvernight" dc:"可过夜"`
	CanCohabitate   int           `json:"canCohabitate" dc:"可同居"`
	HasHealthCheck  int           `json:"hasHealthCheck" dc:"有体检"`
	IsFullMonth     int           `json:"isFullMonth" dc:"满月"`
	IsVirgin        int           `json:"isVirgin" dc:"是否处"`
	AcceptSm        int           `json:"acceptSm" dc:"接受SM"`
	NoCondom        int           `json:"noCondom" dc:"体检后无套"`
	AllowCreampie   int           `json:"allowCreampie" dc:"可内射"`
	HasTattoo       int           `json:"hasTattoo" dc:"有纹身"`
	IsFavorite      int           `json:"isFavorite" dc:"收藏"`
	Status          int           `json:"status" dc:"状态"`
	HasVerification int           `json:"hasVerification" dc:"是否有验证视频"`
	MemberOnlyVideo int           `json:"memberOnlyVideo" dc:"视频是否会员可见"`
	HasVideo        int           `json:"hasVideo" dc:"是否有视频"`
	HasDuplicate    int           `json:"hasDuplicate" dc:"是否重复"`
	CreatedAt       []*gtime.Time `json:"createdAt" dc:"创建时间"`
}

type ContentNoteListModel struct {
	Id                   int64                    `json:"id" dc:"ID"`
	ProfileNo            string                   `json:"profileNo" dc:"资料编号"`
	SourceType           string                   `json:"sourceType" dc:"来源类型"`
	SourceNoteId         int64                    `json:"sourceNoteId" dc:"来源笔记ID"`
	SourceKey            string                   `json:"sourceKey" dc:"来源唯一键"`
	SourceTextHash       string                   `json:"sourceTextHash" dc:"来源文本哈希"`
	ChannelId            int64                    `json:"channelId" dc:"本地频道ID"`
	SourceChannelId      int64                    `json:"sourceChannelId" dc:"来源频道ID"`
	ChannelTitle         string                   `json:"channelTitle" dc:"频道标题"`
	ChannelUsername      string                   `json:"channelUsername" dc:"频道用户名"`
	TgChatId             string                   `json:"tgChatId" dc:"TG Chat ID"`
	SourceMessageId      int64                    `json:"sourceMessageId" dc:"来源消息ID"`
	Title                string                   `json:"title" dc:"标题"`
	Summary              string                   `json:"summary" dc:"摘要"`
	PlainText            string                   `json:"plainText" dc:"正文纯文本"`
	HtmlText             string                   `json:"htmlText" dc:"HTML正文"`
	SourceCategoryCode   string                   `json:"sourceCategoryCode" dc:"FeiNiu分类编码"`
	Province             string                   `json:"province" dc:"省份"`
	City                 string                   `json:"city" dc:"城市"`
	Age                  int                      `json:"age" dc:"年龄"`
	Height               int                      `json:"height" dc:"身高"`
	Weight               int                      `json:"weight" dc:"体重"`
	CupSize              string                   `json:"cupSize" dc:"资料标签"`
	DaysWithEscort       int                      `json:"daysWithEscort" dc:"陪伴天数"`
	ExpectedLivingCost   int                      `json:"expectedLivingCost" dc:"期望生活费"`
	CanFlyToProvince     int                      `json:"canFlyToProvince" dc:"可飞外省"`
	CanGoAbroad          int                      `json:"canGoAbroad" dc:"可出国"`
	CanOvernight         int                      `json:"canOvernight" dc:"可过夜"`
	CanCohabitate        int                      `json:"canCohabitate" dc:"可同居"`
	HasHealthCheck       int                      `json:"hasHealthCheck" dc:"有体检"`
	IsFullMonth          int                      `json:"isFullMonth" dc:"满月"`
	IsVirgin             int                      `json:"isVirgin" dc:"是否处"`
	AcceptSm             int                      `json:"acceptSm" dc:"接受SM"`
	NoCondomAfterCheck   int                      `json:"noCondomAfterCheck" dc:"体检后无套"`
	AllowCreampie        int                      `json:"allowCreampie" dc:"可内射"`
	HasTattoo            int                      `json:"hasTattoo" dc:"有纹身"`
	IsFavorite           int                      `json:"isFavorite" dc:"收藏"`
	SourceEditedAt       *gtime.Time              `json:"sourceEditedAt" dc:"FeiNiu编辑时间"`
	GroupParams          string                   `json:"groupParams" dc:"分组参数"`
	TagParams            string                   `json:"tagParams" dc:"标签参数"`
	TextBlockCount       int                      `json:"textBlockCount" dc:"文本块数"`
	StoragePolicy        string                   `json:"storagePolicy" dc:"存储策略"`
	SourceRemark         string                   `json:"sourceRemark" dc:"FeiNiu备注"`
	SourceCreateBy       string                   `json:"sourceCreateBy" dc:"FeiNiu创建者"`
	SourceUpdateBy       string                   `json:"sourceUpdateBy" dc:"FeiNiu更新者"`
	SourceCreatedAt      *gtime.Time              `json:"sourceCreatedAt" dc:"FeiNiu创建时间"`
	SourceUpdatedAt      *gtime.Time              `json:"sourceUpdatedAt" dc:"FeiNiu更新时间"`
	ImageCount           int                      `json:"imageCount" dc:"图片数"`
	VideoCount           int                      `json:"videoCount" dc:"视频数"`
	HasVerificationVideo int                      `json:"hasVerificationVideo" dc:"是否有验证视频"`
	MemberOnlyVideo      int                      `json:"memberOnlyVideo" dc:"视频是否会员可见"`
	DuplicateOfId        int64                    `json:"duplicateOfId" dc:"重复资料ID"`
	Visibility           string                   `json:"visibility" dc:"可见性"`
	ReviewStatus         string                   `json:"reviewStatus" dc:"审核状态"`
	ImportStatus         string                   `json:"importStatus" dc:"导入状态"`
	AdminRemark          string                   `json:"adminRemark" dc:"后台备注"`
	Status               int                      `json:"status" dc:"状态"`
	PublishedAt          *gtime.Time              `json:"publishedAt" dc:"发布时间"`
	CreatedAt            *gtime.Time              `json:"createdAt" dc:"创建时间"`
	UpdatedAt            *gtime.Time              `json:"updatedAt" dc:"更新时间"`
	Media                []*ContentNoteMediaModel `json:"media" dc:"媒体列表"`
}

type ContentNoteViewInp struct {
	Id int64 `json:"id" v:"required#笔记ID不能为空" dc:"ID"`
}

type ContentNoteUpdateFields struct {
	Title                string `json:"title" dc:"标题"`
	Summary              string `json:"summary" dc:"摘要"`
	PlainText            string `json:"plainText" dc:"正文纯文本"`
	Province             string `json:"province" dc:"省份"`
	City                 string `json:"city" dc:"城市"`
	Age                  int    `json:"age" dc:"年龄"`
	Height               int    `json:"height" dc:"身高"`
	Weight               int    `json:"weight" dc:"体重"`
	CupSize              string `json:"cupSize" dc:"资料标签"`
	HasVerificationVideo int    `json:"hasVerificationVideo" dc:"是否有验证视频"`
	MemberOnlyVideo      int    `json:"memberOnlyVideo" dc:"视频是否仅会员可见"`
	Visibility           string `json:"visibility" dc:"可见性"`
	ReviewStatus         string `json:"reviewStatus" dc:"审核状态"`
	ImportStatus         string `json:"importStatus" dc:"导入状态"`
	AdminRemark          string `json:"adminRemark" dc:"后台备注"`
	Status               int    `json:"status" dc:"状态"`
}

type ContentNoteEditInp struct {
	Id int64 `json:"id" v:"required#笔记ID不能为空" dc:"ID"`
	ContentNoteUpdateFields
}

func (in *ContentNoteEditInp) Filter(ctx context.Context) (err error) {
	if in.Id <= 0 {
		return gerror.New("笔记ID不能为空")
	}
	return
}

type ContentNoteMediaModel struct {
	Id                 int64  `json:"id" dc:"ID"`
	ProfileId          int64  `json:"profileId" dc:"资料ID"`
	SourceAssetId      int64  `json:"sourceAssetId" dc:"来源资源ID"`
	DuplicateOfMediaId int64  `json:"duplicateOfMediaId" dc:"重复媒体ID"`
	MediaType          string `json:"mediaType" dc:"媒体类型"`
	SortIndex          int    `json:"sortIndex" dc:"排序"`
	DisplayStoragePath string `json:"displayStoragePath" dc:"展示路径"`
	PreviewStoragePath string `json:"previewStoragePath" dc:"预览路径"`
	BinaryMd5          string `json:"binaryMd5" dc:"MD5"`
	Width              int    `json:"width" dc:"宽度"`
	Height             int    `json:"height" dc:"高度"`
	Duration           int    `json:"duration" dc:"时长"`
	ProcessStatus      string `json:"processStatus" dc:"处理状态"`
	EncryptStatus      string `json:"encryptStatus" dc:"加密状态"`
	Status             int    `json:"status" dc:"状态"`
}

type ContentNoteMediaUpdateFields struct {
	DisplayStoragePath string `json:"displayStoragePath" dc:"展示路径"`
	PreviewStoragePath string `json:"previewStoragePath" dc:"预览路径"`
	SortIndex          int    `json:"sortIndex" dc:"排序"`
	Status             int    `json:"status" dc:"状态"`
}

type ContentNoteMediaEditInp struct {
	Id int64 `json:"id" v:"required#媒体ID不能为空" dc:"媒体ID"`
	ContentNoteMediaUpdateFields
}

func (in *ContentNoteMediaEditInp) Filter(ctx context.Context) (err error) {
	if in.Id <= 0 {
		return gerror.New("媒体ID不能为空")
	}
	if in.Status <= 0 {
		in.Status = 1
	}
	return
}

type ContentNoteBatchDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#笔记ID不能为空" dc:"ID列表"`
}

type ContentNoteBatchReviewInp struct {
	Ids          []int64 `json:"ids" v:"required#笔记ID不能为空" dc:"ID列表"`
	ReviewStatus string  `json:"reviewStatus" v:"required#审核状态不能为空" dc:"审核状态"`
}

type ContentNoteBatchStatusInp struct {
	Ids    []int64 `json:"ids" v:"required#笔记ID不能为空" dc:"ID列表"`
	Status int     `json:"status" v:"required#状态不能为空" dc:"状态"`
}

type ContentNoteBatchRemarkInp struct {
	Ids         []int64 `json:"ids" v:"required#笔记ID不能为空" dc:"ID列表"`
	AdminRemark string  `json:"adminRemark" dc:"后台备注"`
}

type ContentNoteSourceModel struct {
	SourceType      string `json:"sourceType" dc:"来源类型"`
	SourceKey       string `json:"sourceKey" dc:"来源唯一键"`
	SourceChannelId int64  `json:"sourceChannelId" dc:"来源频道ID"`
	SourceMessageId int64  `json:"sourceMessageId" dc:"来源消息ID"`
	SourceGroupedId int64  `json:"sourceGroupedId" dc:"来源媒体组ID"`
	SourceTextHash  string `json:"sourceTextHash" dc:"来源文本哈希"`
	RawText         string `json:"rawText" dc:"原始文本"`
	RawMessageJson  string `json:"rawMessageJson" dc:"原始消息JSON"`
}

type ContentNoteViewModel struct {
	ContentNoteListModel
	PlainText string                   `json:"plainText" dc:"正文纯文本"`
	Source    *ContentNoteSourceModel  `json:"source" dc:"来源映射"`
	Media     []*ContentNoteMediaModel `json:"media" dc:"媒体列表"`
}
