package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/model/input/form"
)

const (
	PublishAccountTypeAdmin    = "admin"
	PublishAccountTypeUploader = "uploader"

	PublishTaskStatusDraft      = "draft"
	PublishTaskStatusPending    = "pending"
	PublishTaskStatusPublishing = "publishing"
	PublishTaskStatusPublished  = "published"
	PublishTaskStatusFailed     = "failed"
	PublishTaskStatusCanceled   = "canceled"

	PublishTgAccountStatusPending    = "pending"
	PublishTgAccountStatusScanning   = "scanning"
	PublishTgAccountStatusPassword   = "password_required"
	PublishTgAccountStatusAuthorized = "authorized"
	PublishTgAccountStatusExpired    = "expired"
	PublishTgAccountStatusFailed     = "failed"
)

type TenantListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"管理员账号/备注"`
	Status  int    `json:"status" dc:"状态：1启用 2停用"`
}

type TenantModel struct {
	Id             int64       `json:"id" dc:"ID"`
	Name           string      `json:"name" dc:"内部归属名称"`
	AdminAccountId int64       `json:"adminAccountId" dc:"管理员账号ID"`
	Username       string      `json:"username" dc:"管理员登录账号"`
	Remark         string      `json:"remark" dc:"备注"`
	Status         int         `json:"status" dc:"状态"`
	CreatedAt      *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt      *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TenantSaveInp struct {
	Id       int64  `json:"id" dc:"ID"`
	Name     string `json:"name" dc:"内部归属名称"`
	Username string `json:"username" dc:"管理员登录账号，新增账号归属时必填"`
	Password string `json:"password" dc:"管理员登录密码，新增为空自动生成"`
	Remark   string `json:"remark" dc:"备注"`
	Status   int    `json:"status" dc:"状态：1启用 2停用"`
}

func (in *TenantSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	in.Password = strings.TrimSpace(in.Password)
	if in.Id == 0 && in.Username == "" {
		return gerror.New("管理员登录账号不能为空")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("账号归属状态不合法")
	}
	return nil
}

type TenantSaveModel struct {
	Id       int64  `json:"id" dc:"租户ID"`
	Password string `json:"password" dc:"管理员初始密码"`
}

type TenantDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type AccountListInp struct {
	form.PageReq
	TenantId       int64  `json:"tenantId" dc:"租户ID"`
	AccountType    string `json:"accountType" dc:"账号类型：admin/uploader"`
	ExcludeCurrent int    `json:"excludeCurrent" dc:"是否排除当前登录账号：1是 0否"`
	Keyword        string `json:"keyword" dc:"账号/昵称"`
	Status         int    `json:"status" dc:"状态：1启用 2停用"`
}

type AccountModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	TenantId           int64       `json:"tenantId" dc:"租户ID"`
	TenantName         string      `json:"tenantName" dc:"账号归属"`
	ParentId           int64       `json:"parentId" dc:"父账号ID"`
	AccountType        string      `json:"accountType" dc:"账号类型"`
	Nickname           string      `json:"nickname" dc:"昵称"`
	Username           string      `json:"username" dc:"用户名"`
	TelegramUserId     string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername   string      `json:"telegramUsername" dc:"TG用户名"`
	DailyPublishLimit  int         `json:"dailyPublishLimit" dc:"每日上架额度"`
	CanDirectPublish   int         `json:"canDirectPublish" dc:"是否可直接发布"`
	AllowedChannelJson string      `json:"allowedChannelJson" dc:"可发布频道JSON"`
	AllowedRegionJson  string      `json:"allowedRegionJson" dc:"可发布地区JSON"`
	DownCount          int         `json:"downCount" dc:"下架资料数量"`
	LastLoginAt        *gtime.Time `json:"lastLoginAt" dc:"最后登录时间"`
	Remark             string      `json:"remark" dc:"备注"`
	Status             int         `json:"status" dc:"状态"`
	UploadCount        int         `json:"uploadCount" dc:"上架资料数量"`
	CreatedAt          *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt          *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type AccountSaveInp struct {
	Id                 int64  `json:"id" dc:"ID"`
	TenantId           int64  `json:"tenantId" dc:"租户ID"`
	ParentId           int64  `json:"parentId" dc:"父账号ID"`
	AccountType        string `json:"accountType" dc:"账号类型：admin/uploader"`
	Nickname           string `json:"nickname" dc:"昵称"`
	Username           string `json:"username" dc:"用户名"`
	Password           string `json:"password" dc:"登录密码，新增时为空自动生成；编辑时为空不修改"`
	TelegramUserId     string `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername   string `json:"telegramUsername" dc:"TG用户名"`
	DailyPublishLimit  int    `json:"dailyPublishLimit" dc:"每日上架额度"`
	CanDirectPublish   int    `json:"canDirectPublish" dc:"是否可直接发布"`
	AllowedChannelJson string `json:"allowedChannelJson" dc:"可发布频道JSON"`
	AllowedRegionJson  string `json:"allowedRegionJson" dc:"可发布地区JSON"`
	Remark             string `json:"remark" dc:"备注"`
	Status             int    `json:"status" dc:"状态：1启用 2停用"`
}

func (in *AccountSaveInp) Filter(ctx context.Context) error {
	in.AccountType = strings.TrimSpace(in.AccountType)
	if in.AccountType == "" {
		in.AccountType = PublishAccountTypeAdmin
	}
	if in.AccountType != PublishAccountTypeAdmin && in.AccountType != PublishAccountTypeUploader {
		return gerror.New("账号类型不合法")
	}
	in.Nickname = strings.TrimSpace(in.Nickname)
	if in.Nickname == "" {
		in.Nickname = strings.TrimSpace(in.Username)
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" {
		return gerror.New("账号不能为空")
	}
	in.Password = strings.TrimSpace(in.Password)
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("账号状态不合法")
	}
	return nil
}

type AccountDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type AccountResetPasswordInp struct {
	Id       int64  `json:"id" v:"required|min:1#账号ID不能为空|账号ID不能为空" dc:"上架账号ID"`
	Password string `json:"password" dc:"新密码，为空自动生成"`
}

type AccountSaveModel struct {
	Id       int64  `json:"id" dc:"账号ID"`
	Password string `json:"password" dc:"账号初始密码"`
}

type CurrentAccountModel struct {
	Id          int64       `json:"id" dc:"账号ID"`
	TenantId    int64       `json:"tenantId" dc:"租户ID"`
	ParentId    int64       `json:"parentId" dc:"父账号ID"`
	AccountType string      `json:"accountType" dc:"账号类型"`
	Nickname    string      `json:"nickname" dc:"账号名称"`
	Username    string      `json:"username" dc:"用户名"`
	Remark      string      `json:"remark" dc:"个人简介"`
	Status      int         `json:"status" dc:"状态"`
	CreatedAt   *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt   *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type UpdateAccountPasswordInp struct {
	OldPassword string `json:"oldPassword" v:"required#当前密码不能为空" dc:"当前密码"`
	NewPassword string `json:"newPassword" v:"required#新密码不能为空" dc:"新密码"`
}

func (in *UpdateAccountPasswordInp) Filter(ctx context.Context) error {
	in.OldPassword = strings.TrimSpace(in.OldPassword)
	in.NewPassword = strings.TrimSpace(in.NewPassword)
	if in.OldPassword == "" {
		return gerror.New("当前密码不能为空")
	}
	if in.NewPassword == "" {
		return gerror.New("新密码不能为空")
	}
	if len([]rune(in.NewPassword)) < 6 {
		return gerror.New("新密码至少 6 位")
	}
	if in.OldPassword == in.NewPassword {
		return gerror.New("新密码不能与当前密码相同")
	}
	return nil
}

type UpdateAccountProfileInp struct {
	Nickname string `json:"nickname" v:"required#账号名称不能为空" dc:"账号名称"`
	Remark   string `json:"remark" dc:"个人简介"`
}

func (in *UpdateAccountProfileInp) Filter(ctx context.Context) error {
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Nickname == "" {
		return gerror.New("账号名称不能为空")
	}
	return nil
}

type AccountLoginInp struct {
	Username string `json:"username" v:"required#账号不能为空" dc:"账号"`
	Password string `json:"password" v:"required#密码不能为空" dc:"密码"`
}

func (in *AccountLoginInp) Filter(ctx context.Context) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Password = strings.TrimSpace(in.Password)
	if in.Username == "" {
		return gerror.New("账号不能为空")
	}
	if in.Password == "" {
		return gerror.New("密码不能为空")
	}
	return nil
}

type AccountLoginModel struct {
	Id          int64  `json:"id" dc:"账号ID"`
	TenantId    int64  `json:"tenantId" dc:"租户ID"`
	AccountType string `json:"accountType" dc:"账号类型"`
	Username    string `json:"username" dc:"账号"`
	Nickname    string `json:"nickname" dc:"昵称"`
	Token       string `json:"token" dc:"登录token"`
	Expires     int64  `json:"expires" dc:"登录有效期"`
}

type AccountRegisterInp struct {
	Username   string `json:"username" v:"required#账号不能为空" dc:"管理员账号"`
	Password   string `json:"password" v:"required#密码不能为空" dc:"登录密码"`
	Name       string `json:"name" dc:"租户名称"`
	InviteCode string `json:"inviteCode" v:"required#邀请码不能为空" dc:"邀请码"`
}

func (in *AccountRegisterInp) Filter(ctx context.Context) error {
	in.Username = strings.TrimSpace(in.Username)
	in.Password = strings.TrimSpace(in.Password)
	in.Name = strings.TrimSpace(in.Name)
	in.InviteCode = strings.TrimSpace(in.InviteCode)
	if in.Username == "" {
		return gerror.New("账号不能为空")
	}
	if in.Password == "" {
		return gerror.New("密码不能为空")
	}
	if in.InviteCode == "" {
		return gerror.New("邀请码不能为空")
	}
	if in.Name == "" {
		in.Name = in.Username
	}
	return nil
}

type AccountRegisterModel struct {
	*AccountLoginModel
}

type TaskListInp struct {
	form.PageReq
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
	AccountId int64  `json:"accountId" dc:"上架账号ID"`
	Status    string `json:"status" dc:"任务状态"`
	Keyword   string `json:"keyword" dc:"标题/编号"`
}

type TaskModel struct {
	Id              int64       `json:"id" dc:"ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	AccountId       int64       `json:"accountId" dc:"上架账号ID"`
	ProfileId       int64       `json:"profileId" dc:"资料ID"`
	ClientRequestId string      `json:"clientRequestId" dc:"客户端幂等ID"`
	Title           string      `json:"title" dc:"标题"`
	Province        string      `json:"province" dc:"省份"`
	City            string      `json:"city" dc:"城市"`
	PlainText       string      `json:"plainText" dc:"正文"`
	MediaCount      int         `json:"mediaCount" dc:"媒体数量"`
	TgPushEnabled   int         `json:"tgPushEnabled" dc:"是否推送TG"`
	TgStatus        string      `json:"tgStatus" dc:"TG状态"`
	Status          string      `json:"status" dc:"任务状态"`
	ErrorMessage    string      `json:"errorMessage" dc:"错误信息"`
	SubmittedAt     *gtime.Time `json:"submittedAt" dc:"提交时间"`
	PublishedAt     *gtime.Time `json:"publishedAt" dc:"发布时间"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
	TenantName      string      `json:"tenantName" dc:"账号归属"`
	AccountNickname string      `json:"accountNickname" dc:"账号昵称"`
	AccountUsername string      `json:"accountUsername" dc:"账号用户名"`
}

type TaskSaveInp struct {
	Id              int64  `json:"id" dc:"ID"`
	TenantId        int64  `json:"tenantId" dc:"租户ID，后台可传；API端取当前账号"`
	AccountId       int64  `json:"accountId" dc:"上架账号ID，后台可传；API端取当前账号"`
	ClientRequestId string `json:"clientRequestId" dc:"客户端幂等ID"`
	Title           string `json:"title" dc:"标题"`
	Province        string `json:"province" dc:"省份"`
	City            string `json:"city" dc:"城市"`
	PlainText       string `json:"plainText" dc:"正文"`
	MediaCount      int    `json:"mediaCount" dc:"媒体数量"`
	TgPushEnabled   int    `json:"tgPushEnabled" dc:"是否推送TG"`
}

func (in *TaskSaveInp) Filter(ctx context.Context) error {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return gerror.New("标题不能为空")
	}
	if in.TgPushEnabled == 0 {
		in.TgPushEnabled = 1
	}
	return nil
}

type TaskSubmitInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type TaskCancelInp struct {
	Id int64 `json:"id" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type MediaUploadInp struct {
	TaskId    int64  `json:"taskId" dc:"任务ID"`
	MediaType string `json:"mediaType" dc:"媒体类型：image/video"`
	SortIndex int    `json:"sortIndex" dc:"排序"`
}

func (in *MediaUploadInp) Filter(ctx context.Context) error {
	if in.TaskId <= 0 {
		return gerror.New("任务ID不能为空")
	}
	in.MediaType = strings.TrimSpace(in.MediaType)
	if in.MediaType == "" {
		in.MediaType = "image"
	}
	if in.MediaType != "image" && in.MediaType != "video" {
		return gerror.New("媒体类型不合法")
	}
	return nil
}

type MediaListInp struct {
	TaskId int64 `json:"taskId" v:"required|min:1#任务ID不能为空|任务ID不能为空" dc:"任务ID"`
}

type MediaDeleteInp struct {
	Id int64 `json:"id" v:"required|min:1#媒体ID不能为空|媒体ID不能为空" dc:"媒体ID"`
}

type MediaModel struct {
	Id           int64       `json:"id" dc:"ID"`
	TenantId     int64       `json:"tenantId" dc:"租户ID"`
	AccountId    int64       `json:"accountId" dc:"账号ID"`
	TaskId       int64       `json:"taskId" dc:"任务ID"`
	ProfileId    int64       `json:"profileId" dc:"资料ID"`
	AttachmentId int64       `json:"attachmentId" dc:"附件ID"`
	MediaType    string      `json:"mediaType" dc:"媒体类型"`
	Name         string      `json:"name" dc:"文件名"`
	FileUrl      string      `json:"fileUrl" dc:"访问地址"`
	StoragePath  string      `json:"storagePath" dc:"存储路径"`
	MimeType     string      `json:"mimeType" dc:"MIME"`
	Md5          string      `json:"md5" dc:"MD5"`
	Size         int64       `json:"size" dc:"大小"`
	SortIndex    int         `json:"sortIndex" dc:"排序"`
	Status       int         `json:"status" dc:"状态"`
	CreatedAt    *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type BotListInp struct {
	form.PageReq
	TenantId int64  `json:"tenantId" dc:"租户ID，0表示全局"`
	Keyword  string `json:"keyword" dc:"关键词"`
	Status   int    `json:"status" dc:"状态"`
}

type BotModel struct {
	Id          int64       `json:"id" dc:"ID"`
	TenantId    int64       `json:"tenantId" dc:"租户ID，0表示全局"`
	BotName     string      `json:"botName" dc:"Bot名称"`
	BotUsername string      `json:"botUsername" dc:"Bot用户名"`
	BotToken    string      `json:"botToken" dc:"Bot Token"`
	Remark      string      `json:"remark" dc:"备注"`
	Status      int         `json:"status" dc:"状态"`
	CreatedAt   *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt   *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type BotSaveInp struct {
	Id          int64  `json:"id" dc:"ID"`
	TenantId    int64  `json:"tenantId" dc:"租户ID，0表示全局"`
	BotName     string `json:"botName" dc:"Bot名称"`
	BotUsername string `json:"botUsername" dc:"Bot用户名"`
	BotToken    string `json:"botToken" dc:"Bot Token"`
	Remark      string `json:"remark" dc:"备注"`
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

type TgAccountListInp struct {
	form.PageReq
	TenantId int64  `json:"tenantId" dc:"租户ID"`
	Keyword  string `json:"keyword" dc:"TG用户/备注"`
	Status   string `json:"status" dc:"状态"`
}

type TgAccountModel struct {
	Id                int64       `json:"id" dc:"ID"`
	TenantId          int64       `json:"tenantId" dc:"租户ID"`
	AccountId         int64       `json:"accountId" dc:"创建账号ID"`
	DisplayName       string      `json:"displayName" dc:"显示名称"`
	TelegramUserId    string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername  string      `json:"telegramUsername" dc:"TG用户名"`
	TelegramFirstName string      `json:"telegramFirstName" dc:"TG名"`
	TelegramLastName  string      `json:"telegramLastName" dc:"TG姓"`
	TelegramPhone     string      `json:"telegramPhone" dc:"TG手机号"`
	TelegramIsBot     int         `json:"telegramIsBot" dc:"是否Bot"`
	SessionKey        string      `json:"sessionKey" dc:"会话存储键"`
	LoginToken        string      `json:"loginToken" dc:"登录令牌"`
	QrUrl             string      `json:"qrUrl" dc:"二维码地址"`
	Remark            string      `json:"remark" dc:"备注"`
	Status            string      `json:"status" dc:"状态"`
	ErrorMessage      string      `json:"errorMessage" dc:"错误信息"`
	LastLoginAt       *gtime.Time `json:"lastLoginAt" dc:"最后授权时间"`
	ExpiresAt         *gtime.Time `json:"expiresAt" dc:"二维码过期时间"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt         *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TgAccountStartLoginInp struct {
	TenantId    int64  `json:"tenantId" dc:"租户ID"`
	DisplayName string `json:"displayName" dc:"显示名称"`
	Remark      string `json:"remark" dc:"备注"`
}

func (in *TgAccountStartLoginInp) Filter(ctx context.Context) error {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Remark = strings.TrimSpace(in.Remark)
	return nil
}

type TgAccountLoginStatusInp struct {
	LoginToken string `json:"loginToken" v:"required#登录令牌不能为空" dc:"登录令牌"`
}

type TgAccountPasswordInp struct {
	LoginToken string `json:"loginToken" v:"required#登录令牌不能为空" dc:"登录令牌"`
	Password   string `json:"password" v:"required#二次验证密码不能为空" dc:"二次验证密码"`
}

type TgAccountDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的TG账号" dc:"ID列表"`
}

type TgAccountRefreshInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要刷新的TG账号" dc:"TG账号ID列表"`
}

type TgAccountRefreshModel struct {
	Id           int64  `json:"id" dc:"TG账号ID"`
	Status       string `json:"status" dc:"状态"`
	ErrorMessage string `json:"errorMessage" dc:"错误信息"`
}

type ChannelListInp struct {
	form.PageReq
	TenantId    int64  `json:"tenantId" dc:"租户ID"`
	TgAccountId int64  `json:"tgAccountId" dc:"TG账号ID"`
	Keyword     string `json:"keyword" dc:"频道名称/Chat ID"`
	Status      int    `json:"status" dc:"状态：1启用 2停用"`
}

type ChannelModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	TenantId           int64       `json:"tenantId" dc:"租户ID"`
	TgAccountId        int64       `json:"tgAccountId" dc:"TG账号ID"`
	TgAccountName      string      `json:"tgAccountName" dc:"TG账号名称"`
	ChannelTitle       string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername    string      `json:"channelUsername" dc:"频道用户名"`
	TargetChatId       string      `json:"targetChatId" dc:"目标Chat ID"`
	PublishDirection   string      `json:"publishDirection" dc:"上架/下架频道：up/down"`
	BotIds             []int64     `json:"botIds" dc:"绑定Bot ID列表"`
	BotIdJson          string      `json:"botIdJson" dc:"绑定Bot ID JSON"`
	Remark             string      `json:"remark" dc:"备注"`
	Status             int         `json:"status" dc:"状态"`
	LastRefreshStatus  string      `json:"lastRefreshStatus" dc:"最近刷新状态"`
	LastRefreshMessage string      `json:"lastRefreshMessage" dc:"最近刷新信息"`
	LastRefreshAt      *gtime.Time `json:"lastRefreshAt" dc:"最近刷新时间"`
	CreatedAt          *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt          *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ChannelSaveInp struct {
	Id               int64   `json:"id" dc:"ID"`
	TenantId         int64   `json:"tenantId" dc:"租户ID"`
	TgAccountId      int64   `json:"tgAccountId" dc:"TG账号ID"`
	ChannelTitle     string  `json:"channelTitle" dc:"频道名称"`
	ChannelUsername  string  `json:"channelUsername" dc:"频道用户名"`
	TargetChatId     string  `json:"targetChatId" dc:"目标Chat ID"`
	PublishDirection string  `json:"publishDirection" dc:"上架/下架频道：up/down"`
	BotIds           []int64 `json:"botIds" dc:"绑定Bot ID列表"`
	Remark           string  `json:"remark" dc:"备注"`
	Status           int     `json:"status" dc:"状态：1启用 2停用"`
}

func (in *ChannelSaveInp) Filter(ctx context.Context) error {
	if in.TgAccountId <= 0 {
		return gerror.New("请选择TG账号")
	}
	in.ChannelTitle = strings.TrimSpace(in.ChannelTitle)
	in.ChannelUsername = strings.TrimPrefix(strings.TrimSpace(in.ChannelUsername), "@")
	in.TargetChatId = strings.TrimSpace(in.TargetChatId)
	in.PublishDirection = strings.TrimSpace(in.PublishDirection)
	if in.PublishDirection == "" {
		in.PublishDirection = "up"
	}
	if in.PublishDirection != "up" && in.PublishDirection != "down" {
		return gerror.New("频道类型不合法")
	}
	if in.ChannelTitle == "" && in.ChannelUsername == "" && in.TargetChatId == "" {
		return gerror.New("频道名称、用户名或Chat ID至少填写一项")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("频道状态不合法")
	}
	in.BotIds = uniquePositiveInt64(in.BotIds)
	in.Remark = strings.TrimSpace(in.Remark)
	return nil
}

type ChannelDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的频道" dc:"频道ID列表"`
}

type ChannelBatchBotsInp struct {
	Ids    []int64 `json:"ids" v:"required#请选择频道" dc:"频道ID列表"`
	BotIds []int64 `json:"botIds" dc:"绑定Bot ID列表"`
}

type ChannelCacheListInp struct {
	form.PageReq
	TgAccountId int64  `json:"tgAccountId" dc:"TG账号ID"`
	Keyword     string `json:"keyword" dc:"关键词"`
}

type ChannelCacheModel struct {
	Id              int64       `json:"id" dc:"ID"`
	TenantId        int64       `json:"tenantId" dc:"租户ID"`
	TgAccountId     int64       `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId       string      `json:"channelId" dc:"频道ID"`
	AccessHash      string      `json:"accessHash" dc:"AccessHash"`
	ChannelTitle    string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername string      `json:"channelUsername" dc:"频道用户名"`
	IsBroadcast     int         `json:"isBroadcast" dc:"是否频道"`
	IsMegagroup     int         `json:"isMegagroup" dc:"是否群组"`
	CanPostMessages int         `json:"canPostMessages" dc:"账号可发频道消息"`
	CanInviteUsers  int         `json:"canInviteUsers" dc:"账号可邀请用户"`
	CanAddAdmins    int         `json:"canAddAdmins" dc:"账号可添加管理员"`
	LastSyncAt      *gtime.Time `json:"lastSyncAt" dc:"最后同步时间"`
}

type ChannelCacheRefreshInp struct {
	TgAccountId int64 `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号ID"`
}

type ChannelCacheRefreshModel struct {
	Count       int    `json:"count" dc:"同步数量"`
	Message     string `json:"message" dc:"同步结果"`
	SyncedAt    string `json:"syncedAt" dc:"同步时间"`
	TgAccountId int64  `json:"tgAccountId" dc:"TG账号ID"`
}

type ChannelCheckInp struct {
	TgAccountId  int64   `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号ID"`
	TargetChatId string  `json:"targetChatId" v:"required#请选择频道" dc:"频道ID"`
	BotIds       []int64 `json:"botIds" dc:"Bot ID列表"`
}

type ChannelCheckBotModel struct {
	BotId          int64  `json:"botId" dc:"Bot ID"`
	BotName        string `json:"botName" dc:"Bot名称"`
	BotUsername    string `json:"botUsername" dc:"Bot用户名"`
	CanSendMessage int    `json:"canSendMessage" dc:"是否可发送消息"`
	InChannel      int    `json:"inChannel" dc:"是否在频道中"`
	Status         string `json:"status" dc:"检测状态"`
	Message        string `json:"message" dc:"检测信息"`
}

type ChannelCheckModel struct {
	Allowed         int                     `json:"allowed" dc:"是否允许保存"`
	CanAddBot       int                     `json:"canAddBot" dc:"账号是否可添加Bot"`
	CanAddAdmin     int                     `json:"canAddAdmin" dc:"账号是否可添加管理员"`
	CanInviteUsers  int                     `json:"canInviteUsers" dc:"账号是否可邀请用户"`
	ChannelTitle    string                  `json:"channelTitle" dc:"频道名称"`
	ChannelUsername string                  `json:"channelUsername" dc:"频道用户名"`
	Message         string                  `json:"message" dc:"检测信息"`
	TargetChatId    string                  `json:"targetChatId" dc:"频道ID"`
	BotResults      []*ChannelCheckBotModel `json:"botResults" dc:"Bot检测结果"`
}

type ChannelRefreshInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要刷新的频道" dc:"频道ID列表"`
}

type ChannelRefreshModel struct {
	Id                 int64  `json:"id" dc:"频道ID"`
	LastRefreshStatus  string `json:"lastRefreshStatus" dc:"刷新状态"`
	LastRefreshMessage string `json:"lastRefreshMessage" dc:"刷新信息"`
}

func uniquePositiveInt64(values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(values))
	list := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		list = append(list, value)
	}
	return list
}

type TelegramLoginStartInp struct{}

type TelegramLoginModel struct {
	Id               int64       `json:"id" dc:"ID"`
	LoginToken       string      `json:"loginToken" dc:"登录令牌"`
	QrUrl            string      `json:"qrUrl" dc:"二维码地址"`
	TelegramUserId   string      `json:"telegramUserId" dc:"TG用户ID"`
	TelegramUsername string      `json:"telegramUsername" dc:"TG用户名"`
	Status           string      `json:"status" dc:"状态"`
	ErrorMessage     string      `json:"errorMessage" dc:"错误信息"`
	ExpiresAt        *gtime.Time `json:"expiresAt" dc:"过期时间"`
	CreatedAt        *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TelegramLoginStatusInp struct {
	LoginToken string `json:"loginToken" v:"required#登录令牌不能为空" dc:"登录令牌"`
}

type TelegramLoginPasswordInp struct {
	LoginToken string `json:"loginToken" v:"required#登录令牌不能为空" dc:"登录令牌"`
	Password   string `json:"password" v:"required#二次验证密码不能为空" dc:"二次验证密码"`
}
