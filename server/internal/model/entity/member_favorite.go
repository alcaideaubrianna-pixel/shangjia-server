package entity

import "github.com/gogf/gf/v2/os/gtime"

type MemberFavorite struct {
	Id        int64       `json:"id"        orm:"id"         description:"ID"`
	MemberId  int64       `json:"memberId"  orm:"member_id"  description:"用户ID"`
	ProfileId int64       `json:"profileId" orm:"profile_id" description:"资料ID"`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:"更新时间"`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:"删除时间"`
}
