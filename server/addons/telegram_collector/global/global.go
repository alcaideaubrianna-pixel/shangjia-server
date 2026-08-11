package global

import (
	"context"

	"hotgo/internal/library/addons"
)

const AddonName = "telegram_collector"

var skeleton *addons.Skeleton
var ctx context.Context

func GetContext() context.Context { return ctx }

func GetSkeleton() *addons.Skeleton { return skeleton }

func GetAddonName() string {
	if skeleton != nil && skeleton.Name != "" {
		return skeleton.Name
	}
	return AddonName
}
