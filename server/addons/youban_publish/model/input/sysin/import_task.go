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
)

type ImportTaskCreateInp struct {
	SourceName       string        `json:"sourceName" dc:"来源名称"`
	BaseUrl          string        `json:"baseUrl" v:"required#旧站域名不能为空" dc:"旧站域名"`
	Username         string        `json:"username" v:"required#旧站账号不能为空" dc:"旧站账号"`
	Password         string        `json:"password" v:"required#旧站密码不能为空" dc:"旧站密码"`
	LimitCount       int           `json:"limitCount" dc:"测试采集数量"`
	PerPage          int           `json:"perPage" dc:"每页数量"`
	ProxyEnabled     int           `json:"proxyEnabled" dc:"是否启用代理"`
	ProxyPool        string        `json:"proxyPool" dc:"代理池"`
	MediaConcurrency int           `json:"mediaConcurrency" dc:"媒体并发数"`
	AccountId        int64         `json:"accountId" dc:"上架账号ID"`
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
	in.Username = strings.TrimSpace(in.Username)
	if in.BaseUrl == "" {
		return gerror.New("旧站域名不能为空")
	}
	if in.Username == "" {
		return gerror.New("旧站账号不能为空")
	}
	if in.Password == "" {
		return gerror.New("旧站密码不能为空")
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
	AccountName string  `json:"accountName" dc:"上架账号名称"`
	Percent     float64 `json:"percent" dc:"进度百分比"`
}

type ImportTaskViewInp struct {
	Id int64 `json:"id" v:"required#任务ID不能为空" dc:"任务ID"`
}

type ImportTaskActionInp struct {
	Id int64 `json:"id" v:"required#任务ID不能为空" dc:"任务ID"`
}

type ImportTaskQueuePayload struct {
	Id int64 `json:"id"`
}
