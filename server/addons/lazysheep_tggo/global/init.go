// Package global
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package global

import (
	"context"
	"hotgo/internal/library/addons"
)

func Init(ctx context.Context, sk *addons.Skeleton) {
	skeleton = sk
}

func GetSkeleton() *addons.Skeleton {
	if skeleton == nil {
		panic("addon skeleton not initialized")
	}
	return skeleton
}
