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
)

type TenantListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"租户名称/联系人"`
	Status  int    `json:"status" dc:"状态：1启用 2停用"`
}

type TenantModel struct {
	Id           int64       `json:"id" dc:"ID"`
	Name         string      `json:"name" dc:"租户名称"`
	ContactName  string      `json:"contactName" dc:"联系人"`
	ContactPhone string      `json:"contactPhone" dc:"联系电话"`
	Remark       string      `json:"remark" dc:"备注"`
	Status       int         `json:"status" dc:"状态"`
	CreatedAt    *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TenantSaveInp struct {
	Id           int64  `json:"id" dc:"ID"`
	Name         string `json:"name" dc:"租户名称"`
	ContactName  string `json:"contactName" dc:"联系人"`
	ContactPhone string `json:"contactPhone" dc:"联系电话"`
	Remark       string `json:"remark" dc:"备注"`
	Status       int    `json:"status" dc:"状态：1启用 2停用"`
}

func (in *TenantSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return gerror.New("租户名称不能为空")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("租户状态不合法")
	}
	return nil
}

type TenantDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的数据" dc:"ID列表"`
}

type AccountListInp struct {
	form.PageReq
	TenantId    int64  `json:"tenantId" dc:"租户ID"`
	AccountType string `json:"accountType" dc:"账号类型：admin/uploader"`
	Keyword     string `json:"keyword" dc:"账号/昵称"`
	Status      int    `json:"status" dc:"状态：1启用 2停用"`
}

type AccountModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	TenantId           int64       `json:"tenantId" dc:"租户ID"`
	TenantName         string      `json:"tenantName" dc:"租户名称"`
	AdminMemberId      int64       `json:"adminMemberId" dc:"绑定系统账号ID"`
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
	Remark             string      `json:"remark" dc:"备注"`
	Status             int         `json:"status" dc:"状态"`
	CreatedAt          *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt          *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type AccountSaveInp struct {
	Id                 int64  `json:"id" dc:"ID"`
	TenantId           int64  `json:"tenantId" dc:"租户ID"`
	AdminMemberId      int64  `json:"adminMemberId" dc:"绑定系统账号ID"`
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
	if in.TenantId <= 0 {
		return gerror.New("租户ID不能为空")
	}
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

type CurrentAccountModel struct {
	*AccountModel
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
	TenantName      string      `json:"tenantName" dc:"租户名称"`
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
