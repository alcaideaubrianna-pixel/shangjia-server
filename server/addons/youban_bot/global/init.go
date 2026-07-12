package global

import (
	"context"
	"hotgo/internal/library/addons"
)

func Init(ctx context.Context, sk *addons.Skeleton) {
	_ = ctx
	skeleton = sk
}
