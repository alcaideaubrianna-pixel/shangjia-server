package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func (s *sSysBot) AdminBotChannelCacheList(ctx context.Context, in *sysin.BotChannelCacheListInp) (list []*sysin.BotChannelCacheModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotChannelCacheListInp{}
	}
	mod := g.DB().Model(channelCacheTable+" c").Safe().Ctx(ctx).
		LeftJoin(botTable+" b", "b.id=c.bot_id").
		Fields("c.id,c.bot_id,b.bot_username,c.chat_id AS channel_id,c.chat_title AS channel_title,c.chat_username AS channel_username,c.chat_type AS chat_type,c.is_broadcast,c.is_megagroup,c.message_count,c.last_message_text,c.last_message_at,c.created_at,c.updated_at")
	if in.BotId > 0 {
		mod = mod.Where("c.bot_id", in.BotId)
	}
	switch strings.ToLower(strings.TrimSpace(in.Type)) {
	case "channel":
		mod = mod.Where("c.is_broadcast", 1)
	case "group":
		mod = mod.Where("c.is_megagroup", 1)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(c.chat_id LIKE ? OR c.chat_title LIKE ? OR c.chat_username LIKE ? OR b.bot_username LIKE ?)", like, like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("c.last_message_at").OrderDesc("c.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot频道缓存失败")
	}
	if list == nil {
		list = []*sysin.BotChannelCacheModel{}
	}
	return
}
