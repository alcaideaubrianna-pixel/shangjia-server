package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/library/cache"
)

const (
	collectRuleSourceIDField      = "_collect_rule_source_id"
	collectSourceRouteCachePrefix = "youban_publish:collect:source_route"
	collectSourceRouteCacheTTL    = 10 * time.Minute
)

// collectSourceContext separates the delivery entry point from the source
// whose channel-specific rule owns the event.
type collectSourceContext struct {
	TenantID       int64
	AccountID      int64
	IngestSourceID int64
	RuleSourceID   int64
	SourceChatID   string
}

func (s *sSysPublish) resolveCollectSourceContext(ctx context.Context, event gdb.Record, tenantID, accountID int64) (*collectSourceContext, error) {
	resolved := &collectSourceContext{
		TenantID:       tenantID,
		AccountID:      accountID,
		IngestSourceID: event["source_id"].Int64(),
		RuleSourceID:   event["source_id"].Int64(),
		SourceChatID:   canonicalCollectSourceChatID(event["source_chat_id"].String()),
	}
	if resolved.IngestSourceID <= 0 || resolved.SourceChatID == "" || tenantID <= 0 || accountID <= 0 {
		return resolved, nil
	}

	version := s.collectSourceCacheVersion(ctx)
	cacheKey := fmt.Sprintf("%s:%s:%d:%d:%s", collectSourceRouteCachePrefix, version, tenantID, accountID, resolved.SourceChatID)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() && value.Int64() > 0 {
		resolved.RuleSourceID = value.Int64()
		return resolved, nil
	}

	lookupIDs := tgChannelCacheLookupIds(event["source_chat_id"].String())
	if len(lookupIDs) == 0 {
		return resolved, nil
	}
	rows, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantID).
		Where("account_id", accountID).
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereIn("source_chat_id", lookupIDs).
		WhereNull("deleted_at").
		Where("EXISTS (SELECT 1 FROM "+pdao.YoubanPublishCollectSourceRule.Table()+" sr JOIN "+pdao.YoubanPublishCollectRule.Table()+" r ON r.id=sr.rule_id WHERE sr.source_id="+pdao.YoubanPublishCollectSource.Table()+".id AND sr.status=1 AND r.tenant_id=? AND r.account_id=? AND r.status=1 AND r.deleted_at IS NULL)", tenantID, accountID).
		OrderAsc("id").
		Limit(2).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道采集源规则路由失败")
	}
	if len(rows) > 1 {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row["id"].String())
		}
		return nil, gerror.Newf("频道%s存在多个启用的采集源（%s），请停用重复配置", resolved.SourceChatID, strings.Join(ids, ","))
	}
	if len(rows) == 1 && rows[0]["id"].Int64() > 0 {
		resolved.RuleSourceID = rows[0]["id"].Int64()
		_ = cache.Instance().Set(ctx, cacheKey, resolved.RuleSourceID, collectSourceRouteCacheTTL)
	}
	return resolved, nil
}

func collectRuleSourceID(event gdb.Record, rule gdb.Record) int64 {
	if !rule.IsEmpty() && rule[collectRuleSourceIDField].Int64() > 0 {
		return rule[collectRuleSourceIDField].Int64()
	}
	if event.IsEmpty() {
		return 0
	}
	return event["source_id"].Int64()
}

func ensureUniqueActiveCollectChannelSource(ctx context.Context, tx gdb.TX, tenantID, accountID, sourceID int64, sourceChatID string, collectEnabled, status int) error {
	if collectEnabled != 1 || status != 1 {
		return nil
	}
	lookupIDs := tgChannelCacheLookupIds(sourceChatID)
	if len(lookupIDs) == 0 {
		return nil
	}
	model := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantID).
		Where("account_id", accountID).
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereIn("source_chat_id", lookupIDs).
		WhereNull("deleted_at")
	if sourceID > 0 {
		model = model.WhereNot("id", sourceID)
	}
	existing, err := model.Limit(1).One()
	if err != nil {
		return gerror.Wrap(err, "检查重复频道采集源失败")
	}
	if !existing.IsEmpty() {
		return gerror.Newf("该频道已有启用的采集源%d，请直接编辑原配置", existing["id"].Int64())
	}
	return nil
}
