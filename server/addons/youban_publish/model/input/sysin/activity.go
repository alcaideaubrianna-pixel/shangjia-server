package sysin

import (
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/os/gtime"
)

type ActivityModel struct {
	Code            string      `json:"code" dc:"活动编码"`
	Name            string      `json:"name" dc:"活动名称"`
	Description     string      `json:"description" dc:"活动说明"`
	Enabled         bool        `json:"enabled" dc:"是否开启"`
	RewardDays      int         `json:"rewardDays" dc:"奖励天数"`
	EnabledAt       string      `json:"enabledAt" dc:"启用时间"`
	RewardCount     int         `json:"rewardCount" dc:"奖励次数"`
	RewardDaysTotal int         `json:"rewardDaysTotal" dc:"累计奖励天数"`
	LastRewardAt    *gtime.Time `json:"lastRewardAt" dc:"最后奖励时间"`
}

type ActivitySaveInp struct {
	Code       string `json:"code" v:"required#活动编码不能为空" dc:"活动编码"`
	Enabled    bool   `json:"enabled" dc:"是否开启"`
	RewardDays int    `json:"rewardDays" v:"min:1#奖励天数不能小于1" dc:"奖励天数"`
}

type ActivityRewardListInp struct {
	form.PageReq
	ActivityCode string `json:"activityCode" dc:"活动编码"`
	TenantId     int64  `json:"tenantId" dc:"奖励账号归属"`
	NotifyStatus string `json:"notifyStatus" dc:"通知状态"`
	Keyword      string `json:"keyword" dc:"账号归属或账号关键词"`
}

type ActivityRewardModel struct {
	Id                 int64       `json:"id" dc:"ID"`
	ActivityCode       string      `json:"activityCode" dc:"活动编码"`
	ActivityName       string      `json:"activityName" dc:"活动名称"`
	ActivityGeneration int         `json:"activityGeneration" dc:"活动代次"`
	TenantId           int64       `json:"tenantId" dc:"奖励账号归属"`
	TenantName         string      `json:"tenantName" dc:"账号归属名称"`
	AccountId          int64       `json:"accountId" dc:"奖励账号"`
	AccountUsername    string      `json:"accountUsername" dc:"奖励账号名称"`
	TriggerTenantId    int64       `json:"triggerTenantId" dc:"触发账号归属"`
	TriggerAccountId   int64       `json:"triggerAccountId" dc:"触发账号"`
	ChangeDays         int         `json:"changeDays" dc:"奖励天数"`
	AfterExpiredAt     *gtime.Time `json:"afterExpiredAt" dc:"奖励后到期时间"`
	NotifyStatus       string      `json:"notifyStatus" dc:"通知状态"`
	NotifyRetryCount   int         `json:"notifyRetryCount" dc:"通知重试次数"`
	ErrorMessage       string      `json:"errorMessage" dc:"错误信息"`
	Remark             string      `json:"remark" dc:"备注"`
	CreatedAt          *gtime.Time `json:"createdAt" dc:"奖励时间"`
}

type ActivityUserStatusInp struct {
	TenantId int64 `json:"tenantId" v:"required#请选择账号归属" dc:"账号归属"`
}

type ActivityUserStatusModel struct {
	Code          string      `json:"code" dc:"活动编码"`
	Name          string      `json:"name" dc:"活动名称"`
	Enabled       bool        `json:"enabled" dc:"是否开启"`
	Generation    int         `json:"generation" dc:"当前代次"`
	Status        string      `json:"status" dc:"用户活动状态"`
	Reason        string      `json:"reason" dc:"状态说明"`
	EligibleCount int         `json:"eligibleCount" dc:"满足条件数量"`
	RewardCount   int         `json:"rewardCount" dc:"当前代次奖励次数"`
	RewardDays    int         `json:"rewardDays" dc:"当前代次奖励天数"`
	LastRewardAt  *gtime.Time `json:"lastRewardAt" dc:"最后奖励时间"`
}

type ActivityDebugInp struct {
	ActivityUserStatusInp
	Code   string `json:"code" v:"required#活动编码不能为空" dc:"活动编码"`
	Action string `json:"action" v:"in:evaluate,retry#调试动作不合法" dc:"动作：evaluate/retry"`
}

type ActivityResetInp struct {
	ActivityUserStatusInp
	Code   string `json:"code" v:"required#活动编码不能为空" dc:"活动编码"`
	Reason string `json:"reason" v:"required#请填写重置原因" dc:"重置原因"`
}
