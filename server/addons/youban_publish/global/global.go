package global

import "hotgo/internal/library/addons"

const AddonName = "youban_publish"

var skeleton *addons.Skeleton

func GetSkeleton() *addons.Skeleton {
	return skeleton
}

func GetAddonName() string {
	if skeleton != nil && skeleton.Name != "" {
		return skeleton.Name
	}
	return AddonName
}
