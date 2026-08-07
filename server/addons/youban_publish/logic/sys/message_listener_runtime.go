package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/tg"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

const (
	listenerGroupFlushDelay = 1200 * time.Millisecond
	listenerMaxGroups       = 1024
	listenerMaxMessages     = 32
)

type listenerMessageGroup struct {
	entities tg.Entities
	chatIds  []string
	messages []*tg.Message
	timer    *time.Timer
}

type accountMonitorGroupRuntime struct {
	Sources   []accountCollectSourceRuntime
	Listeners []accountListenPlanRuntime
}

type accountListenPlanRuntime struct {
	Id            int64                        `json:"id"`
	TenantId      int64                        `json:"tenantId"`
	TgAccountId   int64                        `json:"tgAccountId"`
	BotId         int64                        `json:"botId"`
	BindCode      string                       `json:"bindCode"`
	NotifyChatId  string                       `json:"notifyChatId"`
	NotifyTitle   string                       `json:"notifyChatTitle"`
	NotifyBoundAt *gtime.Time                  `json:"notifyBoundAt"`
	Name          string                       `json:"name"`
	Status        int                          `json:"status"`
	Keywords      []string                     `json:"keywords"`
	Targets       []accountListenTargetRuntime `json:"targets"`
}

type accountListenTargetRuntime struct {
	Id                 int64  `json:"id"`
	PlanId             int64  `json:"planId"`
	TenantId           int64  `json:"tenantId"`
	TargetChatId       string `json:"targetChatId"`
	TargetChatType     string `json:"targetChatType"`
	TargetChatTitle    string `json:"targetChatTitle"`
	TargetChatUsername string `json:"targetChatUsername"`
	Status             int    `json:"status"`
}

func (s *sSysPublish) enabledAccountMonitorGroups(ctx context.Context) (map[int64]accountMonitorGroupRuntime, error) {
	groups := make(map[int64]accountMonitorGroupRuntime)
	collectGroups, err := s.enabledAccountCollectSources(ctx)
	if err != nil {
		return nil, err
	}
	for tgAccountId, sources := range collectGroups {
		groups[tgAccountId] = accountMonitorGroupRuntime{Sources: append([]accountCollectSourceRuntime(nil), sources...)}
	}
	listenerGroups, err := s.enabledAccountListenPlans(ctx)
	if err != nil {
		return nil, err
	}
	for tgAccountId, listeners := range listenerGroups {
		group := groups[tgAccountId]
		group.Listeners = append(group.Listeners, listeners...)
		groups[tgAccountId] = group
	}
	return groups, nil
}

func (s *sSysPublish) enabledAccountListenPlans(ctx context.Context) (map[int64][]accountListenPlanRuntime, error) {
	if err := ensureMessageListenTables(ctx); err != nil {
		return nil, err
	}
	var plans []*listenerPlanRecord
	if err := g.DB().Model(messageListenPlanTable+" p").Safe().Ctx(ctx).
		InnerJoin(publishTgAccountTable+" ta", "ta.id=p.tg_account_id").
		Fields("p.id,p.tenant_id,p.name,p.tg_account_id,p.bot_id,p.bind_code,p.notify_chat_id,p.notify_chat_title,p.notify_bound_at,p.keywords_json,p.status").
		Where("p.status", publishsysin.MessageListenerStatusEnabled).
		WhereGT("p.tg_account_id", 0).
		Where("ta.status", publishsysin.PublishTgAccountStatusAuthorized).
		WhereNot("ta.session_key", "").
		WhereNull("p.deleted_at").
		WhereNull("ta.deleted_at").
		OrderAsc("p.tg_account_id").
		OrderAsc("p.id").
		Scan(&plans); err != nil {
		return nil, gerror.Wrap(err, "读取监听计划失败")
	}
	planIds := make([]int64, 0, len(plans))
	for _, plan := range plans {
		if plan != nil && plan.Id > 0 {
			planIds = append(planIds, plan.Id)
		}
	}
	targetMap, err := s.listenerPlanTargetsByPlanIds(ctx, planIds, 0)
	if err != nil {
		return nil, err
	}
	groups := make(map[int64][]accountListenPlanRuntime)
	for _, plan := range plans {
		if plan == nil || plan.Id <= 0 {
			continue
		}
		targets := targetMap[plan.Id]
		listenerTargets := make([]accountListenTargetRuntime, 0, len(targets))
		for _, target := range targets {
			if target == nil || target.Status != publishsysin.MessageListenerStatusEnabled || strings.TrimSpace(target.TargetChatId) == "" {
				continue
			}
			listenerTargets = append(listenerTargets, accountListenTargetRuntime{
				Id:                 target.Id,
				PlanId:             target.PlanId,
				TenantId:           target.TenantId,
				TargetChatId:       target.TargetChatId,
				TargetChatType:     target.TargetChatType,
				TargetChatTitle:    target.TargetChatTitle,
				TargetChatUsername: target.TargetChatUsername,
				Status:             target.Status,
			})
		}
		if len(listenerTargets) == 0 {
			continue
		}
		group := accountListenPlanRuntime{
			Id:            plan.Id,
			TenantId:      plan.TenantId,
			TgAccountId:   plan.TgAccountId,
			BotId:         plan.BotId,
			BindCode:      plan.BindCode,
			NotifyChatId:  plan.NotifyChatId,
			NotifyTitle:   plan.NotifyTitle,
			NotifyBoundAt: plan.NotifyBoundAt,
			Name:          plan.Name,
			Status:        plan.Status,
			Keywords:      listenerDecodeStringArray(plan.KeywordsJson),
			Targets:       listenerTargets,
		}
		groups[plan.TgAccountId] = append(groups[plan.TgAccountId], group)
	}
	return groups, nil
}

func accountMonitorGroupSignature(sources []accountCollectSourceRuntime, listeners []accountListenPlanRuntime) string {
	return accountCollectSourceSignature(sources) + "||" + accountListenPlanSignature(listeners)
}

func accountListenPlanSignature(listeners []accountListenPlanRuntime) string {
	parts := make([]string, 0, len(listeners))
	for _, plan := range listeners {
		if plan.Id <= 0 {
			continue
		}
		targetParts := make([]string, 0, len(plan.Targets))
		for _, target := range plan.Targets {
			if target.Id <= 0 {
				continue
			}
			targetParts = append(targetParts, fmt.Sprintf("%d:%s:%s:%s:%d",
				target.Id,
				target.TargetChatId,
				target.TargetChatType,
				target.TargetChatTitle,
				target.Status,
			))
		}
		sort.Strings(targetParts)
		parts = append(parts, fmt.Sprintf("%d:%d:%d:%d:%s:%s:%t:%s:%s",
			plan.Id,
			plan.TenantId,
			plan.TgAccountId,
			plan.Status,
			plan.Name,
			strings.Join(plan.Keywords, ","),
			plan.NotifyBoundAt != nil,
			plan.NotifyChatId,
			strings.Join(targetParts, ";"),
		))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func (w *accountCollectWorker) bufferListenerGroupedMessage(ctx context.Context, entities tg.Entities, msg *tg.Message, chatIds []string) {
	if w == nil || msg == nil {
		return
	}
	w.rememberListenerEntities(entities)
	groupedId := gotdMessageGroupedId(msg)
	if groupedId == "" {
		w.handleListenerMessage(ctx, entities, msg, chatIds)
		return
	}
	sourceChatId := firstListenerChatID(chatIds)
	if sourceChatId == "" {
		sourceChatId = listenerMessageChatID(msg)
	}
	key := sourceChatId + ":" + groupedId
	var flushGroup *listenerMessageGroup
	w.listenerGroupMu.Lock()
	group := w.listenerGroups[key]
	if group == nil {
		if len(w.listenerGroups) >= listenerMaxGroups {
			for oldKey, oldGroup := range w.listenerGroups {
				delete(w.listenerGroups, oldKey)
				if oldGroup != nil && oldGroup.timer != nil {
					oldGroup.timer.Stop()
				}
				flushGroup = oldGroup
				break
			}
		}
		group = &listenerMessageGroup{}
		w.listenerGroups[key] = group
	}
	if len(group.messages) >= listenerMaxMessages {
		delete(w.listenerGroups, key)
		if group.timer != nil {
			group.timer.Stop()
		}
		flushGroup = group
		group = &listenerMessageGroup{}
		w.listenerGroups[key] = group
	}
	group.entities = entities
	group.chatIds = uniqueStrings(append(group.chatIds, chatIds...))
	group.messages = append(group.messages, msg)
	if group.timer != nil {
		group.timer.Stop()
	}
	group.timer = time.AfterFunc(listenerGroupFlushDelay, func() {
		w.flushListenerMessageGroup(key)
	})
	w.listenerGroupMu.Unlock()
	if flushGroup != nil && len(flushGroup.messages) > 0 {
		w.handleListenerMessageGroup(ctx, flushGroup)
	}
}

func (w *accountCollectWorker) flushListenerMessageGroup(key string) {
	if w == nil {
		return
	}
	w.listenerGroupMu.Lock()
	group := w.listenerGroups[key]
	delete(w.listenerGroups, key)
	w.listenerGroupMu.Unlock()
	if group == nil || len(group.messages) == 0 {
		return
	}
	w.handleListenerMessageGroup(context.Background(), group)
}

func (w *accountCollectWorker) handleListenerMessage(ctx context.Context, entities tg.Entities, msg *tg.Message, chatIds []string) {
	if w == nil || msg == nil {
		return
	}
	_, listeners := w.configSnapshot()
	if len(listeners) == 0 {
		return
	}
	w.rememberListenerEntities(entities)
	text := listenerMessageText(msg)
	if text == "" {
		return
	}
	if listenerLooksLikeNotifyMessage(text) {
		return
	}
	sender := w.listenerMessageSender(entities, msg)
	sourceChatId := firstListenerChatID(chatIds)
	if sourceChatId == "" {
		sourceChatId = listenerMessageChatID(msg)
	}
	for _, plan := range listeners {
		if len(plan.Targets) == 0 {
			continue
		}
		for _, target := range plan.Targets {
			if !listenerTargetMatchesChat(target, chatIds) {
				continue
			}
			if plan.Status != publishsysin.MessageListenerStatusEnabled {
				continue
			}
			matchedKeywords := listenerMatchedKeywords(text, plan.Keywords)
			if len(matchedKeywords) == 0 {
				continue
			}
			g.Log().Infof(ctx, "监听计划命中 tgAccountId:%d plan:%d target:%d chat:%s keywords:%s out:%t", plan.TgAccountId, plan.Id, target.Id, sourceChatId, strings.Join(matchedKeywords, ","), msg.Out)
			if err := w.service.listenerNotifyMatch(ctx, plan, target, entities, []*tg.Message{msg}, sourceChatId, target.TargetChatTitle, sender, text, matchedKeywords, nil, listenerMessagesMediaHash([]*tg.Message{msg})); err != nil {
				g.Log().Warningf(ctx, "推送监听命中消息失败 plan:%d target:%d err:%+v", plan.Id, target.Id, err)
			}
		}
	}
}

func (w *accountCollectWorker) handleListenerMessageGroup(ctx context.Context, group *listenerMessageGroup) {
	if w == nil || group == nil || len(group.messages) == 0 {
		return
	}
	_, listeners := w.configSnapshot()
	if len(listeners) == 0 {
		return
	}
	sort.Slice(group.messages, func(i, j int) bool {
		return group.messages[i].ID < group.messages[j].ID
	})
	text := listenerGroupedMessageText(group.messages)
	if text == "" {
		return
	}
	if listenerLooksLikeNotifyMessage(text) {
		return
	}
	msg := group.messages[0]
	sender := w.listenerMessageSender(group.entities, msg)
	sourceChatId := firstListenerChatID(group.chatIds)
	if sourceChatId == "" {
		sourceChatId = listenerMessageChatID(msg)
	}
	sourceMessageIds := listenerGroupedMessageIDs(group.messages)
	mediaHash := listenerMessagesMediaHash(group.messages)
	for _, plan := range listeners {
		if len(plan.Targets) == 0 {
			continue
		}
		for _, target := range plan.Targets {
			if !listenerTargetMatchesChat(target, group.chatIds) {
				continue
			}
			if plan.Status != publishsysin.MessageListenerStatusEnabled {
				continue
			}
			matchedKeywords := listenerMatchedKeywords(text, plan.Keywords)
			if len(matchedKeywords) == 0 {
				continue
			}
			g.Log().Infof(ctx, "监听计划命中媒体组 tgAccountId:%d plan:%d target:%d chat:%s messages:%d keywords:%s", plan.TgAccountId, plan.Id, target.Id, sourceChatId, len(sourceMessageIds), strings.Join(matchedKeywords, ","))
			if err := w.service.listenerNotifyMatch(ctx, plan, target, group.entities, group.messages, sourceChatId, target.TargetChatTitle, sender, text, matchedKeywords, sourceMessageIds, mediaHash); err != nil {
				g.Log().Warningf(ctx, "推送监听命中媒体组失败 plan:%d target:%d err:%+v", plan.Id, target.Id, err)
			}
		}
	}
}

func listenerLooksLikeNotifyMessage(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return strings.Contains(text, "关键字命中") &&
		strings.Contains(text, "来源：")
}

func (s *sSysPublish) listenerNotifyMatch(ctx context.Context, plan accountListenPlanRuntime, target accountListenTargetRuntime, entities tg.Entities, sourceMessages []*tg.Message, sourceChatId string, sourceChatTitle string, sender listenerMessageSenderInfo, normalizedText string, matchedKeywords []string, sourceMessageIds []int, mediaHash string) error {
	sourceMessages = listenerNonNilMessages(sourceMessages)
	if len(sourceMessages) == 0 {
		return nil
	}
	msg := sourceMessages[0]
	notifyChatId := strings.TrimSpace(plan.NotifyChatId)
	if notifyChatId == "" || plan.NotifyBoundAt == nil {
		_, _ = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
			Where("id", plan.Id).
			WhereNull("deleted_at").
			Data(g.Map{
				"last_trigger_at": gtime.Now(),
				"last_result":     "未绑定通知目标",
				"updated_at":      gtime.Now(),
			}).
			Update()
		return nil
	}
	if len(sourceMessageIds) == 0 {
		sourceMessageIds = []int{msg.ID}
	}
	sender = s.resolveListenerSender(ctx, plan, sourceChatId, msg, sender)
	dedupeKey := listenerDedupeKey(plan.Id, sender.UserId, normalizedText, mediaHash)
	now := gtime.Now()
	if s.listenerNoticeInCooldown(ctx, plan.Id, sender, normalizedText, now) {
		return nil
	}
	notice := g.Map{
		"tenant_id":            plan.TenantId,
		"plan_id":              plan.Id,
		"target_id":            target.Id,
		"tg_account_id":        plan.TgAccountId,
		"source_chat_id":       sourceChatId,
		"source_message_id":    int64(msg.ID),
		"sender_user_id":       sender.UserId,
		"sender_username":      sender.Username,
		"normalized_text_hash": listenerHashText(normalizedText),
		"media_hash":           mediaHash,
		"dedupe_key":           dedupeKey,
		"match_keywords_json":  mustJsonEncode(matchedKeywords),
		"created_at":           now,
	}
	if _, err := g.DB().Model(messageListenNoticeTable).Safe().Ctx(ctx).Data(notice).Insert(); err != nil {
		if isDuplicateKeyError(err) {
			return nil
		}
		return gerror.Wrap(err, "写入监听去重记录失败")
	}
	messageURL := listenerMessageURL(entities, sourceChatId, msg)
	text := buildListenerNotifyText(plan, target, sourceChatTitle, normalizedText, messageURL)
	buttonLabel, buttonURL := listenerUserButton(sender)
	mediaSent, notifyErr := s.listenerNotifyMedia(ctx, plan, notifyChatId, sourceChatId, sourceMessages, text, buttonLabel, buttonURL)
	if notifyErr != nil {
		g.Log().Warningf(ctx, "监听媒体推送失败 plan:%d target:%d notifyChat:%s sourceChat:%s messages:%d err:%+v", plan.Id, target.Id, notifyChatId, sourceChatId, len(sourceMessages), notifyErr)
		if mediaSent && shouldFallbackListenerBotToAccount(notifyErr) {
			fallbackErr := s.listenerNotifyByAccount(ctx, plan, notifyChatId, sourceChatId, sourceMessages, text, buttonLabel, buttonURL, true)
			if fallbackErr == nil {
				g.Log().Infof(ctx, "监听媒体Bot推送失败，协议号兜底成功 plan:%d target:%d notifyChat:%s tgAccountId:%d", plan.Id, target.Id, notifyChatId, plan.TgAccountId)
				notifyErr = nil
			} else {
				notifyErr = gerror.Wrapf(fallbackErr, "Bot推送失败且协议号兜底失败，Bot错误：%v", notifyErr)
			}
		}
	}
	if !mediaSent {
		textErr := botService.SysBot().NotifyRich(ctx, &botsysin.NotifyRichInp{
			ChatId:        notifyChatId,
			Text:          text,
			ParseMode:     "HTML",
			DisableNotice: false,
			ButtonLabel:   buttonLabel,
			ButtonURL:     buttonURL,
		})
		if textErr != nil {
			g.Log().Warningf(ctx, "监听文本推送失败 plan:%d target:%d notifyChat:%s err:%+v", plan.Id, target.Id, notifyChatId, textErr)
			if shouldFallbackListenerBotToAccount(textErr) {
				fallbackErr := s.listenerNotifyByAccount(ctx, plan, notifyChatId, sourceChatId, nil, text, buttonLabel, buttonURL, false)
				if fallbackErr == nil {
					g.Log().Infof(ctx, "监听文本Bot推送失败，协议号兜底成功 plan:%d target:%d notifyChat:%s tgAccountId:%d", plan.Id, target.Id, notifyChatId, plan.TgAccountId)
					textErr = nil
				} else {
					textErr = gerror.Wrapf(fallbackErr, "Bot推送失败且协议号兜底失败，Bot错误：%v", textErr)
				}
			}
		}
		if notifyErr == nil {
			notifyErr = textErr
		}
	}
	resultText := ""
	if notifyErr != nil {
		resultText = notifyErr.Error()
	}
	_, _ = g.DB().Model(messageListenNoticeTable).Safe().Ctx(ctx).
		Where("dedupe_key", dedupeKey).
		Data(g.Map{"notify_result": resultText}).Update()
	now = gtime.Now()
	_, _ = g.DB().Model(messageListenTargetTable).Safe().Ctx(ctx).
		Where("id", target.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"last_matched_at":      now,
			"last_matched_text":    normalizedText,
			"last_matched_user_id": sender.UserId,
			"updated_at":           now,
		}).Update()
	_, _ = g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		Where("id", plan.Id).
		WhereNull("deleted_at").
		Data(g.Map{
			"last_trigger_at": now,
			"last_result":     resultText,
			"updated_at":      now,
		}).Update()
	return notifyErr
}

func (s *sSysPublish) listenerNoticeInCooldown(ctx context.Context, planId int64, sender listenerMessageSenderInfo, normalizedText string, now *gtime.Time) bool {
	if planId <= 0 || (strings.TrimSpace(sender.UserId) == "" && strings.TrimSpace(sender.Username) == "") {
		return false
	}
	query := g.DB().Model(messageListenNoticeTable).Safe().Ctx(ctx).
		Where("plan_id", planId).
		Where("normalized_text_hash", listenerHashText(normalizedText)).
		WhereGTE("created_at", now.Add(-10*time.Minute)).
		Where("(sender_user_id = ? OR sender_username = ?)", sender.UserId, sender.Username)
	count, err := query.Count()
	if err != nil {
		g.Log().Warningf(ctx, "读取监听通知去重窗口失败 plan:%d err:%+v", planId, err)
		return false
	}
	return count > 0
}

func buildListenerNotifyText(plan accountListenPlanRuntime, target accountListenTargetRuntime, sourceChatTitle string, normalizedText string, messageURL string) string {
	parts := make([]string, 0, 8)
	parts = append(parts, "<b>关键字命中</b>")
	if strings.TrimSpace(plan.Name) != "" {
		parts = append(parts, "计划："+telegramEscapeText(plan.Name))
	}
	if title := strings.TrimSpace(sourceChatTitle); title != "" {
		parts = append(parts, "来源："+telegramEscapeText(title))
	}
	if strings.TrimSpace(messageURL) != "" {
		parts = append(parts, `消息地址：<a href="`+telegramEscapeText(messageURL)+`">消息来源</a>`)
	}
	if normalizedText != "" {
		parts = append(parts, "")
		parts = append(parts, "<blockquote>"+telegramEscapeText(normalizedText)+"</blockquote>")
	}
	return strings.Join(parts, "\n")
}
