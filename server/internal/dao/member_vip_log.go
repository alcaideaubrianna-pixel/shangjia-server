package dao

import "hotgo/internal/dao/internal"

type memberVipLogDao struct {
	*internal.MemberVipLogDao
}

var MemberVipLog = memberVipLogDao{internal.NewMemberVipLogDao()}
