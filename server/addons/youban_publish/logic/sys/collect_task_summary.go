package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
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
	return g.DB().Model(pdao.YoubanPublishCollectDispatch.Table()+" t").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=t.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		LeftJoin(publishTenantTable+" m", "m.id=ps.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=ps.account_id").
		Fields("t.id,ps.tenant_id,ps.account_id,p.title,p.city,CASE WHEN t.status = 'failed' THEN 'failed' ELSE 'pending' END AS status,t.updated_at,m.name AS tenant_name,a.nickname AS account_nickname,a.username AS account_username").
		WhereIn("t.status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusFailed})
}
