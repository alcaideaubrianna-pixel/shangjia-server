package sysin

import "github.com/gogf/gf/v2/os/gtime"

type ProfileInteractionInp struct {
	EventId    string
	ActorId    string
	ProfileId  int64
	Type       string
	OccurredAt int64
}

const (
	CmsBindingPending  = "pending"
	CmsBindingApproved = "approved"
	CmsBindingRevoked  = "revoked"
	CmsBindingBlocked  = "blocked"
	CmsReviewRequired  = "review_required"
	CmsReviewAutomatic = "auto_approve"
)

type CmsAppModel struct {
	Id              int64       `json:"id"`
	AppId           string      `json:"appId"`
	AppSecret       string      `json:"-" orm:"app_secret"`
	Name            string      `json:"name"`
	BaseUrl         string      `json:"baseUrl"`
	InstanceId      string      `json:"instanceId"`
	SourceIp        string      `json:"sourceIp"`
	CmsVersion      string      `json:"cmsVersion"`
	LastHeartbeatAt *gtime.Time `json:"lastHeartbeatAt"`
	ReviewMode      string      `json:"reviewMode"`
	Status          int         `json:"status"`
	CreatedAt       *gtime.Time `json:"createdAt"`
	UpdatedAt       *gtime.Time `json:"updatedAt"`
}

type CmsInstanceRegisterInp struct {
	InstanceId  string `json:"instanceId" v:"required|length:16,128#实例ID不能为空|实例ID长度应为16到128位"`
	Name        string `json:"name" v:"required|length:2,128#实例名称不能为空|实例名称长度应为2到128位"`
	BaseUrl     string `json:"baseUrl"`
	Version     string `json:"version"`
	EnrollToken string `json:"enrollToken"`
}

type CmsInstanceRegisterModel struct {
	InstanceId  string `json:"instanceId"`
	Status      string `json:"status"`
	EnrollToken string `json:"enrollToken,omitempty"`
	AppId       string `json:"appId,omitempty"`
	AppSecret   string `json:"appSecret,omitempty"`
}

type CmsInstanceHeartbeatInp struct {
	InstanceId  string `json:"instanceId" v:"required#实例ID不能为空"`
	EnrollToken string `json:"enrollToken" v:"required#实例令牌不能为空"`
	BaseUrl     string `json:"baseUrl"`
	Version     string `json:"version"`
}

type CmsBindingModel struct {
	Id          int64       `json:"id"`
	AppId       string      `json:"appId"`
	AppName     string      `json:"appName,omitempty"`
	TenantId    int64       `json:"tenantId"`
	TenantName  string      `json:"tenantName,omitempty"`
	CodeVersion int         `json:"codeVersion"`
	Status      string      `json:"status"`
	Reason      string      `json:"reason"`
	RequestedAt *gtime.Time `json:"requestedAt"`
	ReviewedAt  *gtime.Time `json:"reviewedAt"`
	CreatedAt   *gtime.Time `json:"createdAt"`
	UpdatedAt   *gtime.Time `json:"updatedAt"`
}

type CmsBindingCodeSaveInp struct {
	Code string `json:"code" v:"required|length:8,64#请输入绑定码|绑定码长度应为8到64位"`
}

type CmsBindingClaimInp struct {
	Code string `json:"code" v:"required|length:8,64#请输入绑定码|绑定码长度应为8到64位"`
}

type CmsBindingRevokeInp struct {
	Id int64 `json:"id" v:"required|min:1#绑定记录不能为空"`
}

type CmsAppSettingsInp struct {
	ReviewMode string `json:"reviewMode" v:"required|in:review_required,auto_approve#请选择审核策略|审核策略无效"`
}

type CmsAppSettingsModel struct {
	ReviewMode string `json:"reviewMode"`
}

type CmsBindingListInp struct {
	Status string `json:"status"`
}

type CmsBindingStatusInp struct {
	Id     int64  `json:"id" v:"required|min:1#绑定记录不能为空"`
	Status string `json:"status" v:"required|in:approved,revoked,blocked#请选择绑定状态|绑定状态无效"`
	Reason string `json:"reason"`
}

type CmsBindingCodeModel struct {
	Version int    `json:"version"`
	Hint    string `json:"hint"`
}
type CmsBindingLookupModel struct {
	AppId   string `json:"appId"`
	AppName string `json:"appName"`
}

type CmsAppListInp struct {
	Name   string `json:"name"`
	Status int    `json:"status"`
}

type CmsAppSaveInp struct {
	Id      int64  `json:"id"`
	Name    string `json:"name" v:"required|length:2,128#请输入应用名称|应用名称长度应为2到128位"`
	BaseUrl string `json:"baseUrl"`
	Status  int    `json:"status" v:"required|in:1,2,3,4#请选择状态|状态无效"`
}

type CmsAppCredentialModel struct {
	*CmsAppModel
	AppSecret string `json:"appSecret"`
}

type CmsAppResetSecretInp struct {
	Id int64 `json:"id" v:"required|min:1#请选择CMS应用"`
}
