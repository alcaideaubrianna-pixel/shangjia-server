package dao

import "hotgo/internal/dao/internal"

type memberProfileViewDao struct {
	*internal.MemberProfileViewDao
}

var MemberProfileView = memberProfileViewDao{internal.NewMemberProfileViewDao()}
