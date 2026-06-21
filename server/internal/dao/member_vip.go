package dao

import "hotgo/internal/dao/internal"

type memberVipDao struct {
	*internal.MemberVipDao
}

var MemberVip = memberVipDao{internal.NewMemberVipDao()}
