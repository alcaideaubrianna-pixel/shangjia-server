package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

var listenerBindCodeRegexp = regexp.MustCompile(`^OX[A-Z0-9]{6}$`)

type listenerMessageSenderInfo struct {
	UserId      string
	Username    string
	DisplayName string
}

func listenerTargetModels(records []*listenerTargetRecord) []*sysin.ListenerPlanTargetModel {
	list := make([]*sysin.ListenerPlanTargetModel, 0, len(records))
	for _, item := range records {
		if item == nil {
			continue
		}
		list = append(list, &sysin.ListenerPlanTargetModel{
			Id:                item.Id,
			PlanId:            item.PlanId,
			TenantId:          item.TenantId,
			TargetChatId:      item.TargetChatId,
			TargetChatType:    item.TargetChatType,
			TargetChatTitle:   item.TargetChatTitle,
			TargetChatUser:    item.TargetChatUsername,
			LastMatchedAt:     item.LastMatchedAt,
			LastMatchedText:   item.LastMatchedText,
			LastMatchedUserId: item.LastMatchedUserId,
			CreatedAt:         item.CreatedAt,
			UpdatedAt:         item.UpdatedAt,
		})
	}
	return list
}

func (s *sSysPublish) ensureListenerPlansBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择监听计划")
	}
	count, err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查监听计划权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的监听计划")
	}
	return nil
}

func listenerBindCode(ctx context.Context) string {
	for i := 0; i < 20; i++ {
		code := "OX" + strings.ToUpper(grand.S(6))
		if !listenerBindCodeValid(code) {
			continue
		}
		count, err := g.DB().Model(messageListenPlanTable).Safe().Ctx(ctx).
			Where("bind_code", code).
			WhereNull("deleted_at").
			Count()
		if err == nil && count == 0 {
			return code
		}
	}
	return "OX" + strings.ToUpper(grand.S(6))
}

func listenerBindCodeValid(code string) bool {
	return listenerBindCodeRegexp.MatchString(strings.ToUpper(strings.TrimSpace(code)))
}

func listenerNormalizeBindCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func listenerTargetType(channel *messagePushChannel) string {
	if channel == nil {
		return ""
	}
	if channel.IsBroadcast == 1 {
		return "channel"
	}
	if channel.IsMegagroup == 1 {
		return "group"
	}
	return "group"
}

func listenerDecodeStringArray(value string) []string {
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func listenerMatchedKeywords(text string, keywords []string) []string {
	if text == "" || len(keywords) == 0 {
		return nil
	}
	needle := strings.ToLower(text)
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if strings.Contains(needle, strings.ToLower(keyword)) {
			out = append(out, keyword)
		}
	}
	return out
}

func listenerMessageText(msg *tg.Message) string {
	if msg == nil {
		return ""
	}
	return strings.TrimSpace(msg.Message)
}

func listenerGroupedMessageText(messages []*tg.Message) string {
	parts := make([]string, 0, len(messages))
	seen := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		text := listenerMessageText(msg)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func listenerGroupedMessageIDs(messages []*tg.Message) []int {
	ids := make([]int, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || msg.ID <= 0 {
			continue
		}
		ids = append(ids, msg.ID)
	}
	sort.Ints(ids)
	return ids
}

func listenerNonNilMessages(messages []*tg.Message) []*tg.Message {
	out := make([]*tg.Message, 0, len(messages))
	for _, msg := range messages {
		if msg != nil {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func listenerMessageChatID(msg *tg.Message) string {
	if msg == nil || msg.PeerID == nil {
		return ""
	}
	switch peer := msg.PeerID.(type) {
	case *tg.PeerChannel:
		return normalizeTelegramChannelChatID(strconvFormatInt(peer.ChannelID))
	case *tg.PeerChat:
		return normalizeTelegramChannelChatID(strconvFormatInt(peer.ChatID))
	case *tg.PeerUser:
		return strconvFormatInt(peer.UserID)
	default:
		return ""
	}
}

func listenerMessageSender(entities tg.Entities, msg *tg.Message) listenerMessageSenderInfo {
	if msg == nil {
		return listenerMessageSenderInfo{}
	}
	from, ok := msg.GetFromID()
	if !ok || from == nil {
		return listenerMessageSenderInfo{}
	}
	switch peer := from.(type) {
	case *tg.PeerUser:
		out := listenerMessageSenderInfo{UserId: strconvFormatInt(peer.UserID)}
		if user := entities.Users[peer.UserID]; user != nil {
			out = listenerSenderFromUser(user)
		}
		return out
	default:
		return listenerMessageSenderInfo{}
	}
}

func (w *accountCollectWorker) rememberListenerEntities(entities tg.Entities) {
	if w == nil || len(entities.Users) == 0 {
		return
	}
	w.listenerSenderMu.Lock()
	defer w.listenerSenderMu.Unlock()
	if w.listenerSenders == nil {
		w.listenerSenders = make(map[string]listenerMessageSenderInfo)
	}
	for id, user := range entities.Users {
		if user == nil || id <= 0 {
			continue
		}
		sender := listenerSenderFromUser(user)
		if sender.UserId == "" {
			continue
		}
		w.listenerSenders[sender.UserId] = sender
	}
}

func (w *accountCollectWorker) listenerMessageSender(entities tg.Entities, msg *tg.Message) listenerMessageSenderInfo {
	sender := listenerMessageSender(entities, msg)
	if w == nil || sender.UserId == "" {
		return sender
	}
	if sender.Username != "" || sender.DisplayName != "" {
		w.listenerSenderMu.Lock()
		if w.listenerSenders == nil {
			w.listenerSenders = make(map[string]listenerMessageSenderInfo)
		}
		w.listenerSenders[sender.UserId] = sender
		w.listenerSenderMu.Unlock()
		return sender
	}
	w.listenerSenderMu.Lock()
	cached := w.listenerSenders[sender.UserId]
	w.listenerSenderMu.Unlock()
	if cached.UserId != "" {
		return cached
	}
	return sender
}

func listenerSenderFromUser(user *tg.User) listenerMessageSenderInfo {
	if user == nil || user.ID <= 0 {
		return listenerMessageSenderInfo{}
	}
	return listenerMessageSenderInfo{
		UserId:      strconvFormatInt(user.ID),
		Username:    strings.TrimSpace(user.Username),
		DisplayName: strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " ")),
	}
}

func listenerTargetMatchesChat(target accountListenTargetRuntime, chatIds []string) bool {
	for _, chatId := range chatIds {
		if listenerChatIDMatch(target.TargetChatId, chatId) {
			return true
		}
	}
	return false
}

func listenerChatIDMatch(left, right string) bool {
	left = strings.TrimSpace(normalizeTelegramChannelChatID(left))
	right = strings.TrimSpace(normalizeTelegramChannelChatID(right))
	if left == "" || right == "" {
		return false
	}
	leftIds := tgChannelCacheLookupIds(left)
	rightIds := tgChannelCacheLookupIds(right)
	for _, leftId := range leftIds {
		for _, rightId := range rightIds {
			if strings.TrimSpace(leftId) == strings.TrimSpace(rightId) {
				return true
			}
		}
	}
	return false
}

func firstListenerChatID(chatIds []string) string {
	for _, chatId := range chatIds {
		if chatId = strings.TrimSpace(chatId); chatId != "" {
			return chatId
		}
	}
	return ""
}

func listenerMessageHasMedia(msg *tg.Message) bool {
	if msg == nil {
		return false
	}
	media, ok := msg.GetMedia()
	return ok && media != nil
}

func listenerMessageMediaHash(msg *tg.Message) string {
	if msg == nil {
		return ""
	}
	media, ok := msg.GetMedia()
	if !ok || media == nil {
		return ""
	}
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	default:
		return "media"
	}
}

func listenerMessagesMediaHash(messages []*tg.Message) string {
	if len(messages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d:%s", msg.ID, listenerMessageMediaHash(msg)))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func listenerHashText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func listenerDedupeKey(planId int64, senderUserId string, normalizedText string, mediaHash string) string {
	bucket := gtime.Now().Unix() / 300
	parts := []string{
		fmt.Sprintf("%d", planId),
		senderUserId,
		listenerHashText(normalizedText),
		mediaHash,
		fmt.Sprintf("%d", bucket),
	}
	return strings.Join(parts, ":")
}

func listenerUserButton(sender listenerMessageSenderInfo) (string, string) {
	sender.UserId = strings.TrimSpace(sender.UserId)
	sender.Username = strings.TrimSpace(strings.TrimPrefix(sender.Username, "@"))
	sender.DisplayName = strings.TrimSpace(sender.DisplayName)
	if sender.Username != "" {
		label := "@" + sender.Username
		if sender.DisplayName != "" {
			label = sender.DisplayName + " (@" + sender.Username + ")"
		}
		return label, "https://t.me/" + sender.Username
	}
	if sender.UserId != "" {
		label := "用户 (" + sender.UserId + ")"
		if sender.DisplayName != "" {
			label = sender.DisplayName + " (" + sender.UserId + ")"
		}
		return label, "tg://user?id=" + sender.UserId
	}
	return "", ""
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key")
}

func strconvFormatInt(value int64) string {
	return fmt.Sprintf("%d", value)
}
