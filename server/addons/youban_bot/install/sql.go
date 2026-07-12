package install

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"

	"hotgo/internal/consts"
)

var mysqlSqlFiles = []string{
	"addons/youban_bot/resource/sql/install.sql",
	"addons/youban_bot/resource/sql/upgrade.sql",
	"addons/youban_bot/resource/sql/menu.sql",
}

var pgsqlSqlFiles = []string{
	"addons/youban_bot/resource/sql/install.pgsql.sql",
	"addons/youban_bot/resource/sql/upgrade.pgsql.sql",
	"addons/youban_bot/resource/sql/menu.pgsql.sql",
}

func Install(ctx context.Context) error { return execSql(ctx) }
func Upgrade(ctx context.Context) error { return execSql(ctx) }

func execSql(ctx context.Context) (err error) {
	for _, file := range sqlFiles() {
		if err = execSqlFile(ctx, file); err != nil {
			return gerror.Wrapf(err, "执行全局机器人 SQL 失败：%s", file)
		}
	}
	return nil
}

func sqlFiles() []string {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return pgsqlSqlFiles
	}
	return mysqlSqlFiles
}

func execSqlFile(ctx context.Context, path string) (err error) {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return nil
	}
	for index, sql := range splitSql(content) {
		sql = strings.TrimSpace(sql)
		if sql == "" || strings.HasPrefix(sql, "--") {
			continue
		}
		if _, err = g.DB().Exec(ctx, sql); err != nil {
			if isIgnorableSqlError(err) {
				continue
			}
			return gerror.Wrapf(err, "执行 SQL 失败：%s 第 %d 段", path, index+1)
		}
	}
	return nil
}

func isIgnorableSqlError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") || strings.Contains(message, "duplicate key name") || strings.Contains(message, "already exists") || strings.Contains(message, "duplicate key value")
}

func readSqlFile(path string) string {
	candidates := []string{path, "server/" + path, "../../../" + path, "../../" + path}
	for _, candidate := range candidates {
		if gfile.Exists(candidate) {
			return gfile.GetContents(candidate)
		}
	}
	if !gres.IsEmpty() && gres.Contains(path) {
		return string(gres.GetContent(path))
	}
	return ""
}

func splitSql(content string) []string {
	var list []string
	var builder strings.Builder
	var quote rune
	var escape bool
	for _, r := range content {
		builder.WriteRune(r)
		if escape {
			escape = false
			continue
		}
		if r == '\\' && quote != 0 {
			escape = true
			continue
		}
		switch r {
		case '\'', '"', '`':
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
		case ';':
			if quote == 0 {
				list = append(list, builder.String())
				builder.Reset()
			}
		}
	}
	if tail := strings.TrimSpace(builder.String()); tail != "" {
		list = append(list, tail)
	}
	return list
}
