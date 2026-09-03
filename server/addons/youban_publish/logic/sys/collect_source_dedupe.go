package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysPublish) clearCollectSourceDedupe(ctx context.Context, sourceId, tenantId, accountId int64) error {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return gerror.New("清理采集源去重数据参数不完整")
	}

	cacheKeys, err := releaseCollectSourceDedupeLedger(ctx, tenantId, accountId, sourceId)
	if err != nil {
		return err
	}
	if err = clearCollectDedupeCacheKeys(ctx, tenantId, accountId, sourceId, cacheKeys); err != nil {
		return gerror.Wrap(err, "清理采集源去重缓存失败")
	}
	return nil
}
