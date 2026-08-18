package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/consts"
)

func ensureExternalConversationHiddenColumn(ctx context.Context) error {
	statement := "ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS user_hidden_at timestamp"
	if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
		statement = "ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `user_hidden_at` datetime DEFAULT NULL COMMENT '用户端隐藏时间'"
	}
	_, err := g.DB().Exec(ctx, statement)
	return err
}
