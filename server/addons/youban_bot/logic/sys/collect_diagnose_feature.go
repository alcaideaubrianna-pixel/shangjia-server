package sys

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishservice "hotgo/addons/youban_publish/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

const (
	collectSourceDiagnoseFeatureKey = "collect_source_diagnose"
	collectEventDiagnoseFeatureKey  = "collect_event_diagnose"
	collectDiagnoseAdminWhitelist   = "collectDiagnoseAdminWhitelist"
)

type collectSourceDiagnoseFeature struct{}

func (collectSourceDiagnoseFeature) Key() string         { return collectSourceDiagnoseFeatureKey }
func (collectSourceDiagnoseFeature) Command() string     { return "source" }
func (collectSourceDiagnoseFeature) Description() string { return "查询Telegram消息来源" }
func (collectSourceDiagnoseFeature) Priority() int       { return 205 }
func (collectSourceDiagnoseFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

func (collectSourceDiagnoseFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return append(collectDiagnoseConfigSchema(), &sysin.FeatureConfigSchema{
		Field: "collectDiagnoseAdminWhitelist", Label: "采集调试管理员Telegram用户ID白名单", Component: "input", Default: "",
	})
}

func (f collectSourceDiagnoseFeature) Handle(ctx context.Context, bot *sSysBot, fc *botFeatureContext) (bool, error) {
	if !collectDiagnoseAllowed(ctx, bot, fc, f.Key()) {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "无权限使用该调试命令。")
	}
	ref, err := parseCollectDiagnoseURL(fc.Args)
	if err != nil {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "用法：/source https://t.me/channel/123456")
	}
	result, err := publishservice.SysPublish().AIOpsTelegramMessageContext(ctx, &publishsysin.AIOpsTelegramMessageContextInp{
		URL: ref.Raw,
	})
	if err != nil {
		return true, err
	}
	return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), formatCollectSourceResult(result, ref))
}

type collectEventDiagnoseFeature struct{}

func (collectEventDiagnoseFeature) Key() string         { return collectEventDiagnoseFeatureKey }
func (collectEventDiagnoseFeature) Command() string     { return "collect" }
func (collectEventDiagnoseFeature) Description() string { return "查询Telegram采集链路" }
func (collectEventDiagnoseFeature) Priority() int       { return 204 }
func (collectEventDiagnoseFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

func (collectEventDiagnoseFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return collectDiagnoseConfigSchema()
}

func (f collectEventDiagnoseFeature) Handle(ctx context.Context, bot *sSysBot, fc *botFeatureContext) (bool, error) {
	if !collectDiagnoseAllowed(ctx, bot, fc, f.Key()) {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "无权限使用该调试命令。")
	}
	ref, err := parseCollectDiagnoseURL(fc.Args)
	if err != nil {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "用法：/collect https://t.me/channel/123456")
	}
	text, err := diagnoseCollectBranches(ctx, ref)
	if err != nil {
		return true, err
	}
	return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), text)
}

// These commands are intentionally hidden from both the Telegram command menu
// and the reply keyboard. They remain callable by exact command text.
func collectDiagnoseConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "menuVisible", Label: "菜单可见", Component: "switch", Default: 0},
	}
}

type collectDiagnoseRef struct {
	Raw       string
	Username  string
	ChatID    string
	MessageID int64
}

func parseCollectDiagnoseURL(raw string) (collectDiagnoseRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return collectDiagnoseRef{}, gerror.New("Telegram链接不能为空")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || !strings.EqualFold(u.Hostname(), "t.me") {
		return collectDiagnoseRef{}, gerror.New("Telegram链接格式不正确")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 && !(len(parts) == 3 && parts[0] == "c") {
		return collectDiagnoseRef{}, gerror.New("Telegram链接必须包含频道和消息ID")
	}
	messageID, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil || messageID <= 0 {
		return collectDiagnoseRef{}, gerror.New("Telegram消息ID不正确")
	}
	ref := collectDiagnoseRef{Raw: raw, MessageID: messageID}
	if parts[0] == "c" {
		if _, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return collectDiagnoseRef{}, gerror.New("Telegram频道ID不正确")
		}
		ref.ChatID = normalizeCollectDiagnoseChatID(parts[1])
	} else {
		ref.Username = strings.TrimPrefix(strings.ToLower(parts[0]), "@")
	}
	return ref, nil
}

func normalizeCollectDiagnoseChatID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "-100")
	return "-100" + value
}

func collectDiagnoseAllowed(ctx context.Context, bot *sSysBot, fc *botFeatureContext, key string) bool {
	if bot == nil || fc == nil || fc.Msg == nil || fc.Msg.From == nil {
		return false
	}
	raw := strings.TrimSpace(bot.featureConfigValue(ctx, collectSourceDiagnoseFeatureKey, collectDiagnoseAdminWhitelist))
	if raw == "" {
		raw = strings.TrimSpace(bot.featureConfigValue(ctx, collectEventDiagnoseFeatureKey, collectDiagnoseAdminWhitelist))
	}
	if raw == "" {
		return false
	}
	userID := strconv.FormatInt(fc.Msg.From.ID, 10)
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';'
	}) {
		if strings.TrimSpace(item) == userID {
			return true
		}
	}
	return false
}

func formatCollectSourceResult(result *publishsysin.AIOpsTelegramMessageContextModel, ref collectDiagnoseRef) string {
	if result == nil || !result.Found {
		return fmt.Sprintf("来源查询\n\n消息：%s\n\n未找到系统资料、采集事件或发布记录。", ref.Raw)
	}
	method := result.CollectMethod
	if method == "" {
		method = result.SourceType
	}
	if method == "" {
		method = "系统上架"
	}
	return fmt.Sprintf("来源查询\n\n编号：%s\n资料状态：%d\n归属租户：%d\n归属账户：%d\n上架方式：%s\n采集来源：%s\n来源消息：%s\n采集规则：%s（%d）\n采集事件：%d\n资料ID：%d",
		emptyDiagnoseValue(result.ProfileNo), result.ProfileStatus, result.TenantId, result.AccountId,
		method, emptyDiagnoseValue(result.CollectSourceTitle), ref.Raw, emptyDiagnoseValue(result.CollectRuleName),
		result.CollectRuleId, result.CollectEventId, result.ProfileId)
}

type collectDiagnoseBranch struct {
	TenantID, AccountID, SourceID, EventID, DispatchID, RuleID, ProfileID, JobID                int64
	SourceType, SourceTitle, EventStatus, DispatchStatus, RuleName, ProfileNo, JobStatus, Error string
}

func diagnoseCollectBranches(ctx context.Context, ref collectDiagnoseRef) (string, error) {
	chatIDs := []string{ref.ChatID}
	if ref.Username != "" {
		// The source username is only used to resolve the channel identity.
	} else {
		chatIDs = []string{ref.ChatID, strings.TrimPrefix(ref.ChatID, "-100")}
	}
	model := g.DB().Model("hg_youban_publish_collect_event e").Ctx(ctx).
		LeftJoin("hg_youban_publish_collect_source s", "s.id=e.source_id").
		LeftJoin("hg_youban_publish_collect_dispatch d", "d.event_id=e.id").
		LeftJoin("hg_youban_publish_collect_rule r", "r.id=d.rule_id").
		LeftJoin("hg_content_profile p", "p.id=d.profile_id").
		LeftJoin("hg_youban_publish_tg_job j", "j.collect_event_id=e.id AND j.profile_id=d.profile_id").
		Fields("e.tenant_id,e.account_id,e.source_id,e.id AS event_id,d.id AS dispatch_id,d.rule_id,d.profile_id,s.source_type,s.title AS source_title,e.status AS event_status,d.status AS dispatch_status,r.name AS rule_name,p.profile_no,j.id AS job_id,j.status AS job_status,COALESCE(e.error_message,d.error_message,'') AS error_message").
		Where("e.source_message_id", ref.MessageID)
	if ref.Username != "" {
		model = model.Where("LOWER(TRIM(LEADING '@' FROM s.source_username))", strings.ToLower(ref.Username))
	} else {
		model = model.WhereIn("e.source_chat_id", chatIDs)
	}
	rows, err := model.OrderAsc("e.tenant_id").OrderAsc("e.account_id").OrderAsc("e.source_id").OrderDesc("e.id").All()
	if err != nil {
		return "", gerror.Wrap(err, "查询采集链路失败")
	}
	if len(rows) == 0 {
		return diagnoseCollectorFallback(ctx, ref)
	}
	branches := make([]collectDiagnoseBranch, 0, len(rows))
	for _, row := range rows {
		branches = append(branches, collectDiagnoseBranch{
			TenantID: row["tenant_id"].Int64(), AccountID: row["account_id"].Int64(), SourceID: row["source_id"].Int64(),
			EventID: row["event_id"].Int64(), DispatchID: row["dispatch_id"].Int64(), RuleID: row["rule_id"].Int64(), ProfileID: row["profile_id"].Int64(), JobID: row["job_id"].Int64(),
			SourceType: row["source_type"].String(), SourceTitle: row["source_title"].String(), EventStatus: row["event_status"].String(),
			DispatchStatus: row["dispatch_status"].String(), RuleName: row["rule_name"].String(), ProfileNo: row["profile_no"].String(),
			JobStatus: row["job_status"].String(), Error: row["error_message"].String(),
		})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "采集查询\n\n消息：%s\n发现 %d 个采集分支\n", ref.Raw, len(branches))
	for i, item := range branches {
		fmt.Fprintf(&b, "\n来源 %d\n租户：%d\n账户：%d\n采集源：%s（%d）\n采集方式：%s\n采集事件：%d\n事件状态：%s\n规则：%s（%d）\n分发：%s（%d）\n资料：%s（%d）\nJob：%s（%d）",
			i+1, item.TenantID, item.AccountID, emptyDiagnoseValue(item.SourceTitle), item.SourceID, emptyDiagnoseValue(item.SourceType),
			item.EventID, emptyDiagnoseValue(item.EventStatus), emptyDiagnoseValue(item.RuleName), item.RuleID, emptyDiagnoseValue(item.DispatchStatus),
			emptyDiagnoseValue(item.ProfileNo), item.ProfileID, emptyDiagnoseValue(item.JobStatus), item.JobID)
		if item.Error != "" {
			fmt.Fprintf(&b, "\n错误：%s", item.Error)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func diagnoseCollectorFallback(ctx context.Context, ref collectDiagnoseRef) (string, error) {
	chatIDs := []string{ref.ChatID, strings.TrimPrefix(ref.ChatID, "-100")}
	if ref.Username != "" {
		source, sourceErr := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).
			Fields("source_chat_id").Where("LOWER(TRIM(LEADING '@' FROM source_username))", strings.ToLower(ref.Username)).
			WhereNull("deleted_at").OrderDesc("id").One()
		if sourceErr != nil {
			return "", gerror.Wrap(sourceErr, "查询采集源频道失败")
		}
		if source.IsEmpty() {
			return fmt.Sprintf("采集查询\n\n消息：%s\n\n未找到对应采集源。\n结论：该公开频道当前没有可识别的采集源配置。", ref.Raw), nil
		}
		chatIDs = []string{source["source_chat_id"].String(), strings.TrimPrefix(source["source_chat_id"].String(), "-100")}
	}
	model := g.DB().Model("hg_tg_collector_event").Ctx(ctx).
		Fields("id,source_id,tenant_id,account_id,chat_id,message_id,status,received_at,processed_at,error_message").
		Where("message_id", ref.MessageID)
	model = model.WhereIn("chat_id", chatIDs)
	row, err := model.OrderDesc("id").One()
	if err != nil {
		return "", gerror.Wrap(err, "查询Collector原始事件失败")
	}
	if row.IsEmpty() {
		return fmt.Sprintf("采集查询\n\n消息：%s\n\n未发现采集事件。\n结论：可能未进入Collector、采集源未配置、消息被入口过滤，或历史事实已清理。", ref.Raw), nil
	}
	accessText, accessErr := diagnoseCollectorAccess(ctx, row["tenant_id"].Int64(), row["source_id"].Int64(), row["chat_id"].String())
	if accessErr != nil {
		return "", accessErr
	}
	conclusion := "消息已经进入Collector，但尚未生成发布采集事件。"
	if accessText != "" {
		conclusion = "消息已经进入Collector，但发布侧跳过了该消息：" + accessText
	}
	return fmt.Sprintf("采集查询\n\n消息：%s\n\nCollector原始事件：%d\n状态：%s\n采集源：%d\n租户：%d\n账户：%d\n收到时间：%s\n处理时间：%s\n错误：%s\n%s\n\n结论：%s", ref.Raw,
		row["id"].Int64(), emptyDiagnoseValue(row["status"].String()), row["source_id"].Int64(), row["tenant_id"].Int64(), row["account_id"].Int64(),
		emptyDiagnoseValue(row["received_at"].String()), emptyDiagnoseValue(row["processed_at"].String()), emptyDiagnoseValue(row["error_message"].String()),
		accessText, conclusion), nil
}

func diagnoseCollectorAccess(ctx context.Context, tenantID, sourceID int64, chatID string) (string, error) {
	if tenantID <= 0 || sourceID <= 0 || strings.TrimSpace(chatID) == "" {
		return "", nil
	}
	source, err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).
		Fields("account_id").Where("id", sourceID).Where("tenant_id", tenantID).WhereNull("deleted_at").One()
	if err != nil {
		return "", gerror.Wrap(err, "读取采集源授权归属失败")
	}
	if source.IsEmpty() {
		return "source_unavailable（采集源不存在或已删除）", nil
	}
	channels, err := g.DB().Model("hg_youban_publish_collect_channel").Ctx(ctx).
		Fields("id,title,telegram_chat_id").Where("tenant_id", tenantID).
		WhereIn("telegram_chat_id", []string{chatID, strings.TrimPrefix(chatID, "-100")}).
		WhereNull("deleted_at").OrderDesc("id").Limit(2).All()
	if err != nil {
		return "", gerror.Wrap(err, "读取实际采集频道失败")
	}
	if len(channels) == 0 {
		return "channel_unregistered（实际频道未登记）", nil
	}
	channelIDs := make([]int64, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel["id"].Int64())
	}
	relation, err := g.DB().Model("hg_youban_publish_collect_account_channel").Ctx(ctx).
		Fields("status,channel_id").Where("tenant_id", tenantID).
		Where("account_id", source["account_id"].Int64()).Where("source_id", sourceID).
		WhereIn("channel_id", channelIDs).WhereNull("deleted_at").
		OrderDesc("updated_at").OrderDesc("id").Limit(1).One()
	if err != nil {
		return "", gerror.Wrap(err, "读取频道采集授权失败")
	}
	if relation.IsEmpty() {
		return "approval_missing（未找到账户与采集源的频道授权关系）", nil
	}
	status := strings.ToLower(strings.TrimSpace(relation["status"].String()))
	if status != "enabled" {
		return fmt.Sprintf("approval_blocked（频道采集授权为%s，channelId=%d）", emptyDiagnoseValue(status), relation["channel_id"].Int64()), nil
	}
	return "", nil
}

func emptyDiagnoseValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "无"
	}
	return strings.TrimSpace(value)
}
