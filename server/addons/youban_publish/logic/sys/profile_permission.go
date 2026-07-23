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
	item.Permission = permission
	item.CanEdit = permission == sysin.ProfilePermissionCreator || permission == sysin.ProfilePermissionAdmin
}

func markNotesPermission(list []*sysin.NoteModel, permission string) {
	for _, item := range list {
		if item == nil {
			continue
		}
		markProfilePermission(&item.ProfileModel, permission)
	}
}
