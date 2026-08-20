package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectSourceOptionRow struct {
	SourceId        int64  `orm:"source_id"`
	SourceChatId    string `orm:"source_chat_id"`
	Title           string `orm:"title"`
	SourceUsername  string `orm:"source_username"`
	ChannelTitle    string `orm:"channel_title"`
	ChannelUsername string `orm:"channel_username"`
}

func (s *sSysPublish) AdminCollectSourceOptions(ctx context.Context, keyword string, page, perPage int) (list []*sysin.CollectSourceOptionModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 || perPage > 100 {
		perPage = 20
	}
	var rows []collectSourceOptionRow
	mod := g.DB().Model(publishCollectEventTable+" e").Safe().Ctx(ctx).
		InnerJoin(publishCollectSourceTable+" s", "s.id=e.source_id AND s.tenant_id=e.tenant_id").
		LeftJoin(publishTgChannelTable+" c", "c.tenant_id=e.tenant_id AND c.tg_account_id=e.tg_account_id AND c.channel_id=e.source_chat_id").
		LeftJoin(publishBotChannelCacheTable+" bc", "bc.tenant_id=e.tenant_id AND bc.bot_id=COALESCE(NULLIF(e.bot_id,0),s.bot_id) AND bc.chat_id=e.source_chat_id").
		Where("e.tenant_id", account.TenantId).WhereNull("s.deleted_at").Where("e.source_chat_id != ''").
		Fields("e.source_id,e.source_chat_id,MAX(s.title) AS title,MAX(s.source_username) AS source_username,COALESCE(MAX(c.channel_title),MAX(bc.chat_title)) AS channel_title,COALESCE(MAX(c.channel_username),MAX(bc.chat_username)) AS channel_username").
		Group("e.source_id,e.source_chat_id")
	if keyword = strings.Trim(strings.TrimSpace(keyword), "@"); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(c.channel_title LIKE ? OR c.channel_username LIKE ? OR bc.chat_title LIKE ? OR bc.chat_username LIKE ? OR e.source_chat_id LIKE ? OR s.title LIKE ?)", like, like, like, like, like, like)
	}
	if totalCount, err = mod.Clone().Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集频道失败")
	}
	if err = mod.OrderAsc("title").OrderAsc("source_id").Page(page, perPage).Scan(&rows); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集频道筛选选项失败")
	}
	list = make([]*sysin.CollectSourceOptionModel, 0, len(rows))
	for _, row := range rows {
		label := strings.TrimSpace(row.ChannelTitle)
		username := strings.TrimPrefix(strings.TrimSpace(row.ChannelUsername), "@")
		if label == "" {
			label = strings.TrimSpace(row.Title)
		}
		if label == "" && username != "" {
			label = "@" + username
		}
		if label == "" {
			label = "频道 #" + row.SourceChatId
		}
		if username != "" {
			label += " @" + username
		}
		list = append(list, &sysin.CollectSourceOptionModel{Id: row.SourceId, SourceId: row.SourceId, SourceChatId: row.SourceChatId, Label: label, Username: username})
	}
	return list, totalCount, nil
}
