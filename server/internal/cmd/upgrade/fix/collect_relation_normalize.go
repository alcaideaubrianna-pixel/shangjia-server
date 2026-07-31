package fix

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	collectRuleChannelTable     = "hg_youban_publish_collect_rule_channel"
	collectDispatchChannelTable = "hg_youban_publish_collect_dispatch_channel"
	collectRuleItemTable        = "hg_youban_publish_collect_rule_item"
)

func NormalizeYoubanPublishCollectRelations(ctx context.Context) error {
	if err := ensureCollectRelationTables(ctx); err != nil {
		return err
	}
	if err := migrateCollectRelationRows(ctx, "hg_youban_publish_collect_rule", "id", "target_channel_id_json", collectRuleChannelTable, "rule_id"); err != nil {
		return gerror.Wrap(err, "迁移采集规则频道失败")
	}
	if err := migrateCollectRelationRows(ctx, "hg_youban_publish_collect_dispatch", "id", "target_channel_id_json", collectDispatchChannelTable, "dispatch_id"); err != nil {
		return gerror.Wrap(err, "迁移采集分发频道失败")
	}
	if err := migrateCollectRuleItems(ctx); err != nil {
		return gerror.Wrap(err, "迁移采集规则项失败")
	}
	if err := dropCollectLegacyColumns(ctx); err != nil {
		return err
	}
	g.Log().Info(ctx, "采集规则和分发频道关系迁移完成")
	return nil
}

func migrateCollectRelationRows(ctx context.Context, sourceTable, idColumn, jsonColumn, targetTable, relationColumn string) error {
	rows, err := g.DB().Model(sourceTable).Safe().Ctx(ctx).
		Fields(idColumn + ",tenant_id,account_id," + jsonColumn).
		Where(jsonColumn + " IS NOT NULL").Where(jsonColumn + " <> ''").Where(jsonColumn + " <> '[]'").All()
	if err != nil {
		return err
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, row := range rows {
			id := row[idColumn].Int64()
			channelIds, parseErr := parseCollectRelationIds(row[jsonColumn].String())
			if parseErr != nil {
				return gerror.Wrapf(parseErr, "%s=%d 的频道数据无效", idColumn, id)
			}
			if _, txErr := tx.Model(targetTable).Ctx(ctx).Where(relationColumn, id).Delete(); txErr != nil {
				return txErr
			}
			for _, channelId := range channelIds {
				if _, txErr := tx.Model(targetTable).Ctx(ctx).Data(g.Map{
					"tenant_id": row["tenant_id"].Int64(), "account_id": row["account_id"].Int64(),
					relationColumn: id, "channel_id": channelId, "created_at": gtime.Now(),
				}).Insert(); txErr != nil {
					return txErr
				}
			}
		}
		return nil
	})
}

func parseCollectRelationIds(raw string) ([]int64, error) {
	var values []int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func migrateCollectRuleItems(ctx context.Context) error {
	rows, err := g.DB().Model("hg_youban_publish_collect_rule").Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,keyword_json,tag_json,replace_json,delete_line_text_json,delete_text_json,block_text_json").All()
	if err != nil {
		return err
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, row := range rows {
			ruleId := row["id"].Int64()
			if _, txErr := tx.Model(collectRuleItemTable).Ctx(ctx).Where("rule_id", ruleId).Delete(); txErr != nil {
				return txErr
			}
			sortIndex := 0
			insert := func(itemType, value, replacement string) error {
				value = strings.TrimSpace(value)
				if value == "" {
					return nil
				}
				sortIndex++
				_, txErr := tx.Model(collectRuleItemTable).Ctx(ctx).Data(g.Map{
					"tenant_id": row["tenant_id"].Int64(), "account_id": row["account_id"].Int64(), "rule_id": ruleId,
					"item_type": itemType, "value": value, "replacement": replacement, "sort": sortIndex, "created_at": gtime.Now(),
				}).Insert()
				return txErr
			}
			for _, item := range []struct{ column, itemType string }{
				{"keyword_json", "keyword"}, {"tag_json", "tag"}, {"delete_line_text_json", "delete_line"},
				{"delete_text_json", "delete_text"}, {"block_text_json", "block_text"},
			} {
				values, parseErr := parseCollectStringList(row[item.column].String())
				if parseErr != nil {
					return parseErr
				}
				for _, value := range values {
					if txErr := insert(item.itemType, value, ""); txErr != nil {
						return txErr
					}
				}
			}
			var replacements []struct {
				From string `json:"from"`
				To   string `json:"to"`
			}
			if raw := strings.TrimSpace(row["replace_json"].String()); raw != "" && raw != "[]" && raw != "null" {
				if parseErr := json.Unmarshal([]byte(raw), &replacements); parseErr != nil {
					return parseErr
				}
			}
			for _, replacement := range replacements {
				if txErr := insert("replace", replacement.From, replacement.To); txErr != nil {
					return txErr
				}
			}
		}
		return nil
	})
}

func parseCollectStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func ensureCollectRelationTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_channel" ("id" BIGSERIAL PRIMARY KEY,"tenant_id" bigint NOT NULL DEFAULT 0,"account_id" bigint NOT NULL DEFAULT 0,"rule_id" bigint NOT NULL DEFAULT 0,"channel_id" bigint NOT NULL DEFAULT 0,"created_at" timestamp DEFAULT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_rule_channel" ON "hg_youban_publish_collect_rule_channel" ("rule_id","channel_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_channel_owner" ON "hg_youban_publish_collect_rule_channel" ("tenant_id","account_id","rule_id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_dispatch_channel" ("id" BIGSERIAL PRIMARY KEY,"tenant_id" bigint NOT NULL DEFAULT 0,"account_id" bigint NOT NULL DEFAULT 0,"dispatch_id" bigint NOT NULL DEFAULT 0,"channel_id" bigint NOT NULL DEFAULT 0,"created_at" timestamp DEFAULT NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_dispatch_channel" ON "hg_youban_publish_collect_dispatch_channel" ("dispatch_id","channel_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_dispatch_channel_owner" ON "hg_youban_publish_collect_dispatch_channel" ("tenant_id","account_id","dispatch_id")`,
		`CREATE TABLE IF NOT EXISTS "hg_youban_publish_collect_rule_item" ("id" BIGSERIAL PRIMARY KEY,"tenant_id" bigint NOT NULL DEFAULT 0,"account_id" bigint NOT NULL DEFAULT 0,"rule_id" bigint NOT NULL DEFAULT 0,"item_type" varchar(32) NOT NULL DEFAULT '',"value" text,"replacement" text,"sort" integer NOT NULL DEFAULT 0,"created_at" timestamp DEFAULT NULL)`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_rule" ON "hg_youban_publish_collect_rule_item" ("rule_id","item_type","sort","id")`,
		`CREATE INDEX IF NOT EXISTS "idx_ybp_collect_rule_item_owner" ON "hg_youban_publish_collect_rule_item" ("tenant_id","account_id","rule_id")`,
	}
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("采集关系迁移目前只支持PostgreSQL")
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func dropCollectLegacyColumns(ctx context.Context) error {
	statements := []string{
		`ALTER TABLE "hg_youban_publish_collect_rule" DROP COLUMN IF EXISTS "target_channel_id_json", DROP COLUMN IF EXISTS "bot_id_json", DROP COLUMN IF EXISTS "backup_channel_id", DROP COLUMN IF EXISTS "backup_channel_id_json", DROP COLUMN IF EXISTS "keyword_json", DROP COLUMN IF EXISTS "tag_json", DROP COLUMN IF EXISTS "replace_json", DROP COLUMN IF EXISTS "delete_line_text_json", DROP COLUMN IF EXISTS "delete_text_json", DROP COLUMN IF EXISTS "block_text_json"`,
		`ALTER TABLE "hg_youban_publish_collect_review" DROP COLUMN IF EXISTS "target_channel_id_json", DROP COLUMN IF EXISTS "bot_id_json", DROP COLUMN IF EXISTS "media_json"`,
		`ALTER TABLE "hg_youban_publish_collect_dispatch" DROP COLUMN IF EXISTS "target_channel_id_json", DROP COLUMN IF EXISTS "bot_id_json"`,
		`ALTER TABLE "hg_youban_publish_collect_content" DROP COLUMN IF EXISTS "media_json"`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "删除采集旧字段失败")
		}
	}
	return nil
}
