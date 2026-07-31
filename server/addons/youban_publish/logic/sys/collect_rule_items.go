package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const collectRuleItemTable = "hg_youban_publish_collect_rule_item"

const (
	collectRuleItemKeyword    = "keyword"
	collectRuleItemTag        = "tag"
	collectRuleItemReplace    = "replace"
	collectRuleItemDeleteLine = "delete_line"
	collectRuleItemDeleteText = "delete_text"
	collectRuleItemBlockText  = "block_text"
)

type collectRuleItems struct {
	Keywords     []string
	Tags         []string
	Replacements []collectReplaceRule
	DeleteLines  []string
	DeleteTexts  []string
	BlockedTexts []string
}

func collectRuleItemMap(ctx context.Context, ruleIds []int64) (map[int64]*collectRuleItems, error) {
	result := make(map[int64]*collectRuleItems)
	ruleIds = uniqueIds(ruleIds)
	if len(ruleIds) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model(collectRuleItemTable).Safe().Ctx(ctx).
		Fields("rule_id,item_type,value,replacement,sort").WhereIn("rule_id", ruleIds).
		OrderAsc("rule_id").OrderAsc("item_type").OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集规则项失败")
	}
	for _, row := range rows {
		ruleId := row["rule_id"].Int64()
		items := result[ruleId]
		if items == nil {
			items = &collectRuleItems{}
			result[ruleId] = items
		}
		value := strings.TrimSpace(row["value"].String())
		switch row["item_type"].String() {
		case collectRuleItemKeyword:
			items.Keywords = append(items.Keywords, value)
		case collectRuleItemTag:
			items.Tags = append(items.Tags, value)
		case collectRuleItemReplace:
			items.Replacements = append(items.Replacements, collectReplaceRule{From: value, To: row["replacement"].String()})
		case collectRuleItemDeleteLine:
			items.DeleteLines = append(items.DeleteLines, value)
		case collectRuleItemDeleteText:
			items.DeleteTexts = append(items.DeleteTexts, value)
		case collectRuleItemBlockText:
			items.BlockedTexts = append(items.BlockedTexts, value)
		}
	}
	return result, nil
}

func attachCollectRuleItems(ctx context.Context, rules []gdb.Record) error {
	ruleIds := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ruleIds = append(ruleIds, rule["id"].Int64())
	}
	itemMap, err := collectRuleItemMap(ctx, ruleIds)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		items := itemMap[rule["id"].Int64()]
		if items == nil {
			items = &collectRuleItems{}
		}
		rule["keywords"] = gvar.New(items.Keywords)
		rule["tags"] = gvar.New(items.Tags)
		rule["delete_lines"] = gvar.New(items.DeleteLines)
		rule["delete_texts"] = gvar.New(items.DeleteTexts)
		rule["blocked_texts"] = gvar.New(items.BlockedTexts)
		from := make([]string, 0, len(items.Replacements))
		to := make([]string, 0, len(items.Replacements))
		for _, item := range items.Replacements {
			from = append(from, item.From)
			to = append(to, item.To)
		}
		rule["replace_from"] = gvar.New(from)
		rule["replace_to"] = gvar.New(to)
	}
	return nil
}

func collectRuleStrings(rule gdb.Record, key string) []string {
	return trimCollectValues(gconv.Strings(rule[key].Interface()))
}

func collectRuleReplacements(rule gdb.Record) []collectReplaceRule {
	from := gconv.Strings(rule["replace_from"].Interface())
	to := gconv.Strings(rule["replace_to"].Interface())
	items := make([]collectReplaceRule, 0, len(from))
	for index, value := range from {
		replacement := ""
		if index < len(to) {
			replacement = to[index]
		}
		if strings.TrimSpace(value) != "" {
			items = append(items, collectReplaceRule{From: value, To: replacement})
		}
	}
	return items
}

func syncCollectRuleItemsTx(ctx context.Context, tx gdb.TX, tenantId, accountId, ruleId int64, items collectRuleItems) error {
	if _, err := tx.Model(collectRuleItemTable).Ctx(ctx).Where("rule_id", ruleId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集规则项失败")
	}
	now := gtime.Now()
	sortIndex := 0
	insert := func(itemType, value, replacement string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		sortIndex++
		_, err := tx.Model(collectRuleItemTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantId, "account_id": accountId, "rule_id": ruleId,
			"item_type": itemType, "value": value, "replacement": replacement,
			"sort": sortIndex, "created_at": now,
		}).Insert()
		return err
	}
	for _, value := range items.Keywords {
		if err := insert(collectRuleItemKeyword, value, ""); err != nil {
			return err
		}
	}
	for _, value := range items.Tags {
		if err := insert(collectRuleItemTag, value, ""); err != nil {
			return err
		}
	}
	for _, value := range items.DeleteLines {
		if err := insert(collectRuleItemDeleteLine, value, ""); err != nil {
			return err
		}
	}
	for _, value := range items.DeleteTexts {
		if err := insert(collectRuleItemDeleteText, value, ""); err != nil {
			return err
		}
	}
	for _, value := range items.BlockedTexts {
		if err := insert(collectRuleItemBlockText, value, ""); err != nil {
			return err
		}
	}
	for _, value := range items.Replacements {
		if err := insert(collectRuleItemReplace, value.From, value.To); err != nil {
			return err
		}
	}
	return nil
}
