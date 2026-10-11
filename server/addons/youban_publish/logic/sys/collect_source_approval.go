package sys

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	pdao "hotgo/addons/youban_publish/internal/dao"
)

func (s *sSysPublish) BotCollectSourceApproval(ctx context.Context, sourceId, tenantId, accountId int64, approved bool) error {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return gerror.New("采集源参数不完整")
	}
	data := g.Map{"collect_enabled": 0, "status": 2}
	if approved {
		data = g.Map{"collect_enabled": 1, "status": 1}
	}
	result, err := pdao.YoubanPublishCollectSource.Ctx(ctx).Where("id", sourceId).Where("tenant_id", tenantId).Where("account_id", accountId).WhereNull("deleted_at").Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集源审批状态失败")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return gerror.New("采集源不存在或已失效")
	}
	return nil
}
