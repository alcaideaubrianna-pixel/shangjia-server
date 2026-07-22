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
	Id            int64  `json:"id" dc:"任务ID"`
	AccountId     int64  `json:"accountId" dc:"资料归属账号"`
	TgAccountId   int64  `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号"`
	SourceChatId  string `json:"sourceChatId" v:"required#请选择频道" dc:"频道ID"`
	PullLimitDays int    `json:"pullLimitDays" dc:"最多拉取天数"`
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
