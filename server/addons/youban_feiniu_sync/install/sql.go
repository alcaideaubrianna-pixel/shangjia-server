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
	"addons/youban_feiniu_sync/resource/sql/install.sql",
	"addons/youban_feiniu_sync/resource/sql/menu.sql",
}
var pgsqlSqlFiles = []string{
	"addons/youban_feiniu_sync/resource/sql/install.pgsql.sql",
	"addons/youban_feiniu_sync/resource/sql/menu.pgsql.sql",
}

func Install(ctx context.Context) error { return execBusinessSql(ctx) }
func Upgrade(ctx context.Context) error { return execBusinessSql(ctx) }

func execBusinessSql(ctx context.Context) error {
	for _, file := range businessSqlFiles() {
		if err := execSqlFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}
func businessSqlFiles() []string {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return pgsqlSqlFiles
	}
	return mysqlSqlFiles
}

func execSqlFile(ctx context.Context, path string) error {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("SQL 文件不存在或为空：%s", path)
	}
	for _, sql := range splitSql(content) {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			if isIgnorableSqlError(err) {
				continue
			}
			return gerror.Wrapf(err, "执行 SQL 失败：%s", path)
		}
	}
	return nil
}
func readSqlFile(path string) string {
	for _, c := range []string{path, "server/" + path, "../../../" + path, "../../" + path} {
		if gfile.Exists(c) {
			return gfile.GetContents(c)
		}
	}
	if !gres.IsEmpty() && gres.Contains(path) {
		return string(gres.GetContent(path))
	}
	return ""
}
func splitSql(content string) []string {
	var list []string
	var b strings.Builder
	var q rune
	for _, r := range content {
		b.WriteRune(r)
		switch r {
		case '\'', '"', '`':
			if q == 0 {
				q = r
			} else if q == r {
				q = 0
			}
		case ';':
			if q == 0 {
				list = append(list, b.String())
				b.Reset()
			}
		}
	}
	if tail := strings.TrimSpace(b.String()); tail != "" {
		list = append(list, tail)
	}
	return list
}
func isIgnorableSqlError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "already exists")
}
