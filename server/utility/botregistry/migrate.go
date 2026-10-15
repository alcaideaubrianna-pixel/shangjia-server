package botregistry

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"
)

const migrationTable = "hg_youban_bot_registry_migration"

// Upgrade moves the three identity stores and every persisted Bot reference together.
// Stop Account and other Bot consumers before running this one-time cutover.
func Upgrade(ctx context.Context) error {
	if err := EnsureSchema(ctx); err != nil {
		return err
	}
	if g.DB().GetConfig().Type != "pgsql" {
		return gerror.New("统一Bot历史迁移目前仅支持PostgreSQL，请使用PostgreSQL执行切换")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SELECT pg_advisory_xact_lock(70320261015)"); err != nil {
			return err
		}
		if _, err := tx.Exec("SET LOCAL lock_timeout='5s'"); err != nil {
			return err
		}
		if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS " + migrationTable + "(source_table varchar(128) NOT NULL,source_id bigint NOT NULL,bot_id bigint NOT NULL,PRIMARY KEY(source_table,source_id))"); err != nil {
			return err
		}
		sources := []string{"hg_youban_publish_bot", "hg_youban_bot_bot", "hg_youban_two_way_bot_bot"}
		maps := map[string]map[int64]int64{}
		for _, source := range sources {
			exists, err := tx.GetValue("SELECT to_regclass(?) IS NOT NULL", source)
			if err != nil {
				return err
			}
			if !exists.Bool() {
				continue
			}
			rows, err := tx.Model(source).Safe().Ctx(ctx).Unscoped().OrderAsc("id").All()
			if err != nil {
				return err
			}
			ids := map[int64]int64{}
			for _, row := range rows {
				oldID := row["id"].Int64()
				id, err := migrateIdentity(ctx, tx, source, row)
				if err != nil {
					return gerror.Wrapf(err, "迁移Bot失败 source:%s id:%d", source, oldID)
				}
				ids[oldID] = id
				if _, err := tx.Model(migrationTable).Ctx(ctx).Data(g.Map{"source_table": source, "source_id": oldID, "bot_id": id}).Insert(); err != nil {
					return err
				}
			}
			maps[source] = ids
			if _, err := tx.Exec("SELECT setval(pg_get_serial_sequence('" + Table + "','id'),COALESCE((SELECT MAX(id) FROM " + Table + "),1),(SELECT COUNT(*)>0 FROM " + Table + "))"); err != nil {
				return err
			}
		}
		if len(maps) == 0 {
			return nil
		}
		if err := remapReferences(ctx, tx, maps); err != nil {
			return err
		}
		if err := migrateRelayConfig(ctx, tx); err != nil {
			return err
		}
		for _, source := range sources {
			if _, ok := maps[source]; ok {
				if _, err := tx.Exec("DROP TABLE " + source); err != nil {
					return err
				}
			}
		}
		// IDs are allocated by the registry sequence, never copied from conflicting namespaces.
		return nil
	})
}

func tableExists(ctx context.Context, name string) (bool, error) {
	v, err := g.DB().GetValue(ctx, "SELECT to_regclass(?) IS NOT NULL", name)
	return v.Bool(), err
}

func migrateIdentity(ctx context.Context, tx gdb.TX, source string, row gdb.Record) (int64, error) {
	data := row.Map()
	delete(data, "id")
	id, err := TelegramID(gconv.String(data["bot_token"]))
	if err != nil {
		return 0, err
	}
	data["bot_user_id"] = id
	switch source {
	case "hg_youban_bot_bot":
		data["bot_type"] = TypeMain
	case "hg_youban_two_way_bot_bot":
		data["bot_type"] = TypeTwoWay
		data["bot_name"] = data["name"]
		delete(data, "name")
		data["created_by"] = data["account_id"]
		if gconv.Int(data["status"]) == 0 {
			data["status"] = 2
		}
	default:
		if gconv.String(data["bot_type"]) == "" {
			data["bot_type"] = TypeNormal
		}
	}
	var existing gdb.Record
	existing, err = tx.Model(Table).Ctx(ctx).Unscoped().Where("bot_user_id", id).LockUpdate().One()
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		if gconv.Int64(data["tenant_id"]) > 0 && existing["tenant_id"].Int64() > 0 && gconv.Int64(data["tenant_id"]) != existing["tenant_id"].Int64() {
			return 0, gerror.New("同一个Telegram Bot存在跨租户归属冲突，请核对迁移清单")
		}
		if source != "hg_youban_bot_bot" && gconv.Int64(data["account_id"]) > 0 && existing["account_id"].Int64() > 0 && gconv.Int64(data["account_id"]) != existing["account_id"].Int64() {
			return 0, gerror.New("同一个Telegram Bot存在跨账户归属冲突，请核对迁移清单")
		}
		if source == "hg_youban_bot_bot" {
			// Relay mirror is not a main Bot. Copy only global flags when it really was official/default.
			if gconv.Int(data["is_official"]) != 1 && gconv.Int(data["is_default"]) != 1 {
				return existing["id"].Int64(), nil
			}
			if existing["bot_type"].String() == TypeTwoWay {
				return 0, gerror.New("主Bot与双向Bot身份冲突")
			}
			for _, field := range []string{"tenant_id", "account_id", "created_by", "updated_by", "deleted_by"} {
				delete(data, field)
			}
		} else if source == "hg_youban_two_way_bot_bot" && existing["bot_type"].String() == TypeMain {
			return 0, gerror.New("双向Bot不能覆盖主Bot")
		}
		if _, err := tx.Model(Table).Ctx(ctx).Unscoped().WherePri(existing["id"].Int64()).Data(data).Update(); err != nil {
			return 0, err
		}
		return existing["id"].Int64(), nil
	}
	if source == "hg_youban_publish_bot" {
		data["id"] = row["id"].Int64()
	}
	return tx.Model(Table).Ctx(ctx).Unscoped().Data(data).InsertAndGetId()
}

type referenceColumn struct {
	TableName  string
	ColumnName string
}

func remapReferences(ctx context.Context, tx gdb.TX, maps map[string]map[int64]int64) error {
	var refs []referenceColumn
	err := tx.GetScan(&refs, `SELECT table_name,column_name FROM information_schema.columns WHERE table_schema=current_schema() AND column_name LIKE '%bot_id' AND table_name LIKE 'hg_youban_%' ORDER BY table_name,column_name`)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		var source string
		switch {
		case ref.ColumnName == "two_way_bot_id" && strings.HasPrefix(ref.TableName, "hg_youban_two_way_bot_"):
			source = "hg_youban_two_way_bot_bot"
		case strings.HasPrefix(ref.TableName, "hg_youban_bot_") && ref.TableName != migrationTable && ref.TableName != Table:
			source = "hg_youban_bot_bot"
		case strings.HasPrefix(ref.TableName, "hg_youban_two_way_bot_") && !strings.Contains(ref.TableName, "cooperation_"):
			source = "hg_youban_two_way_bot_bot"
		case strings.HasPrefix(ref.TableName, "hg_youban_publish_"), strings.Contains(ref.TableName, "cooperation_"):
			source = "hg_youban_publish_bot"
		default:
			continue
		}
		ids := maps[source]
		if len(ids) == 0 {
			continue
		}
		keys := make([]int64, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		cases := []string{}
		args := []interface{}{}
		for _, id := range keys {
			cases = append(cases, "WHEN ? THEN ?")
			args = append(args, id, ids[id])
		}
		// One CASE expression prevents remapping an already updated ID a second time.
		sql := fmt.Sprintf("UPDATE %s SET %s=CASE %s %s ELSE %s END", ref.TableName, ref.ColumnName, ref.ColumnName, strings.Join(cases, " "), ref.ColumnName)
		if _, err := tx.Exec(sql, args...); err != nil {
			return err
		}
	}
	return nil
}

func migrateRelayConfig(ctx context.Context, tx gdb.TX) error {
	exists, err := tx.GetValue("SELECT to_regclass(?) IS NOT NULL", "hg_youban_bot_relay_config")
	if err != nil || !exists.Bool() {
		return err
	}
	sqls := []string{
		"DROP INDEX IF EXISTS uk_ybb_relay_account", "DROP INDEX IF EXISTS uk_ybb_relay_source_bot",
		"ALTER TABLE hg_youban_bot_relay_config DROP COLUMN IF EXISTS source_bot_id",
		"ALTER TABLE hg_youban_bot_relay_config DROP COLUMN IF EXISTS owner_id",
		"UPDATE hg_youban_bot_relay_config c SET tenant_id=b.tenant_id,account_id=b.account_id FROM " + Table + " b WHERE b.id=c.bot_id",
	}
	for _, sql := range sqls {
		if _, err := tx.Exec(sql); err != nil {
			return err
		}
	}
	return nil
}

type SourceCount struct {
	Table string `json:"table"`
	Count int    `json:"count"`
}

func Plan(ctx context.Context) ([]SourceCount, error) {
	result := []SourceCount{}
	for _, table := range []string{"hg_youban_publish_bot", "hg_youban_bot_bot", "hg_youban_two_way_bot_bot"} {
		exists, err := tableExists(ctx, table)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		count, err := g.DB().Model(table).Ctx(ctx).Unscoped().Count()
		if err != nil {
			return nil, err
		}
		result = append(result, SourceCount{Table: table, Count: count})
	}
	return result, nil
}
