package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type collectTaskSummary struct {
	Id              int64       `json:"id"`
	TenantId        int64       `json:"tenantId"`
	AccountId       int64       `json:"accountId"`
	Title           string      `json:"title"`
	City            string      `json:"city"`
	Status          string      `json:"status"`
	TenantName      string      `json:"tenantName"`
	AccountNickname string      `json:"accountNickname"`
	AccountUsername string      `json:"accountUsername"`
	UpdatedAt       *gtime.Time `json:"updatedAt"`
}

func (s *sSysPublish) collectTaskSummaryModel(ctx context.Context) *gdb.Model {
	return g.DB().Model(publishTaskTable+" t").Safe().Ctx(ctx).
		LeftJoin(publishTenantTable+" m", "m.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		Fields("t.id,t.tenant_id,t.account_id,t.title,t.city,t.status,t.updated_at,m.name AS tenant_name,a.nickname AS account_nickname,a.username AS account_username").
		Where("(t.collect_source_id > 0 OR t.collect_event_id > 0 OR t.client_request_id LIKE ?)", "collect:%").
		WhereNull("t.deleted_at")
}
