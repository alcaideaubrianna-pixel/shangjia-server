package bootstrap

import (
	"context"
	"hotgo/internal/consts"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"
)

var databaseInitSqlFiles = map[string][]string{
	consts.DBMysql: {"storage/data/hotgo.sql"},
	consts.DBPgsql: {"storage/data/hotgo-pg.sql"},
}

// InitDatabase 初始化 HotGo 基础空库。该方法仅供显式初始化流程调用，普通启动不执行业务补丁。
func InitDatabase(ctx context.Context) (err error) {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	tables, err := getDatabaseTables(ctx, dbType)
	if err != nil {
		return gerror.Wrap(err, "检查数据库状态失败")
	}
	if len(tables) == 0 {
		g.Log().Info(ctx, "检测到默认数据库为空，开始初始化数据库")
		sqlFiles, ok := databaseInitSqlFiles[dbType]
		if !ok {
			return gerror.Newf("暂不支持当前数据库初始化：%s", dbType)
		}
		for _, file := range sqlFiles {
			if err = execSqlFile(ctx, file); err != nil {
				return gerror.Wrapf(err, "执行初始化 SQL 失败：%s", file)
			}
		}
		g.Log().Info(ctx, "数据库初始化完成")
	}
	return nil
}

func getDatabaseTables(ctx context.Context, dbType string) (tables gdb.Result, err error) {
	switch dbType {
	case consts.DBPgsql:
		return g.DB().GetAll(ctx, "SELECT tablename FROM pg_tables WHERE schemaname='public'")
	default:
		return g.DB().GetAll(ctx, "SHOW TABLES")
	}
}

func execSqlFile(ctx context.Context, path string) (err error) {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("初始化 SQL 文件不存在或为空：%s", path)
	}

	for index, sql := range splitSql(content) {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if _, err = g.DB().Exec(ctx, sql); err != nil {
			return gerror.Wrapf(err, "执行 SQL 失败：%s 第 %d 段\n%s", path, index+1, sql)
		}
	}
	return nil
}

func readSqlFile(path string) string {
	if gfile.Exists(path) {
		return gfile.GetContents(path)
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
		quote   rune
		escape  bool
	)

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
