package global

import (
	"context"

	"hotgo/internal/library/addons"
)

func Init(valueCtx context.Context, valueSkeleton *addons.Skeleton) {
	ctx = valueCtx
	skeleton = valueSkeleton
}
