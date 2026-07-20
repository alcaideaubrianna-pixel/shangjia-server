package global

import "hotgo/internal/library/addons"

var skeleton *addons.Skeleton

func Init(s *addons.Skeleton) {
	skeleton = s
}

func GetSkeleton() *addons.Skeleton {
	return skeleton
}
