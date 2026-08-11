package install

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"
)

var installSQLFiles = map[string]string{
	"pgsql": "addons/telegram_collector/resource/sql/install.pgsql.sql",
	"mysql": "addons/telegram_collector/resource/sql/install.sql",
}

func Install(ctx context.Context) error {
	return executeSQLFile(ctx, installSQLFiles[strings.ToLower(g.DB().GetConfig().Type)])
}

func Upgrade(ctx context.Context) error {
	return executeSQLFile(ctx, installSQLFiles[strings.ToLower(g.DB().GetConfig().Type)])
}

func executeSQLFile(ctx context.Context, path string) error {
	if path == "" {
		return gerror.New("Telegram采集插件暂不支持当前数据库类型")
	}
	content := readSQLFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("Telegram采集插件SQL文件不存在或为空：%s", path)
	}
	for index, statement := range strings.Split(content, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrapf(err, "执行Telegram采集插件SQL失败：%s 第%d段", path, index+1)
		}
	}
	return nil
}

func readSQLFile(path string) string {
	for _, candidate := range []string{path, "server/" + path, "../../" + path, "../../../" + path} {
		if gfile.Exists(candidate) {
			return gfile.GetContents(candidate)
		}
	}
	if !gres.IsEmpty() && gres.Contains(path) {
		return string(gres.GetContent(path))
	}
	return ""
}
