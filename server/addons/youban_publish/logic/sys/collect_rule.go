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

func (s *sSysPublish) CollectRuleList(ctx context.Context, in *sysin.CollectRuleListInp) (list []*sysin.CollectRuleModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err = ensureCollectRuleColumns(ctx); err != nil {
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
	return
}

func (s *sSysPublish) CollectRuleSave(ctx context.Context, in *sysin.CollectRuleSaveInp) (id int64, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	if err = ensureCollectRuleColumns(ctx); err != nil {
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
		"tenant_id":              account.TenantId,
		"account_id":             account.Id,
		"name":                   in.Name,
		"global_enabled":         switchInt(in.GlobalEnabled),
		"target_channel_id_json": in.TargetChannelIdJson,
		"bot_id_json":            in.BotIdJson,
		"backup_channel_id":      in.BackupChannelId,
		"backup_channel_id_json": in.BackupChannelIdJson,
		"review_enabled":         switchInt(in.ReviewEnabled),
		"dedupe_enabled":         switchDefaultOn(in.DedupeEnabled),
		"dedupe_days":            in.DedupeDays,
		"full_match_enabled":     switchInt(in.FullMatchEnabled),
		"keyword_json":           in.KeywordJson,
		"tag_json":               in.TagJson,
		"replace_json":           in.ReplaceJson,
		"delete_line_text_json":  in.DeleteLineTextJson,
		"delete_text_json":       in.DeleteTextJson,
		"block_text_json":        in.BlockTextJson,
		"block_link":             switchDefaultOn(in.BlockLink),
		"block_username":         switchDefaultOn(in.BlockUsername),
		"block_plain_text":       switchDefaultOn(in.BlockPlainText),
		"show_unique_no":         switchInt(in.ShowUniqueNo),
		"header_enabled":         switchInt(in.HeaderEnabled),
		"header_markdown":        in.HeaderMarkdown,
		"footer_enabled":         switchInt(in.FooterEnabled),
		"footer_markdown":        in.FooterMarkdown,
		"sort":                   in.Sort,
		"status":                 in.Status,
		"updated_by":             account.Id,
		"updated_at":             now,
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishCollectRule.Ctx(ctx).
			Where("id", in.Id).
			Where("tenant_id", account.TenantId).
			Where("account_id", account.Id).
			WhereNull("deleted_at").
			Data(data).
			Update()
		if err != nil {
			return 0, gerror.Wrap(err, "更新采集规则失败")
		}
		s.refreshCollectEventRulesCache(ctx)
		s.refreshPendingCollectTasksForRuleAsync(in.Id, account.TenantId, account.Id)
		return in.Id, nil
	}
	data["created_by"] = account.Id
	data["created_at"] = now
	id, err = pdao.YoubanPublishCollectRule.Ctx(ctx).Data(data).InsertAndGetId()
	if err == nil {
		s.refreshCollectEventRulesCache(ctx)
	}
	return id, gerror.Wrap(err, "创建采集规则失败")
}

func (s *sSysPublish) CollectRuleDelete(ctx context.Context, in *sysin.IdsInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择采集规则")
	}
	_, err = pdao.YoubanPublishCollectRule.Ctx(ctx).
		WhereIn("id", uniqueIds(in.Ids)).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Data(g.Map{"deleted_at": gtime.Now(), "deleted_by": account.Id}).
		Update()
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
