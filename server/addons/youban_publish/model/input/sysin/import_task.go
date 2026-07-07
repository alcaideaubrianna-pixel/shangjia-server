package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/internal/model/input/form"
)

const (
	ImportTaskStatusPending  = "pending"
	ImportTaskStatusRunning  = "running"
	ImportTaskStatusSuccess  = "success"
	ImportTaskStatusFailed   = "failed"
	ImportTaskStatusCanceled = "canceled"

	ImportTaskModeIncremental = "incremental"
	ImportTaskModeOverwrite   = "overwrite"

	ImportTaskScanModeRecent = "recent"
	ImportTaskScanModeAll    = "all"

	ImportRunTypeImport = "import"
	ImportRunTypeScan   = "scan"
	ImportRunTypeRepair = "repair"
)

type ImportTaskCreateInp struct {
	Id               int64         `json:"id" dc:"任务ID"`
	SourceName       string        `json:"sourceName" dc:"来源名称"`
	TenantId         int64         `json:"tenantId" dc:"租户ID"`
	AccountId        int64         `json:"accountId" dc:"上架账号ID"`
	BaseUrl          string        `json:"baseUrl" v:"required#旧站域名不能为空" dc:"旧站域名"`
	ServerIp         string        `json:"serverIp" dc:"旧站服务器IP，DNS失效时使用"`
	Username         string        `json:"username" dc:"旧站账号"`
	Password         string        `json:"password" dc:"旧站密码"`
	LegacyCookie     string        `json:"legacyCookie" dc:"旧站登录Cookie"`
	LimitCount       int           `json:"limitCount" dc:"测试采集数量"`
	PerPage          int           `json:"perPage" dc:"每页数量"`
	ProxyEnabled     int           `json:"proxyEnabled" dc:"是否启用代理"`
	ProxyPool        string        `json:"proxyPool" dc:"代理池"`
	MediaConcurrency int           `json:"mediaConcurrency" dc:"媒体并发数"`
	ImportMode       string        `json:"importMode" dc:"导入方式：incremental增量 overwrite覆盖"`
	ChannelIds       []int64       `json:"channelIds" dc:"匹配频道ID"`
	TgRange          []*gtime.Time `json:"tgRange" dc:"TG消息时间范围"`
	Remark           string        `json:"remark" dc:"备注"`
}

func (in *ImportTaskCreateInp) Filter(ctx context.Context) error {
	in.SourceName = strings.TrimSpace(in.SourceName)
	if in.SourceName == "" {
		in.SourceName = "lyy_cms"
	}
	in.BaseUrl = strings.TrimRight(strings.TrimSpace(in.BaseUrl), "/")
	in.ServerIp = strings.TrimSpace(in.ServerIp)
	in.Username = strings.TrimSpace(in.Username)
	in.LegacyCookie = strings.TrimSpace(in.LegacyCookie)
	if in.BaseUrl == "" {
		return gerror.New("旧站域名不能为空")
	}
	if in.LegacyCookie == "" {
		if in.Username == "" {
			return gerror.New("旧站账号不能为空")
		}
		if in.Id <= 0 && in.Password == "" {
			return gerror.New("旧站密码不能为空")
		}
	}
	if in.PerPage <= 0 {
		in.PerPage = 12
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}
	if in.LimitCount < 0 {
		in.LimitCount = 0
	}
	if in.MediaConcurrency <= 0 {
		in.MediaConcurrency = 4
	}
	if in.MediaConcurrency > 20 {
		in.MediaConcurrency = 20
	}
	in.ImportMode = strings.TrimSpace(in.ImportMode)
	if in.ImportMode == "" {
		in.ImportMode = ImportTaskModeIncremental
	}
	if in.ImportMode != ImportTaskModeIncremental && in.ImportMode != ImportTaskModeOverwrite {
		return gerror.New("导入方式不合法")
	}
	if in.ProxyEnabled != 1 {
		in.ProxyEnabled = 0
	}
	in.Remark = strings.TrimSpace(in.Remark)
	return nil
}

type ImportTaskCreateModel struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type ImportTaskListInp struct {
	form.PageReq
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
	AccountId int64  `json:"accountId" dc:"上架账号ID"`
	Status    string `json:"status" dc:"状态"`
	Keyword   string `json:"keyword" dc:"关键词"`
}

type ImportTaskModel struct {
	entity.YoubanPublishImportTask
	TenantName  string  `json:"tenantName" dc:"租户名称"`
	AccountName string  `json:"accountName" dc:"上架账号名称"`
	Percent     float64 `json:"percent" dc:"进度百分比"`
}

type ImportTaskViewInp struct {
	Id int64 `json:"id" v:"required#任务ID不能为空" dc:"任务ID"`
}

type ImportTaskActionInp struct {
	Id int64 `json:"id" v:"required#任务ID不能为空" dc:"任务ID"`
}

type ImportTaskScanInp struct {
	Id          int64  `json:"id" v:"required#任务ID不能为空" dc:"任务ID"`
	ScanMode    string `json:"scanMode" dc:"扫描范围：recent最近N个 all全量"`
	RecentCount int    `json:"recentCount" dc:"最近扫描数量"`
}

func (in *ImportTaskScanInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("扫描参数不能为空")
	}
	in.ScanMode = strings.TrimSpace(in.ScanMode)
	if in.ScanMode == "" {
		in.ScanMode = ImportTaskScanModeRecent
	}
	if in.ScanMode != ImportTaskScanModeRecent && in.ScanMode != ImportTaskScanModeAll {
		return gerror.New("扫描范围不合法")
	}
	if in.ScanMode == ImportTaskScanModeRecent {
		if in.RecentCount <= 0 {
			in.RecentCount = 100
		}
		if in.RecentCount > 2000 {
			in.RecentCount = 2000
		}
	} else {
		in.RecentCount = 0
	}
	return nil
}

type ImportTaskRepairInp struct {
	ImportTaskScanInp
	ImportMode string `json:"importMode" dc:"补全方式：incremental增量 overwrite覆盖"`
}

func (in *ImportTaskRepairInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("补全参数不能为空")
	}
	if err := in.ImportTaskScanInp.Filter(ctx); err != nil {
		return err
	}
	in.ImportMode = strings.TrimSpace(in.ImportMode)
	if in.ImportMode == "" {
		in.ImportMode = ImportTaskModeIncremental
	}
	if in.ImportMode != ImportTaskModeIncremental && in.ImportMode != ImportTaskModeOverwrite {
		return gerror.New("补全方式不合法")
	}
	return nil
}

type ImportTaskScanModel struct {
	TaskId              int64                 `json:"taskId" dc:"任务ID"`
	ScanMode            string                `json:"scanMode" dc:"扫描范围"`
	RecentCount         int                   `json:"recentCount" dc:"最近数量"`
	SourceTotal         int                   `json:"sourceTotal" dc:"旧站资料数"`
	ExistingTotal       int                   `json:"existingTotal" dc:"已存在资料数"`
	MissingTotal        int                   `json:"missingTotal" dc:"缺失资料数"`
	MediaTotal          int                   `json:"mediaTotal" dc:"本地媒体数"`
	MediaMissingStorage int                   `json:"mediaMissingStorage" dc:"未迁移到当前存储媒体数"`
	CanRepairTotal      int                   `json:"canRepairTotal" dc:"可补齐数量"`
	Items               []*ImportTaskScanItem `json:"items" dc:"差异明细"`
	ScannedAt           *gtime.Time           `json:"scannedAt" dc:"扫描时间"`
}

type ImportTaskScanItem struct {
	SourceNoteId        int64  `json:"sourceNoteId" dc:"旧站资料ID"`
	SourceStatus        string `json:"sourceStatus" dc:"旧站状态：published/down/unpublished"`
	SourceStatusLabel   string `json:"sourceStatusLabel" dc:"旧站状态文本"`
	ClientRequestId     string `json:"clientRequestId" dc:"幂等ID"`
	Status              string `json:"status" dc:"状态：missing existing"`
	TaskId              int64  `json:"taskId" dc:"本地任务ID"`
	ProfileId           int64  `json:"profileId" dc:"本地资料ID"`
	MediaTotal          int    `json:"mediaTotal" dc:"本地媒体数"`
	MediaMissingStorage int    `json:"mediaMissingStorage" dc:"未迁移到当前存储媒体数"`
}

type ImportTaskQueuePayload struct {
	Id int64 `json:"id"`
}

type ImportRunCreateInp struct {
	TaskId      int64  `json:"taskId" v:"required#任务ID不能为空" dc:"任务ID"`
	RunType     string `json:"runType" dc:"执行类型：import/scan/repair"`
	ImportMode  string `json:"importMode" dc:"导入方式"`
	ScanMode    string `json:"scanMode" dc:"扫描范围"`
	RecentCount int    `json:"recentCount" dc:"最近扫描数量"`
}

func (in *ImportRunCreateInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("执行参数不能为空")
	}
	in.RunType = strings.TrimSpace(in.RunType)
	if in.RunType == "" {
		in.RunType = ImportRunTypeImport
	}
	if in.RunType != ImportRunTypeImport && in.RunType != ImportRunTypeScan && in.RunType != ImportRunTypeRepair {
		return gerror.New("执行类型不合法")
	}
	in.ImportMode = strings.TrimSpace(in.ImportMode)
	if in.ImportMode == "" {
		in.ImportMode = ImportTaskModeIncremental
	}
	if in.ImportMode != ImportTaskModeIncremental && in.ImportMode != ImportTaskModeOverwrite {
		return gerror.New("导入方式不合法")
	}
	scan := &ImportTaskScanInp{Id: in.TaskId, ScanMode: in.ScanMode, RecentCount: in.RecentCount}
	if err := scan.Filter(ctx); err != nil {
		return err
	}
	in.ScanMode = scan.ScanMode
	in.RecentCount = scan.RecentCount
	return nil
}

type ImportRunListInp struct {
	form.PageReq
	TaskId    int64  `json:"taskId" dc:"任务ID"`
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
	AccountId int64  `json:"accountId" dc:"上架账号ID"`
	RunType   string `json:"runType" dc:"执行类型"`
	Status    string `json:"status" dc:"状态"`
	Keyword   string `json:"keyword" dc:"关键词"`
}

type ImportRunModel struct {
	Id                  int64       `json:"id" dc:"记录ID"`
	TaskId              int64       `json:"taskId" dc:"任务ID"`
	TenantId            int64       `json:"tenantId" dc:"租户ID"`
	AccountId           int64       `json:"accountId" dc:"账号ID"`
	TenantName          string      `json:"tenantName" dc:"租户名称"`
	AccountName         string      `json:"accountName" dc:"账号名称"`
	SourceName          string      `json:"sourceName" dc:"来源"`
	BaseUrl             string      `json:"baseUrl" dc:"旧站域名"`
	Username            string      `json:"username" dc:"旧站账号"`
	RunType             string      `json:"runType" dc:"执行类型"`
	ImportMode          string      `json:"importMode" dc:"导入方式"`
	ScanMode            string      `json:"scanMode" dc:"扫描范围"`
	RecentCount         int         `json:"recentCount" dc:"最近数量"`
	Status              string      `json:"status" dc:"状态"`
	Stage               string      `json:"stage" dc:"阶段"`
	ProgressTotal       int         `json:"progressTotal" dc:"总进度"`
	ProgressDone        int         `json:"progressDone" dc:"已完成进度"`
	Percent             float64     `json:"percent" dc:"进度百分比"`
	PageTotal           int         `json:"pageTotal" dc:"总页数"`
	PageDone            int         `json:"pageDone" dc:"已完成页数"`
	ItemTotal           int         `json:"itemTotal" dc:"资料总数"`
	ItemDone            int         `json:"itemDone" dc:"已处理资料数"`
	Imported            int         `json:"imported" dc:"导入数量"`
	Duplicate           int         `json:"duplicate" dc:"重复数量"`
	MediaTotal          int         `json:"mediaTotal" dc:"媒体总数"`
	MediaDone           int         `json:"mediaDone" dc:"已处理媒体数"`
	MediaImported       int         `json:"mediaImported" dc:"媒体导入数量"`
	MediaMissingStorage int         `json:"mediaMissingStorage" dc:"未迁移到当前存储媒体数"`
	TgTotal             int         `json:"tgTotal" dc:"TG消息总数"`
	TgDone              int         `json:"tgDone" dc:"TG已处理数"`
	TgMatched           int         `json:"tgMatched" dc:"TG匹配数量"`
	ErrorMessage        string      `json:"errorMessage" dc:"错误信息"`
	ResultJson          string      `json:"resultJson" dc:"结果JSON"`
	StartedAt           *gtime.Time `json:"startedAt" dc:"开始时间"`
	FinishedAt          *gtime.Time `json:"finishedAt" dc:"结束时间"`
	CreatedAt           *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ImportRunActionInp struct {
	Id int64 `json:"id" v:"required#记录ID不能为空" dc:"记录ID"`
}

type ImportRunLogListInp struct {
	form.PageReq
	RunId int64 `json:"runId" v:"required#记录ID不能为空" dc:"记录ID"`
}

type ImportRunLogModel struct {
	Id        int64       `json:"id" dc:"日志ID"`
	RunId     int64       `json:"runId" dc:"记录ID"`
	Level     string      `json:"level" dc:"级别"`
	Stage     string      `json:"stage" dc:"阶段"`
	Message   string      `json:"message" dc:"消息"`
	Context   string      `json:"context" dc:"上下文JSON"`
	CreatedAt *gtime.Time `json:"createdAt" dc:"创建时间"`
}
