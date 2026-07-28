package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
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
			label = fmt.Sprintf("采集源 %d", row.Id)
		}
		list = append(list, &sysin.CollectSourceOptionModel{Id: row.Id, Label: label, Username: row.SourceUsername})
	}
	return list, nil
}
