package entity

import "github.com/gogf/gf/v2/os/gtime"

type MemberVip struct {
	Id        int64       `json:"id"        orm:"id"         description:"ID"`
	MemberId  int64       `json:"memberId"  orm:"member_id"  description:"用户ID"`
	Level     int         `json:"level"     orm:"level"      description:"会员等级"`
	Status    int         `json:"status"    orm:"status"     description:"状态"`
	OpenedAt  *gtime.Time `json:"openedAt"  orm:"opened_at"  description:"开通时间"`
	ExpiredAt *gtime.Time `json:"expiredAt" orm:"expired_at" description:"到期时间"`
	Remark    string      `json:"remark"    orm:"remark"     description:"备注"`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:"更新时间"`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:"删除时间"`
}
