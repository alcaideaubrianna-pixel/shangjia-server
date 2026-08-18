package sysin

import (
	"strings"
	"unicode/utf8"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	TwoWayBotStatusEnabled  = 1
	TwoWayBotStatusDisabled = 0

	TwoWayBotSetupPending        = "pending"
	TwoWayBotSetupManualRequired = "manual_required"
	TwoWayBotSetupReady          = "ready"
	TwoWayBotSetupFailed         = "failed"

	TwoWayBotWebhookPending = "pending"
	TwoWayBotWebhookPolling = "polling"
	TwoWayBotWebhookReady   = "ready"
	TwoWayBotWebhookFailed  = "failed"
)

type BotListInp struct {
	form.PageReq
	PerPageAlias int    `json:"perPage" dc:"每页数量（兼容旧参数）"`
	Keyword      string `json:"keyword" dc:"关键词"`
	Status       *int   `json:"status" dc:"状态"`
}

func (in *BotListInp) GetPerPage() int {
	if in == nil {
		return 0
	}
	if in.PerPage > 0 {
		return in.PerPage
	}
	if in.PerPageAlias > 0 {
		return in.PerPageAlias
	}
	return in.PageReq.GetPerPage()
}

type BotSaveInp struct {
	Id              int64  `json:"id" dc:"ID"`
	Name            string `json:"name" dc:"名称"`
	WelcomeMessage  string `json:"welcomeMessage" dc:"欢迎语"`
	BotToken        string `json:"botToken" dc:"Bot Token"`
	ExistingGroupId string `json:"existingGroupId" dc:"已有管理群ID"`
	TgAccountId     int64  `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG协议号ID"`
	SupergroupId    string `json:"supergroupId" dc:"管理群ID"`
	SupergroupTitle string `json:"supergroupTitle" dc:"管理群名称"`
	InviteLink      string `json:"inviteLink" dc:"邀请链接"`
	Status          *int   `json:"status" dc:"状态"`
}

func (in *BotSaveInp) Filter() error {
	in.Name = strings.TrimSpace(in.Name)
	in.WelcomeMessage = strings.TrimSpace(in.WelcomeMessage)
	in.BotToken = strings.TrimSpace(in.BotToken)
	in.ExistingGroupId = strings.TrimSpace(in.ExistingGroupId)
	in.SupergroupId = strings.TrimSpace(in.SupergroupId)
	in.SupergroupTitle = strings.TrimSpace(in.SupergroupTitle)
	in.InviteLink = strings.TrimSpace(in.InviteLink)
	if in.Id <= 0 && in.BotToken == "" {
		return gerror.New("请输入Bot Token")
	}
	if in.Id <= 0 && in.Status == nil {
		// 创建请求未传状态时默认启用；指针用于区分未传和明确停用。
		status := TwoWayBotStatusEnabled
		in.Status = &status
	}
	if in.Status != nil && *in.Status != TwoWayBotStatusEnabled && *in.Status != TwoWayBotStatusDisabled {
		return gerror.New("状态参数无效")
	}
	return nil
}

type BotDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择机器人" dc:"ID列表"`
}

type BotSettingsInp struct {
	Id             int64  `json:"id" v:"required|min:1#请选择机器人|请选择机器人" dc:"ID"`
	Name           string `json:"name" dc:"名称"`
	WelcomeMessage string `json:"welcomeMessage" dc:"欢迎语"`
}

func (in *BotSettingsInp) Filter() error {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择机器人")
	}
	in.Name = strings.TrimSpace(in.Name)
	in.WelcomeMessage = strings.TrimSpace(in.WelcomeMessage)
	if in.Name == "" {
		return gerror.New("请输入机器人名称")
	}
	if utf8.RuneCountInString(in.Name) > 128 {
		return gerror.New("机器人名称不能超过128个字符")
	}
	if utf8.RuneCountInString(in.WelcomeMessage) > 1000 {
		return gerror.New("欢迎语不能超过1000个字符")
	}
	return nil
}

type BotActionInp struct {
	Id int64 `json:"id" v:"required|min:1#请选择机器人|请选择机器人" dc:"ID"`
}

type BotModel struct {
	Id                   int64       `json:"id" dc:"ID"`
	TenantId             int64       `json:"tenantId" dc:"租户ID"`
	AccountId            int64       `json:"accountId" dc:"创建账号ID"`
	TgAccountId          int64       `json:"tgAccountId" dc:"TG协议号ID"`
	TgAccountName        string      `json:"tgAccountName" dc:"TG账号昵称"`
	Name                 string      `json:"name" dc:"名称"`
	WelcomeMessage       string      `json:"welcomeMessage" dc:"欢迎语"`
	BotUserId            string      `json:"botUserId" dc:"Bot用户ID"`
	BotUsername          string      `json:"botUsername" dc:"Bot用户名"`
	SupergroupId         string      `json:"supergroupId" dc:"管理群ID"`
	SupergroupAccessHash string      `json:"supergroupAccessHash" dc:"管理群AccessHash"`
	SupergroupTitle      string      `json:"supergroupTitle" dc:"管理群名称"`
	InviteLink           string      `json:"inviteLink" dc:"邀请链接"`
	SetupStatus          string      `json:"setupStatus" dc:"初始化状态"`
	WebhookStatus        string      `json:"webhookStatus" dc:"Webhook状态"`
	Status               int         `json:"status" dc:"状态"`
	ErrorMessage         string      `json:"errorMessage" dc:"错误信息"`
	LastSetupAt          *gtime.Time `json:"lastSetupAt" dc:"最后初始化时间"`
	LastWebhookAt        *gtime.Time `json:"lastWebhookAt" dc:"最后Webhook时间"`
	CreatedAt            *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt            *gtime.Time `json:"updatedAt" dc:"更新时间"`
}
