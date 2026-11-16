package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
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
	base := collectSourceOptionsModel(ctx, account.TenantId, account.Id, keyword)
	if totalCount, err = countCollectSourceOptionGroups(ctx, base); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集频道失败")
	}
	// A Telegram channel is identified by source_chat_id. Historical source
	// rows may point at the same channel, so expose one option per channel.
	mod := base.Fields("MIN(ac.source_id) AS source_id,c.telegram_chat_id AS source_chat_id,MAX(s.title) AS title,MAX(s.source_username) AS source_username,MAX(c.title) AS channel_title,MAX(c.username) AS channel_username").
		Group("c.telegram_chat_id")
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

func countCollectSourceOptionGroups(ctx context.Context, mod *gdb.Model) (int, error) {
	row, err := mod.Clone().Fields(gdb.Raw("COUNT(DISTINCT c.telegram_chat_id) AS total")).One()
	if err != nil {
		return 0, err
	}
	return row["total"].Int(), nil
}

func collectSourceOptionsModel(ctx context.Context, tenantID, accountID int64, keyword string) *gdb.Model {
	// Channel options describe configured collection relations, not historical
	// messages. Keep disabled/pending relations visible for history filtering.
	base := g.DB().Model(pdao.YoubanPublishCollectAccountChannel.Table()+" ac").Safe().Ctx(ctx).
		InnerJoin(pdao.YoubanPublishCollectChannel.Table()+" c", "c.id=ac.channel_id AND c.tenant_id=ac.tenant_id AND c.deleted_at IS NULL").
		InnerJoin(publishCollectSourceTable+" s", "s.id=ac.source_id AND s.tenant_id=ac.tenant_id AND s.account_id=ac.account_id AND s.deleted_at IS NULL").
		Where("ac.tenant_id", tenantID).Where("ac.account_id", accountID).
		WhereNull("ac.deleted_at").Where("c.telegram_chat_id != ''")
	if keyword = strings.Trim(strings.TrimSpace(keyword), "@"); keyword != "" {
		like := "%" + keyword + "%"
		base = base.Where("(c.title LIKE ? OR c.username LIKE ? OR c.telegram_chat_id LIKE ? OR s.title LIKE ?)", like, like, like, like)
	}
	return base
}
