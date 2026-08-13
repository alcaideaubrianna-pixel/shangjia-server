package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/model/input/form"
)

const (
	PublishAccountTypeAdmin    = "admin"
	PublishAccountTypeUploader = "uploader"

	ProfilePermissionCreator = "creator"
	ProfilePermissionAdmin   = "admin"
	ProfilePermissionVisitor = "visitor"

	PublishTaskStatusPending    = "pending"
	PublishTaskStatusPublishing = "publishing"
	PublishTaskStatusPublished  = "published"
	PublishTaskStatusFailed     = "failed"
	PublishTaskStatusCanceled   = "canceled"

	PublishTgAccountStatusPending    = "pending"
	PublishTgAccountStatusScanning   = "scanning"
	PublishTgAccountStatusCode       = "code_required"
	PublishTgAccountStatusPassword   = "password_required"
	PublishTgAccountStatusAuthorized = "authorized"
	PublishTgAccountStatusExpired    = "expired"
	PublishTgAccountStatusFailed     = "failed"

	PublishTagReviewPending  = "pending"
	PublishTagReviewApproved = "approved"
	PublishTagReviewRejected = "rejected"
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
	VipLevel       int         `json:"vipLevel" dc:"会员等级"`
	VipStatus      int         `json:"vipStatus" dc:"会员状态"`
	VipExpiredAt   *gtime.Time `json:"vipExpiredAt" dc:"会员到期时间"`
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
	TenantId         int64  `json:"tenantId" dc:"租户ID"`
	ManagerAccountId int64  `json:"-" dc:"管理账号ID，内部权限过滤"`
	AccountType      string `json:"accountType" dc:"账号类型：admin/uploader"`
	ExcludeCurrent   int    `json:"excludeCurrent" dc:"是否排除当前登录账号：1是 0否"`
	Keyword          string `json:"keyword" dc:"账号/昵称"`
	Status           int    `json:"status" dc:"状态：1启用 2停用"`
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

type AccountTransferPreviewInp struct {
	FromAccountId int64 `json:"fromAccountId" v:"required|min:1#原账号ID不能为空|原账号ID不能为空" dc:"原账号ID"`
}

type AccountTransferPreviewModel struct {
	FromAccountId int64 `json:"fromAccountId" dc:"原账号ID"`
	ProfileCount  int   `json:"profileCount" dc:"资料数量"`
	TaskCount     int   `json:"taskCount" dc:"任务数量"`
	MediaCount    int   `json:"mediaCount" dc:"媒体数量"`
}

type AccountTransferProfilesInp struct {
	FromAccountId       int64 `json:"fromAccountId" v:"required|min:1#原账号ID不能为空|原账号ID不能为空" dc:"原账号ID"`
	ToAccountId         int64 `json:"toAccountId" v:"required|min:1#目标账号ID不能为空|目标账号ID不能为空" dc:"目标账号ID"`
	DeleteAfterTransfer int   `json:"deleteAfterTransfer" dc:"转移后删除原账号：1是 0否"`
}

type AccountTransferProfilesModel struct {
	FromAccountId int64 `json:"fromAccountId" dc:"原账号ID"`
	ToAccountId   int64 `json:"toAccountId" dc:"目标账号ID"`
	ProfileCount  int   `json:"profileCount" dc:"资料数量"`
	TaskCount     int   `json:"taskCount" dc:"任务数量"`
	MediaCount    int   `json:"mediaCount" dc:"媒体数量"`
	DeletedSource int   `json:"deletedSource" dc:"是否已删除原账号"`
}

type AccountResetPasswordInp struct {
	Id       int64  `json:"id" v:"required|min:1#账号ID不能为空|账号ID不能为空" dc:"上架账号ID"`
	Password string `json:"password" dc:"新密码，为空自动生成"`
}

type AccountSaveModel struct {
	Id       int64  `json:"id" dc:"账号ID"`
	Password string `json:"password" dc:"账号初始密码"`
}

type AccountSettingViewInp struct {
	AccountId int64 `json:"accountId" v:"required|min:1#账号ID不能为空|账号ID不能为空" dc:"账号ID"`
}

type AccountSettingSaveInp struct {
	AccountId       int64  `json:"accountId" v:"required|min:1#账号ID不能为空|账号ID不能为空" dc:"账号ID"`
	EnableSuffix    int    `json:"enableSuffix" dc:"是否启用发送后缀"`
	SuffixContent   string `json:"suffixContent" dc:"发送后缀内容"`
	EnableTitleMark int    `json:"enableTitleMark" dc:"是否启用编号标识"`
	MarkMode        string `json:"markMode" dc:"前缀模式：nickname/custom"`
	NumberSource    string `json:"numberSource" dc:"编号来源：sequence/random"`
	CustomMarkText  string `json:"customMarkText" dc:"自定义前缀"`
	MarkPosition    string `json:"markPosition" dc:"显示位置：top/bottom/feeLine"`
}

func (in *AccountSettingSaveInp) Filter(ctx context.Context) error {
	in.SuffixContent = strings.TrimSpace(in.SuffixContent)
	in.MarkMode = strings.TrimSpace(in.MarkMode)
	in.NumberSource = strings.TrimSpace(in.NumberSource)
	in.CustomMarkText = strings.TrimSpace(in.CustomMarkText)
	in.MarkPosition = strings.TrimSpace(in.MarkPosition)
	if in.MarkMode == "" {
		in.MarkMode = "nickname"
	}
	if in.MarkMode != "nickname" && in.MarkMode != "custom" {
		return gerror.New("标识模式不合法")
	}
	if in.NumberSource == "" {
		in.NumberSource = "sequence"
	}
	if in.NumberSource != "sequence" && in.NumberSource != "random" {
		return gerror.New("编号来源不合法")
	}
	if in.MarkPosition == "" {
		in.MarkPosition = "top"
	}
	if in.MarkPosition != "top" && in.MarkPosition != "bottom" && in.MarkPosition != "feeLine" {
		return gerror.New("标识显示位置不合法")
	}
	if in.EnableSuffix != 0 && in.EnableSuffix != 1 {
		return gerror.New("发送后缀开关不合法")
	}
	if in.EnableTitleMark != 0 && in.EnableTitleMark != 1 {
		return gerror.New("编号标识开关不合法")
	}
	return nil
}

type AccountSettingModel struct {
	AccountId       int64       `json:"accountId" dc:"账号ID"`
	EnableSuffix    int         `json:"enableSuffix" dc:"是否启用发送后缀"`
	SuffixContent   string      `json:"suffixContent" dc:"发送后缀内容"`
	EnableTitleMark int         `json:"enableTitleMark" dc:"是否启用编号标识"`
	MarkMode        string      `json:"markMode" dc:"前缀模式"`
	NumberSource    string      `json:"numberSource" dc:"编号来源"`
	CustomMarkText  string      `json:"customMarkText" dc:"自定义前缀"`
	MarkPosition    string      `json:"markPosition" dc:"显示位置"`
	PreviewMark     string      `json:"previewMark" dc:"编号标识预览"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type CurrentAccountModel struct {
	Id          int64                 `json:"id" dc:"账号ID"`
	TenantId    int64                 `json:"tenantId" dc:"租户ID"`
	ParentId    int64                 `json:"parentId" dc:"父账号ID"`
	AccountType string                `json:"accountType" dc:"账号类型"`
	Nickname    string                `json:"nickname" dc:"账号名称"`
	Username    string                `json:"username" dc:"用户名"`
	Remark      string                `json:"remark" dc:"个人简介"`
	Status      int                   `json:"status" dc:"状态"`
	CreatedAt   *gtime.Time           `json:"createdAt" dc:"创建时间"`
	UpdatedAt   *gtime.Time           `json:"updatedAt" dc:"更新时间"`
	Vip         *TenantVipStatusModel `json:"vip" dc:"会员状态"`
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

type InviteConsumeInp struct {
	Code          string `json:"code" v:"required#邀请码不能为空" dc:"邀请码"`
	InviterApp    string `json:"inviterApp" dc:"邀请人应用"`
	UsedTenantId  int64  `json:"usedTenantId" dc:"注册租户ID"`
	UsedAccountId int64  `json:"usedAccountId" dc:"注册账号ID"`
}

type MediaUploadInp struct {
	ProfileId      int64  `json:"profileId" dc:"资料ID"`
	MediaId        int64  `json:"mediaId" dc:"媒体ID，编辑已有媒体时传入"`
	MediaType      string `json:"mediaType" dc:"媒体类型：image/video"`
	MustSend       *bool  `json:"mustSend" dc:"是否每次推送必发"`
	Purpose        string `json:"purpose" dc:"用途：display/verify"`
	SortIndex      int    `json:"sortIndex" dc:"排序"`
	EditConfigJson string `json:"editConfigJson" dc:"图片编辑配置"`
	EditStatus     string `json:"editStatus" dc:"编辑状态：raw/edited"`
	UploadTraceId  string `json:"uploadTraceId" dc:"前端上传链路ID"`
	UploadUid      string `json:"uploadUid" dc:"前端上传文件ID"`
}

func (in *MediaUploadInp) Filter(ctx context.Context) error {
	if in.ProfileId <= 0 {
		return gerror.New("资料ID不能为空")
	}
	in.MediaType = strings.TrimSpace(in.MediaType)
	if in.MediaType == "" {
		in.MediaType = "image"
	}
	if in.MediaType != "image" && in.MediaType != "video" {
		return gerror.New("媒体类型不合法")
	}
	in.Purpose = strings.TrimSpace(in.Purpose)
	if in.Purpose == "" {
		in.Purpose = "display"
	}
	if in.Purpose != "display" && in.Purpose != "verify" {
		return gerror.New("媒体用途不合法")
	}
	return nil
}

type MediaListInp struct {
	ProfileId int64 `json:"profileId" dc:"资料ID"`
}

type MediaDeleteInp struct {
	Id int64 `json:"id" v:"required|min:1#媒体ID不能为空|媒体ID不能为空" dc:"媒体ID"`
}

type MediaModel struct {
	Id                   int64       `json:"id" dc:"ID"`
	TenantId             int64       `json:"tenantId" dc:"租户ID"`
	AccountId            int64       `json:"accountId" dc:"账号ID"`
	ProfileId            int64       `json:"profileId" dc:"资料ID"`
	AttachmentId         int64       `json:"attachmentId" dc:"附件ID"`
	OriginalAttachmentId int64       `json:"originalAttachmentId" dc:"原始附件ID"`
	EditedAttachmentId   int64       `json:"editedAttachmentId" dc:"编辑后附件ID"`
	MediaType            string      `json:"mediaType" dc:"媒体类型"`
	MustSend             bool        `json:"mustSend" dc:"是否每次推送必发"`
	Purpose              string      `json:"purpose" dc:"用途：display/verify"`
	Name                 string      `json:"name" dc:"文件名"`
	FileUrl              string      `json:"fileUrl" dc:"访问地址"`
	OriginalFileUrl      string      `json:"originalFileUrl" dc:"原始访问地址"`
	EditedFileUrl        string      `json:"editedFileUrl" dc:"编辑后访问地址"`
	PosterUrl            string      `json:"posterUrl" dc:"视频封面"`
	StoragePath          string      `json:"storagePath" dc:"存储路径"`
	OriginalStoragePath  string      `json:"originalStoragePath" dc:"原始存储路径"`
	EditedStoragePath    string      `json:"editedStoragePath" dc:"编辑后存储路径"`
	PosterStoragePath    string      `json:"posterStoragePath" dc:"视频封面存储路径"`
	MimeType             string      `json:"mimeType" dc:"MIME"`
	Md5                  string      `json:"md5" dc:"MD5"`
	PerceptualHash       string      `json:"perceptualHash" dc:"图片感知哈希"`
	ProcessingStatus     string      `json:"processingStatus" dc:"媒体处理状态"`
	ProcessingError      string      `json:"processingError" dc:"媒体处理错误"`
	EditConfigJson       string      `json:"editConfigJson" dc:"图片编辑配置"`
	EditStatus           string      `json:"editStatus" dc:"编辑状态：raw/edited"`
	TgFileId             string      `json:"tgFileId" dc:"TG文件ID"`
	TgThumbFileId        string      `json:"tgThumbFileId" dc:"TG缩略图ID"`
	TgCacheAssetHash     string      `json:"tgCacheAssetHash" dc:"TG缓存素材Hash"`
	TgCacheStatus        string      `json:"tgCacheStatus" dc:"TG缓存状态"`
	Size                 int64       `json:"size" dc:"大小"`
	SortIndex            int         `json:"sortIndex" dc:"排序"`
	Status               int         `json:"status" dc:"状态"`
	CreatedAt            *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt            *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type BotMediaCacheFileInp struct {
	Media *MediaModel `json:"media" dc:"媒体信息"`
}

type BotMediaCacheFileModel struct {
	Path string `json:"path" dc:"本地缓存文件路径"`
}

type ProfileListInp struct {
	form.PageReq
	TenantId        int64  `json:"tenantId" dc:"租户ID"`
	AccountId       int64  `json:"accountId" dc:"上架账号ID"`
	CollectSourceId int64  `json:"collectSourceId" dc:"采集源频道ID"`
	SourceScope     string `json:"sourceScope" dc:"资料来源范围：all/collected/manual"`
	AccountScope    string `json:"accountScope" dc:"账号范围：all/mine/following"`
	Keyword         string `json:"keyword" dc:"标题/编号/正文"`
	Province        string `json:"province" dc:"省份"`
	City            string `json:"city" dc:"城市"`
	Tag             string `json:"tag" dc:"标签"`
	ReviewStatus    string `json:"reviewStatus" dc:"审核状态"`
	Visibility      string `json:"visibility" dc:"可见性"`
	Status          int    `json:"status" dc:"状态：1上架 2下架"`
}

type ProfileViewInp struct {
	Id   int64  `json:"id" dc:"资料ID"`
	Uuid string `json:"uuid" dc:"资料UUID"`
}

type AdminProfilePublishInp struct {
	ProfileViewInp
	BatchId string `json:"batchId" dc:"批量操作ID"`
}

type AdminProfileBatchCancelInp struct {
	BatchId string `json:"batchId" v:"required#批量操作ID不能为空" dc:"批量操作ID"`
}

type AdminProfileBatchCancelModel struct {
	Canceled int `json:"canceled" dc:"已取消TG任务数"`
	Sending  int `json:"sending" dc:"已进入发送流程、无法取消的任务数"`
}

type ProfileModel struct {
	Id                     int64       `json:"id" dc:"资料ID"`
	NoteIndexId            int64       `json:"-" dc:"资料索引ID"`
	Uuid                   string      `json:"uuid" dc:"资料UUID"`
	TenantId               int64       `json:"tenantId" dc:"租户ID"`
	AccountId              int64       `json:"accountId" dc:"上架账号ID"`
	SourceType             string      `json:"sourceType" dc:"资料来源类型"`
	IsCollected            bool        `json:"isCollected" dc:"是否采集资料"`
	CollectSourceId        int64       `json:"collectSourceId" dc:"采集源频道ID"`
	CollectSourceName      string      `json:"collectSourceName" dc:"采集源频道名称"`
	CollectSourceUsername  string      `json:"collectSourceUsername" dc:"采集源频道用户名"`
	CollectSourceChatId    string      `json:"collectSourceChatId" dc:"采集来源TG Chat ID"`
	CollectSourceMessageId int64       `json:"collectSourceMessageId" dc:"采集来源消息ID"`
	CollectSourceUrl       string      `json:"collectSourceUrl" dc:"采集来源地址"`
	TenantName             string      `json:"tenantName" dc:"账号归属"`
	AccountName            string      `json:"accountName" dc:"上架账号昵称"`
	Nickname               string      `json:"nickname" dc:"账号名称"`
	Username               string      `json:"username" dc:"上架账号用户名"`
	ChannelIds             []int64     `json:"channelIds" dc:"推送频道ID列表"`
	AntiScanEnabled        int         `json:"antiScanEnabled" dc:"是否防扫图处理"`
	CustomerRemark         string      `json:"customerRemark" dc:"客服备注"`
	TaskStatus             string      `json:"taskStatus" dc:"上架任务状态"`
	TgStatus               string      `json:"tgStatus" dc:"TG推送状态"`
	TgPushEnabled          int         `json:"tgPushEnabled" dc:"是否推送TG"`
	ProfileNo              string      `json:"profileNo" dc:"资料编号"`
	Title                  string      `json:"title" dc:"标题"`
	Summary                string      `json:"summary" dc:"摘要"`
	PlainText              string      `json:"plainText" dc:"正文"`
	Province               string      `json:"province" dc:"省份"`
	City                   string      `json:"city" dc:"城市"`
	Tag                    string      `json:"tag" dc:"标签"`
	Visibility             string      `json:"visibility" dc:"可见性"`
	ReviewStatus           string      `json:"reviewStatus" dc:"审核状态"`
	Status                 int         `json:"status" dc:"状态"`
	ImageCount             int         `json:"imageCount" dc:"图片数"`
	VideoCount             int         `json:"videoCount" dc:"视频数"`
	CanEdit                bool        `json:"canEdit" dc:"当前账号是否可编辑"`
	Permission             string      `json:"permission" dc:"当前账号权限：creator/admin/visitor"`
	PublishedAt            *gtime.Time `json:"publishedAt" dc:"发布时间"`
	CreatedAt              *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt              *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ProfileViewModel struct {
	Profile      *ProfileModel              `json:"profile" dc:"资料详情"`
	Media        []*MediaModel              `json:"media" dc:"媒体列表"`
	PushChannels []*ProfilePushChannelModel `json:"pushChannels" dc:"资料推送频道"`
}

type ProfilePushChannelModel struct {
	ChannelId           int64       `json:"channelId" dc:"频道ID"`
	ChannelTitle        string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername     string      `json:"channelUsername" dc:"频道用户名"`
	Status              int         `json:"status" dc:"频道状态"`
	CyclePublishEnabled int         `json:"cyclePublishEnabled" dc:"是否循环推送"`
	CyclePublishDays    int         `json:"cyclePublishDays" dc:"循环推送天数"`
	FirstPushAt         *gtime.Time `json:"firstPushAt" dc:"首次推送时间"`
	NextPushAt          *gtime.Time `json:"nextPushAt" dc:"循环到期时间"`
}

type ProfileOptionsModel struct {
	Channels []*ChannelModel `json:"channels" dc:"可选推送频道"`
}

type ProfileImageSearchInp struct {
	ProfileListInp
	Threshold int `json:"threshold" dc:"相似度阈值，越小越严格"`
}

type BotProfileImageSearchInp struct {
	ProfileImageSearchInp
	ImageUrl    string `json:"imageUrl" dc:"图片地址"`
	AccountType string `json:"accountType" dc:"账号类型：admin/uploader"`
}

type BotMediaSearchItem struct {
	FileUrl   string `json:"fileUrl" dc:"图片或视频预览图地址"`
	MediaType string `json:"mediaType" dc:"媒体类型：image/video"`
}

type BotMediaSearchInp struct {
	TenantId    int64                 `json:"tenantId" dc:"租户ID"`
	AccountId   int64                 `json:"accountId" dc:"绑定账号ID"`
	AccountType string                `json:"accountType" dc:"账号类型：admin/uploader"`
	Items       []*BotMediaSearchItem `json:"items" dc:"待搜索媒体"`
	Threshold   int                   `json:"threshold" dc:"相似度阈值"`
}

type ProfileSaveInp struct {
	Id               int64                   `json:"id" dc:"资料ID"`
	Uuid             string                  `json:"uuid" dc:"资料UUID"`
	ChannelIds       []int64                 `json:"channelIds" dc:"推送频道ID列表"`
	Title            string                  `json:"title" v:"required#标题不能为空" dc:"标题"`
	Province         string                  `json:"province" dc:"省份"`
	City             string                  `json:"city" dc:"城市"`
	PlainText        string                  `json:"plainText" dc:"正文"`
	Tag              string                  `json:"tag" dc:"标签"`
	CustomerRemark   string                  `json:"customerRemark" dc:"客服备注"`
	AntiScanEnabled  int                     `json:"antiScanEnabled" dc:"是否防扫图处理"`
	PublishAt        string                  `json:"publishAt" dc:"定时上架时间"`
	Visibility       string                  `json:"visibility" dc:"可见性：private/public/member_only"`
	Status           int                     `json:"status" dc:"状态：1上架 2下架"`
	Media            []*ProfileMediaSaveItem `json:"media" dc:"资料媒体清单"`
	KeepPublishState bool                    `json:"-"`
}

type ProfileMediaSaveItem struct {
	MediaId   int64  `json:"mediaId" dc:"媒体ID"`
	MustSend  *bool  `json:"mustSend" dc:"是否每次推送必发"`
	Purpose   string `json:"purpose" dc:"用途：display/verify"`
	SortIndex int    `json:"sortIndex" dc:"排序"`
}

func (in *ProfileSaveInp) Filter(ctx context.Context) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Province = strings.TrimSpace(in.Province)
	in.City = strings.TrimSpace(in.City)
	in.PlainText = strings.TrimSpace(in.PlainText)
	in.Tag = strings.TrimSpace(in.Tag)
	in.CustomerRemark = strings.TrimSpace(in.CustomerRemark)
	in.PublishAt = strings.TrimSpace(in.PublishAt)
	in.Visibility = strings.TrimSpace(in.Visibility)
	if in.Title == "" {
		return gerror.New("标题不能为空")
	}
	if in.Visibility == "" {
		in.Visibility = "private"
	}
	if in.Visibility != "private" && in.Visibility != "public" && in.Visibility != "member_only" {
		return gerror.New("可见性不合法")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("资料状态不合法")
	}
	if in.AntiScanEnabled != 0 && in.AntiScanEnabled != 1 {
		return gerror.New("防扫图配置不合法")
	}
	if in.PublishAt != "" && gtime.NewFromStr(in.PublishAt) == nil {
		return gerror.New("定时上架时间不合法")
	}
	for _, item := range in.Media {
		if item == nil || item.MediaId <= 0 {
			return gerror.New("资料媒体ID不能为空")
		}
		item.Purpose = strings.TrimSpace(item.Purpose)
		if item.Purpose == "" {
			item.Purpose = "display"
		}
		if item.Purpose != "display" && item.Purpose != "verify" {
			return gerror.New("资料媒体用途不合法")
		}
		if item.SortIndex <= 0 {
			return gerror.New("资料媒体排序不能为空")
		}
	}
	return nil
}

type ProfileSaveModel struct {
	Id        int64  `json:"id" dc:"资料ID"`
	Uuid      string `json:"uuid" dc:"资料UUID"`
	ProfileNo string `json:"profileNo" dc:"资料编号"`
}

type ProfileDeleteInp struct {
	Ids   []int64  `json:"ids" dc:"资料ID列表"`
	Uuids []string `json:"uuids" dc:"资料UUID列表"`
}

type ProfileStatusInp struct {
	Ids    []int64  `json:"ids" dc:"资料ID列表"`
	Uuids  []string `json:"uuids" dc:"资料UUID列表"`
	Status int      `json:"status" v:"required#状态不能为空" dc:"状态：1上架 2下架"`
}

type ProfileStatusModel struct {
	NeedRepair  int    `json:"needRepair" dc:"是否需要修复TG消息"`
	RepairRunId int64  `json:"repairRunId" dc:"修复任务ID"`
	Message     string `json:"message" dc:"提示信息"`
}

type CollectProfileMediaRebuildResult struct {
	Candidates  int     `json:"candidates"`
	Recoverable int     `json:"recoverable"`
	Requeued    int     `json:"requeued"`
	ProfileIDs  []int64 `json:"profileIds"`
}

type TgMessageRepairStartInp struct {
	ProfileId int64  `json:"profileId" dc:"资料ID"`
	Uuid      string `json:"uuid" dc:"资料UUID"`
}

type TgMessageRepairViewInp struct {
	RunId int64 `json:"runId" v:"required#修复任务ID不能为空" dc:"修复任务ID"`
}

type TgMessageRepairModel struct {
	Id           int64       `json:"id" dc:"ID"`
	TenantId     int64       `json:"tenantId" dc:"租户ID"`
	AccountId    int64       `json:"accountId" dc:"账号ID"`
	ProfileId    int64       `json:"profileId" dc:"资料ID"`
	TaskId       int64       `json:"taskId" dc:"任务ID"`
	Status       string      `json:"status" dc:"状态"`
	Stage        string      `json:"stage" dc:"阶段"`
	Progress     int         `json:"progress" dc:"进度"`
	ChannelCount int         `json:"channelCount" dc:"频道数量"`
	ScannedCount int         `json:"scannedCount" dc:"扫描消息数"`
	MatchedCount int         `json:"matchedCount" dc:"匹配消息数"`
	ErrorMessage string      `json:"errorMessage" dc:"错误信息"`
	CreatedAt    *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt" dc:"更新时间"`
	FinishedAt   *gtime.Time `json:"finishedAt" dc:"完成时间"`
}

type ProfileReviewInp struct {
	Ids          []int64  `json:"ids" dc:"资料ID列表"`
	Uuids        []string `json:"uuids" dc:"资料UUID列表"`
	ReviewStatus string   `json:"reviewStatus" v:"required#审核状态不能为空" dc:"审核状态"`
}

func (in *ProfileReviewInp) Filter(ctx context.Context) error {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return gerror.New("请选择要审核的资料")
	}
	in.ReviewStatus = strings.TrimSpace(in.ReviewStatus)
	if in.ReviewStatus != consts.ContentReviewApproved && in.ReviewStatus != consts.ContentReviewRejected && in.ReviewStatus != consts.ContentReviewPending {
		return gerror.New("审核状态不合法")
	}
	return nil
}

type NoteListInp struct {
	ProfileListInp
	Cursor string `json:"cursor" dc:"下一页游标"`
}

type NoteModel struct {
	ProfileModel
	Media        []*MediaModel `json:"media" dc:"媒体列表"`
	MatchedMedia *MediaModel   `json:"matchedMedia,omitempty" dc:"图片搜索实际命中的媒体"`
}

type AdminNoteMediaModel struct {
	Id        int64  `json:"id" dc:"ID"`
	ProfileId int64  `json:"profileId" dc:"资料ID"`
	MediaType string `json:"mediaType" dc:"媒体类型"`
	FileUrl   string `json:"fileUrl" dc:"访问地址"`
	PosterUrl string `json:"posterUrl" dc:"视频封面地址"`
	SortIndex int    `json:"sortIndex" dc:"排序"`
}

type AdminNoteListModel struct {
	Id                    int64                  `json:"id" dc:"资料ID"`
	Uuid                  string                 `json:"uuid" dc:"资料UUID"`
	AccountId             int64                  `json:"accountId" dc:"上架账号ID"`
	SourceType            string                 `json:"sourceType" dc:"资料来源类型"`
	IsCollected           bool                   `json:"isCollected" dc:"是否采集资料"`
	CollectSourceId       int64                  `json:"collectSourceId" dc:"采集源频道ID"`
	CollectSourceName     string                 `json:"collectSourceName" dc:"采集源频道名称"`
	CollectSourceUsername string                 `json:"collectSourceUsername" dc:"采集源频道用户名"`
	AccountName           string                 `json:"accountName" dc:"上架账号昵称"`
	Nickname              string                 `json:"nickname" dc:"账号名称"`
	Username              string                 `json:"username" dc:"上架账号用户名"`
	ProfileNo             string                 `json:"profileNo" dc:"资料编号"`
	Title                 string                 `json:"title" dc:"标题"`
	Province              string                 `json:"province" dc:"省份"`
	City                  string                 `json:"city" dc:"城市"`
	Tag                   string                 `json:"tag" dc:"标签"`
	Status                int                    `json:"status" dc:"状态"`
	TaskStatus            string                 `json:"taskStatus" dc:"上架任务状态"`
	CanEdit               bool                   `json:"canEdit" dc:"当前账号是否可编辑"`
	Permission            string                 `json:"permission" dc:"当前账号权限：creator/admin/visitor"`
	CreatedAt             *gtime.Time            `json:"createdAt" dc:"创建时间"`
	UpdatedAt             *gtime.Time            `json:"updatedAt" dc:"更新时间"`
	Media                 []*AdminNoteMediaModel `json:"media" dc:"封面媒体"`
}

type AdminNotePageModel struct {
	List       []*AdminNoteListModel `json:"list" dc:"笔记列表"`
	HasMore    bool                  `json:"hasMore" dc:"是否还有下一页"`
	NextCursor string                `json:"nextCursor" dc:"下一页游标"`
}

type AdminNoteBatchIdsModel struct {
	Ids   []int64 `json:"ids" dc:"批量执行资料ID"`
	Total int     `json:"total" dc:"资料总数"`
}

type FollowNoteMediaModel struct {
	Id                int64  `json:"id" dc:"ID"`
	ProfileId         int64  `json:"profileId" dc:"资料ID"`
	MediaType         string `json:"mediaType" dc:"媒体类型"`
	Purpose           string `json:"purpose" dc:"用途：display/verify"`
	Name              string `json:"name" dc:"文件名"`
	FileUrl           string `json:"fileUrl" dc:"访问地址"`
	EditedFileUrl     string `json:"editedFileUrl" dc:"编辑后访问地址"`
	PosterUrl         string `json:"posterUrl" dc:"视频封面"`
	StoragePath       string `json:"storagePath" dc:"存储路径"`
	EditedStoragePath string `json:"editedStoragePath" dc:"编辑后存储路径"`
	PosterStoragePath string `json:"posterStoragePath" dc:"视频封面存储路径"`
	SortIndex         int    `json:"sortIndex" dc:"排序"`
}

type FollowNoteModel struct {
	Id            int64                   `json:"id" dc:"资料ID"`
	Uuid          string                  `json:"uuid" dc:"资料UUID"`
	AccountId     int64                   `json:"accountId" dc:"上架账号ID"`
	AccountName   string                  `json:"accountName" dc:"上架账号昵称"`
	Nickname      string                  `json:"nickname" dc:"账号名称"`
	Username      string                  `json:"username" dc:"上架账号用户名"`
	ProfileNo     string                  `json:"profileNo" dc:"资料编号"`
	Title         string                  `json:"title" dc:"标题"`
	Summary       string                  `json:"summary" dc:"摘要"`
	Province      string                  `json:"province" dc:"省份"`
	City          string                  `json:"city" dc:"城市"`
	Tag           string                  `json:"tag" dc:"标签"`
	ReviewStatus  string                  `json:"reviewStatus" dc:"审核状态"`
	Status        int                     `json:"status" dc:"状态"`
	ImageCount    int                     `json:"imageCount" dc:"图片数"`
	VideoCount    int                     `json:"videoCount" dc:"视频数"`
	TaskStatus    string                  `json:"taskStatus" dc:"上架任务状态"`
	TgStatus      string                  `json:"tgStatus" dc:"TG推送状态"`
	TgPushEnabled int                     `json:"tgPushEnabled" dc:"是否推送TG"`
	CanEdit       bool                    `json:"canEdit" dc:"当前账号是否可编辑"`
	Permission    string                  `json:"permission" dc:"当前账号权限：creator/admin/visitor"`
	PublishedAt   *gtime.Time             `json:"publishedAt" dc:"发布时间"`
	CreatedAt     *gtime.Time             `json:"createdAt" dc:"创建时间"`
	UpdatedAt     *gtime.Time             `json:"updatedAt" dc:"更新时间"`
	Media         []*FollowNoteMediaModel `json:"media" dc:"媒体列表"`
}

// BotProfileSearchInp is used by youban_bot to search profiles for a bound publish account.
type BotProfileSearchInp struct {
	form.PageReq
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
	AccountId int64  `json:"accountId" dc:"上架账号ID"`
	Keyword   string `json:"keyword" dc:"标题/编号/正文"`
	ProfileNo string `json:"profileNo" dc:"资料编号"`
	Status    int    `json:"status" dc:"状态：1上架 2下架"`
}

// BotProfileViewInp is used by youban_bot to view a profile by id or profile_no.
type BotProfileViewInp struct {
	TenantId    int64   `json:"tenantId" dc:"租户ID"`
	AccountId   int64   `json:"accountId" dc:"上架账号ID"`
	AccountIds  []int64 `json:"accountIds" dc:"可见上架账号ID列表"`
	AccountType string  `json:"accountType" dc:"账号类型：admin/uploader"`
	ProfileId   int64   `json:"profileId" dc:"资料ID"`
	ProfileNo   string  `json:"profileNo" dc:"资料编号"`
	PublicOnly  bool    `json:"publicOnly" dc:"是否只允许公开/上架资料"`
}

// BotProfileStatusInp is used by youban_bot to update profile status.
type BotProfileStatusInp struct {
	TenantId  int64    `json:"tenantId" dc:"租户ID"`
	AccountId int64    `json:"accountId" dc:"上架账号ID"`
	Ids       []int64  `json:"ids" dc:"资料ID列表"`
	Nos       []string `json:"nos" dc:"资料编号列表"`
	Status    int      `json:"status" dc:"状态：1上架 2下架"`
}

// BotProfileCreateInp is used by youban_bot to create a text profile quickly.
type BotProfileCreateInp struct {
	TenantId     int64                      `json:"tenantId" dc:"租户ID"`
	AccountId    int64                      `json:"accountId" dc:"上架账号ID"`
	Title        string                     `json:"title" dc:"标题"`
	PlainText    string                     `json:"plainText" dc:"展示正文"`
	DisplayMedia []*MessageTemplateMediaInp `json:"displayMedia" dc:"展示媒体"`
	VerifyText   string                     `json:"verifyText" dc:"验证正文"`
	VerifyMedia  []*MessageTemplateMediaInp `json:"verifyMedia" dc:"验证媒体"`
	Status       int                        `json:"status" dc:"状态：1上架 2下架"`
}

// BotProfileEditInp is used by youban_bot to edit text fields and profile_no.
type BotProfileEditInp struct {
	TenantId     int64                      `json:"tenantId" dc:"租户ID"`
	AccountId    int64                      `json:"accountId" dc:"上架账号ID"`
	ProfileNo    string                     `json:"profileNo" dc:"资料编号"`
	NewNo        string                     `json:"newNo" dc:"新资料编号"`
	Title        string                     `json:"title" dc:"标题"`
	PlainText    string                     `json:"plainText" dc:"正文"`
	DisplayMedia []*MessageTemplateMediaInp `json:"displayMedia" dc:"展示媒体"`
	VerifyText   string                     `json:"verifyText" dc:"验证正文"`
	VerifyMedia  []*MessageTemplateMediaInp `json:"verifyMedia" dc:"验证媒体"`
}

// BotProfileQueueCancelInp cancels pending publish jobs for profiles.
type BotProfileQueueCancelInp struct {
	TenantId  int64    `json:"tenantId" dc:"租户ID"`
	AccountId int64    `json:"accountId" dc:"上架账号ID"`
	Nos       []string `json:"nos" dc:"资料编号列表，为空取消当前账号全部待推送"`
}

type BotProfileQueueCancelModel struct {
	Cleared int `json:"cleared" dc:"取消数量"`
}

// BotChannelCycleSaveInp updates channel cycle publish settings.
type BotChannelCycleSaveInp struct {
	TenantId  int64  `json:"tenantId" dc:"租户ID"`
	AccountId int64  `json:"accountId" dc:"账号ID"`
	ChannelId int64  `json:"channelId" dc:"频道ID"`
	Enabled   int    `json:"enabled" dc:"是否启用"`
	Days      int    `json:"days" dc:"循环天数"`
	Time      string `json:"time" dc:"循环时间 HH:mm"`
}

// BotChannelActionInp is used by youban_bot channel operations.
type BotChannelActionInp struct {
	TenantId  int64 `json:"tenantId" dc:"租户ID"`
	AccountId int64 `json:"accountId" dc:"账号ID"`
	ChannelId int64 `json:"channelId" dc:"频道ID"`
}

type TagListInp struct {
	form.PageReq
	Keyword      string `json:"keyword" dc:"关键词"`
	ReviewStatus string `json:"reviewStatus" dc:"审核状态"`
	Status       int    `json:"status" dc:"状态"`
}

type TagModel struct {
	Id                int64       `json:"id" dc:"ID"`
	Name              string      `json:"name" dc:"标签名"`
	ReviewStatus      string      `json:"reviewStatus" dc:"审核状态"`
	Status            int         `json:"status" dc:"状态"`
	UseCount          int         `json:"useCount" dc:"使用数量"`
	CreatedBy         int64       `json:"createdBy" dc:"创建人ID"`
	CreatorUsername   string      `json:"creatorUsername" dc:"创建人账号"`
	CreatorTenantId   int64       `json:"creatorTenantId" dc:"创建人租户ID"`
	CreatorTenantName string      `json:"creatorTenantName" dc:"创建人租户"`
	CreatedAt         *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt         *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type TagSaveInp struct {
	Name         string `json:"name" v:"required#标签名称不能为空" dc:"标签名"`
	ReviewStatus string `json:"reviewStatus" dc:"审核状态"`
	Status       int    `json:"status" dc:"状态"`
}

func (in *TagSaveInp) Filter(ctx context.Context) error {
	in.Name = strings.TrimSpace(in.Name)
	in.ReviewStatus = strings.TrimSpace(in.ReviewStatus)
	if in.Name == "" {
		return gerror.New("标签名称不能为空")
	}
	if in.ReviewStatus == "" {
		in.ReviewStatus = PublishTagReviewPending
	}
	if in.ReviewStatus != PublishTagReviewPending && in.ReviewStatus != PublishTagReviewApproved && in.ReviewStatus != PublishTagReviewRejected {
		return gerror.New("标签审核状态不合法")
	}
	if in.Status == 0 {
		in.Status = 1
	}
	if in.Status != 1 && in.Status != 2 {
		return gerror.New("标签状态不合法")
	}
	return nil
}

type TagDeleteInp struct {
	Ids []int64 `json:"ids" v:"required#请选择要删除的标签" dc:"标签ID列表"`
}

type CityForwardInp struct {
	ParentId int64 `json:"parentId" dc:"父级地区ID，为0返回省份"`
}

type CityForwardModel struct {
	List []*CityOptionModel `json:"list" dc:"省市选项"`
}

type CityOptionModel struct {
	Label    string             `json:"label" dc:"地区名称"`
	Value    int64              `json:"value" dc:"地区ID"`
	Level    int                `json:"level" dc:"地区等级"`
	IsLeaf   bool               `json:"isLeaf" dc:"是否叶子"`
	Children []*CityOptionModel `json:"children,omitempty" dc:"子级"`
}

type TrendInp struct {
	AccountId int64  `json:"accountId" dc:"账号ID，管理员趋势查询可选"`
	Days      int    `json:"days" dc:"趋势天数，默认7，最多90"`
	StartDate string `json:"startDate" dc:"趋势开始日期，格式YYYY-MM-DD"`
	EndDate   string `json:"endDate" dc:"趋势结束日期，格式YYYY-MM-DD"`
}

type TrendPointModel struct {
	Date         string `json:"date" dc:"日期"`
	ProfileCount int    `json:"profileCount" dc:"新增资料数"`
	UpCount      int    `json:"upCount" dc:"上架数"`
	DownCount    int    `json:"downCount" dc:"下架数"`
}

type ProfileStatsModel struct {
	Total     int                `json:"total" dc:"资料总数"`
	UpCount   int                `json:"upCount" dc:"上架数"`
	DownCount int                `json:"downCount" dc:"下架数"`
	Pending   int                `json:"pending" dc:"待审核数"`
	Approved  int                `json:"approved" dc:"审核通过数"`
	Rejected  int                `json:"rejected" dc:"审核拒绝数"`
	Trend     []*TrendPointModel `json:"trend" dc:"趋势"`
}

type BotListInp struct {
	form.PageReq
	TenantId int64  `json:"tenantId" dc:"租户ID，0表示全局"`
	Keyword  string `json:"keyword" dc:"关键词"`
	Status   int    `json:"status" dc:"状态"`
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
	IsPrivate       int         `json:"isPrivate" dc:"是否私聊"`
	IsBroadcast     int         `json:"isBroadcast" dc:"是否频道"`
	IsMegagroup     int         `json:"isMegagroup" dc:"是否群聊"`
	MessageCount    int         `json:"messageCount" dc:"消息数"`
	LastMessageText string      `json:"lastMessageText" dc:"最后消息"`
	LastMessageAt   *gtime.Time `json:"lastMessageAt" dc:"最后消息时间"`
	CreatedAt       *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt" dc:"更新时间"`
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

type BotCreateInp struct {
	BotName     string `json:"botName" v:"required#Bot名称不能为空" dc:"Bot名称"`
	BotUsername string `json:"botUsername" v:"required#Bot用户名不能为空" dc:"Bot用户名"`
	Remark      string `json:"remark" dc:"备注"`
	Status      int    `json:"status" dc:"状态：1启用 2停用"`
	TgAccountId int64  `json:"tgAccountId" v:"required#请选择TG账号" dc:"TG账号ID"`
}

type BotUsernameCheckInp struct {
	BotUsername string `json:"botUsername" v:"required#Bot用户名不能为空" dc:"Bot用户名"`
	TgAccountId int64  `json:"tgAccountId" v:"required#请选择TG账号" dc:"TG账号ID"`
}

type BotUsernameCheckModel struct {
	Available   bool   `json:"available" dc:"是否可用"`
	BotUsername string `json:"botUsername" dc:"Bot用户名"`
	Message     string `json:"message" dc:"提示信息"`
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
	TgAccountId int64  `json:"tgAccountId" dc:"重新登录的TG账号ID"`
	DisplayName string `json:"displayName" dc:"显示名称"`
	Remark      string `json:"remark" dc:"备注"`
}

type TgAccountPhoneStartInp struct {
	TgAccountStartLoginInp
	Phone string `json:"phone" v:"required#手机号不能为空" dc:"国际格式手机号"`
}

func (in *TgAccountPhoneStartInp) Filter(ctx context.Context) error {
	if err := in.TgAccountStartLoginInp.Filter(ctx); err != nil {
		return err
	}
	in.Phone = strings.TrimSpace(in.Phone)
	return nil
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

type TgAccountCodeInp struct {
	LoginToken string `json:"loginToken" v:"required#登录令牌不能为空" dc:"登录令牌"`
	Code       string `json:"code" v:"required#Telegram验证码不能为空" dc:"Telegram验证码"`
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
	TenantId         int64  `json:"tenantId" dc:"租户ID"`
	TgAccountId      int64  `json:"tgAccountId" dc:"TG账号ID"`
	PublishDirection string `json:"publishDirection" dc:"上架/下架频道：up/down"`
	Keyword          string `json:"keyword" dc:"频道名称/Chat ID"`
	Status           int    `json:"status" dc:"状态：1启用 2停用"`
}

type ChannelModel struct {
	Id                      int64       `json:"id" dc:"ID"`
	TenantId                int64       `json:"tenantId" dc:"租户ID"`
	TenantUsername          string      `json:"tenantUsername" dc:"归属租户账号"`
	TgAccountId             int64       `json:"tgAccountId" dc:"TG账号ID"`
	TgAccountName           string      `json:"tgAccountName" dc:"TG账号名称"`
	ChannelTitle            string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername         string      `json:"channelUsername" dc:"频道用户名"`
	TargetChatId            string      `json:"targetChatId" dc:"目标Chat ID"`
	PublishDirection        string      `json:"publishDirection" dc:"上架/下架频道：up/down"`
	CyclePublishEnabled     int         `json:"cyclePublishEnabled" dc:"是否循环上架"`
	CyclePublishDays        int         `json:"cyclePublishDays" dc:"循环时间，生产按天，开发按秒"`
	CyclePublishTime        string      `json:"cyclePublishTime" dc:"循环上架时间"`
	IsDefaultSelected       int         `json:"isDefaultSelected" dc:"是否默认选中"`
	PublishVisible          int         `json:"publishVisible" dc:"上架端资料选择可见：1可见 2隐藏"`
	AntiScanEnabled         int         `json:"antiScanEnabled" dc:"频道防扫图开关"`
	TextObfuscationEnabled  int         `json:"textObfuscationEnabled" dc:"频道文本混淆开关"`
	AutoDeleteEnabled       int         `json:"autoDeleteEnabled" dc:"频道自动删除开关"`
	BotIds                  []int64     `json:"botIds" dc:"绑定Bot ID列表"`
	BotIdJson               string      `json:"botIdJson" dc:"绑定Bot ID JSON"`
	BotPermissionStatusJson string      `json:"botPermissionStatusJson" dc:"Bot权限检测结果JSON"`
	BotPermissionStatus     string      `json:"botPermissionStatus" dc:"Bot权限状态"`
	BotPermissionMessage    string      `json:"botPermissionMessage" dc:"Bot权限提示"`
	Remark                  string      `json:"remark" dc:"备注"`
	Status                  int         `json:"status" dc:"状态"`
	LastRefreshStatus       string      `json:"lastRefreshStatus" dc:"最近刷新状态"`
	LastRefreshMessage      string      `json:"lastRefreshMessage" dc:"最近刷新信息"`
	LastRefreshAt           *gtime.Time `json:"lastRefreshAt" dc:"最近刷新时间"`
	CreatedAt               *gtime.Time `json:"createdAt" dc:"创建时间"`
	UpdatedAt               *gtime.Time `json:"updatedAt" dc:"更新时间"`
}

type ChannelSaveInp struct {
	Id                     int64   `json:"id" dc:"ID"`
	TenantId               int64   `json:"tenantId" dc:"租户ID"`
	TgAccountId            int64   `json:"tgAccountId" dc:"TG账号ID"`
	ChannelTitle           string  `json:"channelTitle" dc:"频道名称"`
	ChannelUsername        string  `json:"channelUsername" dc:"频道用户名"`
	TargetChatId           string  `json:"targetChatId" dc:"目标Chat ID"`
	PublishDirection       string  `json:"publishDirection" dc:"上架/下架频道：up/down"`
	CyclePublishEnabled    int     `json:"cyclePublishEnabled" dc:"是否循环上架"`
	CyclePublishDays       int     `json:"cyclePublishDays" dc:"循环时间，生产按天，开发按秒"`
	CyclePublishTime       string  `json:"cyclePublishTime" dc:"循环上架时间"`
	IsDefaultSelected      int     `json:"isDefaultSelected" dc:"是否默认选中"`
	PublishVisible         int     `json:"publishVisible" dc:"上架端资料选择可见：1可见 2隐藏"`
	AntiScanEnabled        int     `json:"antiScanEnabled" dc:"频道防扫图开关"`
	TextObfuscationEnabled int     `json:"textObfuscationEnabled" dc:"频道文本混淆开关"`
	AutoDeleteEnabled      int     `json:"autoDeleteEnabled" dc:"频道自动删除开关"`
	BotIds                 []int64 `json:"botIds" dc:"绑定Bot ID列表"`
	Remark                 string  `json:"remark" dc:"备注"`
	Status                 int     `json:"status" dc:"状态：1启用 2停用"`
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
	if in.IsDefaultSelected == 0 {
		in.IsDefaultSelected = 1
	}
	if in.IsDefaultSelected != 1 && in.IsDefaultSelected != 2 {
		return gerror.New("默认选中配置不合法")
	}
	if in.PublishVisible == 0 {
		in.PublishVisible = 1
	}
	if in.PublishVisible != 1 && in.PublishVisible != 2 {
		return gerror.New("上架端可见配置不合法")
	}
	if in.CyclePublishEnabled != 0 && in.CyclePublishEnabled != 1 {
		return gerror.New("循环上架开关不合法")
	}
	if in.AntiScanEnabled != 0 && in.AntiScanEnabled != 1 {
		return gerror.New("频道防扫图开关不合法")
	}
	if in.TextObfuscationEnabled != 0 && in.TextObfuscationEnabled != 1 {
		return gerror.New("频道文本混淆开关不合法")
	}
	if in.AutoDeleteEnabled != 0 && in.AutoDeleteEnabled != 1 {
		return gerror.New("频道自动删除开关不合法")
	}
	if in.CyclePublishEnabled == 1 && in.CyclePublishDays <= 0 {
		in.CyclePublishDays = 4
	}
	if in.CyclePublishDays < 0 || in.CyclePublishDays > 365 {
		return gerror.New("循环时间不合法")
	}
	in.CyclePublishTime = strings.TrimSpace(in.CyclePublishTime)
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
	TgAccountId     int64    `json:"tgAccountId" dc:"TG账号ID"`
	Keyword         string   `json:"keyword" dc:"关键词"`
	DisplayType     string   `json:"displayType" dc:"显示类型: channel/group"`
	ManagementRole  string   `json:"managementRole" dc:"当前TG账号角色: owner/admin/member，多个逗号分隔"`
	ManagementRoles []string `json:"managementRoles" dc:"当前TG账号角色列表"`
	CanPostMessages int      `json:"canPostMessages" dc:"筛选账号可发消息：1是"`
	CanInviteUsers  int      `json:"canInviteUsers" dc:"筛选账号可邀请用户：1是"`
	CanAddAdmins    int      `json:"canAddAdmins" dc:"筛选账号可添加管理员：1是"`
}

func (in *ChannelCacheListInp) Filter(ctx context.Context) error {
	_ = ctx
	in.DisplayType = strings.TrimSpace(strings.ToLower(in.DisplayType))
	if in.DisplayType != "" && in.DisplayType != "channel" && in.DisplayType != "group" {
		return gerror.New("显示类型不合法")
	}
	in.ManagementRoles = normalizeChannelCacheRoleInputs(append(in.ManagementRoles, strings.Split(in.ManagementRole, ",")...))
	return nil
}

func normalizeChannelCacheRoleInputs(values []string) []string {
	roles := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		role := strings.TrimSpace(strings.ToLower(value))
		if role == "creator" {
			role = "owner"
		}
		if role != "owner" && role != "admin" && role != "member" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles
}

type ChannelCacheModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	TenantId           int64       `json:"tenantId" dc:"租户ID"`
	TgAccountId        int64       `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId          string      `json:"channelId" dc:"频道ID"`
	AccessHash         string      `json:"accessHash" dc:"AccessHash"`
	ChannelTitle       string      `json:"channelTitle" dc:"频道名称"`
	ChannelUsername    string      `json:"channelUsername" dc:"频道用户名"`
	DisplayType        string      `json:"displayType" dc:"显示类型: channel/group"`
	ManagementRole     string      `json:"managementRole" orm:"management_role" dc:"当前TG账号角色: owner/admin/member"`
	IsBroadcast        int         `json:"isBroadcast" dc:"是否频道"`
	IsMegagroup        int         `json:"isMegagroup" dc:"是否群组"`
	CanPostMessages    int         `json:"canPostMessages" dc:"账号可发频道消息"`
	CanInviteUsers     int         `json:"canInviteUsers" dc:"账号可邀请用户"`
	CanAddAdmins       int         `json:"canAddAdmins" dc:"账号可添加管理员"`
	LastSyncAt         *gtime.Time `json:"lastSyncAt" dc:"最后同步时间"`
	ManagementRoleText string      `json:"managementRoleText" dc:"当前TG账号角色文本"`
}

type ChannelCacheResolveInp struct {
	TgAccountId   int64    `json:"tgAccountId" dc:"TG账号ID"`
	TargetChatIds []string `json:"targetChatIds" dc:"目标群聊或频道Chat ID"`
}

type ChannelCacheResolveModel struct {
	TgAccountId     int64  `json:"tgAccountId" dc:"TG账号ID"`
	ChannelId       string `json:"channelId" dc:"频道ID"`
	ChannelTitle    string `json:"channelTitle" dc:"频道名称"`
	ChannelUsername string `json:"channelUsername" dc:"频道用户名"`
}

func (in *ChannelCacheResolveInp) Filter(ctx context.Context) error {
	_ = ctx
	in.TargetChatIds = uniqueStringInputs(in.TargetChatIds)
	if in.TgAccountId <= 0 {
		return gerror.New("请选择TG账号")
	}
	if len(in.TargetChatIds) == 0 {
		return gerror.New("请选择目标群聊或频道")
	}
	if len(in.TargetChatIds) > 200 {
		return gerror.New("一次最多解析200个目标")
	}
	return nil
}

type ChannelCacheRefreshInp struct {
	TgAccountId int64 `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号ID"`
}

type ChannelCacheRefreshModel struct {
	Count        int    `json:"count" dc:"同步数量"`
	Message      string `json:"message" dc:"同步结果"`
	SyncedAt     string `json:"syncedAt" dc:"同步时间"`
	TgAccountId  int64  `json:"tgAccountId" dc:"TG账号ID"`
	TaskId       int64  `json:"taskId" dc:"异步任务ID"`
	Status       string `json:"status" dc:"任务状态"`
	ErrorMessage string `json:"errorMessage" dc:"失败原因"`
}

type ChannelCacheRefreshStatusInp struct {
	TaskId int64 `json:"taskId" v:"required|min:1#刷新任务不能为空|刷新任务无效" dc:"异步任务ID"`
}

type ChannelCheckInp struct {
	ChannelId    int64   `json:"channelId" dc:"已有频道配置ID"`
	TgAccountId  int64   `json:"tgAccountId" v:"required|min:1#请选择TG账号|请选择TG账号" dc:"TG账号ID"`
	TargetChatId string  `json:"targetChatId" v:"required#请选择频道" dc:"频道ID"`
	BotIds       []int64 `json:"botIds" dc:"Bot ID列表"`
}

type ChannelCheckBotModel struct {
	BotId             int64  `json:"botId" dc:"Bot ID"`
	BotName           string `json:"botName" dc:"Bot名称"`
	BotUsername       string `json:"botUsername" dc:"Bot用户名"`
	CanSendMessage    int    `json:"canSendMessage" dc:"是否可发送消息"`
	CanDeleteMessages int    `json:"canDeleteMessages" dc:"是否可删除消息"`
	InChannel         int    `json:"inChannel" dc:"是否在频道中"`
	Status            string `json:"status" dc:"检测状态"`
	Message           string `json:"message" dc:"检测信息"`
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

type ChannelFullPushInp struct {
	ChannelId int64 `json:"channelId" v:"required|min:1#请选择频道|请选择频道" dc:"频道ID"`
}

type ChannelFullPushModel struct {
	ChannelId     int64  `json:"channelId" dc:"频道ID"`
	Queued        int    `json:"queued" dc:"本次预计入队数量"`
	ExistingQueue int    `json:"existingQueue" dc:"触发前频道未完成队列数量"`
	BatchNo       string `json:"batchNo" dc:"全量推送批次号"`
	Status        string `json:"status" dc:"批次状态"`
}

type ChannelCycleRunInp struct {
	ChannelId int64 `json:"channelId" v:"required|min:1#请选择频道|请选择频道" dc:"频道ID"`
}

type ChannelClearQueueInp struct {
	ChannelId int64 `json:"channelId" v:"required|min:1#请选择频道|请选择频道" dc:"频道ID"`
}

type ChannelClearQueueModel struct {
	ChannelId int64 `json:"channelId" dc:"频道ID"`
	Cleared   int   `json:"cleared" dc:"清空数量"`
	Sending   int   `json:"sending" dc:"发送中数量"`
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
