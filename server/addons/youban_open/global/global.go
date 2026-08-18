package global

import "hotgo/internal/library/addons"

var skeleton *addons.Skeleton

func Init(value *addons.Skeleton) { skeleton = value }

func GetSkeleton() *addons.Skeleton { return skeleton }
