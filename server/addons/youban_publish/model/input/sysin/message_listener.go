package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	MessageListenerStatusEnabled  = 1
	MessageListenerStatusDisabled = 2
)

type ListenerPlanListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态：1启用 2停用"`
}

type ListenerPlanSaveInp struct {
	Id            int64    `json:"id" dc:"ID"`
	Name          string   `json:"name" dc:"计划名称"`
	TgAccountId   int64    `json:"tgAccountId" dc:"监听账号ID"`
	BotId         int64    `json:"botId" dc:"推送Bot ID，为空使用官方Bot"`
	KeywordText   string   `json:"keywordText" dc:"关键字文本"`
	Keywords      []string `json:"keywords" dc:"关键字列表"`
	TargetChatIds []string `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	Status        int      `json:"status" dc:"状态：1启用 2停用"`
}

type ListenerPlanDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type ListenerPlanStatusInp struct {
	Id     int64 `json:"id" v:"required|min:1#计划ID不能为空|计划ID不能为空" dc:"ID"`
	Status int   `json:"status" v:"required#状态不能为空" dc:"状态：1启用 2停用"`
}

type ListenerPlanUnbindInp struct {
	Id       int64 `json:"id" v:"required|min:1#计划ID不能为空|计划ID不能为空" dc:"计划ID"`
	TargetId int64 `json:"targetId" dc:"兼容旧版目标ID，通知目标解绑不再使用"`
}

type ListenerPlanSaveModel struct {
	Id int64 `json:"id" dc:"ID"`
}

type ListenerPlanModel struct {
	Id            int64                      `json:"id" dc:"ID"`
	TenantId      int64                      `json:"tenantId" dc:"租户ID"`
	Name          string                     `json:"name" dc:"计划名称"`
	TgAccountId   int64                      `json:"tgAccountId" dc:"监听账号ID"`
	BotId         int64                      `json:"botId" dc:"推送Bot ID"`
	KeywordText   string                     `json:"keywordText" dc:"关键字文本"`
	Keywords      []string                   `json:"keywords" dc:"关键字列表"`
	TargetChatIds []string                   `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
	TargetCount   int                        `json:"targetCount" dc:"目标数量"`
	BoundCount    int                        `json:"boundCount" dc:"已绑定数量"`
	BindCode      string                     `json:"bindCode" dc:"绑定ID"`
	NotifyChatId  string                     `json:"notifyChatId" dc:"通知目标Chat ID"`
	NotifyChatTyp string                     `json:"notifyChatType" dc:"通知目标Chat类型"`
	NotifyTitle   string                     `json:"notifyChatTitle" dc:"通知目标标题"`
	NotifyBoundAt *gtime.Time                `json:"notifyBoundAt" dc:"通知目标绑定时间"`
	Status        int                        `json:"status" dc:"状态：1启用 2停用"`
	LastTriggerAt *gtime.Time                `json:"lastTriggerAt" dc:"最近触发时间"`
	LastResult    string                     `json:"lastResult" dc:"最近执行结果"`
	CreatedBy     int64                      `json:"createdBy" dc:"创建人"`
	UpdatedBy     int64                      `json:"updatedBy" dc:"更新人"`
	DeletedBy     int64                      `json:"deletedBy" dc:"删除人"`
	CreatedAt     *gtime.Time                `json:"createdAt" dc:"创建时间"`
	UpdatedAt     *gtime.Time                `json:"updatedAt" dc:"更新时间"`
	DeletedAt     *gtime.Time                `json:"deletedAt" dc:"删除时间"`
	Targets       []*ListenerPlanTargetModel `json:"targets" dc:"目标列表"`
}

type ListenerPlanTargetModel struct {
	Id                int64       `json:"id" dc:"ID"`
	PlanId            int64       `json:"planId" dc:"计划ID"`
	TenantId          int64       `json:"tenantId" dc:"租户ID"`
	TargetChatId      string      `json:"targetChatId" dc:"目标Chat ID"`
	TargetChatType    string      `json:"targetChatType" dc:"目标Chat类型"`
	TargetChatTitle   string      `json:"targetChatTitle" dc:"目标Chat标题"`
	TargetChatUser    string      `json:"targetChatUsername" dc:"目标Chat用户名"`
	LastMatchedAt     *gtime.Time `json:"lastMatchedAt" dc:"最近命中时间"`
	LastMatchedText   string      `json:"lastMatchedText" dc:"最近命中文本"`
	LastMatchedUserId string      `json:"lastMatchedUserId" dc:"最近命中用户ID"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt         *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type BotMessageInp struct {
	BotId     int64  `json:"botId" dc:"Bot ID"`
	ChatId    string `json:"chatId" dc:"来源Chat ID"`
	ChatTitle string `json:"chatTitle" dc:"来源Chat标题"`
	ChatType  string `json:"chatType" dc:"来源Chat类型"`
	Text      string `json:"text" dc:"消息文本"`
}

func (in *ListenerPlanListInp) Filter(ctx context.Context) error {
	_ = ctx
	in.Keyword = strings.TrimSpace(in.Keyword)
	return nil
}

func (in *ListenerPlanSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.KeywordText = strings.TrimSpace(in.KeywordText)
	in.Keywords = listenerKeywordInputs(in.KeywordText, in.Keywords)
	in.TargetChatIds = uniqueStringInputs(in.TargetChatIds)
	if in.Name == "" {
		return gerror.New("计划名称不能为空")
	}
	if in.TgAccountId <= 0 {
		return gerror.New("请选择监听账号")
	}
	if in.BotId < 0 {
		in.BotId = 0
	}
	if len(in.Keywords) == 0 {
		return gerror.New("请输入关键字")
	}
	if len(in.TargetChatIds) == 0 {
		return gerror.New("请选择频道或群聊")
	}
	if in.Status == 0 {
		in.Status = MessageListenerStatusEnabled
	}
	if in.Status != MessageListenerStatusEnabled && in.Status != MessageListenerStatusDisabled {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func (in *ListenerPlanStatusInp) Filter(ctx context.Context) error {
	_ = ctx
	if in.Id <= 0 {
		return gerror.New("计划ID不能为空")
	}
	if in.Status != MessageListenerStatusEnabled && in.Status != MessageListenerStatusDisabled {
		return gerror.New("计划状态不合法")
	}
	return nil
}

func (in *ListenerPlanUnbindInp) Filter(ctx context.Context) error {
	_ = ctx
	if in.Id <= 0 {
		return gerror.New("计划ID不能为空")
	}
	return nil
}

func listenerKeywordInputs(keywordText string, keywords []string) []string {
	out := make([]string, 0, len(keywords)+4)
	seen := make(map[string]struct{}, len(keywords)+4)
	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		value = strings.Trim(value, ",;|")
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, item := range keywords {
		for _, value := range strings.FieldsFunc(item, func(r rune) bool {
			switch r {
			case ',', '，', ';', '；', '\n', '\r', '\t':
				return true
			default:
				return false
			}
		}) {
			appendValue(value)
		}
	}
	for _, value := range strings.FieldsFunc(keywordText, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	}) {
		appendValue(value)
	}
	return out
}
