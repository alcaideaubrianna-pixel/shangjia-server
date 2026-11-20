package botregistry

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

const (
	RoleUnassigned       = "unassigned"
	RoleCollector        = "collector"
	RoleRelay            = "relay"
	RoleUploader         = "uploader"
	UploaderGatewayOwner = "youban_publish_uploader"

	table = "hg_youban_bot_role_registry"
)

// TelegramBotID extracts the stable identity without returning token material.
func TelegramBotID(token string) (int64, error) {
	prefix, secret, ok := strings.Cut(strings.TrimSpace(token), ":")
	if !ok || secret == "" || strings.Trim(prefix, "0123456789") != "" {
		return 0, gerror.New("机器人 Token 格式无效")
	}
	id, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || id <= 0 {
		return 0, gerror.New("机器人 Token 格式无效")
	}
	return id, nil
}

// Claim reserves the identity for a tenant and role in the caller's transaction.
// Repeated claims for the same tenant and role preserve existing configuration.
func Claim(ctx context.Context, token string, tenantID int64, role string) error {
	id, err := TelegramBotID(token)
	if err != nil {
		return err
	}
	return ClaimIdentity(ctx, id, tenantID, role)
}

// ClaimIdentity reserves a numeric identity, including during legacy backfill.
func ClaimIdentity(ctx context.Context, id, tenantID int64, role string) error {
	if id <= 0 || tenantID <= 0 || (role != RoleUnassigned && role != RoleCollector && role != RoleRelay && role != RoleUploader) {
		return gerror.New("机器人登记身份、租户或角色无效")
	}
	db := g.DB()
	return db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		query := "INSERT INTO " + table + " (telegram_bot_id, tenant_id, role) VALUES (?, ?, ?)"
		if db.GetConfig().Type == consts.DBPgsql {
			query += " ON CONFLICT (telegram_bot_id) DO NOTHING"
		} else {
			query += " ON DUPLICATE KEY UPDATE telegram_bot_id = telegram_bot_id"
		}
		if _, err := tx.Exec(query, id, tenantID, role); err != nil {
			return gerror.Wrap(err, "登记机器人失败")
		}
		row, err := tx.GetOne("SELECT tenant_id, role FROM "+table+" WHERE telegram_bot_id = ? FOR UPDATE", id)
		if err != nil {
			return gerror.Wrap(err, "读取机器人登记失败")
		}
		if row["tenant_id"].Int64() != tenantID {
			return gerror.New("该机器人已登记在其他租户")
		}
		if current := row["role"].String(); current != role {
			if current == RoleUnassigned {
				_, err := tx.Exec("UPDATE "+table+" SET role = ? WHERE telegram_bot_id = ?", role, id)
				return err
			}
			switch current {
			case RoleCollector:
				return gerror.New("该机器人已配置为采集机器人，无法分配其他角色")
			case RoleRelay:
				return gerror.New("该机器人已配置为双向机器人，无法分配其他角色")
			default:
				return gerror.New("该机器人已配置为上架机器人，无法分配其他角色")
			}
		}
		return nil
	})
}

// CollectorConfig requires a registered collector with an active collect source.
func CollectorConfig(ctx context.Context, token string, tenantID int64) (configured bool, requireRegistration bool, err error) {
	id, err := TelegramBotID(token)
	if err != nil {
		return false, false, err
	}
	db := g.DB()
	tokenIdentity := "TRIM(LEADING '0' FROM SUBSTRING_INDEX(TRIM(b.bot_token), ':', 1))"
	if db.GetConfig().Type == consts.DBPgsql {
		tokenIdentity = "LTRIM(SPLIT_PART(TRIM(b.bot_token), ':', 1), '0')"
	}
	row, err := db.GetOne(ctx, "SELECT r.require_registration FROM "+table+" r"+
		" WHERE r.telegram_bot_id = ? AND r.tenant_id = ? AND r.role = ?"+
		" AND EXISTS (SELECT 1 FROM hg_youban_publish_collect_source s"+
		" JOIN hg_youban_publish_bot b ON b.id = s.bot_id AND b.tenant_id = s.tenant_id"+
		" WHERE s.tenant_id = r.tenant_id AND s.source_type = 'bot' AND s.status = 1"+
		" AND s.collect_enabled = 1 AND s.deleted_at IS NULL AND b.status = 1"+
		" AND b.deleted_at IS NULL AND POSITION(':' IN b.bot_token) > 0 AND "+tokenIdentity+" = ?)",
		id, tenantID, RoleCollector, strconv.FormatInt(id, 10))
	if err != nil {
		return false, false, gerror.Wrap(err, "读取采集机器人配置失败")
	}
	if row.IsEmpty() {
		return false, false, nil
	}
	return true, row["require_registration"].Bool(), nil
}

// CollectorClaims returns durable collector claims for the supplied identities.
func CollectorClaims(ctx context.Context, botIDs []int64) (map[int64]int64, error) {
	claims := make(map[int64]int64)
	if len(botIDs) == 0 {
		return claims, nil
	}
	rows, err := g.DB().Model(table).Ctx(ctx).Fields("telegram_bot_id", "tenant_id").
		Where("role", RoleCollector).WhereIn("telegram_bot_id", botIDs).All()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		claims[row["telegram_bot_id"].Int64()] = row["tenant_id"].Int64()
	}
	return claims, nil
}

func UploaderConfig(ctx context.Context, token string, tenantID int64) (configured, requireRegistration bool, err error) {
	id, err := TelegramBotID(token)
	if err != nil {
		return false, false, err
	}
	row, err := g.DB().GetOne(ctx, "SELECT require_registration FROM "+table+" WHERE telegram_bot_id = ? AND tenant_id = ? AND role = ?", id, tenantID, RoleUploader)
	if err != nil || row.IsEmpty() {
		return false, false, err
	}
	return true, row["require_registration"].Bool(), nil
}

func SetUploaderRequireRegistration(ctx context.Context, token string, tenantID int64, required bool) error {
	id, err := TelegramBotID(token)
	if err != nil {
		return err
	}
	value := 0
	if required {
		value = 1
	}
	_, err = g.DB().Exec(ctx, "UPDATE "+table+" SET require_registration = ? WHERE telegram_bot_id = ? AND tenant_id = ? AND role = ?", value, id, tenantID, RoleUploader)
	return err
}

// SetCollectorRequireRegistration updates the collector configuration for its owning tenant.
func SetCollectorRequireRegistration(ctx context.Context, token string, tenantID int64, required bool) error {
	id, err := TelegramBotID(token)
	if err != nil {
		return err
	}
	value := 0
	if required {
		value = 1
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, err := tx.GetOne("SELECT telegram_bot_id FROM "+table+
			" WHERE telegram_bot_id = ? AND tenant_id = ? AND role = ? FOR UPDATE", id, tenantID, RoleCollector)
		if err != nil {
			return gerror.Wrap(err, "读取采集机器人登记失败")
		}
		if row.IsEmpty() {
			return gerror.New("当前租户未登记该采集机器人")
		}
		_, err = tx.Exec("UPDATE "+table+" SET require_registration = ? WHERE telegram_bot_id = ?", value, id)
		return gerror.Wrap(err, "保存采集机器人配置失败")
	})
}
