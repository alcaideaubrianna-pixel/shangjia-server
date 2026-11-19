package install

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
	"hotgo/internal/library/botregistry"
)

const (
	publishBotTable    = "hg_youban_publish_bot"
	collectSourceTable = "hg_youban_publish_collect_source"
	legacyTwoWayTable  = "hg_youban_two_way_bot_bot"
)

type legacyBotClaim struct {
	identity int64
	tenantID int64
	role     string
	origin   string
}

func backfillBotRegistry(ctx context.Context) error {
	db := g.DB()
	return db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		schema := "DATABASE()"
		if db.GetConfig().Type == consts.DBPgsql {
			schema = "current_schema()"
		}
		tables, err := tx.GetAll("SELECT table_name FROM information_schema.tables WHERE table_schema = "+schema+
			" AND table_name IN (?, ?, ?)", publishBotTable, collectSourceTable, legacyTwoWayTable)
		if err != nil {
			return gerror.Wrap(err, "检查机器人迁移来源失败")
		}
		available := make(map[string]bool, len(tables))
		for _, row := range tables {
			available[row["table_name"].String()] = true
		}

		claims, issues, err := readLegacyBotClaims(tx, available)
		if err != nil {
			return err
		}
		unique, conflicts := auditLegacyBotClaims(claims)
		issues = append(issues, conflicts...)
		if len(issues) > 0 {
			sort.Strings(issues)
			return gerror.New("机器人登记迁移发现冲突：" + strings.Join(issues, "；"))
		}
		for _, claim := range unique {
			if err := botregistry.ClaimIdentity(ctx, claim.identity, claim.tenantID, claim.role); err != nil {
				return gerror.Wrapf(err, "机器人 %d 回填失败（%s）", claim.identity, claim.origin)
			}
		}
		return nil
	})
}

func readLegacyBotClaims(tx gdb.TX, available map[string]bool) ([]legacyBotClaim, []string, error) {
	var claims []legacyBotClaim
	var issues []string
	add := func(rows gdb.Result, role, origin string) {
		for _, row := range rows {
			source := fmt.Sprintf("%s %d", origin, row["id"].Int64())
			identity, err := botregistry.TelegramBotID(row["bot_token"].String())
			tenantID := row["tenant_id"].Int64()
			if err != nil || tenantID <= 0 || row["bot_tenant_id"].Int64() != tenantID {
				issues = append(issues, source+" 的机器人身份或租户归属无效")
				continue
			}
			claims = append(claims, legacyBotClaim{identity, tenantID, role, source})
		}
	}
	if available[collectSourceTable] {
		if !available[publishBotTable] {
			return nil, nil, gerror.New("采集来源表存在，但上架机器人表缺失")
		}
		rows, err := tx.GetAll("SELECT s.id, s.tenant_id, b.tenant_id AS bot_tenant_id, b.bot_token" +
			" FROM " + collectSourceTable + " s LEFT JOIN " + publishBotTable + " b ON b.id = s.bot_id AND b.deleted_at IS NULL" +
			" WHERE s.source_type = 'bot' AND s.deleted_at IS NULL")
		if err != nil {
			return nil, nil, gerror.Wrap(err, "读取历史采集来源失败")
		}
		add(rows, botregistry.RoleCollector, "采集来源")
	}
	if available[publishBotTable] {
		rows, err := tx.GetAll("SELECT id, tenant_id, tenant_id AS bot_tenant_id, bot_token FROM " + publishBotTable +
			" WHERE bot_type = 'two_way' AND deleted_at IS NULL")
		if err != nil {
			return nil, nil, gerror.Wrap(err, "读取历史双向机器人失败")
		}
		add(rows, botregistry.RoleRelay, "双向机器人")
	}

	if available[legacyTwoWayTable] {
		rows, err := tx.GetAll("SELECT id, tenant_id, tenant_id AS bot_tenant_id, bot_token FROM " + legacyTwoWayTable +
			" WHERE deleted_at IS NULL")
		if err != nil {
			return nil, nil, gerror.Wrap(err, "读取旧双向机器人失败")
		}
		add(rows, botregistry.RoleRelay, "旧双向机器人")
	}

	// Old relay rows without a source reference can point to a global Bot.
	query := "SELECT r.id, r.tenant_id, r.tenant_id AS bot_tenant_id, b.bot_token" +
		" FROM hg_youban_bot_relay_config r LEFT JOIN hg_youban_bot_bot b ON b.id = r.bot_id AND b.deleted_at IS NULL" +
		" WHERE r.source_bot_id = 0"
	if available[publishBotTable] {
		query = "SELECT r.id, r.tenant_id, CASE WHEN p.id IS NOT NULL THEN p.tenant_id ELSE r.tenant_id END AS bot_tenant_id," +
			" COALESCE(p.bot_token, b.bot_token) AS bot_token FROM hg_youban_bot_relay_config r" +
			" LEFT JOIN " + publishBotTable + " p ON p.deleted_at IS NULL AND" +
			" (p.id = r.source_bot_id OR (r.source_bot_id = 0 AND p.id = r.bot_id AND p.bot_type = 'two_way' AND p.tenant_id = r.tenant_id))" +
			" LEFT JOIN hg_youban_bot_bot b ON r.source_bot_id = 0 AND b.id = r.bot_id AND b.deleted_at IS NULL"
	}
	rows, err := tx.GetAll(query)
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取历史双向配置失败")
	}
	add(rows, botregistry.RoleRelay, "双向配置")
	if !available[publishBotTable] {
		row, err := tx.GetOne("SELECT id FROM hg_youban_bot_relay_config WHERE source_bot_id > 0 LIMIT 1")
		if err != nil {
			return nil, nil, gerror.Wrap(err, "检查历史双向配置失败")
		}
		if !row.IsEmpty() {
			issues = append(issues, "双向配置引用上架机器人，但上架机器人表缺失")
		}
	}
	return claims, issues, nil
}

func auditLegacyBotClaims(claims []legacyBotClaim) ([]legacyBotClaim, []string) {
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].identity != claims[j].identity {
			return claims[i].identity < claims[j].identity
		}
		return claims[i].origin < claims[j].origin
	})
	var unique []legacyBotClaim
	var conflicts []string
	for start := 0; start < len(claims); {
		first := claims[start]
		end := start + 1
		hasConflict := false
		for end < len(claims) && claims[end].identity == first.identity {
			claim := claims[end]
			hasConflict = hasConflict || first.tenantID != claim.tenantID || first.role != claim.role
			end++
		}
		unique = append(unique, first)
		if hasConflict {
			origins := make([]string, 0, end-start)
			for _, claim := range claims[start:end] {
				origins = append(origins, fmt.Sprintf("%s（租户 %d，角色 %s）", claim.origin, claim.tenantID, claim.role))
			}
			conflicts = append(conflicts, fmt.Sprintf("机器人 %d 的历史登记冲突：%s", first.identity, strings.Join(origins, "、")))
		}
		start = end
	}

	return unique, conflicts
}
