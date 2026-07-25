package install

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"

	"hotgo/internal/consts"
)

var mysqlBusinessSqlFiles = []string{
	"addons/youban_publish/resource/sql/install.sql",
	"addons/youban_publish/resource/sql/upgrade.sql",
	"addons/youban_publish/resource/sql/menu.sql",
}

var pgsqlBusinessSqlFiles = []string{
	"addons/youban_publish/resource/sql/install.pgsql.sql",
	"addons/youban_publish/resource/sql/upgrade.pgsql.sql",
	"addons/youban_publish/resource/sql/menu.pgsql.sql",
}

const publishInstallLockKey = "youban_publish:install"

func Install(ctx context.Context) error {
	if err := syncStaticResources(ctx); err != nil {
		return gerror.Wrap(err, "同步默认静态资源失败")
	}
	if err := execBusinessSql(ctx, businessSqlFiles(true)); err != nil {
		return err
	}
	return ensureVipOrderCloseCron(ctx)
}

func Upgrade(ctx context.Context) error {
	if err := syncStaticResources(ctx); err != nil {
		return gerror.Wrap(err, "同步默认静态资源失败")
	}
	if err := execBusinessSql(ctx, businessSqlFiles(false)); err != nil {
		return err
	}
	return ensureVipOrderCloseCron(ctx)
}

func execBusinessSql(ctx context.Context, files []string) error {
	const maxAttempts = 3
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = execBusinessSqlOnce(ctx, files)
		if err == nil || !isRetryableInstallError(err) || attempt == maxAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return err
}

func execBusinessSqlOnce(ctx context.Context, files []string) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
			if _, err := tx.Exec("SET LOCAL lock_timeout = '5s'"); err != nil {
				return gerror.Wrap(err, "设置上架系统 SQL 锁等待超时失败")
			}
		}
		if err := acquireInstallLock(tx); err != nil {
			return err
		}
		if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
			defer func() {
				_, _ = tx.Exec("SELECT RELEASE_LOCK(?)", publishInstallLockKey)
			}()
		}
		for _, file := range files {
			if err := execSqlFile(ctx, tx, file); err != nil {
				return gerror.Wrapf(err, "执行上架系统 SQL 失败：%s", file)
			}
		}
		return nil
	})
}

func businessSqlFiles(includeInstall bool) []string {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		if includeInstall {
			return pgsqlBusinessSqlFiles
		}
		return []string{
			"addons/youban_publish/resource/sql/upgrade.pgsql.sql",
			"addons/youban_publish/resource/sql/menu.pgsql.sql",
		}
	}
	if includeInstall {
		return mysqlBusinessSqlFiles
	}
	return []string{
		"addons/youban_publish/resource/sql/upgrade.sql",
		"addons/youban_publish/resource/sql/menu.sql",
	}
}

func isRetryableInstallError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "could not obtain lock") ||
		strings.Contains(message, "lock timeout") ||
		strings.Contains(message, "serialization failure")
}

func execSqlFile(ctx context.Context, tx gdb.TX, path string) (err error) {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("SQL 文件不存在或为空：%s", path)
	}
	for index, sql := range splitSql(content) {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if _, err = tx.Exec(sql); err != nil {
			if isIgnorableSqlError(err) {
				continue
			}
			return gerror.Wrapf(err, "执行 SQL 失败：%s 第 %d 段\n%s", path, index+1, sql)
		}
	}
	return nil
}

func acquireInstallLock(tx gdb.TX) error {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	switch dbType {
	case consts.DBPgsql:
		if _, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", publishInstallLockKey); err != nil {
			return gerror.Wrap(err, "获取上架系统安装锁失败")
		}
	default:
		value, err := tx.GetValue("SELECT GET_LOCK(?, 60)", publishInstallLockKey)
		if err != nil {
			return gerror.Wrap(err, "获取上架系统安装锁失败")
		}
		if value.Int() != 1 {
			return gerror.New("获取上架系统安装锁超时")
		}
	}
	return nil
}

func isIgnorableSqlError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") ||
		strings.Contains(message, "duplicate key name") ||
		strings.Contains(message, "already exists") ||
		strings.Contains(message, "duplicate key value")
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
	var (
		list    []string
		builder strings.Builder
		quote   byte
		escape  bool
		dollar  string
	)
	for index := 0; index < len(content); index++ {
		if dollar != "" {
			if strings.HasPrefix(content[index:], dollar) {
				builder.WriteString(dollar)
				index += len(dollar) - 1
				dollar = ""
				continue
			}
			builder.WriteByte(content[index])
			continue
		}

		if quote == 0 && content[index] == '$' {
			if tag := sqlDollarQuoteTag(content[index:]); tag != "" {
				builder.WriteString(tag)
				index += len(tag) - 1
				dollar = tag
				continue
			}
		}

		builder.WriteByte(content[index])
		if escape {
			escape = false
			continue
		}
		if content[index] == '\\' && quote != 0 {
			escape = true
			continue
		}
		switch content[index] {
		case '\'', '"', '`':
			if quote == 0 {
				quote = content[index]
			} else if quote == content[index] {
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

func sqlDollarQuoteTag(value string) string {
	if !strings.HasPrefix(value, "$") {
		return ""
	}
	relativeEnd := strings.IndexByte(value[1:], '$')
	if relativeEnd < 0 {
		return ""
	}
	name := value[1 : relativeEnd+1]
	if name == "" {
		return "$$"
	}
	for index, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return ""
	}
	return value[:relativeEnd+2]
}
