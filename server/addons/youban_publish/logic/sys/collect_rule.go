package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/model"
)

func (s *sSysPublish) CollectRuleList(ctx context.Context, in *sysin.CollectRuleListInp) (list []*sysin.CollectRuleModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectRuleListInp{}
	}
	mod := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at")
	if in.GlobalEnabled > 0 {
		mod = mod.Where("global_enabled", in.GlobalEnabled-1)
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("name", "%"+keyword+"%")
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集规则失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderAsc("sort").OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集规则失败")
	}
	if err = fillCollectRuleDetails(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) CollectRuleView(ctx context.Context, in *sysin.CollectRuleViewInp) (res *sysin.CollectRuleModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("规则ID不能为空")
	}
	if err = pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at").
		Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取采集规则失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("采集规则不存在")
	}
	if err = fillCollectRuleDetails(ctx, []*sysin.CollectRuleModel{res}); err != nil {
		return nil, err
	}
	return res, nil
}

// BotCollectRuleView reads a rule with an explicit tenant/account scope,
// allowing Telegram handlers to reuse the same rule details without relying
// on HTTP login context.
func (s *sSysPublish) BotCollectRuleView(ctx context.Context, ruleId, tenantId, accountId int64) (*sysin.CollectRuleModel, error) {
	if ruleId <= 0 || tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("规则参数不完整")
	}
	var res *sysin.CollectRuleModel
	if err := pdao.YoubanPublishCollectRule.Ctx(ctx).Where("id", ruleId).Where("tenant_id", tenantId).Where("account_id", accountId).WhereNull("deleted_at").Scan(&res); err != nil {
		return nil, gerror.Wrap(err, "获取采集规则失败")
	}
	if res == nil || res.Id <= 0 {
		return nil, gerror.New("采集规则不存在")
	}
	if err := fillCollectRuleDetails(ctx, []*sysin.CollectRuleModel{res}); err != nil {
		return nil, err
	}
	return res, nil
}

// BotCollectRuleSave persists a rule under an explicit tenant/account scope.
// The existing save implementation remains the single source of mutation;
// this adapter temporarily injects the account identity into context.
func (s *sSysPublish) BotCollectRuleSave(ctx context.Context, in *sysin.CollectRuleSaveInp, tenantId, accountId int64) (int64, error) {
	if tenantId <= 0 || accountId <= 0 {
		return 0, gerror.New("账号参数不完整")
	}
	if err := s.ensureAccountBelongsTenant(ctx, accountId, tenantId); err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("规则参数不能为空")
	}
	// Reuse the canonical save transaction by providing an explicit, scoped identity.
	scoped := context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: accountId}})
	return s.CollectRuleSave(scoped, in)
}

func fillCollectRuleDetails(ctx context.Context, list []*sysin.CollectRuleModel) error {
	ruleIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil {
			ruleIds = append(ruleIds, item.Id)
		}
	}
	channelMap, err := collectRuleChannelMap(ctx, ruleIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item != nil {
			item.TargetChannelIds = channelMap[item.Id]
		}
	}
	itemMap, err := collectRuleItemMap(ctx, ruleIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil || itemMap[item.Id] == nil {
			continue
		}
		config := itemMap[item.Id]
		item.Keywords = config.Keywords
		item.Tags = config.Tags
		item.DeleteLineTexts = config.DeleteLines
		item.DeleteTexts = config.DeleteTexts
		if config.TruncateIntroFee {
			item.TruncateIntroFeeEnabled = 1
		}
		item.IntroFeeSuffix = config.IntroFeeSuffix
		if config.IntroFeeSuffix != "" {
			item.IntroFeeSuffixEnabled = 1
		}
		item.BlockTexts = config.BlockedTexts
		item.Replacements = make([]sysin.CollectRuleReplaceModel, 0, len(config.Replacements))
		for _, replacement := range config.Replacements {
			item.Replacements = append(item.Replacements, sysin.CollectRuleReplaceModel{From: replacement.From, To: replacement.To})
		}
	}
	return nil
}

func (s *sSysPublish) CollectRuleSave(ctx context.Context, in *sysin.CollectRuleSaveInp) (id int64, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("规则参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":          account.TenantId,
		"account_id":         account.Id,
		"name":               in.Name,
		"global_enabled":     switchInt(in.GlobalEnabled),
		"review_enabled":     switchInt(in.ReviewEnabled),
		"dedupe_enabled":     switchDefaultOn(in.DedupeEnabled),
		"dedupe_days":        in.DedupeDays,
		"full_match_enabled": switchInt(in.FullMatchEnabled),
		"block_link":         switchDefaultOn(in.BlockLink),
		"block_username":     switchDefaultOn(in.BlockUsername),
		"block_plain_text":   switchDefaultOn(in.BlockPlainText),
		"header_enabled":     switchInt(in.HeaderEnabled),
		"header_markdown":    in.HeaderMarkdown,
		"footer_enabled":     switchInt(in.FooterEnabled),
		"footer_markdown":    in.FooterMarkdown,
		"sort":               in.Sort,
		"status":             in.Status,
		"updated_by":         account.Id,
		"updated_at":         now,
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if in.Id > 0 {
			result, txErr := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).
				Where("id", in.Id).Where("tenant_id", account.TenantId).Where("account_id", account.Id).
				WhereNull("deleted_at").Data(data).Update()
			if txErr != nil {
				return gerror.Wrap(txErr, "更新采集规则失败")
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				return gerror.New("采集规则不存在")
			}
			id = in.Id
		} else {
			data["created_by"] = account.Id
			data["created_at"] = now
			var txErr error
			id, txErr = tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).Data(data).InsertAndGetId()
			if txErr != nil {
				return gerror.Wrap(txErr, "创建采集规则失败")
			}
		}
		if txErr := syncCollectRuleChannelsTx(ctx, tx, account.TenantId, account.Id, id, in.TargetChannelIds); txErr != nil {
			return txErr
		}
		replacements := make([]collectReplaceRule, 0, len(in.Replacements))
		for _, replacement := range in.Replacements {
			replacements = append(replacements, collectReplaceRule{From: replacement.From, To: replacement.To})
		}
		introFeeSuffix := ""
		if in.IntroFeeSuffixEnabled == 1 {
			introFeeSuffix = in.IntroFeeSuffix
		}
		return syncCollectRuleItemsTx(ctx, tx, account.TenantId, account.Id, id, collectRuleItems{
			Keywords: in.Keywords, Tags: in.Tags, Replacements: replacements,
			DeleteLines: in.DeleteLineTexts, DeleteTexts: in.DeleteTexts, TruncateIntroFee: in.TruncateIntroFeeEnabled == 1,
			IntroFeeSuffix: introFeeSuffix, BlockedTexts: in.BlockTexts,
		})
	})
	if err != nil {
		return 0, err
	}
	s.refreshCollectEventRulesCache(ctx)
	return id, nil
}

func (s *sSysPublish) CollectRuleDelete(ctx context.Context, in *sysin.IdsInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择采集规则")
	}
	ids := uniqueIds(in.Ids)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, txErr := tx.Model(collectRuleItemTable).Ctx(ctx).WhereIn("rule_id", ids).Delete(); txErr != nil {
			return gerror.Wrap(txErr, "清理采集规则项失败")
		}
		if _, txErr := tx.Model(collectRuleChannelTable).Ctx(ctx).WhereIn("rule_id", ids).Delete(); txErr != nil {
			return gerror.Wrap(txErr, "清理采集规则频道失败")
		}
		_, txErr := tx.Model(pdao.YoubanPublishCollectRule.Table()).Ctx(ctx).
			WhereIn("id", ids).Where("tenant_id", account.TenantId).Where("account_id", account.Id).
			Data(g.Map{"deleted_at": gtime.Now(), "deleted_by": account.Id}).Update()
		return txErr
	})
	if err == nil {
		s.refreshCollectEventRulesCache(ctx)
	}
	return gerror.Wrap(err, "删除采集规则失败")
}

func switchInt(value int) int {
	if value == 1 {
		return 1
	}
	return 0
}

func switchDefaultOn(value int) int {
	if value == 0 {
		return 0
	}
	return 1
}
