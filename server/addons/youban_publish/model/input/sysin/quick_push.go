package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

type QuickPushPlanListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态：1启用 2停用"`
}

type QuickPushPlanSaveInp struct {
	Id            int64    `json:"id" dc:"ID"`
	Name          string   `json:"name" dc:"计划名称"`
	AccountId     int64    `json:"accountId" dc:"TG账号ID"`
	TargetChatIds []string `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	Status        int      `json:"status" dc:"状态：1启用 2停用"`
}

type QuickPushPlanDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type QuickPushPlanStatusInp struct {
	Id     int64 `json:"id" v:"required|min:1#计划ID不能为空|计划ID不能为空" dc:"ID"`
	Status int   `json:"status" v:"required#状态不能为空" dc:"状态：1启用 2停用"`
}

type QuickPushPlanSaveModel struct {
	Id int64 `json:"id" dc:"ID"`
}

type QuickPushPlanModel struct {
	Id               int64             `json:"id" dc:"ID"`
	SerialNo         string            `json:"serialNo" dc:"唯一序号"`
	TenantId         int64             `json:"tenantId" dc:"租户ID"`
	Name             string            `json:"name" dc:"计划名称"`
	AccountId        int64             `json:"accountId" dc:"TG账号ID"`
	TargetChatIds    []string          `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	TargetChatLabels map[string]string `json:"targetChatLabels" dc:"目标群聊或频道名称"`
	Status           int               `json:"status" dc:"状态：1启用 2停用"`
	CreatedBy        int64             `json:"createdBy" dc:"创建人"`
	UpdatedBy        int64             `json:"updatedBy" dc:"更新人"`
	DeletedBy        int64             `json:"deletedBy" dc:"删除人"`
	CreatedAt        *gtime.Time       `json:"createdAt" dc:"创建时间"`
	UpdatedAt        *gtime.Time       `json:"updatedAt" dc:"更新时间"`
	DeletedAt        *gtime.Time       `json:"deletedAt" dc:"删除时间"`
}

type QuickPushBotAccountModel struct {
	AccountId   int64  `json:"accountId" dc:"上架账号ID"`
	TenantId    int64  `json:"tenantId" dc:"租户ID"`
	Username    string `json:"username" dc:"账号"`
	Nickname    string `json:"nickname" dc:"昵称"`
	AccountType string `json:"accountType" dc:"账号类型"`
}

type QuickPushBotExecuteInp struct {
	TenantId              int64                      `json:"tenantId" dc:"租户ID"`
	OperatorAccountId     int64                      `json:"operatorAccountId" dc:"操作上架账号ID"`
	TemplateId            int64                      `json:"templateId" dc:"已保存消息模板ID"`
	PlanIds               []int64                    `json:"planIds" dc:"快速推送计划ID"`
	Text                  string                     `json:"text" dc:"推送文本"`
	Media                 []*MessageTemplateMediaInp `json:"media" dc:"推送媒体"`
	SourceMessageRecordId int64                      `json:"sourceMessageRecordId" dc:"来源TG消息记录ID"`
}

type QuickPushBotSaveTemplateInp struct {
	TenantId              int64                      `json:"tenantId" dc:"租户ID"`
	OperatorAccountId     int64                      `json:"operatorAccountId" dc:"操作上架账号ID"`
	Text                  string                     `json:"text" dc:"模板文本"`
	Media                 []*MessageTemplateMediaInp `json:"media" dc:"模板媒体"`
	SourceMessageRecordId int64                      `json:"sourceMessageRecordId" dc:"来源TG消息记录ID"`
}

type QuickPushBotExecuteModel struct {
	Failed  int                               `json:"failed" dc:"失败数"`
	Results []*MessageTemplatePushResultModel `json:"results" dc:"推送结果"`
	Success int                               `json:"success" dc:"成功数"`
	Total   int                               `json:"total" dc:"总数"`
}

func (in *QuickPushPlanListInp) Filter(ctx context.Context) error {
	in.Keyword = strings.TrimSpace(in.Keyword)
	return nil
}

func (in *QuickPushPlanSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return gerror.New("计划名称不能为空")
	}
	if in.AccountId <= 0 {
		return gerror.New("请选择推送账号")
	}
	in.TargetChatIds = uniqueStringInputs(in.TargetChatIds)
	if len(in.TargetChatIds) == 0 {
		return gerror.New("请选择推送群聊或频道")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func (in *QuickPushPlanStatusInp) Filter(ctx context.Context) error {
	if in.Id <= 0 {
		return gerror.New("计划ID不能为空")
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func (in *QuickPushBotExecuteInp) Filter(ctx context.Context) error {
	if in.TenantId <= 0 || in.OperatorAccountId <= 0 {
		return gerror.New("上架管理员账号不能为空")
	}
	in.PlanIds = uniqueInt64Inputs(in.PlanIds)
	if len(in.PlanIds) == 0 {
		return gerror.New("请选择快速推送计划")
	}
	return filterQuickPushContent(&in.Text, in.Media)
}

func (in *QuickPushBotSaveTemplateInp) Filter(ctx context.Context) error {
	if in.TenantId <= 0 || in.OperatorAccountId <= 0 {
		return gerror.New("上架管理员账号不能为空")
	}
	return filterQuickPushContent(&in.Text, in.Media)
}

func filterQuickPushContent(text *string, media []*MessageTemplateMediaInp) error {
	*text = strings.TrimSpace(*text)
	if *text == "" && len(media) == 0 {
		return gerror.New("快速推送文本和媒体不能同时为空")
	}
	if len(media) > 10 {
		return gerror.New("每次快速推送最多支持10个媒体")
	}
	for index, item := range media {
		if item == nil {
			return gerror.New("快速推送媒体不能为空")
		}
		item.MediaType = strings.TrimSpace(item.MediaType)
		if item.MediaType == "" {
			item.MediaType = "image"
		}
		if item.MediaType != "image" && item.MediaType != "video" {
			return gerror.New("快速推送媒体类型不合法")
		}
		item.FileUrl = strings.TrimSpace(item.FileUrl)
		item.StoragePath = strings.TrimSpace(item.StoragePath)
		item.TgFileId = strings.TrimSpace(item.TgFileId)
		if item.FileUrl == "" && item.StoragePath == "" && item.TgFileId == "" {
			return gerror.New("快速推送媒体文件地址不能为空")
		}
		if item.SortIndex <= 0 {
			item.SortIndex = index + 1
		}
	}
	return nil
}
