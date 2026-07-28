package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type collectSourceOptionRow struct {
	Id             int64  `orm:"id"`
	Title          string `orm:"title"`
	SourceUsername string `orm:"source_username"`
}

func (s *sSysPublish) AdminCollectSourceOptions(ctx context.Context) ([]*sysin.CollectSourceOptionModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	cacheKey := fmt.Sprintf("youban_publish:collect_source_options:%d", account.TenantId)
	if cached, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !cached.IsNil() {
		var list []*sysin.CollectSourceOptionModel
		if scanErr := cached.Scan(&list); scanErr == nil && list != nil {
			return list, nil
		}
	}
	var rows []collectSourceOptionRow
	columns := pdao.YoubanPublishCollectSource.Columns()
	if err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(columns.TenantId, account.TenantId).
		WhereNull(columns.DeletedAt).
		Fields(columns.Id, columns.Title, columns.SourceUsername).
		OrderAsc(columns.Title).OrderAsc(columns.Id).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取采集源筛选选项失败")
	}
	list := make([]*sysin.CollectSourceOptionModel, 0, len(rows))
	for _, row := range rows {
		label := strings.TrimSpace(row.Title)
		if label == "" && strings.TrimSpace(row.SourceUsername) != "" {
			label = "@" + strings.TrimPrefix(strings.TrimSpace(row.SourceUsername), "@")
		}
		if label == "" {
			label = fmt.Sprintf("频道 #%d", row.Id)
		}
		list = append(list, &sysin.CollectSourceOptionModel{Id: row.Id, Label: label, Username: row.SourceUsername})
	}
	_ = cache.Instance().Set(ctx, cacheKey, list, 5*time.Minute)
	return list, nil
}
