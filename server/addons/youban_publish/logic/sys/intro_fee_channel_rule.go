package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
)

const introFeeChannelRuleTable = "hg_youban_publish_intro_fee_channel_rule"

func (s *sSysPublish) resolveIntroFeeChannelSuffix(ctx context.Context, event gdb.Record, rule gdb.Record) (string, bool, error) {
	chatID := strings.TrimSpace(event["source_chat_id"].String())
	if chatID == "" {
		return "", false, nil
	}
	botID := event["bot_id"].Int64()
	if botID == 0 {
		botID = rule["bot_id"].Int64()
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).
		Fields("suffix").Where("tenant_id", event["tenant_id"]).Where("rule_id", rule["id"]).Where("channel_id", chatID).Where("status", 1).WhereNull("deleted_at")
	if botID > 0 {
		mod = mod.Where("bot_id", botID)
	} else {
		mod = mod.Where("bot_id", 0)
	}
	row, err := mod.OrderDesc("id").Limit(1).One()
	if err != nil {
		return "", false, gerror.Wrap(err, "读取介绍费频道后缀规则失败")
	}
	if row.IsEmpty() {
		return "", false, nil
	}
	return strings.TrimSpace(row["suffix"].String()), true, nil
}

func (s *sSysPublish) IntroFeeChannelRuleList(ctx context.Context, in *sysin.IntroFeeChannelRuleListInp) (list []*sysin.IntroFeeChannelRuleItem, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.IntroFeeChannelRuleListInp{}
	}
	mod := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", account.TenantId).Where("rule_id", in.RuleId).Where("bot_id", in.BotId).WhereNull("deleted_at")
	if kw := strings.TrimSpace(in.Keyword); kw != "" {
		like := "%" + kw + "%"
		mod = mod.Where("(channel_id LIKE ? OR channel_title LIKE ? OR channel_username LIKE ?)", like, like, like)
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("updated_at").ScanAndCount(&list, &totalCount, false)
	return
}

func (s *sSysPublish) IntroFeeChannelRuleSave(ctx context.Context, in *sysin.IntroFeeChannelRuleSaveInp) (int64, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil || in.RuleId <= 0 || strings.TrimSpace(in.ChannelId) == "" {
		return 0, gerror.New("规则ID和频道ID不能为空")
	}
	now := gtime.Now()
	data := g.Map{"tenant_id": account.TenantId, "rule_id": in.RuleId, "bot_id": in.BotId, "channel_id": strings.TrimSpace(in.ChannelId), "suffix": strings.TrimSpace(in.Suffix), "status": in.Status, "updated_at": now}
	if in.Status == 0 {
		data["status"] = 1
	}
	if in.Id > 0 {
		_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", in.Id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(data).Update()
		return in.Id, err
	}
	data["created_at"] = now
	id, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Data(data).InsertAndGetId()
	return id, err
}

func (s *sSysPublish) IntroFeeChannelRuleDelete(ctx context.Context, id int64) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if id <= 0 {
		return gerror.New("规则ID无效")
	}
	_, err = g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Data(g.Map{"deleted_at": gtime.Now()}).Update()
	return err
}
