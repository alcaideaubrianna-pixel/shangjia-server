package sys

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type aiOpsTelegramMessageRef struct {
	Username  string
	ChatId    string
	MessageId int64
}

func (s *sSysPublish) AIOpsTelegramMessageContext(ctx context.Context, messageURL string) (*sysin.AIOpsTelegramMessageContextModel, error) {
	ref, err := parseAIOpsTelegramMessageURL(messageURL)
	if err != nil {
		return nil, err
	}
	result := &sysin.AIOpsTelegramMessageContextModel{
		MessageURL: messageURL, MessageUsername: ref.Username, MessageChatId: ref.ChatId, MessageId: ref.MessageId,
	}
	if err = s.resolveAIOpsPublishedMessage(ctx, ref, result); err != nil {
		return nil, err
	}
	if !result.Found {
		if err = s.resolveAIOpsCollectedMessage(ctx, ref, result); err != nil {
			return nil, err
		}
	}
	if !result.Found {
		result.Diagnosis = "未找到消息映射；历史发送明细可能已按保留策略清理，或该消息不属于当前系统"
		return result, nil
	}
	if err = s.fillAIOpsCollectionContext(ctx, result); err != nil {
		return nil, err
	}
	g.Log().Info(ctx, "AI运维查询Telegram消息链路", g.Map{
		"url": messageURL, "matchType": result.MatchType, "profileId": result.ProfileId, "eventId": result.CollectEventId,
	})
	return result, nil
}

func (s *sSysPublish) resolveAIOpsPublishedMessage(ctx context.Context, ref aiOpsTelegramMessageRef, result *sysin.AIOpsTelegramMessageContextModel) error {
	mod := g.DB().Model("hg_youban_publish_tg_message m").Ctx(ctx).
		Fields("m.job_id,m.profile_id,j.status AS job_status,j.channel_id,j.collect_event_id,j.collect_source_id,j.collect_source_chat_id,j.collect_source_message_id,c.channel_title").
		LeftJoin("hg_youban_publish_tg_job j", "j.id=m.job_id").
		LeftJoin("hg_youban_publish_channel c", "c.id=j.channel_id").
		Where("m.tg_message_id", ref.MessageId).
		WhereNull("m.deleted_at")
	if ref.Username != "" {
		mod = mod.Where("LOWER(TRIM(LEADING '@' FROM c.channel_username))", strings.ToLower(ref.Username))
	} else {
		mod = mod.Where("m.target_chat_id", ref.ChatId)
	}
	row, err := mod.OrderDesc("m.id").One()
	if err != nil {
		return gerror.Wrap(err, "查询Telegram发送消息失败")
	}
	if row.IsEmpty() {
		return nil
	}
	result.Found, result.MatchType = true, "published_message"
	result.JobId, result.ProfileId = row["job_id"].Int64(), row["profile_id"].Int64()
	result.JobStatus, result.ChannelId = row["job_status"].String(), row["channel_id"].Int64()
	result.ChannelTitle = row["channel_title"].String()
	result.CollectEventId, result.CollectSourceId = row["collect_event_id"].Int64(), row["collect_source_id"].Int64()
	result.CollectSourceChatId = row["collect_source_chat_id"].String()
	result.CollectSourceMessageId = row["collect_source_message_id"].Int64()
	return nil
}

func (s *sSysPublish) resolveAIOpsCollectedMessage(ctx context.Context, ref aiOpsTelegramMessageRef, result *sysin.AIOpsTelegramMessageContextModel) error {
	mod := g.DB().Model("hg_youban_publish_collect_event e").Ctx(ctx).
		Fields("e.id,e.source_id,e.source_chat_id,e.source_message_id,e.status,d.profile_id,d.rule_id").
		LeftJoin("hg_youban_publish_collect_source s", "s.id=e.source_id").
		LeftJoin("hg_youban_publish_collect_dispatch d", "d.event_id=e.id").
		Where("e.source_message_id", ref.MessageId)
	if ref.Username != "" {
		mod = mod.Where("LOWER(TRIM(LEADING '@' FROM s.source_username))", strings.ToLower(ref.Username))
	} else {
		mod = mod.Where("e.source_chat_id", ref.ChatId)
	}
	row, err := mod.OrderDesc("d.id").OrderDesc("e.id").One()
	if err != nil {
		return gerror.Wrap(err, "查询Telegram采集消息失败")
	}
	if row.IsEmpty() {
		return nil
	}
	result.Found, result.MatchType = true, "collected_message"
	result.CollectEventId, result.CollectEventStatus = row["id"].Int64(), row["status"].String()
	result.CollectSourceId = row["source_id"].Int64()
	result.CollectSourceChatId, result.CollectSourceMessageId = row["source_chat_id"].String(), row["source_message_id"].Int64()
	result.ProfileId, result.CollectRuleId = row["profile_id"].Int64(), row["rule_id"].Int64()
	return nil
}

func (s *sSysPublish) fillAIOpsCollectionContext(ctx context.Context, result *sysin.AIOpsTelegramMessageContextModel) error {
	if result.ProfileId > 0 {
		profile, err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
			Fields("profile_no,status").Where("id", result.ProfileId).One()
		if err != nil {
			return gerror.Wrap(err, "读取资料失败")
		}
		result.ProfileNo, result.ProfileStatus = profile["profile_no"].String(), profile["status"].Int()
	}
	if result.ProfileId > 0 && (result.CollectEventId <= 0 || result.CollectRuleId <= 0 || result.CollectSourceId <= 0) {
		dispatch, err := g.DB().Model("hg_youban_publish_collect_dispatch").Ctx(ctx).
			Where("profile_id", result.ProfileId).OrderDesc("id").One()
		if err != nil {
			return gerror.Wrap(err, "读取采集分发记录失败")
		}
		if !dispatch.IsEmpty() {
			if result.CollectEventId <= 0 {
				result.CollectEventId = dispatch["event_id"].Int64()
			}
			if result.CollectSourceId <= 0 {
				result.CollectSourceId = dispatch["source_id"].Int64()
			}
			if result.CollectRuleId <= 0 {
				result.CollectRuleId = dispatch["rule_id"].Int64()
			}
			if result.CollectSourceChatId == "" {
				result.CollectSourceChatId = dispatch["source_chat_id_snapshot"].String()
			}
			if result.CollectSourceMessageId <= 0 {
				result.CollectSourceMessageId = dispatch["source_message_id_snapshot"].Int64()
			}
		}
	}
	if result.CollectEventId > 0 {
		if err := s.fillAIOpsCollectEvent(ctx, result); err != nil {
			return err
		}
	}
	if result.CollectSourceId > 0 {
		source, err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).
			Where("id", result.CollectSourceId).One()
		if err != nil {
			return gerror.Wrap(err, "读取采集来源失败")
		}
		result.CollectSourceTitle = source["title"].String()
		result.CollectSourceUsername = source["source_username"].String()
		if result.CollectSourceChatId == "" {
			result.CollectSourceChatId = source["source_chat_id"].String()
		}
	}
	result.CollectSourceURL = collectedTelegramMessageURL(
		result.CollectSourceChatId, result.CollectSourceUsername, result.CollectSourceMessageId,
	)
	if result.CollectRuleId > 0 {
		rule, err := g.DB().Model("hg_youban_publish_collect_rule").Ctx(ctx).
			Fields("name,status").Where("id", result.CollectRuleId).One()
		if err != nil {
			return gerror.Wrap(err, "读取采集规则失败")
		}
		result.CollectRuleName, result.CollectRuleStatus = rule["name"].String(), rule["status"].Int()
	}
	if result.VerifyMediaCount > 0 {
		result.Diagnosis = "验证媒体已采集"
	} else {
		result.Diagnosis = "未发现验证媒体，请检查源消息配对和媒体缓存阶段"
	}
	return nil
}

func (s *sSysPublish) fillAIOpsCollectEvent(ctx context.Context, result *sysin.AIOpsTelegramMessageContextModel) error {
	event, err := g.DB().Model("hg_youban_publish_collect_event").Ctx(ctx).
		Where("id", result.CollectEventId).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	if !event.IsEmpty() {
		result.CollectEventStatus = event["status"].String()
		if result.CollectSourceId <= 0 {
			result.CollectSourceId = event["source_id"].Int64()
		}
		if result.CollectSourceChatId == "" {
			result.CollectSourceChatId = event["source_chat_id"].String()
		}
		if result.CollectSourceMessageId <= 0 {
			result.CollectSourceMessageId = event["source_message_id"].Int64()
		}
	}
	counts, err := g.DB().Model("hg_youban_publish_collect_event_media em").Ctx(ctx).
		Fields("COUNT(*) FILTER (WHERE COALESCE(em.media_type,'') <> 'video') AS display_count,COUNT(*) FILTER (WHERE em.media_type='video') AS verify_count").
		Where("em.event_id=? OR em.event_id IN (SELECT id FROM hg_youban_publish_collect_event WHERE material_parent_event_id=?)", result.CollectEventId, result.CollectEventId).
		One()
	if err != nil {
		return gerror.Wrap(err, "统计采集媒体失败")
	}
	result.DisplayMediaCount, result.VerifyMediaCount = counts["display_count"].Int(), counts["verify_count"].Int()
	return nil
}

func parseAIOpsTelegramMessageURL(raw string) (aiOpsTelegramMessageRef, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Hostname(), "t.me") {
		return aiOpsTelegramMessageRef{}, gerror.New("Telegram消息地址格式不正确")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 && !(len(parts) == 3 && parts[0] == "c") {
		return aiOpsTelegramMessageRef{}, gerror.New("Telegram消息地址必须包含频道和消息ID")
	}
	messageId, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil || messageId <= 0 {
		return aiOpsTelegramMessageRef{}, gerror.New("Telegram消息ID不正确")
	}
	ref := aiOpsTelegramMessageRef{MessageId: messageId}
	if parts[0] == "c" {
		if _, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return aiOpsTelegramMessageRef{}, gerror.New("Telegram私有频道ID不正确")
		}
		ref.ChatId = "-100" + strings.TrimPrefix(parts[1], "-100")
	} else {
		ref.Username = strings.TrimPrefix(parts[0], "@")
	}
	return ref, nil
}
