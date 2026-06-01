package sysin

import (
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/os/gtime"
)

type ContentProfileListInp struct {
	form.PageReq
	Keyword  string `json:"keyword" dc:"关键词"`
	Province string `json:"province" dc:"省份"`
	City     string `json:"city" dc:"城市"`
	Sort     string `json:"sort" dc:"排序"`
}

type ContentProfileListModel struct {
	Id          int64       `json:"id" dc:"ID"`
	ProfileNo   string      `json:"profileNo" dc:"资料编号"`
	Name        string      `json:"name" dc:"昵称"`
	Title       string      `json:"title" dc:"标题"`
	Summary     string      `json:"summary" dc:"摘要"`
	Province    string      `json:"province" dc:"省份"`
	City        string      `json:"city" dc:"城市"`
	Age         int         `json:"age" dc:"年龄"`
	Height      int         `json:"height" dc:"身高"`
	Weight      int         `json:"weight" dc:"体重"`
	Cup         string      `json:"cup" dc:"资料标签"`
	Avatar      string      `json:"avatar" dc:"主图"`
	CoverUrl    string      `json:"coverUrl" dc:"封面"`
	HasVideo    bool        `json:"hasVideo" dc:"是否有视频"`
	VideoLocked bool        `json:"videoLocked" dc:"视频是否锁定"`
	Verified    bool        `json:"verified" dc:"是否认证"`
	PublishedAt *gtime.Time `json:"publishedAt" dc:"发布时间"`
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
	ProcessDone bool   `json:"processDone" dc:"是否处理完成"`
}

type ContentProfileViewModel struct {
	ContentProfileListModel
	Intro      string               `json:"intro" dc:"简介"`
	PlainText  string               `json:"plainText" dc:"正文"`
	Photos     []string             `json:"photos" dc:"图片展示地址"`
	Media      []*ContentMediaModel `json:"media" dc:"媒体列表"`
	ImageCount int                  `json:"imageCount" dc:"图片数"`
	VideoCount int                  `json:"videoCount" dc:"视频数"`
	MemberOnly bool                 `json:"memberOnly" dc:"会员可见"`
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
