package install

import (
	"context"
	"hotgo/internal/consts"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gres"
)

var mysqlBusinessSqlFiles = []string{
	"storage/data/generate/youban_hotgo_compat.sql",
	"storage/data/generate/content_profile.sql",
	"storage/data/generate/content_import_monitor_menu.sql",
	"storage/data/generate/admin_notice_extension.sql",
	"storage/data/generate/app_announcement.sql",
	"storage/data/generate/member_vip.sql",
	"storage/data/generate/member_app_settings.sql",
	"storage/data/generate/member_share.sql",
	"storage/data/generate/member_profile_view.sql",
	"storage/data/generate/member_profile_action.sql",
	"storage/data/generate/content_media_video_display_fix.sql",
	"storage/data/generate/youban_cdn_config.sql",
	"storage/data/generate/member_vip_money_config.sql",
	"storage/data/generate/rainbow_pay_config.sql",
}

var pgsqlBusinessSqlFiles = []string{
	"storage/data/generate/pgsql/youban_business.sql",
	"storage/data/generate/pgsql/youban_cdn_config.sql",
	"storage/data/generate/pgsql/member_vip_money_config.sql",
	"storage/data/generate/pgsql/rainbow_pay_config.sql",
}

func Install(ctx context.Context) error {
	return execBusinessSql(ctx)
}

func Upgrade(ctx context.Context) error {
	return execBusinessSql(ctx)
}

func execBusinessSql(ctx context.Context) (err error) {
	for _, file := range businessSqlFiles() {
		if err = execSqlFile(ctx, file); err != nil {
			return gerror.Wrapf(err, "执行悦伴业务 SQL 失败：%s", file)
		}
	}
	if err = installMenus(ctx); err != nil {
		return gerror.Wrap(err, "初始化悦伴后台菜单失败")
	}
	if err = refreshAdminMenuTree(ctx); err != nil {
		return gerror.Wrap(err, "刷新后台菜单关系树失败")
	}
	return nil
}

func businessSqlFiles() []string {
	if strings.ToLower(g.DB().GetConfig().Type) == consts.DBPgsql {
		return pgsqlBusinessSqlFiles
	}
	return mysqlBusinessSqlFiles
}

func execSqlFile(ctx context.Context, path string) (err error) {
	content := readSqlFile(path)
	if strings.TrimSpace(content) == "" {
		return gerror.Newf("SQL 文件不存在或为空：%s", path)
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
	candidates := []string{
		path,
		"server/" + path,
		"../../../" + path,
	}
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
