package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func (s *sSysPublish) collectManageChannelSuffixes(ctx context.Context, tenantID int64, rows gdb.Result, ruleItems map[int64]*collectRuleItems) ([]string, error) {
	suffixes := make([]string, len(rows))
	if !strings.EqualFold(g.DB().GetConfig().Type, consts.DBPgsql) {
		for index, row := range rows {
			ruleID := row["rule_id"].Int64()
			if ruleID <= 0 {
				continue
			}
			fallback := ""
			if items := ruleItems[ruleID]; items != nil {
				fallback = items.IntroFeeSuffix
			}
			suffix, _, err := s.resolveIntroFeeChannelSuffix(ctx, gdb.Record{
				"tenant_id": g.NewVar(tenantID), "account_id": row["account_id"], "bot_id": row["bot_id"], "source_chat_id": row["telegram_chat_id"],
			}, gdb.Record{"id": row["rule_id"], "intro_fee_suffix": g.NewVar(fallback)})
			if err != nil {
				return nil, err
			}
			suffixes[index] = suffix
		}
		return suffixes, nil
	}

	tuples := make([]string, 0, len(rows))
	args := make([]any, 0, 1+6*len(rows))
	args = append(args, tenantID)
	for index, row := range rows {
		ruleID := row["rule_id"].Int64()
		chatID := canonicalCollectSourceChatID(row["telegram_chat_id"].String())
		if ruleID <= 0 || chatID == "" {
			continue
		}
		if items := ruleItems[ruleID]; items != nil {
			suffixes[index] = strings.TrimSpace(items.IntroFeeSuffix)
		}
		aliases := tgChannelCacheLookupIds(chatID)
		tuples = append(tuples, "(CAST(? AS integer),CAST(? AS bigint),CAST(? AS bigint),CAST(? AS bigint),CAST(? AS text),CAST(? AS text))")
		args = append(args, index, row["account_id"].Int64(), ruleID, row["bot_id"].Int64(), aliases[0], aliases[len(aliases)-1])
	}
	if len(tuples) == 0 {
		return suffixes, nil
	}

	// Correlate only the current page. Indexed lookups preserve the precedence
	// and explicit empty overrides of the single-channel resolver.
	query := `WITH scope(tenant_id) AS (VALUES (CAST(? AS bigint))),
	page(row_index,account_id,rule_id,bot_id,chat_id,chat_alias) AS (VALUES ` + strings.Join(tuples, ",") + `)
	SELECT p.row_index,
	       CASE WHEN rule_suffix.id IS NOT NULL THEN COALESCE(rule_suffix.suffix,'')
	            WHEN bot_suffix.id IS NOT NULL THEN COALESCE(bot_suffix.suffix,'')
	            ELSE source_suffix.channel_suffix END AS suffix
	FROM page p CROSS JOIN scope t
	LEFT JOIN LATERAL (
		SELECT r.id,r.suffix FROM ` + introFeeChannelRuleTable + ` r
		WHERE r.tenant_id=t.tenant_id AND r.account_id IN (p.account_id,0)
		  AND r.rule_id=p.rule_id AND r.bot_id IN (p.bot_id,0)
		  AND r.channel_id IN (p.chat_id,p.chat_alias) AND r.status=1 AND r.deleted_at IS NULL
		ORDER BY r.account_id DESC,r.bot_id DESC,r.id DESC LIMIT 1
	) rule_suffix ON true
	LEFT JOIN LATERAL (
		SELECT r.id,r.suffix FROM ` + introFeeChannelRuleTable + ` r
		WHERE rule_suffix.id IS NULL AND p.bot_id>0
		  AND r.tenant_id=t.tenant_id AND r.account_id IN (p.account_id,0)
		  AND r.rule_id=0 AND r.bot_id=p.bot_id
		  AND r.channel_id IN (p.chat_id,p.chat_alias) AND r.status=1 AND r.deleted_at IS NULL
		ORDER BY r.account_id DESC,r.id DESC LIMIT 1
	) bot_suffix ON true
	LEFT JOIN LATERAL (
		SELECT src.channel_suffix FROM ` + publishCollectSourceTable + ` src
		WHERE rule_suffix.id IS NULL AND bot_suffix.id IS NULL
		  AND src.tenant_id=t.tenant_id AND src.account_id IN (p.account_id,0)
		  AND src.source_chat_id IN (p.chat_id,p.chat_alias)
		  AND (p.bot_id<=0 OR src.bot_id IN (p.bot_id,0))
		  AND src.deleted_at IS NULL AND src.channel_suffix<>''
		ORDER BY src.account_id DESC,src.bot_id DESC,src.updated_at DESC LIMIT 1
	) source_suffix ON true`
	resolved, err := g.DB().GetAll(ctx, query, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "批量读取采集频道后缀失败")
	}
	for _, row := range resolved {
		if suffix := row["suffix"]; !suffix.IsNil() {
			suffixes[row["row_index"].Int()] = strings.TrimSpace(suffix.String())
		}
	}
	return suffixes, nil
}
