package dao

import "hotgo/internal/dao/internal"

type memberSettingDao struct {
	*internal.MemberSettingDao
}

var MemberSetting = memberSettingDao{internal.NewMemberSettingDao()}
