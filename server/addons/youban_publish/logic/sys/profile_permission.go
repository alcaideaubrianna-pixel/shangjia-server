package sys

import "hotgo/addons/youban_publish/model/input/sysin"

func markProfilesPermission(list []*sysin.ProfileModel, permission string) {
	for _, item := range list {
		markProfilePermission(item, permission)
	}
}

func markProfilePermission(item *sysin.ProfileModel, permission string) {
	if item == nil {
		return
	}
	if permission == "" {
		permission = sysin.ProfilePermissionVisitor
	}
	item.CanViewDetail = permission == sysin.ProfilePermissionCreator || permission == sysin.ProfilePermissionAdmin || permission == sysin.ProfilePermissionShared
	item.Permission = permission
	item.CanEdit = permission == sysin.ProfilePermissionCreator || permission == sysin.ProfilePermissionAdmin
	// Customer remarks are visible only to the profile creator and the tenant
	// administrator. Keep the field out of shared/visitor responses even when
	// the base SQL selected it.
	if permission != sysin.ProfilePermissionCreator && permission != sysin.ProfilePermissionAdmin {
		item.CustomerRemark = ""
	}
}

func sanitizeProfileMediaOriginals(list []*sysin.MediaModel, canEdit bool) {
	if canEdit {
		return
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.OriginalAttachmentId = 0
		item.OriginalFileUrl = ""
		item.OriginalStoragePath = ""
	}
}

func markNotesPermission(list []*sysin.NoteModel, permission string) {
	for _, item := range list {
		if item == nil {
			continue
		}
		markProfilePermission(&item.ProfileModel, permission)
	}
}
