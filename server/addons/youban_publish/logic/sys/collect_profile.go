package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
)

const collectProfileSourceType = "youban_collect"

func (s *sSysPublish) upsertCollectProfile(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return 0, err
	}
	return s.commitCollectPreparedProfile(ctx, event, prepared, rule, text)
}
