package bootstrap

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"
)

var databaseInitSqlFiles = []string{
	"storage/data/hotgo.sql",
	"storage/data/generate/content_profile.sql",
	"storage/data/generate/content_import_monitor_menu.sql",
}

// InitDatabase 初始化空数据库。必须在 global.Init 前执行，因为 HotGo 启动会先读取系统配置表。
func InitDatabase(ctx context.Context) (err error) {
	tables, err := g.DB().GetAll(ctx, "SHOW TABLES")
	if err != nil {
		return gerror.Wrap(err, "检查数据库状态失败")
	}
	if len(tables) > 0 {
		return nil
	}

	g.Log().Info(ctx, "检测到默认数据库为空，开始初始化数据库")
	for _, file := range databaseInitSqlFiles {
		if err = execSqlFile(ctx, file); err != nil {
			return gerror.Wrapf(err, "执行初始化 SQL 失败：%s", file)
		}
	}
	g.Log().Info(ctx, "数据库初始化完成")
	return nil
}

func execSqlFile(ctx context.Context, path string) (err error) {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("初始化 SQL 文件不存在或为空：%s", path)
	}

	for _, sql := range splitSql(content) {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		if _, err = g.DB().Exec(ctx, sql); err != nil {
			return err
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
