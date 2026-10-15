// Package botregistry defines the shared identity store used by Bot plugins.
package botregistry

import (
	"context"
	_ "embed"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	Table      = "hg_youban_bot_registry"
	TypeMain   = "main"
	TypeNormal = "normal"
	TypeTwoWay = "two_way"
)

//go:embed schema.pgsql.sql
var postgresSchema string

//go:embed schema.mysql.sql
var mysqlSchema string

// EnsureSchema creates the sole identity table; it never changes existing Bots.
func EnsureSchema(ctx context.Context) error {
	sql := postgresSchema
	if g.DB().GetConfig().Type == "mysql" {
		sql = mysqlSchema
	}
	_, err := g.DB().Exec(ctx, sql)
	return gerror.Wrap(err, "初始化统一Bot主表失败")
}

// TelegramID remains stable when Telegram rotates a Bot token.
func TelegramID(token string) (string, error) {
	id, _, ok := strings.Cut(strings.TrimSpace(token), ":")
	if !ok || id == "" {
		return "", gerror.New("Bot Token格式不正确")
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return "", gerror.New("Bot Token中的机器人ID不合法")
		}
	}
	return id, nil
}
