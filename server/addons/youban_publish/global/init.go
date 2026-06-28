package global

import (
	"context"

	"hotgo/internal/library/addons"
)

func Init(ctx context.Context, sk *addons.Skeleton) {
	skeleton = sk
}
