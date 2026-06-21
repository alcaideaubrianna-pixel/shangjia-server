package entity

import "github.com/gogf/gf/v2/os/gtime"

type MemberVipLog struct {
	Id              int64       `json:"id" orm:"id" description:"ID"`
	MemberId        int64       `json:"memberId" orm:"member_id" description:"用户ID"`
	OperatorId      int64       `json:"operatorId" orm:"operator_id" description:"操作人ID"`
	Source          string      `json:"source" orm:"source" description:"来源"`
	Action          string      `json:"action" orm:"action" description:"动作"`
	BeforeStatus    int         `json:"beforeStatus" orm:"before_status" description:"变更前状态"`
	AfterStatus     int         `json:"afterStatus" orm:"after_status" description:"变更后状态"`
	BeforeLevel     int         `json:"beforeLevel" orm:"before_level" description:"变更前等级"`
	AfterLevel      int         `json:"afterLevel" orm:"after_level" description:"变更后等级"`
	BeforeExpiredAt *gtime.Time `json:"beforeExpiredAt" orm:"before_expired_at" description:"变更前到期时间"`
	AfterExpiredAt  *gtime.Time `json:"afterExpiredAt" orm:"after_expired_at" description:"变更后到期时间"`
	Remark          string      `json:"remark" orm:"remark" description:"备注"`
	CreatedAt       *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
}
