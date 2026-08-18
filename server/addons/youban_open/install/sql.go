package install

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"hotgo/internal/consts"
	"strings"
)

func Install(ctx context.Context) error { return exec(ctx, true) }
func Upgrade(ctx context.Context) error { return exec(ctx, false) }
func exec(ctx context.Context, includeSchema bool) error {
	files := []string{"addons/youban_open/resource/sql/upgrade.pgsql.sql", "addons/youban_open/resource/sql/menu.pgsql.sql"}
	if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
		files = []string{"addons/youban_open/resource/sql/upgrade.sql", "addons/youban_open/resource/sql/menu.sql"}
	}
	if includeSchema {
		if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
			files = append([]string{"addons/youban_open/resource/sql/install.pgsql.sql"}, files...)
		} else {
			files = append([]string{"addons/youban_open/resource/sql/install.sql"}, files...)
		}
	}
	for _, file := range files {
		content := gfile.GetContents(file)
		if strings.TrimSpace(content) == "" {
			continue
		}
		for _, statement := range strings.Split(content, ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err := g.DB().Exec(ctx, statement); err != nil {
				message := strings.ToLower(err.Error())
				if !strings.Contains(message, "already exists") && !strings.Contains(message, "duplicate column") {
					return err
				}
			}
		}
	}
	return nil
}
