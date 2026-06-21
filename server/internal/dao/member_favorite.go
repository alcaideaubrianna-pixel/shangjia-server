package dao

import "hotgo/internal/dao/internal"

type memberFavoriteDao struct {
	*internal.MemberFavoriteDao
}

var MemberFavorite = memberFavoriteDao{internal.NewMemberFavoriteDao()}
