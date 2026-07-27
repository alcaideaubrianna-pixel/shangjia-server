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
	MaterialImportStatusPending  = "pending"
	MaterialImportStatusRunning  = "running"
	MaterialImportStatusWaiting  = "waiting"
	MaterialImportStatusSuccess  = "success"
	MaterialImportStatusFailed   = "failed"
	MaterialImportStatusCanceled = "canceled"

	MaterialImportStageCreated   = "created"
	MaterialImportStagePulling   = "pulling"
	MaterialImportStageMedia     = "media"
	MaterialImportStageFinished  = "finished"
	MaterialImportStageCancelled = "canceled"
)

type MaterialImportListInp struct {
	form.PageReq
	AccountId   int64  `json:"accountId" dc:"资料归属账号"`
	TgAccountId int64  `json:"tgAccountId" dc:"TG账号"`
	Status      string `json:"status" dc:"状态"`
	Keyword     string `json:"keyword" dc:"关键词"`
}

type MaterialImportTaskSaveInp struct {
	Id            int64   `json:"id" dc:"任务ID"`
	AccountId     int64   `json:"accountId" dc:"资料归属账号"`
	TgAccountId   int64   `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号"`
	SourceChatId  string  `json:"sourceChatId" v:"required#请选择频道" dc:"频道ID"`
	ChannelIds    []int64 `json:"channelIds" dc:"导入资料默认上架频道ID列表"`
	PullLimitDays int     `json:"pullLimitDays" dc:"最多拉取天数"`
}

// MaterialImportTaskServerCreateInp 是超级管理员为任意租户账号创建TG资料导入任务的参数。
type MaterialImportTaskServerCreateInp struct {
	TenantId      int64   `json:"tenantId" v:"required|min:1#请选择账号归属|请选择账号归属" dc:"租户ID"`
	AccountId     int64   `json:"accountId" v:"required|min:1#请选择归属账号|请选择归属账号" dc:"资料归属账号"`
	TgAccountId   int64   `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号"`
	ChannelUrl    string  `json:"channelUrl" v:"required#请输入TG频道连接" dc:"TG频道连接"`
	ChannelIds    []int64 `json:"channelIds" dc:"导入资料默认上架频道ID列表"`
	PullLimitDays int     `json:"pullLimitDays" dc:"最多拉取天数"`
}

func (in *MaterialImportTaskServerCreateInp) Filter(ctx context.Context) error {
	_ = ctx
	in.ChannelUrl = strings.TrimSpace(in.ChannelUrl)
	if in.ChannelUrl == "" {
		return gerror.New("请输入TG频道连接")
	}
	if in.PullLimitDays <= 0 {
		in.PullLimitDays = 365
	}
	if in.PullLimitDays > 365 {
		in.PullLimitDays = 365
	}
	return nil
}

func (in *MaterialImportTaskSaveInp) Filter(ctx context.Context) error {
	in.SourceChatId = strings.TrimSpace(in.SourceChatId)
	if in.SourceChatId == "" {
		return gerror.New("请选择频道")
	}
	if in.PullLimitDays <= 0 {
		in.PullLimitDays = 365
	}
	if in.PullLimitDays > 365 {
		in.PullLimitDays = 365
	}
	return nil
}

type MaterialImportTaskViewInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type MaterialImportTaskActionInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type MaterialImportTaskModel struct {
	entity.YoubanPublishMaterialImportTask
	ChannelIdJson     string                      `json:"channelIdJson" dc:"导入资料默认上架频道ID JSON"`
	ChannelIds        []int64                     `json:"channelIds" dc:"导入资料默认上架频道ID列表"`
	TenantName        string                      `json:"tenantName" dc:"租户名称"`
	AccountName       string                      `json:"accountName" dc:"资料归属账号"`
	TgAccountNickname string                      `json:"tgAccountNickname" dc:"TG账号昵称"`
	TgAccountUsername string                      `json:"tgAccountUsername" dc:"TG账号用户名"`
	Percent           float64                     `json:"percent" dc:"进度"`
	Groups            []*MaterialImportGroupModel `json:"groups" dc:"分组明细"`
}

type MaterialImportGroupModel struct {
	entity.YoubanPublishMaterialImportGroup
	Percent float64 `json:"percent" dc:"媒体进度"`
}

type MaterialImportTaskProgressModel struct {
	Id         int64       `json:"id" dc:"任务ID"`
	Status     string      `json:"status" dc:"状态"`
	Stage      string      `json:"stage" dc:"阶段"`
	Percent    float64     `json:"percent" dc:"进度"`
	Message    string      `json:"message" dc:"提示"`
	StartedAt  *gtime.Time `json:"startedAt" dc:"开始时间"`
	FinishedAt *gtime.Time `json:"finishedAt" dc:"完成时间"`
}
