package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	SyncStatusEnabled  = 1
	SyncStatusDisabled = 2

	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
)

type OptionListInp struct {
	TenantId int64  `json:"tenantId" dc:"租户ID"`
	Keyword  string `json:"keyword" dc:"关键词"`
}
type TenantOptionModel struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	Username       string `json:"username"`
	AdminAccountId int64  `json:"adminAccountId"`
	Label          string `json:"label"`
	Value          int64  `json:"value"`
}
type AccountOptionModel struct {
	Id          int64  `json:"id"`
	TenantId    int64  `json:"tenantId"`
	TenantName  string `json:"tenantName"`
	Nickname    string `json:"nickname"`
	Username    string `json:"username"`
	AccountType string `json:"accountType"`
	Label       string `json:"label"`
	Value       int64  `json:"value"`
}
type ChannelCopyInp struct {
	ChannelMapId    int64 `json:"channelMapId" dc:"频道映射ID"`
	ConfigId        int64 `json:"configId" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"配置ID"`
	YoubanAccountId int64 `json:"youbanAccountId" v:"required|min:1#源上架账号不能为空|源上架账号不能为空" dc:"源上架账号ID"`
	TargetTenantId  int64 `json:"targetTenantId" dc:"目标租户ID"`
	TargetAccountId int64 `json:"targetAccountId" v:"required|min:1#目标上架账号不能为空|目标上架账号不能为空" dc:"目标上架账号ID"`
}
type ChannelCopyModel struct {
	ProfileCount int `json:"profileCount" dc:"复制资料数"`
	TaskCount    int `json:"taskCount" dc:"复制任务数"`
	MediaCount   int `json:"mediaCount" dc:"复制媒体数"`
}
type ChannelDisableInp struct {
	ChannelMapId    int64 `json:"channelMapId" dc:"频道映射ID"`
	ConfigId        int64 `json:"configId" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"配置ID"`
	YoubanAccountId int64 `json:"youbanAccountId" v:"required|min:1#上架账号不能为空|上架账号不能为空" dc:"上架账号ID"`
}
type ChannelDisableModel struct {
	ProfileCount int `json:"profileCount" dc:"停用资料数"`
	TaskCount    int `json:"taskCount" dc:"停用任务数"`
	MediaCount   int `json:"mediaCount" dc:"停用媒体数"`
	AccountCount int `json:"accountCount" dc:"停用账号数"`
}

type ConfigListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态"`
}

type ConfigModel struct {
	Id                    int64       `json:"id" dc:"ID"`
	Name                  string      `json:"name" dc:"配置名称"`
	DbType                string      `json:"dbType" dc:"数据库类型"`
	DbHost                string      `json:"dbHost" dc:"数据库地址"`
	DbPort                int         `json:"dbPort" dc:"端口"`
	DbName                string      `json:"dbName" dc:"数据库名"`
	DbUser                string      `json:"dbUser" dc:"账号"`
	TargetTenantId        int64       `json:"targetTenantId" dc:"目标租户ID"`
	TargetParentAccountId int64       `json:"targetParentAccountId" dc:"目标父账号ID"`
	AutoCreateAccount     int         `json:"autoCreateAccount" dc:"自动创建账号"`
	SyncMedia             int         `json:"syncMedia" dc:"同步媒体"`
	SyncVerifyMedia       int         `json:"syncVerifyMedia" dc:"同步验证资料"`
	AutoSyncEnabled       int         `json:"autoSyncEnabled" dc:"自动同步开关"`
	SyncIntervalMinutes   int         `json:"syncIntervalMinutes" dc:"同步间隔分钟"`
	BatchSize             int         `json:"batchSize" dc:"单批数量"`
	Status                int         `json:"status" dc:"状态"`
	LastRunAt             *gtime.Time `json:"lastRunAt" dc:"最近运行时间"`
	LastSuccessAt         *gtime.Time `json:"lastSuccessAt" dc:"最近成功时间"`
	LastError             string      `json:"lastError" dc:"最近错误"`
	CreatedAt             *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt             *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ConfigSaveInp struct {
	Id                    int64  `json:"id" dc:"ID"`
	Name                  string `json:"name" dc:"配置名称"`
	DbType                string `json:"dbType" dc:"数据库类型"`
	DbHost                string `json:"dbHost" dc:"数据库地址"`
	DbPort                int    `json:"dbPort" dc:"端口"`
	DbName                string `json:"dbName" dc:"数据库名"`
	DbUser                string `json:"dbUser" dc:"账号"`
	DbPassword            string `json:"dbPassword" dc:"密码"`
	TargetTenantId        int64  `json:"targetTenantId" dc:"目标租户ID"`
	TargetParentAccountId int64  `json:"targetParentAccountId" dc:"目标父账号ID"`
	AutoCreateAccount     int    `json:"autoCreateAccount" dc:"自动创建账号"`
	SyncMedia             int    `json:"syncMedia" dc:"同步媒体"`
	SyncVerifyMedia       int    `json:"syncVerifyMedia" dc:"同步验证资料"`
	AutoSyncEnabled       int    `json:"autoSyncEnabled" dc:"自动同步开关"`
	SyncIntervalMinutes   int    `json:"syncIntervalMinutes" dc:"同步间隔分钟"`
	BatchSize             int    `json:"batchSize" dc:"单批数量"`
	Status                int    `json:"status" dc:"状态"`
}

func (in *ConfigSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.DbType = strings.ToLower(strings.TrimSpace(in.DbType))
	in.DbHost = strings.TrimSpace(in.DbHost)
	in.DbName = strings.TrimSpace(in.DbName)
	in.DbUser = strings.TrimSpace(in.DbUser)
	if in.Name == "" {
		return gerror.New("配置名称不能为空")
	}
	if in.DbType == "" {
		in.DbType = "mysql"
	}
	if in.DbType != "mysql" && in.DbType != "pgsql" && in.DbType != "postgresql" {
		return gerror.New("数据库类型仅支持 mysql/pgsql")
	}
	if in.DbHost == "" || in.DbName == "" || in.DbUser == "" {
		return gerror.New("FeiNiu 数据库连接信息不能为空")
	}
	if in.DbPort <= 0 {
		if in.DbType == "mysql" {
			in.DbPort = 3306
		} else {
			in.DbPort = 5432
		}
	}
	if in.TargetTenantId <= 0 || in.TargetParentAccountId <= 0 {
		return gerror.New("请指派目标上架租户和管理员账号")
	}
	if in.AutoCreateAccount == 0 {
		in.AutoCreateAccount = 1
	}
	if in.SyncMedia == 0 {
		in.SyncMedia = 1
	}
	if in.SyncVerifyMedia == 0 {
		in.SyncVerifyMedia = 1
	}
	if in.AutoSyncEnabled == 0 {
		in.AutoSyncEnabled = 1
	}
	if in.AutoSyncEnabled != 1 && in.AutoSyncEnabled != 2 {
		return gerror.New("自动同步开关不合法")
	}
	if in.SyncIntervalMinutes <= 0 {
		in.SyncIntervalMinutes = 10
	}
	if in.SyncIntervalMinutes < 1 || in.SyncIntervalMinutes > 1440 {
		return gerror.New("同步间隔需在 1 到 1440 分钟之间")
	}
	if in.BatchSize <= 0 {
		in.BatchSize = 100
	}
	if in.Status == 0 {
		in.Status = SyncStatusEnabled
	}
	if in.Status != SyncStatusEnabled && in.Status != SyncStatusDisabled {
		return gerror.New("状态不合法")
	}
	return nil
}

type ConfigViewInp struct {
	Id int64 `json:"id" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"ID"`
}
type ConfigDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的配置" dc:"ID列表"`
}
type ConfigAutoSyncInp struct {
	Id              int64 `json:"id" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"配置ID"`
	AutoSyncEnabled int   `json:"autoSyncEnabled" v:"required|in:1,2#自动同步状态不能为空|自动同步状态不合法" dc:"自动同步状态"`
}
type ConfigTestInp struct{ ConfigSaveInp }
type ConfigTestModel struct {
	Success bool   `json:"success" dc:"是否成功"`
	Message string `json:"message" dc:"消息"`
}
type RunStartInp struct {
	ConfigId int64 `json:"configId" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"配置ID"`
	Limit    int   `json:"limit" dc:"限制数量"`
}
type RunStartModel struct {
	RunId int64 `json:"runId" dc:"运行ID"`
}

type DashboardInp struct {
	ConfigId  int64  `json:"configId" dc:"配置ID"`
	StartDate string `json:"startDate" dc:"开始日期"`
	EndDate   string `json:"endDate" dc:"结束日期"`
}
type DashboardModel struct {
	ConfigCount  int       `json:"configCount"`
	ChannelCount int       `json:"channelCount"`
	ProfileCount int       `json:"profileCount"`
	RunningCount int       `json:"runningCount"`
	FailedCount  int       `json:"failedCount"`
	LastRun      *RunModel `json:"lastRun"`
}
type DashboardSummaryModel struct {
	ConfigCount   int       `json:"configCount"`
	ChannelCount  int       `json:"channelCount"`
	ProfileCount  int       `json:"profileCount"`
	RunCount      int       `json:"runCount"`
	TotalCount    int       `json:"totalCount"`
	SuccessCount  int       `json:"successCount"`
	CreatedCount  int       `json:"createdCount"`
	UpdatedCount  int       `json:"updatedCount"`
	SkippedCount  int       `json:"skippedCount"`
	FailedCount   int       `json:"failedCount"`
	RunningCount  int       `json:"runningCount"`
	SuccessRate   float64   `json:"successRate"`
	AvgDurationMs int64     `json:"avgDurationMs"`
	LastRun       *RunModel `json:"lastRun"`
	UpdatedAt     string    `json:"updatedAt"`
}
type DashboardTrendModel struct {
	Date         string `json:"date"`
	TotalCount   int    `json:"totalCount"`
	CreatedCount int    `json:"createdCount"`
	UpdatedCount int    `json:"updatedCount"`
	SkippedCount int    `json:"skippedCount"`
	FailedCount  int    `json:"failedCount"`
}
type DashboardChannelRankModel struct {
	FeiniuChannelId       int64  `json:"feiniuChannelId"`
	FeiniuTgChatId        int64  `json:"feiniuTgChatId"`
	FeiniuChannelTitle    string `json:"feiniuChannelTitle"`
	YoubanAccountId       int64  `json:"youbanAccountId"`
	YoubanAccountUsername string `json:"youbanAccountUsername"`
	TotalCount            int    `json:"totalCount"`
	CreatedCount          int    `json:"createdCount"`
	UpdatedCount          int    `json:"updatedCount"`
	SkippedCount          int    `json:"skippedCount"`
	FailedCount           int    `json:"failedCount"`
}

type ChannelMapListInp struct {
	form.PageReq
	ConfigId   int64  `json:"configId" dc:"配置ID"`
	Keyword    string `json:"keyword" dc:"关键词"`
	SyncStatus string `json:"syncStatus" dc:"同步状态"`
}
type ChannelClearInp struct {
	ConfigId int64 `json:"configId" v:"required|min:1#配置ID不能为空|配置ID不能为空" dc:"配置ID"`
}
type ChannelClearModel struct {
	ProfileCount int `json:"profileCount" dc:"清理资料数"`
	TaskCount    int `json:"taskCount" dc:"清理任务数"`
	AccountCount int `json:"accountCount" dc:"清理自动账号数"`
}
type ChannelMapModel struct {
	Id                    int64       `json:"id"`
	ConfigId              int64       `json:"configId"`
	FeiniuChannelId       int64       `json:"feiniuChannelId"`
	FeiniuTgChatId        int64       `json:"feiniuTgChatId"`
	FeiniuChannelTitle    string      `json:"feiniuChannelTitle"`
	FeiniuUsername        string      `json:"feiniuUsername"`
	YoubanTenantId        int64       `json:"youbanTenantId"`
	YoubanAccountId       int64       `json:"youbanAccountId"`
	YoubanAccountUsername string      `json:"youbanAccountUsername"`
	AccountNoteCount      int         `json:"accountNoteCount"`
	LastSourceUpdateTime  *gtime.Time `json:"lastSourceUpdateTime"`
	LastSourceNoteId      int64       `json:"lastSourceNoteId"`
	SyncStatus            string      `json:"syncStatus"`
	ErrorMessage          string      `json:"errorMessage"`
	CreatedAt             *gtime.Time `json:"createdAt"`
	UpdatedAt             *gtime.Time `json:"updatedAt"`
}

type RunListInp struct {
	form.PageReq
	ConfigId  int64  `json:"configId" dc:"配置ID"`
	Status    string `json:"status" dc:"状态"`
	StartDate string `json:"startDate" dc:"开始日期"`
	EndDate   string `json:"endDate" dc:"结束日期"`
}
type RunViewInp struct {
	Id int64 `json:"id" v:"required|min:1#运行ID不能为空|运行ID不能为空" dc:"运行ID"`
}
type RunItemListInp struct {
	form.PageReq
	RunId           int64  `json:"runId" v:"required|min:1#运行ID不能为空|运行ID不能为空" dc:"运行ID"`
	Status          string `json:"status" dc:"状态"`
	Action          string `json:"action" dc:"动作"`
	Keyword         string `json:"keyword" dc:"关键词"`
	FeiniuChannelId int64  `json:"feiniuChannelId" dc:"频道ID"`
}
type RunModel struct {
	Id           int64       `json:"id"`
	ConfigId     int64       `json:"configId"`
	RunType      string      `json:"runType"`
	Status       string      `json:"status"`
	TotalCount   int         `json:"totalCount"`
	CreatedCount int         `json:"createdCount"`
	UpdatedCount int         `json:"updatedCount"`
	SkippedCount int         `json:"skippedCount"`
	FailedCount  int         `json:"failedCount"`
	StartedAt    *gtime.Time `json:"startedAt"`
	FinishedAt   *gtime.Time `json:"finishedAt"`
	ErrorMessage string      `json:"errorMessage"`
	RuntimeLog   string      `json:"runtimeLog"`
	CreatedAt    *gtime.Time `json:"createdAt"`
}

type RunItemModel struct {
	Id                 int64       `json:"id"`
	RunId              int64       `json:"runId"`
	ConfigId           int64       `json:"configId"`
	FeiniuNoteId       int64       `json:"feiniuNoteId"`
	FeiniuNoteCode     string      `json:"feiniuNoteCode"`
	FeiniuChannelId    int64       `json:"feiniuChannelId"`
	FeiniuChannelTitle string      `json:"feiniuChannelTitle"`
	YoubanProfileId    int64       `json:"youbanProfileId"`
	YoubanTaskId       int64       `json:"youbanTaskId"`
	Action             string      `json:"action"`
	Status             string      `json:"status"`
	ErrorMessage       string      `json:"errorMessage"`
	SourceUpdatedAt    *gtime.Time `json:"sourceUpdatedAt"`
	DurationMs         int64       `json:"durationMs"`
	CreatedAt          *gtime.Time `json:"createdAt"`
}
