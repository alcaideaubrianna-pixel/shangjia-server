package global

import "hotgo/internal/library/addons"

var skeleton *addons.Skeleton

func GetSkeleton() *addons.Skeleton {
	return skeleton
}
