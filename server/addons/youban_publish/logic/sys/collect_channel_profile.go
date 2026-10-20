package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const publishCollectChannelProfileTable = "hg_youban_publish_collect_channel_profile"

func saveCollectMaterialOriginTx(ctx context.Context, tx gdb.TX, profileID int64, event, rule gdb.Record) error {
	if err := saveCollectProfileOriginTx(ctx, tx, profileID, event); err != nil {
		return err
	}
	return indexCollectChannelProfileModel(tx.Model(publishCollectChannelProfileTable).Ctx(ctx), event["tenant_id"].Int64(), event["account_id"].Int64(), collectRuleSourceID(event, rule), rule["id"].Int64(), profileID, event["source_chat_id"].String())
}

// Keep rule ownership for the lifetime of a profile, independently of configuration.
func indexCollectChannelProfile(ctx context.Context, tenantID, accountID, sourceID, ruleID, profileID int64, sourceChatID string) error {
	return indexCollectChannelProfileModel(g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx), tenantID, accountID, sourceID, ruleID, profileID, sourceChatID)
}

func indexCollectChannelProfileModel(model *gdb.Model, tenantID, accountID, sourceID, ruleID, profileID int64, sourceChatID string) error {
	channelID := canonicalCollectSourceChatID(sourceChatID)
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 || profileID <= 0 || channelID == "" {
		return nil
	}
	_, err := model.Data(g.Map{"tenant_id": tenantID, "account_id": accountID, "source_id": sourceID, "rule_id": ruleID, "channel_id": channelID, "profile_id": profileID, "deleted_at": nil}).
		OnConflict("tenant_id,account_id,rule_id,channel_id,profile_id").OnDuplicate("source_id,updated_at,deleted_at").Save()
	return gerror.Wrap(err, "保存采集频道资料归属失败")
}

// Preview, deletion and page counts share precisely the same source evidence.
// Channel IDs are bound values, never publishing channel primary keys.
func collectChannelProfileEvidenceSQL(ctx context.Context, tenantID, accountID, ruleID int64, channelIDs []string) (string, []interface{}, error) {
	if tenantID <= 0 || accountID <= 0 || len(channelIDs) == 0 {
		return "", nil, gerror.New("采集频道资料查询范围不完整")
	}
	lookups := []string{}
	for _, id := range channelIDs {
		if id = canonicalCollectSourceChatID(id); id != "" {
			lookups = append(lookups, tgChannelCacheLookupIds(id)...)
		}
	}
	lookups = uniqueStrings(lookups)
	if len(lookups) == 0 {
		return "", nil, gerror.New("采集频道不能为空")
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(lookups)), ",")
	args := []interface{}{}
	parts := []string{}
	add := func(query, alias, chat string, hasRule bool) {
		query += " WHERE " + alias + ".tenant_id=? AND " + alias + ".account_id=? AND " + chat + " IN (" + marks + ") AND " + alias + ".profile_id>0"
		args = append(args, tenantID, accountID)
		for _, id := range lookups {
			args = append(args, id)
		}
		if ruleID > 0 {
			if hasRule {
				query += " AND " + alias + ".rule_id=?"
			} else {
				// Origins/jobs have no rule ID: require the exact event dispatch or the
				// retained profile + source chat + rule ownership, not merely any dispatch.
				eventField := alias + ".event_id"
				if alias == "j" {
					eventField = "j.collect_event_id"
				}
				query += " AND (EXISTS (SELECT 1 FROM " + publishCollectDispatchTable + " d WHERE d.tenant_id=" + alias + ".tenant_id AND d.account_id=" + alias + ".account_id AND d.profile_id=" + alias + ".profile_id AND d.event_id=" + eventField + " AND d.rule_id=?) OR EXISTS (SELECT 1 FROM " + publishCollectChannelProfileTable + " c WHERE c.tenant_id=" + alias + ".tenant_id AND c.account_id=" + alias + ".account_id AND c.profile_id=" + alias + ".profile_id AND c.rule_id=? AND c.deleted_at IS NULL AND (c.channel_id=" + chat + " OR c.channel_id=CASE WHEN " + chat + " LIKE '-100%' THEN SUBSTRING(" + chat + ",5) ELSE CONCAT('-100'," + chat + ") END)))"
				args = append(args, ruleID)
			}
			args = append(args, ruleID)
		}
		parts = append(parts, query)
	}
	add("SELECT c.channel_id AS chat_id,c.profile_id FROM "+publishCollectChannelProfileTable+" c", "c", "c.channel_id", true)
	parts[len(parts)-1] += " AND c.deleted_at IS NULL"
	add("SELECT COALESCE(NULLIF(d.source_chat_id_snapshot,''),e.source_chat_id) AS chat_id,d.profile_id FROM "+publishCollectDispatchTable+" d LEFT JOIN "+publishCollectEventTable+" e ON e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id", "d", "COALESCE(NULLIF(d.source_chat_id_snapshot,''),e.source_chat_id)", true)
	add("SELECT j.collect_source_chat_id AS chat_id,j.profile_id FROM "+publishTgJobTable+" j", "j", "j.collect_source_chat_id", false)
	enabled, err := profileOriginEnabled(ctx)
	if err != nil {
		return "", nil, err
	}
	if enabled {
		add("SELECT o.chat_id,o.profile_id FROM hg_youban_publish_profile_origin o", "o", "o.chat_id", false)
		parts[len(parts)-1] += " AND o.origin_kind='collect'"
	}
	query := "SELECT evidence.chat_id,evidence.profile_id FROM (" + strings.Join(parts, " UNION ") + ") evidence WHERE EXISTS (SELECT 1 FROM hg_content_profile p JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id WHERE p.id=evidence.profile_id AND p.deleted_at IS NULL AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)"
	args = append(args, tenantID, accountID)
	return query, args, nil
}

func collectChannelProfileIDs(ctx context.Context, tenantID, accountID, ruleID int64, channelID string) ([]int64, error) {
	query, args, err := collectChannelProfileEvidenceSQL(ctx, tenantID, accountID, ruleID, []string{channelID})
	if err != nil {
		return nil, err
	}
	rows, err := g.DB().GetAll(ctx, "SELECT DISTINCT profile_id FROM ("+query+") evidence", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集频道历史资料失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["profile_id"].Int64())
	}
	return uniqueIds(ids), nil
}

func collectChannelProfileCounts(ctx context.Context, tenantID, accountID, ruleID int64, channelIDs []string) (map[string]int, error) {
	// Cancel expensive historical reads instead of leaving dashboard queries
	// running indefinitely on the shared production database.
	if gdb.TXFromCtx(ctx, g.DB().GetGroup()) == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
	}
	counts := map[string]int{}
	if len(channelIDs) == 0 {
		return counts, nil
	}
	query, args, err := collectChannelProfileEvidenceSQL(ctx, tenantID, accountID, ruleID, channelIDs)
	if err != nil {
		return nil, err
	}
	// A single aggregate query for at most the current page; no per-channel scans.
	rows, err := g.DB().GetAll(ctx, "SELECT CASE WHEN chat_id LIKE '-100%' THEN SUBSTRING(chat_id,5) ELSE chat_id END AS channel_id,COUNT(DISTINCT profile_id) AS total FROM ("+query+") evidence GROUP BY CASE WHEN chat_id LIKE '-100%' THEN SUBSTRING(chat_id,5) ELSE chat_id END", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "统计采集频道历史资料失败")
	}
	for _, row := range rows {
		counts[canonicalCollectSourceChatID(row["channel_id"].String())] = row["total"].Int()
	}
	return counts, nil
}

func normalizeChannelProfileID(value string) string {
	return strings.TrimSpace(canonicalCollectSourceChatID(value))
}
