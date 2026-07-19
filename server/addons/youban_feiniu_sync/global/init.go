package global

import (
	"context"
	"hotgo/internal/library/addons"
)

func Init(ctx context.Context, s *addons.Skeleton) { skeleton = s }
