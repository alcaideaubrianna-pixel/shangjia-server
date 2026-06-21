package dao

import "hotgo/internal/dao/internal"

type memberProfileActionDao struct {
	*internal.MemberProfileActionDao
}

var MemberProfileAction = memberProfileActionDao{internal.NewMemberProfileActionDao()}
