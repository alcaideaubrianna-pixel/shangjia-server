package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	MessagePushStatusPending = "pending"
	MessagePushStatusSending = "sending"
	MessagePushStatusSent    = "sent"
	MessagePushStatusFailed  = "failed"
)

type MessageTemplateListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态"`
}

type MessageTemplateMediaInp struct {
	Id                int64  `json:"id" dc:"ID"`
	MediaType         string `json:"mediaType" dc:"媒体类型"`
	Name              string `json:"name" dc:"名称"`
	FileUrl           string `json:"fileUrl" dc:"文件地址"`
	StoragePath       string `json:"storagePath" dc:"存储路径"`
	PosterUrl         string `json:"posterUrl" dc:"封面地址"`
	PosterStoragePath string `json:"posterStoragePath" dc:"封面存储路径"`
	TgFileId          string `json:"tgFileId" dc:"TG文件ID"`
	TgThumbFileId     string `json:"tgThumbFileId" dc:"TG缩略图ID"`
	AssetHash         string `json:"assetHash" dc:"资源哈希"`
	SortIndex         int    `json:"sortIndex" dc:"排序"`
}

type MessageTemplateMediaUploadInp struct {
	MediaType string `json:"mediaType" dc:"媒体类型：image/video"`
	SortIndex int    `json:"sortIndex" dc:"排序"`
}

type MessageTemplateSaveInp struct {
	Id     int64                      `json:"id" dc:"ID"`
	Name   string                     `json:"name" dc:"模板名称"`
	Text   string                     `json:"text" dc:"文案"`
	Media  []*MessageTemplateMediaInp `json:"media" dc:"媒体"`
	Status int                        `json:"status" dc:"状态"`
}

type MessageTemplateDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type MessageTemplatePushInp struct {
	AccountId     int64    `json:"accountId" v:"required|min:1#推送账号不能为空|推送账号不能为空" dc:"TG账号ID"`
	TemplateId    int64    `json:"templateId" v:"required|min:1#模板ID不能为空|模板ID不能为空" dc:"模板ID"`
	ChannelIds    []int64  `json:"channelIds" dc:"已配置频道ID"`
	TargetChatIds []string `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
}

type MessagePushPlanListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态：1启用 2停用"`
}

type MessagePushPlanSaveInp struct {
	Id              int64    `json:"id" dc:"ID"`
	Name            string   `json:"name" dc:"计划名称"`
	AccountId       int64    `json:"accountId" dc:"TG账号ID"`
	TemplateIds     []int64  `json:"templateIds" dc:"模板ID"`
	TargetChatIds   []string `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	Times           []string `json:"times" dc:"每天推送时间"`
	IntervalSeconds int      `json:"intervalSeconds" dc:"多次推送间隔秒数"`
	Status          int      `json:"status" dc:"状态：1启用 2停用"`
}

type MessagePushPlanDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type MessagePushPlanStatusInp struct {
	Id     int64 `json:"id" v:"required|min:1#计划ID不能为空|计划ID不能为空" dc:"ID"`
	Status int   `json:"status" v:"required#状态不能为空" dc:"状态：1启用 2停用"`
}

type MessageTemplateSaveModel struct {
	Id int64 `json:"id" dc:"ID"`
}

type MessageTemplatePushModel struct {
	Failed  int                               `json:"failed" dc:"失败数"`
	Results []*MessageTemplatePushResultModel `json:"results" dc:"推送结果"`
	Success int                               `json:"success" dc:"成功数"`
	Total   int                               `json:"total" dc:"总数"`
}

type MessageTemplatePushResultModel struct {
	ChannelId    int64  `json:"channelId" dc:"频道ID"`
	JobId        int64  `json:"jobId" dc:"任务ID"`
	Message      string `json:"message" dc:"结果消息"`
	Status       string `json:"status" dc:"状态"`
	TargetChatId string `json:"targetChatId" dc:"目标Chat ID"`
}

type MessagePushPlanSaveModel struct {
	Id int64 `json:"id" dc:"ID"`
}

type MessagePushPlanModel struct {
	Id              int64       `json:"id" dc:"ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	Name            string      `json:"name" dc:"计划名称"`
	AccountId       int64       `json:"accountId" dc:"TG账号ID"`
	TemplateIds     []int64     `json:"templateIds" dc:"模板ID"`
	TargetChatIds   []string    `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	Times           []string    `json:"times" dc:"每天推送时间"`
	IntervalSeconds int         `json:"intervalSeconds" dc:"多次推送间隔秒数"`
	Status          int         `json:"status" dc:"状态：1启用 2停用"`
	NextRunAt       *gtime.Time `json:"nextRunAt" dc:"下次执行时间"`
	LastRunAt       *gtime.Time `json:"lastRunAt" dc:"最后执行时间"`
	LastResult      string      `json:"lastResult" dc:"最后执行结果"`
	CreatedBy       int64       `json:"createdBy" dc:"创建人"`
	UpdatedBy       int64       `json:"updatedBy" dc:"更新人"`
	DeletedBy       int64       `json:"deletedBy" dc:"删除人"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
	DeletedAt       *gtime.Time `json:"deletedAt" dc:"删除时间"`
}

type MessageTemplateModel struct {
	Id         int64                        `json:"id" dc:"ID"`
	TenantId   int64                        `json:"tenantId" dc:"租户ID"`
	Name       string                       `json:"name" dc:"模板名称"`
	Text       string                       `json:"text" dc:"文案"`
	MediaCount int                          `json:"mediaCount" dc:"媒体数"`
	Media      []*MessageTemplateMediaModel `json:"media" dc:"媒体"`
	Status     int                          `json:"status" dc:"状态"`
	CreatedBy  int64                        `json:"createdBy" dc:"创建人"`
	UpdatedBy  int64                        `json:"updatedBy" dc:"更新人"`
	DeletedBy  int64                        `json:"deletedBy" dc:"删除人"`
	CreatedAt  *gtime.Time                  `json:"createdAt" dc:"创建时间"`
	UpdatedAt  *gtime.Time                  `json:"updatedAt" dc:"更新时间"`
	DeletedAt  *gtime.Time                  `json:"deletedAt" dc:"删除时间"`
}

type MessageTemplateMediaModel struct {
	Id                int64       `json:"id" dc:"ID"`
	TemplateId        int64       `json:"templateId" dc:"模板ID"`
	TenantId          int64       `json:"tenantId" dc:"租户ID"`
	MediaType         string      `json:"mediaType" dc:"媒体类型"`
	Name              string      `json:"name" dc:"名称"`
	FileUrl           string      `json:"fileUrl" dc:"文件地址"`
	StoragePath       string      `json:"storagePath" dc:"存储路径"`
	PosterUrl         string      `json:"posterUrl" dc:"封面地址"`
	PosterStoragePath string      `json:"posterStoragePath" dc:"封面存储路径"`
	TgFileId          string      `json:"tgFileId" dc:"TG文件ID"`
	TgThumbFileId     string      `json:"tgThumbFileId" dc:"TG缩略图ID"`
	AssetHash         string      `json:"assetHash" dc:"资源哈希"`
	SortIndex         int         `json:"sortIndex" dc:"排序"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt         *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

func (in *MessageTemplateListInp) Filter(ctx context.Context) error {
	in.Keyword = strings.TrimSpace(in.Keyword)
	return nil
}

func (in *MessageTemplateSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Text = strings.TrimSpace(in.Text)
	if in.Name == "" {
		return gerror.New("模板名称不能为空")
	}
	if in.Text == "" && len(in.Media) == 0 {
		return gerror.New("模板文案和媒体不能同时为空")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("模板状态不合法")
	}
	if len(in.Media) > 10 {
		return gerror.New("每个模板最多上传10个媒体")
	}
	for index, item := range in.Media {
		if item == nil {
			return gerror.New("媒体不能为空")
		}
		item.MediaType = strings.TrimSpace(item.MediaType)
		if item.MediaType == "" {
			item.MediaType = "image"
		}
		if item.MediaType != "image" && item.MediaType != "video" {
			return gerror.New("媒体类型不合法")
		}
		item.FileUrl = strings.TrimSpace(item.FileUrl)
		item.StoragePath = strings.TrimSpace(item.StoragePath)
		item.TgFileId = strings.TrimSpace(item.TgFileId)
		if item.FileUrl == "" && item.StoragePath == "" && item.TgFileId == "" {
			return gerror.New("媒体文件地址不能为空")
		}
		if item.SortIndex <= 0 {
			item.SortIndex = index + 1
		}
	}
	return nil
}

func (in *MessageTemplateMediaUploadInp) Filter(ctx context.Context) error {
	in.MediaType = strings.TrimSpace(in.MediaType)
	if in.MediaType == "" {
		in.MediaType = "image"
	}
	if in.MediaType != "image" && in.MediaType != "video" {
		return gerror.New("媒体类型不合法")
	}
	return nil
}

func (in *MessageTemplatePushInp) Filter(ctx context.Context) error {
	if in.AccountId <= 0 {
		return gerror.New("请选择推送账号")
	}
	if in.TemplateId <= 0 {
		return gerror.New("请选择消息模板")
	}
	if len(in.ChannelIds) == 0 && len(in.TargetChatIds) == 0 {
		return gerror.New("请选择推送群聊或频道")
	}
	in.TargetChatIds = uniqueStringInputs(in.TargetChatIds)
	return nil
}

func (in *MessagePushPlanListInp) Filter(ctx context.Context) error {
	in.Keyword = strings.TrimSpace(in.Keyword)
	return nil
}

func (in *MessagePushPlanSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return gerror.New("计划名称不能为空")
	}
	if in.AccountId <= 0 {
		return gerror.New("请选择推送账号")
	}
	in.TemplateIds = uniqueInt64Inputs(in.TemplateIds)
	if len(in.TemplateIds) == 0 {
		return gerror.New("请选择推送模板")
	}
	in.TargetChatIds = uniqueStringInputs(in.TargetChatIds)
	if len(in.TargetChatIds) == 0 {
		return gerror.New("请选择推送群聊或频道")
	}
	in.Times = uniqueStringInputs(in.Times)
	if len(in.Times) == 0 {
		return gerror.New("请至少设置一个推送时间")
	}
	for _, value := range in.Times {
		if len(value) != 8 {
			return gerror.New("推送时间格式必须为 HH:mm:ss")
		}
	}
	if in.IntervalSeconds <= 0 {
		in.IntervalSeconds = 60
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func (in *MessagePushPlanStatusInp) Filter(ctx context.Context) error {
	if in.Id <= 0 {
		return gerror.New("计划ID不能为空")
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func uniqueInt64Inputs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueStringInputs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
