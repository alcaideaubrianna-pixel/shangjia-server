package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysChat) deletedExternalConversation(ctx context.Context, ownerId, profileId int64) (*chatConversationRow, error) {
	var row *chatConversationRow
	err := g.DB().Model(chatConversationTable).Ctx(ctx).Unscoped().
		Where("member_id", ownerId).
		Where("profile_id", profileId).
		WhereNotNull("deleted_at").
		OrderDesc("updated_at").
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取已删除客服会话失败")
	}
	return row, nil
}
