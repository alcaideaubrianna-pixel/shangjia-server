package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	BotCodeSceneLogin = "login"
	BotCodeSceneBind  = "bind"

	BotCodeStatusPending    = "pending"
	BotCodeStatusAuthorized = "authorized"
	BotCodeStatusFailed     = "failed"
	BotCodeStatusExpired    = "expired"

	BotAppAdmin = "admin"
	BotAppApi   = "api"
)

type BotListInp struct {
	form.PageReq
	Keyword    string `json:"keyword" dc:"关键词"`
	IsOfficial int    `json:"isOfficial" dc:"是否官方Bot：1是 2否"`
	Status     int    `json:"status" dc:"状态"`
}

type BotModel struct {
	Id          int64       `json:"id" dc:"ID"`
	BotName     string      `json:"botName" dc:"Bot名称"`
	BotUsername string      `json:"botUsername" dc:"Bot用户名"`
	BotToken    string      `json:"botToken" dc:"Bot Token"`
	IsOfficial  int         `json:"isOfficial" dc:"是否官方Bot"`
	IsDefault   int         `json:"isDefault" dc:"是否默认官方Bot"`
	Remark      string      `json:"remark" dc:"备注"`
	RunMode     string      `json:"runMode" dc:"运行模式:auto/webhook/polling"`
	WebhookUrl  string      `json:"webhookUrl" dc:"Webhook地址"`
	Status      int         `json:"status" dc:"状态"`
	CreatedAt   *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt   *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type BotSaveInp struct {
	Id          int64  `json:"id" dc:"ID"`
	BotName     string `json:"botName" dc:"Bot名称"`
	BotUsername string `json:"botUsername" dc:"Bot用户名"`
	BotToken    string `json:"botToken" dc:"Bot Token"`
	IsOfficial  int    `json:"isOfficial" dc:"是否官方Bot：1是 0否"`
	IsDefault   int    `json:"isDefault" dc:"是否默认官方Bot：1是 0否"`
	Remark      string `json:"remark" dc:"备注"`
	RunMode     string `json:"runMode" dc:"运行模式:auto/webhook/polling"`
	WebhookUrl  string `json:"webhookUrl" dc:"Webhook地址"`
	Status      int    `json:"status" dc:"状态：1启用 2停用"`
}

type BotDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type BotRefreshInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要刷新的Bot" dc:"Bot ID列表"`
}

type BotRefreshModel struct {
	Id           int64  `json:"id" dc:"Bot ID"`
	BotUsername  string `json:"botUsername" dc:"Bot用户名"`
	Status       int    `json:"status" dc:"状态"`
	ErrorMessage string `json:"errorMessage" dc:"错误信息"`
}

type FeatureListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status" dc:"状态"`
}

type FeatureModel struct {
	Id           int64                  `json:"id" dc:"ID"`
	FeatureKey   string                 `json:"featureKey" dc:"功能Key"`
	Name         string                 `json:"name" dc:"功能名称"`
	Command      string                 `json:"command" dc:"Telegram命令"`
	Description  string                 `json:"description" dc:"功能描述"`
	ConfigJson   string                 `json:"configJson" dc:"配置JSON"`
	Sort         int                    `json:"sort" dc:"排序"`
	Status       int                    `json:"status" dc:"状态"`
	CreatedAt    string                 `json:"createdAt" dc:"创建时间"`
	UpdatedAt    string                 `json:"updatedAt" dc:"更新时间"`
	ConfigSchema []*FeatureConfigSchema `json:"configSchema" dc:"配置项协议"`
	ConfigValues map[string]interface{} `json:"configValues" dc:"配置值"`
}

type FeatureConfigOption struct {
	Label string      `json:"label" dc:"选项名称"`
	Value interface{} `json:"value" dc:"选项值"`
}

type FeatureConfigSchema struct {
	Field       string                 `json:"field" dc:"字段"`
	Label       string                 `json:"label" dc:"名称"`
	Component   string                 `json:"component" dc:"组件：switch/input/textarea/select"`
	Placeholder string                 `json:"placeholder" dc:"提示"`
	Default     interface{}            `json:"default" dc:"默认值"`
	Options     []*FeatureConfigOption `json:"options" dc:"选项"`
}

type FeatureSaveInp struct {
	Id          int64  `json:"id" dc:"ID"`
	FeatureKey  string `json:"featureKey" dc:"功能Key"`
	Name        string `json:"name" dc:"功能名称"`
	Command     string `json:"command" dc:"Telegram命令"`
	Description string `json:"description" dc:"功能描述"`
	ConfigJson  string `json:"configJson" dc:"配置JSON"`
	Sort        int    `json:"sort" dc:"排序"`
	Status      int    `json:"status" dc:"状态"`
}

type UserListInp struct {
	form.PageReq
	BotId   int64   `json:"botId" dc:"Bot ID"`
	BotIds  []int64 `json:"botIds" dc:"Bot ID列表"`
	Keyword string  `json:"keyword" dc:"关键词"`
	IsBound int     `json:"isBound" dc:"绑定状态：1已绑定 2未绑定"`
	BindApp string  `json:"bindApp" dc:"绑定应用：admin/api"`
	Status  int     `json:"status" dc:"状态"`
}

type UserModel struct {
	Id                int64       `json:"id" dc:"ID"`
	BotId             int64       `json:"botId" dc:"Bot ID"`
	BotUsername       string      `json:"botUsername" dc:"Bot用户名"`
	TelegramUserId    string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername  string      `json:"telegramUsername" dc:"TG用户名"`
	TelegramFirstName string      `json:"telegramFirstName" dc:"TG名"`
	TelegramLastName  string      `json:"telegramLastName" dc:"TG姓"`
	ChatId            string      `json:"chatId" dc:"Chat ID"`
	ChatType          string      `json:"chatType" dc:"Chat类型"`
	ChatTitle         string      `json:"chatTitle" dc:"Chat标题"`
	MessageCount      int         `json:"messageCount" dc:"消息数"`
	LastMessageText   string      `json:"lastMessageText" dc:"最后消息"`
	LastMessageAt     *gtime.Time `json:"lastMessageAt" dc:"最后消息时间"`
	IsBound           bool        `json:"isBound" dc:"是否已绑定"`
	BindApp           string      `json:"bindApp" dc:"绑定应用"`
	BindAccountId     int64       `json:"bindAccountId" dc:"绑定账号ID"`
	BindTenantId      int64       `json:"bindTenantId" dc:"绑定租户ID"`
	BindAccountName   string      `json:"bindAccountName" dc:"绑定账号"`
	Status            int         `json:"status" dc:"状态"`
	IsSuperAdmin      int         `json:"isSuperAdmin" dc:"是否超级管理员"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt         *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type MessageListInp struct {
	form.PageReq
	BotId          int64  `json:"botId" dc:"Bot ID"`
	TelegramUserId string `json:"telegramUserId" dc:"TG用户ID"`
	Keyword        string `json:"keyword" dc:"关键词"`
	MessageType    string `json:"messageType" dc:"消息类型"`
}

type UserSwitchSuperAdminInp struct {
	Id           int64 `json:"id" v:"required#请选择用户" dc:"用户ID"`
	IsSuperAdmin int   `json:"isSuperAdmin" dc:"是否超级管理员"`
}

type SendMessageInp struct {
	BotId         int64  `json:"botId" dc:"Bot ID"`
	ChatId        string `json:"chatId" v:"required#Chat ID不能为空" dc:"Chat ID"`
	Text          string `json:"text" v:"required#消息内容不能为空" dc:"消息内容"`
	DisableNotice bool   `json:"disableNotice" dc:"静默发送"`
}

type MessageModel struct {
	Id               int64       `json:"id" dc:"ID"`
	BotId            int64       `json:"botId" dc:"Bot ID"`
	BotUsername      string      `json:"botUsername" dc:"Bot用户名"`
	TelegramUserId   string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername string      `json:"telegramUsername" dc:"TG用户名"`
	ChatId           string      `json:"chatId" dc:"Chat ID"`
	ChatType         string      `json:"chatType" dc:"Chat类型"`
	MessageId        int64       `json:"messageId" dc:"消息ID"`
	MessageType      string      `json:"messageType" dc:"消息类型"`
	Text             string      `json:"text" dc:"消息内容"`
	RawJson          string      `json:"rawJson" dc:"原始消息JSON"`
	CreatedAt        *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type BotChannelCacheListInp struct {
	form.PageReq
	BotId   int64  `json:"botId" dc:"Bot ID"`
	Keyword string `json:"keyword" dc:"关键词"`
	Type    string `json:"type" dc:"类型：all/channel/group"`
}

type BotChannelCacheModel struct {
	Id              int64       `json:"id" dc:"ID"`
	BotId           int64       `json:"botId" dc:"Bot ID"`
	BotUsername     string      `json:"botUsername" dc:"Bot用户名"`
	ChannelId       string      `json:"channelId" dc:"频道/群聊ID"`
	ChannelTitle    string      `json:"channelTitle" dc:"频道/群聊名称"`
	ChannelUsername string      `json:"channelUsername" dc:"频道/群聊用户名"`
	ChatType        string      `json:"chatType" dc:"聊天类型"`
	IsBroadcast     int         `json:"isBroadcast" dc:"是否频道"`
	IsMegagroup     int         `json:"isMegagroup" dc:"是否群聊"`
	MessageCount    int         `json:"messageCount" dc:"消息数"`
	LastMessageText string      `json:"lastMessageText" dc:"最后消息"`
	LastMessageAt   *gtime.Time `json:"lastMessageAt" dc:"最后消息时间"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CodeStartInp struct {
	App string `json:"app" dc:"应用：admin/api"`
}

type CodeStartModel struct {
	Code        string      `json:"code" dc:"六位验证码"`
	Scene       string      `json:"scene" dc:"场景"`
	App         string      `json:"app" dc:"应用"`
	BotUsername string      `json:"botUsername" dc:"官方Bot用户名"`
	ExpiresAt   *gtime.Time `json:"expiresAt" dc:"过期时间"`
}

type CodeStatusInp struct {
	Code string `json:"code" v:"required#验证码不能为空" dc:"六位验证码"`
}

type CodeStatusModel struct {
	Code             string      `json:"code" dc:"六位验证码"`
	Scene            string      `json:"scene" dc:"场景"`
	App              string      `json:"app" dc:"应用"`
	Status           string      `json:"status" dc:"状态"`
	ErrorMessage     string      `json:"errorMessage" dc:"错误信息"`
	Token            string      `json:"token" dc:"登录Token"`
	AccessToken      string      `json:"accessToken" dc:"登录Token兼容字段"`
	Expires          int64       `json:"expires" dc:"过期时间戳"`
	AccountId        int64       `json:"id" dc:"账号ID"`
	TenantId         int64       `json:"tenantId" dc:"租户ID"`
	AccountType      string      `json:"accountType" dc:"账号类型"`
	Username         string      `json:"username" dc:"账号"`
	Nickname         string      `json:"nickname" dc:"昵称"`
	TelegramUserId   string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername string      `json:"telegramUsername" dc:"TG用户名"`
	ExpiresAt        *gtime.Time `json:"expiresAt" dc:"验证码过期时间"`
}

type BindInfoModel struct {
	Bound            bool   `json:"bound" dc:"是否已绑定"`
	TelegramUserId   string `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername string `json:"telegramUsername" dc:"TG用户名"`
	BotUsername      string `json:"botUsername" dc:"官方Bot用户名"`
}

type InviteInfoModel struct {
	Code           string      `json:"code" dc:"邀请码"`
	Source         string      `json:"source" dc:"来源"`
	ExpiresAt      *gtime.Time `json:"expiresAt" dc:"过期时间"`
	InviteCount    int         `json:"inviteCount" dc:"已邀请数量"`
	UsedCount      int         `json:"usedCount" dc:"已使用数量"`
	ExpireDays     int         `json:"expireDays" dc:"有效期天数"`
	InviteUrl      string      `json:"inviteUrl" dc:"注册链接"`
	BotInviteHint  string      `json:"botInviteHint" dc:"机器人提示"`
	WebInviteHint  string      `json:"webInviteHint" dc:"网页提示"`
	CanGenerateBot bool        `json:"canGenerateBot" dc:"是否可通过机器人生成"`
}

type InviteListInp struct {
	form.PageReq
	PerPageAlias int    `json:"perPage" dc:"每页数量（兼容旧参数）"`
	Keyword      string `json:"keyword" dc:"邀请码/账号/租户关键词"`
	Source       string `json:"source" dc:"来源:web/bot"`
	Status       string `json:"status" dc:"状态:active/used/expired"`
}

func (in *InviteListInp) GetPage() int {
	if in == nil {
		return 0
	}
	return in.PageReq.GetPage()
}

func (in *InviteListInp) GetPerPage() int {
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

type InviteModel struct {
	Id               int64       `json:"id" dc:"ID"`
	Code             string      `json:"code" dc:"邀请码"`
	Source           string      `json:"source" dc:"来源"`
	InviterApp       string      `json:"inviterApp" dc:"邀请来源应用"`
	InviterTenantId  int64       `json:"inviterTenantId" dc:"邀请人租户ID"`
	InviterAccountId int64       `json:"inviterAccountId" dc:"邀请人账号ID"`
	InviterUsername  string      `json:"inviterUsername" dc:"邀请人账号"`
	InviterNickname  string      `json:"inviterNickname" dc:"邀请人昵称"`
	UsedTenantId     int64       `json:"usedTenantId" dc:"注册租户ID"`
	UsedTenantName   string      `json:"usedTenantName" dc:"注册租户名称"`
	UsedAccountId    int64       `json:"usedAccountId" dc:"注册账号ID"`
	UsedAccountName  string      `json:"usedAccountName" dc:"注册账号"`
	Status           string      `json:"status" dc:"状态"`
	ExpiresAt        *gtime.Time `json:"expiresAt" dc:"过期时间"`
	UsedAt           *gtime.Time `json:"usedAt" dc:"使用时间"`
	CreatedAt        *gtime.Time `json:"createdAt" dc:"创建时间"`
}

type InviteCreateInp struct {
	Source   string `json:"source" dc:"来源:web/bot"`
	ForceNew int    `json:"forceNew" dc:"是否强制生成新邀请码：1是 0否"`
}

type InviteCreateModel struct {
	Code      string      `json:"code" dc:"邀请码"`
	Source    string      `json:"source" dc:"来源"`
	ExpiresAt *gtime.Time `json:"expiresAt" dc:"过期时间"`
	InviteUrl string      `json:"inviteUrl" dc:"注册链接"`
}

type WebhookInp struct {
	BotId int64  `json:"botId" dc:"Bot ID"`
	Body  []byte `json:"-" dc:"原始消息"`
}

type NotifyInp struct {
	BotId         int64  `json:"botId" dc:"Bot ID，为空使用官方Bot"`
	ChatId        string `json:"chatId" dc:"目标Chat ID"`
	Text          string `json:"text" dc:"消息内容"`
	ParseMode     string `json:"parseMode" dc:"解析模式"`
	DisableNotice bool   `json:"disableNotice" dc:"是否静默"`
}

type NotifyAccountInp struct {
	BotId         int64  `json:"botId" dc:"Bot ID，为空优先使用绑定Bot"`
	App           string `json:"app" dc:"应用：admin/api"`
	AccountId     int64  `json:"accountId" dc:"系统账号ID"`
	Text          string `json:"text" dc:"消息内容"`
	ParseMode     string `json:"parseMode" dc:"解析模式"`
	DisableNotice bool   `json:"disableNotice" dc:"是否静默"`
}

type NotifyRichInp struct {
	BotId            int64  `json:"botId" dc:"Bot ID，为空使用官方Bot"`
	ChatId           string `json:"chatId" dc:"目标Chat ID"`
	Text             string `json:"text" dc:"消息内容"`
	ParseMode        string `json:"parseMode" dc:"解析模式"`
	DisableNotice    bool   `json:"disableNotice" dc:"是否静默"`
	ButtonLabel      string `json:"buttonLabel" dc:"按钮文案"`
	ButtonURL        string `json:"buttonUrl" dc:"按钮链接"`
	SourceChatId     string `json:"sourceChatId" dc:"来源Chat ID"`
	SourceMessageId  int    `json:"sourceMessageId" dc:"来源消息ID"`
	SourceMessageIds []int  `json:"sourceMessageIds" dc:"来源消息ID列表"`
	SourceHasMedia   bool   `json:"sourceHasMedia" dc:"是否包含媒体"`
}

func NormalizeApp(app string) string {
	app = strings.TrimSpace(app)
	if app == "" || app == BotAppApi {
		return BotAppApi
	}
	if app == BotAppAdmin {
		return BotAppAdmin
	}
	return app
}

func (in *CodeStartInp) Filter(ctx context.Context) error {
	_ = ctx
	in.App = NormalizeApp(in.App)
	if in.App != BotAppAdmin && in.App != BotAppApi {
		return gerror.New("登录应用不合法")
	}
	return nil
}
