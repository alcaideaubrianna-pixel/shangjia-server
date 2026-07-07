package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectReviewList(ctx context.Context, in *sysin.CollectReviewListInp) (list []*sysin.CollectReviewModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectReviewListInp{}
	}
	mod := pdao.YoubanPublishCollectReview.DB().Model(pdao.YoubanPublishCollectReview.Table()+" r").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=r.source_id").
		LeftJoin(pdao.YoubanPublishCollectRule.Table()+" rule", "rule.id=r.rule_id").
		Where("r.tenant_id", account.TenantId).
		Where("r.account_id", account.Id)
	if in.Status != "" {
		mod = mod.Where("r.status", strings.TrimSpace(in.Status))
	}
	if in.SourceId > 0 {
		mod = mod.Where("r.source_id", in.SourceId)
	}
	if in.RuleId > 0 {
		mod = mod.Where("r.rule_id", in.RuleId)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("r.raw_text", "%"+keyword+"%")
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集审核失败")
	}
	fields := "r.*,s.title AS source_title,rule.name AS rule_name"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("r.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集审核失败")
	}
	return
}

func (s *sSysPublish) CollectReviewAction(ctx context.Context, in *sysin.CollectReviewActionInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("审核参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectReview.Ctx(ctx).
		WhereIn("id", uniqueIds(in.Ids)).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("status", sysin.CollectReviewStatusPending).
		Data(g.Map{
			"status":        in.Status,
			"review_reason": in.Reason,
			"reviewed_by":   account.Id,
			"reviewed_at":   now,
			"updated_at":    now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集审核失败")
	}
	if in.Status == sysin.CollectReviewStatusApproved {
		for _, id := range uniqueIds(in.Ids) {
			if err = s.approveCollectReview(ctx, id, account.TenantId, account.Id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) approveCollectReview(ctx context.Context, reviewId int64, tenantId int64, accountId int64) error {
	review, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("id", reviewId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集审核失败")
	}
	if review.IsEmpty() || review["status"].String() != sysin.CollectReviewStatusApproved {
		return nil
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", review["event_id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).Where("id", review["rule_id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集规则失败")
	}
	if event.IsEmpty() || rule.IsEmpty() {
		return gerror.New("采集审核关联数据不存在")
	}
	taskId, err := s.createCollectPublishTask(ctx, event, rule, review["raw_text"].String())
	if err != nil {
		return err
	}
	if err = s.ensureCollectTgJobs(ctx, taskId, rule); err != nil {
		return err
	}
	return s.markCollectDispatchQueued(ctx, review["dispatch_id"].Int64(), taskId)
}
