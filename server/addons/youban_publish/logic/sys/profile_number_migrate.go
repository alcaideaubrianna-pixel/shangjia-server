package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/library/cache"
)

const legacyProfileNoMigrationCacheKey = "youban_publish:profile_number:legacy_migrated"

func (s *sSysPublish) ensureLegacyProfileNosOnce(ctx context.Context) error {
	if value, err := cache.Instance().Get(ctx, legacyProfileNoMigrationCacheKey); err == nil && !value.IsNil() && value.String() == "1" {
		return nil
	}
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return s.ensureLegacyProfileNos(ctx, tx)
	}); err != nil {
		return err
	}
	return cache.Instance().Set(ctx, legacyProfileNoMigrationCacheKey, "1", 24*time.Hour)
}
