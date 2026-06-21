package dao

import "hotgo/internal/dao/internal"

type appAnnouncementDao struct {
	*internal.AppAnnouncementDao
}

var AppAnnouncement = appAnnouncementDao{internal.NewAppAnnouncementDao()}
