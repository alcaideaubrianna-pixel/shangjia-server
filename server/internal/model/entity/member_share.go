package entity

import "github.com/gogf/gf/v2/os/gtime"

type MemberShare struct {
	Id            int64       `json:"id"            orm:"id"             description:"ID"`
	MemberId      int64       `json:"memberId"      orm:"member_id"      description:"分享用户ID"`
	ProfileId     int64       `json:"profileId"     orm:"profile_id"     description:"资料ID"`
	ShareToken    string      `json:"shareToken"    orm:"share_token"    description:"分享TOKEN"`
	VisitCount    int         `json:"visitCount"    orm:"visit_count"    description:"访问次数"`
	RegisterCount int         `json:"registerCount" orm:"register_count" description:"注册次数"`
	LastVisitAt   *gtime.Time `json:"lastVisitAt"   orm:"last_visit_at"  description:"最后访问时间"`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:"创建时间"`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:"更新时间"`
	DeletedAt     *gtime.Time `json:"deletedAt"     orm:"deleted_at"     description:"删除时间"`
}
