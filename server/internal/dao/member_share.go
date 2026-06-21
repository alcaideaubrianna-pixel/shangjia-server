package dao

import "hotgo/internal/dao/internal"

type memberShareDao struct {
	*internal.MemberShareDao
}

var MemberShare = memberShareDao{internal.NewMemberShareDao()}
