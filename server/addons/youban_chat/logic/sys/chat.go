package sys

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	converter "github.com/yazmeyaa/telegram_sticker_converter"
	"github.com/yazmeyaa/telegram_sticker_converter/tgs"

	"hotgo/addons/youban_chat/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	"hotgo/internal/library/telegrammedia"
	"hotgo/internal/model/entity"
	internalService "hotgo/internal/service"
)

type sSysChat struct {
	telegramFeatureMu         sync.RWMutex
	telegramFeatures          map[string]*chatFeatureRow
	telegramFeatureAt         time.Time
	telegramFeatureDefaultsAt time.Time
	externalSchemaMu          sync.Mutex
	externalSchemaReady       bool
	telegramCapabilityMu      sync.RWMutex
	telegramForumCapabilities map[string]telegramForumCapability
	telegramTopicLocks        sync.Map
}

type telegramForumCapability struct {
	enabled   bool
	expiresAt time.Time
}

const (
	chatConversationTable   = "hg_youban_chat_conversation"
	chatMessageTable        = "hg_youban_chat_message"
	chatBotTable            = "hg_youban_chat_bot"
	chatBindingTable        = "hg_youban_chat_binding"
	chatBindingChannelTable = "hg_youban_chat_binding_channel"
	chatOperatorTable       = "hg_youban_chat_operator"
	chatFeatureTable        = "hg_youban_chat_feature"
)

func chatAliasField(alias string, column string) string {
	return alias + "." + column
}

type chatConversationRow struct {
	Id                 int64       `json:"id"`
	MemberId           int64       `json:"member_id"`
	ProfileId          int64       `json:"profile_id"`
	ChatSessionId      string      `json:"pocketping_session_id"`
	TgChatId           string      `json:"tg_chat_id"`
	TgMessageThreadId  int64       `json:"tg_message_thread_id"`
	BotId              int64       `json:"bot_id"`
	RoutingRuleId      int64       `json:"routing_rule_id"`
	AssignedOperatorId int64       `json:"assigned_operator_id"`
	LastMessage        string      `json:"last_message"`
	LastMessageAt      *gtime.Time `json:"last_message_at"`
	UnreadCount        int         `json:"unread_count"`
	Status             string      `json:"status"`
	PinnedAt           *gtime.Time `json:"pinned_at"`
	HiddenBeforeAt     *gtime.Time `json:"hidden_before_at"`
}

type chatMessageRow struct {
	Id                int64       `json:"id"`
	ConversationId    int64       `json:"conversation_id"`
	ExternalMessageId string      `json:"pocketping_message_id"`
	Direction         string      `json:"direction"`
	Content           string      `json:"content"`
	ContentType       string      `json:"content_type"`
	Status            string      `json:"status"`
	SenderName        string      `json:"sender_name"`
	AttachmentsJson   string      `json:"attachments_json"`
	ReplyToMessageId  int64       `json:"reply_to_message_id"`
	ReactionsJson     string      `json:"reactions_json"`
	TgChatId          string      `json:"tg_chat_id"`
	TgMessageThreadId int64       `json:"tg_message_thread_id"`
	TgMessageId       int64       `json:"tg_message_id"`
	CreatedAt         *gtime.Time `json:"created_at"`
	ReadAt            *gtime.Time `json:"read_at"`
}

type adminChatConversationRow struct {
	Id                int64       `json:"id"`
	MemberId          int64       `json:"member_id"`
	MemberUsername    string      `json:"member_username"`
	MemberRealName    string      `json:"member_real_name"`
	MemberMobile      string      `json:"member_mobile"`
	MemberEmail       string      `json:"member_email"`
	MemberAvatar      string      `json:"member_avatar"`
	MemberRemark      string      `json:"member_remark"`
	ProfileId         int64       `json:"profile_id"`
	ProfileNo         string      `json:"profile_no"`
	ProfileTitle      string      `json:"profile_title"`
	Province          string      `json:"province"`
	City              string      `json:"city"`
	ProfileText       string      `json:"profile_text"`
	ChatSessionId     string      `json:"pocketping_session_id"`
	TgChatId          string      `json:"tg_chat_id"`
	TgMessageThreadId int64       `json:"tg_message_thread_id"`
	LastMessage       string      `json:"last_message"`
	LastMessageAt     *gtime.Time `json:"last_message_at"`
	UnreadCount       int         `json:"unread_count"`
	MessageCount      int         `json:"message_count"`
	Status            string      `json:"status"`
	CreatedAt         *gtime.Time `json:"created_at"`
	UpdatedAt         *gtime.Time `json:"updated_at"`
}

type profileBrief struct {
	Id                int64  `json:"id"`
	ProfileNo         string `json:"profile_no"`
	Title             string `json:"title"`
	ChannelId         int64  `json:"channel_id"`
	SourceChannelId   int64  `json:"source_channel_id"`
	SourceMessageId   int64  `json:"source_message_id"`
	SourceChannelName string `json:"source_channel_name"`
	Province          string `json:"province"`
	City              string `json:"city"`
	Age               int    `json:"age"`
	Height            int    `json:"height"`
	Weight            int    `json:"weight"`
	CupSize           string `json:"cup_size"`
	PlainText         string `json:"plain_text"`
}

type chatBotRow struct {
	Id          int64       `json:"id"`
	AppId       string      `json:"app_id"`
	BotName     string      `json:"bot_name"`
	BotUsername string      `json:"bot_username"`
	BotToken    string      `json:"bot_token"`
	Remark      string      `json:"remark"`
	Status      int         `json:"status"`
	CreatedAt   *gtime.Time `json:"created_at"`
	UpdatedAt   *gtime.Time `json:"updated_at"`
}

type chatBindingRow struct {
	Id               int64       `json:"id"`
	AppId            string      `json:"app_id"`
	BindCode         string      `json:"bind_code"`
	BindType         string      `json:"bind_type"`
	SourceChannelId  int64       `json:"source_channel_id"`
	ContentChannelId int64       `json:"content_channel_id"`
	ChannelIds       []int64     `json:"channel_ids"`
	ChannelTitle     string      `json:"channel_title"`
	ChannelUsername  string      `json:"channel_username"`
	BotId            int64       `json:"bot_id"`
	BotName          string      `json:"bot_name"`
	BotToken         string      `json:"bot_token"`
	TgChatId         string      `json:"tg_chat_id"`
	TgChatTitle      string      `json:"tg_chat_title"`
	Remark           string      `json:"remark"`
	Status           int         `json:"status"`
	CreatedAt        *gtime.Time `json:"created_at"`
	UpdatedAt        *gtime.Time `json:"updated_at"`
}

type chatFeatureRow struct {
	Id          int64       `json:"id"`
	FeatureKey  string      `json:"feature_key"`
	Name        string      `json:"name"`
	Command     string      `json:"command"`
	Description string      `json:"description"`
	ConfigJson  string      `json:"config_json"`
	Sort        int         `json:"sort"`
	Status      int         `json:"status"`
	CreatedAt   *gtime.Time `json:"created_at"`
	UpdatedAt   *gtime.Time `json:"updated_at"`
}

func NewSysChat() *sSysChat {
	return &sSysChat{}
}

func init() {
	registerChatGateway(NewSysChat())
}

func (s *sSysChat) Start(ctx context.Context, in *sysin.ChatStartInp) (res *sysin.ChatStartModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	var profile *profileBrief
	if in.ProfileId > 0 {
		profile, err = s.getProfileBrief(ctx, in.ProfileId)
		if err != nil {
			return
		}
	}
	row, err := s.getConversation(ctx, memberId, in.ProfileId)
	if err != nil {
		return
	}
	binding, err := s.matchBinding(ctx, profile)
	if err != nil {
		return
	}
	if binding == nil || strings.TrimSpace(binding.TgChatId) == "" || strings.TrimSpace(binding.BotToken) == "" {
		err = gerror.New("客服群未绑定，请先在后台生成绑定码并在Telegram群内完成绑定")
		return
	}
	if in.ProfileId == 0 {
		row, err = s.ensureGlobalConversation(ctx, memberId)
		if err != nil {
			return
		}
		if row == nil {
			err = gerror.New("全局客服群未绑定，请先在后台生成绑定码并在Telegram群内完成绑定")
			return
		}
		if err = s.ensureConversationRoute(ctx, row, binding); err != nil {
			return
		}
		res = s.packStart(row)
		return
	}
	if row != nil {
		if row.ChatSessionId == "" {
			row.ChatSessionId = chatSessionId(row.Id)
			_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"pocketping_session_id": row.ChatSessionId,
				"updated_at":            gtime.Now(),
			}).Update()
			if err != nil {
				err = gerror.Wrap(err, "更新聊天会话失败")
				return
			}
		}
		if err = s.ensureConversationRoute(ctx, row, binding); err != nil {
			return
		}
		res = s.packStart(row)
		return
	}

	id, err := g.DB().Model(chatConversationTable).Ctx(ctx).Data(g.Map{
		"member_id":            memberId,
		"profile_id":           in.ProfileId,
		"bot_id":               routeBotId(binding),
		"tg_chat_id":           routeTargetChatId(binding),
		"routing_rule_id":      0,
		"assigned_operator_id": 0,
		"status":               "opened",
		"last_message":         "会话已创建",
		"last_message_at":      gtime.Now(),
		"created_at":           gtime.Now(),
		"updated_at":           gtime.Now(),
	}).InsertAndGetId()
	if err != nil {
		err = gerror.Wrap(err, "保存聊天会话失败")
		return
	}
	row = &chatConversationRow{Id: id, MemberId: memberId, ProfileId: in.ProfileId, ChatSessionId: chatSessionId(id), BotId: routeBotId(binding), TgChatId: routeTargetChatId(binding), Status: "opened"}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", id).Data(g.Map{
		"pocketping_session_id": row.ChatSessionId,
		"updated_at":            gtime.Now(),
	}).Update()
	if err != nil {
		err = gerror.Wrap(err, "更新聊天会话失败")
		return
	}
	res = s.packStart(row)
	return
}

func (s *sSysChat) Send(ctx context.Context, in *sysin.ChatSendInp) (res *sysin.ChatSendModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		err = gerror.New("消息不能为空")
		return
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	member, err := s.getMember(ctx, memberId)
	if err != nil {
		return
	}
	profile, err := s.getConversationProfileBrief(ctx, row)
	if err != nil {
		return
	}
	messageId := normalizeClientMessageId(in.ClientMessageId, memberId)
	id, inserted, err := s.saveMessage(ctx, row.Id, messageId, "mine", content, "text", "sent", memberDisplayName(member), nil)
	if err != nil {
		return
	}
	if !inserted {
		return &sysin.ChatSendModel{MessageId: id, ClientMessageId: messageId, Status: "read"}, nil
	}
	_, _ = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("id", row.Id).
		Data(g.Map{"last_message": content, "last_message_at": gtime.Now(), "updated_at": gtime.Now()}).
		Update()
	if err = s.notifyTelegramMessage(ctx, row, profile, member, messageId, content, nil); err != nil {
		_, _ = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", id).Data(g.Map{"status": "failed", "updated_at": gtime.Now()}).Update()
		err = gerror.Wrap(err, "发送 Telegram 消息失败")
		return
	}
	_ = s.markMineMessageReadById(ctx, row, id)
	res = &sysin.ChatSendModel{MessageId: id, ClientMessageId: messageId, Status: "read"}
	return
}

func (s *sSysChat) Messages(ctx context.Context, in *sysin.ChatMessagesInp) (res *sysin.ChatMessagesModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	mod := g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("conversation_id", row.Id).
		WhereGTE("created_at", gtime.Now().AddDate(0, 0, -7)).
		WhereNull("deleted_at")
	if row.HiddenBeforeAt != nil {
		mod = mod.WhereGT("created_at", row.HiddenBeforeAt)
	}
	if in.AfterId > 0 {
		mod = mod.WhereGT("id", in.AfterId)
	}
	var rows []*chatMessageRow
	if err = mod.OrderAsc("id").Limit(200).Scan(&rows); err != nil {
		err = gerror.Wrap(err, "读取聊天消息失败")
		return
	}
	list := make([]*sysin.ChatMessageModel, 0, len(rows))
	for _, item := range rows {
		list = append(list, packApiChatMessage(item))
	}
	res = &sysin.ChatMessagesModel{List: list}
	return
}

func (s *sSysChat) Pin(ctx context.Context, in *sysin.ChatConversationPinInp) (err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		return gerror.New("请先登录")
	}
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	data := g.Map{"updated_at": gtime.Now()}
	if row.ProfileId == 0 || in.Pinned == 1 {
		data["pinned_at"] = gtime.Now()
	} else {
		data["pinned_at"] = nil
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新会话置顶失败")
	}
	return nil
}

func (s *sSysChat) Clear(ctx context.Context, in *sysin.ChatConversationClearInp) (err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		return gerror.New("请先登录")
	}
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	now := gtime.Now()
	data := g.Map{
		"hidden_before_at": now,
		"unread_count":     0,
		"last_message":     "",
		"last_message_at":  nil,
		"updated_at":       now,
	}
	if row.ProfileId == 0 {
		data["pinned_at"] = now
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "清空聊天记录失败")
	}
	return nil
}

func (s *sSysChat) Read(ctx context.Context, in *sysin.ChatReadInp) (err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		return gerror.New("请先登录")
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("id", row.Id).
		Data(g.Map{"unread_count": 0, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "标记聊天已读失败")
		return
	}
	if err = s.markServiceMessagesRead(ctx, row); err != nil {
		return
	}
	return
}

func (s *sSysChat) Upload(ctx context.Context, in *sysin.ChatUploadInp, file *ghttp.UploadFile) (res *sysin.ChatUploadModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if file == nil {
		err = gerror.New("没有找到上传的文件")
		return
	}
	row, err := s.getConversationById(ctx, memberId, in.ConversationId)
	if err != nil {
		return
	}
	member, err := s.getMember(ctx, memberId)
	if err != nil {
		return
	}
	profile, err := s.getConversationProfileBrief(ctx, row)
	if err != nil {
		return
	}
	attachment, err := localUploadAttachment(ctx, file)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(in.Content)
	messageId := fmt.Sprintf("msg_%d_%d", memberId, time.Now().UnixNano())
	id, inserted, err := s.saveMessage(ctx, row.Id, messageId, "mine", content, "text", "sent", memberDisplayName(member), []*sysin.ChatMessageAttachmentModel{attachment})
	if err != nil {
		return
	}
	if !inserted {
		return &sysin.ChatUploadModel{Message: &sysin.ChatMessageModel{Id: id, ConversationId: row.Id, Direction: "mine", Content: content, ContentType: attachment.FileType, Status: "read", Attachments: []*sysin.ChatMessageAttachmentModel{attachment}}}, nil
	}
	message := &sysin.ChatMessageModel{
		Id:             id,
		ConversationId: row.Id,
		Direction:      "mine",
		Content:        content,
		ContentType:    "text",
		Status:         "sent",
		SenderName:     memberDisplayName(member),
		CreatedAt:      gtime.Now().String(),
		Attachments:    []*sysin.ChatMessageAttachmentModel{attachment},
	}
	_, _ = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("id", row.Id).
		Data(g.Map{"last_message": attachmentLastMessage(message), "last_message_at": gtime.Now(), "updated_at": gtime.Now()}).
		Update()
	if err = s.notifyTelegramMessage(ctx, row, profile, member, messageId, content, []*sysin.ChatMessageAttachmentModel{attachment}); err != nil {
		_, _ = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", id).Data(g.Map{"status": "failed", "updated_at": gtime.Now()}).Update()
		err = gerror.Wrap(err, "发送 Telegram 附件失败")
		return
	}
	_ = s.markMineMessageReadById(ctx, row, id)
	message.Status = "read"
	message.ReadAt = gtime.Now().String()
	res = &sysin.ChatUploadModel{Message: message}
	return
}

func (s *sSysChat) Unread(ctx context.Context) (res *sysin.ChatUnreadModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	value, err := g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("member_id", memberId).
		Where("(hidden_before_at IS NULL OR last_message_at IS NULL OR last_message_at > hidden_before_at)").
		Where("(last_message_at IS NULL OR last_message_at >= ?)", gtime.Now().AddDate(0, 0, -7)).
		WhereNull("deleted_at").
		Fields("COALESCE(SUM(unread_count),0)").
		Value()
	if err != nil {
		err = gerror.Wrap(err, "获取聊天未读数失败")
		return
	}
	res = &sysin.ChatUnreadModel{UnreadCount: value.Int()}
	return
}

func (s *sSysChat) TelegramWebhook(ctx context.Context, in *sysin.TelegramWebhookInp) (err error) {
	if in == nil {
		return nil
	}
	if in.MessageReaction != nil {
		return s.applyTelegramReaction(ctx, in.MessageReaction)
	}
	msg := in.Message
	if msg == nil {
		msg = in.EditedMessage
	}
	if msg == nil || msg.Chat == nil {
		return nil
	}
	text := telegramMessageText(msg)
	if handled, err := s.dispatchTelegramFeatures(ctx, in, msg); handled || err != nil {
		return err
	}
	if code := extractTelegramBindCode(text); code != "" {
		return s.bindTelegramChatByCode(ctx, code, msg, in.BotId)
	}
	if msg.Chat.Id == 0 || msg.MessageId <= 0 {
		return nil
	}
	row, err := s.getConversationByTelegramTopic(ctx, fmt.Sprintf("%d", msg.Chat.Id), msg.MessageThreadId)
	if err == nil && row == nil && msg.MessageThreadId <= 0 {
		row, err = s.getLatestConversationByTelegramChat(ctx, fmt.Sprintf("%d", msg.Chat.Id))
	}
	if err != nil || row == nil {
		return err
	}
	operatorName := telegramUserName(msg.From)
	messageId := fmt.Sprintf("telegram_%d_%d", msg.Chat.Id, msg.MessageId)
	attachments, err := s.telegramMessageAttachments(ctx, row, msg)
	if err != nil {
		return
	}
	if telegramHasVisualEmoji(msg) && len(attachments) > 0 {
		text = ""
	}
	if strings.TrimSpace(text) == "" && len(attachments) == 0 {
		return nil
	}
	contentType := "text"
	if len(attachments) > 0 {
		contentType = attachments[0].FileType
	}
	_, inserted, err := s.saveTelegramInboundMessage(ctx, row, msg, messageId, text, contentType, fallbackString(operatorName, "客服"), attachments)
	if err != nil {
		return
	}
	if !inserted {
		return nil
	}
	if err = s.markMineMessagesReadByTelegramReply(ctx, row); err != nil {
		return
	}
	lastMessage := text
	if lastMessage == "" {
		lastMessage = attachmentLastMessage(&sysin.ChatMessageModel{Attachments: attachments})
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("id", row.Id).
		Data(g.Map{"last_message": lastMessage, "last_message_at": gtime.Now(), "updated_at": gtime.Now(), "unread_count": gdb.Raw("unread_count + 1")}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新聊天未读数失败")
	}
	return nil
}

func (s *sSysChat) applyTelegramReaction(ctx context.Context, in *sysin.TelegramReactionInp) error {
	if in == nil || in.ChatId == 0 || in.MessageId <= 0 {
		return nil
	}
	if err := s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return err
	}
	var message *chatMessageRow
	if err := g.DB().Model(chatMessageTable).Ctx(ctx).Where("tg_chat_id", fmt.Sprintf("%d", in.ChatId)).Where("tg_message_id", in.MessageId).Scan(&message); err != nil {
		return gerror.Wrap(err, "读取Telegram反应消息失败")
	}
	if message == nil {
		return nil
	}
	reactions := map[string][]string{}
	_ = json.Unmarshal([]byte(message.ReactionsJson), &reactions)
	actor := fmt.Sprintf("telegram:%d", in.ActorId)
	for emoji, values := range reactions {
		filtered := values[:0]
		for _, value := range values {
			if value != actor {
				filtered = append(filtered, value)
			}
		}
		if len(filtered) == 0 {
			delete(reactions, emoji)
		} else {
			reactions[emoji] = filtered
		}
	}
	for _, emoji := range in.NewReaction {
		if strings.TrimSpace(emoji) != "" {
			reactions[emoji] = append(reactions[emoji], actor)
		}
	}
	encoded, _ := json.Marshal(reactions)
	_, err := g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", message.Id).Data(g.Map{"reactions_json": string(encoded), "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysChat) List(ctx context.Context, in *sysin.ChatConversationListInp) (list []*sysin.ChatConversationListModel, totalCount int, err error) {
	if contexts.GetUserId(ctx) <= 0 {
		err = gerror.New("请先登录")
		return
	}
	memberId := contexts.GetUserId(ctx)
	if err = s.ensureConversationUserColumns(ctx); err != nil {
		return
	}
	if _, err = s.ensureGlobalConversation(ctx, memberId); err != nil {
		return
	}
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	mod := g.DB().Model(chatConversationTable+" c").Ctx(ctx).
		LeftJoin(dao.ContentProfile.Table()+" p", chatAliasField("p", profileColumns.Id)+"="+chatAliasField("c", "profile_id")).
		LeftJoin(dao.ContentMedia.Table()+" m", chatAliasField("m", mediaColumns.ProfileId)+"="+chatAliasField("p", profileColumns.Id)+" AND "+chatAliasField("m", mediaColumns.MediaType)+"='image' AND "+chatAliasField("m", mediaColumns.SortIndex)+"=0 AND "+chatAliasField("m", mediaColumns.DeletedAt)+" IS NULL").
		Where(chatAliasField("c", "member_id"), memberId).
		WhereNull(chatAliasField("c", "deleted_at")).
		Where("("+chatAliasField("c", "profile_id")+"=0 OR "+chatAliasField("c", "last_message_at")+" IS NOT NULL OR "+chatAliasField("c", "unread_count")+">0)").
		Where("("+chatAliasField("c", "profile_id")+"=0 OR "+chatAliasField("c", "last_message_at")+" IS NULL OR "+chatAliasField("c", "last_message_at")+">=?)", gtime.Now().AddDate(0, 0, -7)).
		Where("(" + chatAliasField("c", "profile_id") + "=0 OR " + chatAliasField("c", "hidden_before_at") + " IS NULL OR " + chatAliasField("c", "last_message_at") + " IS NULL OR " + chatAliasField("c", "last_message_at") + ">" + chatAliasField("c", "hidden_before_at") + ")").
		Fields(strings.Join([]string{
			chatAliasField("c", "id"),
			chatAliasField("c", "id") + " AS conversation_id",
			chatAliasField("c", "profile_id"),
			"CASE WHEN " + chatAliasField("c", "profile_id") + "=0 THEN true ELSE false END AS is_global",
			"CASE WHEN " + chatAliasField("c", "profile_id") + "=0 OR " + chatAliasField("c", "pinned_at") + " IS NOT NULL THEN true ELSE false END AS is_pinned",
			"CASE WHEN " + chatAliasField("c", "profile_id") + "=0 THEN false ELSE true END AS can_delete",
			chatAliasField("c", "last_message"),
			chatAliasField("c", "last_message_at"),
			chatAliasField("c", "unread_count"),
			chatAliasField("c", "status"),
			chatAliasField("p", profileColumns.ProfileNo),
			chatAliasField("p", profileColumns.Province),
			chatAliasField("p", profileColumns.City),
			chatAliasField("p", profileColumns.Age),
			chatAliasField("p", profileColumns.Height),
			"COALESCE(" + chatAliasField("m", mediaColumns.PreviewStoragePath) + "," + chatAliasField("m", mediaColumns.DisplayStoragePath) + ",'') AS avatar",
		}, ","))
	mod = mod.Page(in.Page, in.PerPage).
		Order(gdb.Raw("CASE WHEN " + chatAliasField("c", "profile_id") + "=0 THEN 1 ELSE 0 END DESC")).
		Order(gdb.Raw("CASE WHEN " + chatAliasField("c", "pinned_at") + " IS NULL THEN 0 ELSE 1 END DESC")).
		OrderDesc(chatAliasField("c", "pinned_at")).
		OrderDesc(chatAliasField("c", "updated_at")).
		OrderDesc(chatAliasField("c", "id"))
	if err = mod.ScanAndCount(&list, &totalCount, false); err != nil {
		err = gerror.Wrap(err, "获取聊天会话列表失败")
		return
	}
	for _, item := range list {
		if item.IsGlobal {
			item.Name = "悦伴客服"
			item.LastMessage = firstNonEmptyString(item.LastMessage, "有问题随时联系悦伴客服")
			continue
		}
		item.Name = strings.ToLower(item.ProfileNo)
		if item.LastMessage == "" {
			item.LastMessage = "暂无消息"
		}
	}
	return
}

func (s *sSysChat) WidgetSession(ctx context.Context, in *sysin.ChatWidgetSessionInp) (res *sysin.ChatWidgetSessionModel, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	member, err := s.getMember(ctx, memberId)
	if err != nil {
		return
	}
	shortId := memberShortId(member.Id)
	displayName := memberDisplayName(member)
	user := &sysin.ChatWidgetUserModel{Identifier: fmt.Sprintf("youban-member-%d", member.Id), Name: fmt.Sprintf("%s（%s）", displayName, shortId), Email: member.Email, Phone: member.Mobile}
	attributes := map[string]interface{}{"youban_member_id": member.Id, "youban_member_short_id": shortId, "youban_member_username": member.Username, "youban_member_nickname": displayName, "youban_member_mobile": member.Mobile}
	var profileModel *sysin.ChatWidgetProfileModel
	if in.ProfileId > 0 {
		profile, profileErr := s.getProfileBrief(ctx, in.ProfileId)
		if profileErr != nil {
			err = profileErr
			return
		}
		profileTitle := strings.TrimSpace(profile.Title)
		if profileTitle == "" {
			profileTitle = profile.ProfileNo
		}
		profileModel = &sysin.ChatWidgetProfileModel{Id: profile.Id, ProfileNo: profile.ProfileNo, Title: profileTitle, Province: profile.Province, City: profile.City, Age: profile.Age, Height: profile.Height}
		attributes["youban_profile_id"] = profile.Id
		attributes["youban_profile_no"] = profile.ProfileNo
		attributes["youban_profile_title"] = profileTitle
		attributes["youban_profile_area"] = strings.TrimSpace(profile.Province + " " + profile.City)
		attributes["youban_admin_profile_url"] = fmt.Sprintf("/admin#/content/note/view?id=%d", profile.Id)
	}
	res = &sysin.ChatWidgetSessionModel{LauncherTitle: "悦伴客服", User: user, Profile: profileModel, CustomAttributes: attributes}
	return
}

func (s *sSysChat) AdminList(ctx context.Context, in *sysin.AdminChatConversationListInp) (list []*sysin.AdminChatConversationListModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AdminChatConversationListInp{}
	}
	mod := s.adminConversationModel(ctx)
	mod = s.applyAdminConversationFilter(mod, in)
	var rows []*adminChatConversationRow
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(chatAliasField("c", "updated_at")).OrderDesc(chatAliasField("c", "id")).ScanAndCount(&rows, &totalCount, false); err != nil {
		err = gerror.Wrap(err, "获取客服会话列表失败")
		return
	}
	list = make([]*sysin.AdminChatConversationListModel, 0, len(rows))
	for _, row := range rows {
		list = append(list, packAdminChatConversation(row))
	}
	return
}

func (s *sSysChat) AdminView(ctx context.Context, in *sysin.AdminChatConversationViewInp) (res *sysin.AdminChatConversationViewModel, err error) {
	if in == nil || in.Id <= 0 {
		err = gerror.New("会话ID不能为空")
		return
	}
	var row *adminChatConversationRow
	err = s.adminConversationModel(ctx).Where(chatAliasField("c", "id"), in.Id).Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "获取客服会话详情失败")
		return
	}
	if row == nil {
		err = gerror.New("客服会话不存在")
		return
	}
	res = &sysin.AdminChatConversationViewModel{
		AdminChatConversationListModel: packAdminChatConversation(row),
		MemberAvatar:                   row.MemberAvatar,
		MemberRemark:                   row.MemberRemark,
		ProfileText:                    row.ProfileText,
	}
	return
}

func (s *sSysChat) AdminMessages(ctx context.Context, in *sysin.AdminChatMessageListInp) (list []*sysin.ChatMessageModel, totalCount int, err error) {
	if in == nil || in.ConversationId <= 0 {
		err = gerror.New("会话ID不能为空")
		return
	}
	mod := g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("conversation_id", in.ConversationId).
		WhereNull("deleted_at")
	if direction := strings.TrimSpace(in.Direction); direction != "" {
		mod = mod.Where("direction", direction)
	}
	var rows []*chatMessageRow
	if err = mod.Page(in.Page, in.PerPage).OrderAsc("id").ScanAndCount(&rows, &totalCount, false); err != nil {
		err = gerror.Wrap(err, "获取客服聊天记录失败")
		return
	}
	list = make([]*sysin.ChatMessageModel, 0, len(rows))
	for _, item := range rows {
		list = append(list, packLocalChatMessage(item))
	}
	return
}

func (s *sSysChat) AdminClear(ctx context.Context, in *sysin.AdminChatConversationClearInp) (err error) {
	if in == nil || in.ConversationId <= 0 {
		err = gerror.New("会话ID不能为空")
		return
	}
	var row *chatConversationRow
	if err = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("id", in.ConversationId).
		WhereNull("deleted_at").
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "读取客服会话失败")
		return
	}
	if row == nil {
		return gerror.New("客服会话不存在")
	}
	now := gtime.Now()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err = tx.Model(chatMessageTable).Ctx(ctx).
			Where("conversation_id", in.ConversationId).
			WhereNull("deleted_at").
			Data(g.Map{"deleted_at": now, "updated_at": now}).
			Update(); err != nil {
			return gerror.Wrap(err, "清空客服聊天消息失败")
		}
		data := g.Map{
			"hidden_before_at": now,
			"unread_count":     0,
			"last_message":     "",
			"last_message_at":  nil,
			"updated_at":       now,
		}
		if _, err = tx.Model(chatConversationTable).Ctx(ctx).
			Where("id", in.ConversationId).
			Data(data).
			Update(); err != nil {
			return gerror.Wrap(err, "更新客服会话失败")
		}
		return nil
	})
	return
}

func (s *sSysChat) AdminBotList(ctx context.Context, in *sysin.AdminChatBotListInp) (list []*sysin.AdminChatBotModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AdminChatBotListInp{}
	}
	mod := g.DB().Model(chatBotTable).Ctx(ctx).WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ? OR remark LIKE ?)", like, like, like)
	}
	var rows []*chatBotRow
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot列表失败")
	}
	list = make([]*sysin.AdminChatBotModel, 0, len(rows))
	for _, row := range rows {
		item := &sysin.AdminChatBotModel{Id: row.Id, BotName: row.BotName, BotUsername: row.BotUsername, BotToken: row.BotToken, Remark: row.Remark, Status: row.Status}
		if row.CreatedAt != nil {
			item.CreatedAt = row.CreatedAt.String()
		}
		if row.UpdatedAt != nil {
			item.UpdatedAt = row.UpdatedAt.String()
		}
		list = append(list, item)
	}
	return
}

func (s *sSysChat) AdminSaveBot(ctx context.Context, in *sysin.AdminChatBotSaveInp) (err error) {
	if in == nil {
		return gerror.New("Bot配置不能为空")
	}
	botToken := strings.TrimSpace(in.BotToken)
	if botToken == "" {
		return gerror.New("Bot Token不能为空")
	}
	tgUser, err := s.telegramBotProfile(ctx, botToken)
	if err != nil {
		return gerror.Wrap(err, "校验Bot Token失败")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	botName := strings.TrimSpace(in.BotName)
	if botName == "" {
		botName = telegramBotDisplayName(tgUser)
	}
	data := g.Map{
		"bot_name":     botName,
		"bot_username": strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@"),
		"bot_token":    botToken,
		"remark":       strings.TrimSpace(in.Remark),
		"status":       status,
		"updated_at":   gtime.Now(),
	}
	if in.Id > 0 {
		_, err = g.DB().Model(chatBotTable).Ctx(ctx).Where("id", in.Id).Data(data).Update()
	} else {
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(chatBotTable).Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存Bot配置失败")
	}
	if err = s.syncTelegramBotMenu(ctx, botToken); err != nil {
		g.Log().Warningf(ctx, "同步Telegram菜单失败 bot:%s err:%+v", tgUser.Username, err)
	}
	return nil
}

func (s *sSysChat) AdminBindingList(ctx context.Context, in *sysin.AdminChatBindingListInp) (list []*sysin.AdminChatBindingModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AdminChatBindingListInp{}
	}
	channelColumns := dao.ContentChannel.Columns()
	mod := g.DB().Model(chatBindingTable+" b").Ctx(ctx).
		LeftJoin(chatBotTable+" bot", "bot.id=b.bot_id AND bot.deleted_at IS NULL").
		LeftJoin(dao.ContentChannel.Table()+" ch", chatAliasField("ch", channelColumns.Id)+"="+chatAliasField("b", "content_channel_id")).
		WhereNull("b.deleted_at").
		Fields(strings.Join([]string{
			"b.id",
			"b.bind_code",
			"b.bind_type",
			"b.source_channel_id",
			"b.content_channel_id",
			chatAliasField("ch", channelColumns.Title) + " AS channel_title",
			chatAliasField("ch", channelColumns.Username) + " AS channel_username",
			"b.bot_id",
			"bot.bot_name",
			"b.tg_chat_id",
			"b.tg_chat_title",
			"b.remark",
			"b.status",
			"b.created_at",
			"b.updated_at",
		}, ","))
	if in.Status > 0 {
		mod = mod.Where("b.status", in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where(fmt.Sprintf("(%s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ?)",
			"b.bind_code",
			"b.tg_chat_id",
			"b.tg_chat_title",
			"bot.bot_name",
			chatAliasField("ch", channelColumns.Title),
		), like, like, like, like, like)
	}
	var rows []*chatBindingRow
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("b.id").ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道绑定列表失败")
	}
	list = make([]*sysin.AdminChatBindingModel, 0, len(rows))
	for _, row := range rows {
		row.ChannelIds, _ = s.bindingChannelIds(ctx, row.Id)
		item := &sysin.AdminChatBindingModel{Id: row.Id, BindCode: row.BindCode, BindType: row.BindType, SourceChannelId: row.SourceChannelId, ContentChannelId: row.ContentChannelId, ChannelTitle: row.ChannelTitle, ChannelUsername: row.ChannelUsername, BotId: row.BotId, BotName: row.BotName, TgChatId: row.TgChatId, TgChatTitle: row.TgChatTitle, Remark: row.Remark, Status: row.Status}
		item.ChannelIds = row.ChannelIds
		if row.CreatedAt != nil {
			item.CreatedAt = row.CreatedAt.String()
		}
		if row.UpdatedAt != nil {
			item.UpdatedAt = row.UpdatedAt.String()
		}
		list = append(list, item)
	}
	return
}

func (s *sSysChat) AdminSaveBinding(ctx context.Context, in *sysin.AdminChatBindingSaveInp) (err error) {
	if in == nil {
		return gerror.New("绑定配置不能为空")
	}
	bindType := strings.TrimSpace(in.BindType)
	if bindType == "" {
		bindType = "channel"
	}
	if bindType != "global" && bindType != "channel" {
		return gerror.New("绑定类型不正确")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	channelIds := uniquePositiveInt64s(in.ChannelIds)
	if bindType == "channel" && len(channelIds) == 0 {
		return gerror.New("请选择关联频道")
	}
	if bindType == "global" {
		channelIds = nil
		if err = s.ensureSingleGlobalBinding(ctx, in.Id); err != nil {
			return err
		}
	}
	firstChannelId := int64(0)
	if len(channelIds) > 0 {
		firstChannelId = channelIds[0]
	}
	data := g.Map{
		"bind_type":          bindType,
		"source_channel_id":  firstChannelId,
		"content_channel_id": firstChannelId,
		"remark":             strings.TrimSpace(in.Remark),
		"status":             status,
		"updated_at":         gtime.Now(),
	}
	var bindingId int64
	if in.Id > 0 {
		_, err = g.DB().Model(chatBindingTable).Ctx(ctx).Where("id", in.Id).Data(data).Update()
		bindingId = in.Id
	} else {
		data["bind_code"] = randomBindCode()
		data["tg_chat_id"] = ""
		data["tg_chat_title"] = ""
		data["created_at"] = gtime.Now()
		bindingId, err = g.DB().Model(chatBindingTable).Ctx(ctx).Data(data).InsertAndGetId()
	}
	if err != nil {
		return gerror.Wrap(err, "保存频道绑定失败")
	}
	if err = s.saveBindingChannels(ctx, bindingId, channelIds); err != nil {
		return err
	}
	return nil
}

func (s *sSysChat) AdminChannelOptions(ctx context.Context) (list []*sysin.AdminChatChannelOptionModel, err error) {
	columns := dao.ContentChannel.Columns()
	var rows []*entity.ContentChannel
	err = dao.ContentChannel.Ctx(ctx).
		Fields(columns.Id, columns.Title, columns.Username, columns.SourceChannelId).
		WhereNull(columns.DeletedAt).
		Group(columns.Id).
		OrderDesc(columns.Id).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "获取频道选项失败")
	}
	list = make([]*sysin.AdminChatChannelOptionModel, 0, len(rows))
	seenLabels := map[string]struct{}{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		label := strings.TrimSpace(row.Title)
		if label == "" {
			label = strings.TrimSpace(row.Username)
		}
		if label == "" {
			label = fmt.Sprintf("频道 #%d", row.Id)
		}
		if row.Username != "" && row.Username != label {
			label = fmt.Sprintf("%s (@%s)", label, strings.TrimPrefix(row.Username, "@"))
		}
		if row.SourceChannelId > 0 {
			label = fmt.Sprintf("%s / 来源:%d", label, row.SourceChannelId)
		}
		if _, ok := seenLabels[label]; ok {
			label = fmt.Sprintf("%s / ID:%d", label, row.Id)
		}
		seenLabels[label] = struct{}{}
		list = append(list, &sysin.AdminChatChannelOptionModel{Label: label, Value: row.Id})
	}
	return list, nil
}

func (s *sSysChat) ensureSingleGlobalBinding(ctx context.Context, currentId int64) error {
	mod := g.DB().Model(chatBindingTable).Ctx(ctx).
		Where("bind_type", "global").
		WhereNull("deleted_at")
	if currentId > 0 {
		mod = mod.WhereNot("id", currentId)
	}
	count, err := mod.Count()
	if err != nil {
		return gerror.Wrap(err, "校验全局默认绑定失败")
	}
	if count > 0 {
		return gerror.New("全局默认绑定已存在")
	}
	return nil
}

func (s *sSysChat) bindingChannelIds(ctx context.Context, bindingId int64) ([]int64, error) {
	if bindingId <= 0 {
		return nil, nil
	}
	var rows []struct {
		ChannelId int64 `json:"channel_id"`
	}
	err := g.DB().Model(chatBindingChannelTable).Ctx(ctx).
		Fields("channel_id").
		Where("binding_id", bindingId).
		OrderAsc("channel_id").
		Scan(&rows)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "读取绑定频道失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ChannelId > 0 {
			ids = append(ids, row.ChannelId)
		}
	}
	return ids, nil
}

func (s *sSysChat) saveBindingChannels(ctx context.Context, bindingId int64, channelIds []int64) error {
	if bindingId <= 0 {
		return nil
	}
	_, err := g.DB().Model(chatBindingChannelTable).Ctx(ctx).Where("binding_id", bindingId).Delete()
	if err != nil {
		if isMissingTableError(err) {
			return nil
		}
		return gerror.Wrap(err, "清理绑定频道失败")
	}
	channelIds = uniquePositiveInt64s(channelIds)
	if len(channelIds) == 0 {
		return nil
	}
	data := make([]g.Map, 0, len(channelIds))
	now := gtime.Now()
	for _, channelId := range channelIds {
		data = append(data, g.Map{"binding_id": bindingId, "channel_id": channelId, "created_at": now})
	}
	if _, err = g.DB().Model(chatBindingChannelTable).Ctx(ctx).Data(data).Insert(); err != nil {
		return gerror.Wrap(err, "保存绑定频道失败")
	}
	return nil
}

func (s *sSysChat) AdminOperatorList(ctx context.Context, in *sysin.AdminChatOperatorListInp) (list []*sysin.AdminChatOperatorModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.AdminChatOperatorListInp{}
	}
	memberColumns := dao.AdminMember.Columns()
	mod := g.DB().Model(chatOperatorTable+" o").Ctx(ctx).
		LeftJoin(dao.AdminMember.Table()+" u", chatAliasField("u", memberColumns.Id)+"="+chatAliasField("o", "admin_member_id")).
		WhereNull(chatAliasField("o", "deleted_at")).
		Fields(strings.Join([]string{
			chatAliasField("o", "id"),
			chatAliasField("o", "admin_member_id"),
			chatAliasField("u", memberColumns.Username) + " AS admin_username",
			chatAliasField("u", memberColumns.RealName) + " AS admin_real_name",
			chatAliasField("o", "telegram_user_id"),
			chatAliasField("o", "telegram_username"),
			chatAliasField("o", "display_name"),
			chatAliasField("o", "bind_code"),
			chatAliasField("o", "remark"),
			chatAliasField("o", "status"),
			chatAliasField("o", "created_at"),
			chatAliasField("o", "updated_at"),
		}, ","))
	if in.Status > 0 {
		mod = mod.Where(chatAliasField("o", "status"), in.Status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where(fmt.Sprintf("(%s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ?)",
			chatAliasField("o", "display_name"),
			chatAliasField("o", "telegram_username"),
			chatAliasField("u", memberColumns.Username),
			chatAliasField("u", memberColumns.RealName),
		), like, like, like, like)
	}
	var rows []struct {
		Id               int64       `json:"id"`
		AdminMemberId    int64       `json:"admin_member_id"`
		AdminUsername    string      `json:"admin_username"`
		AdminRealName    string      `json:"admin_real_name"`
		TelegramUserId   string      `json:"telegram_user_id"`
		TelegramUsername string      `json:"telegram_username"`
		DisplayName      string      `json:"display_name"`
		BindCode         string      `json:"bind_code"`
		Remark           string      `json:"remark"`
		Status           int         `json:"status"`
		CreatedAt        *gtime.Time `json:"created_at"`
		UpdatedAt        *gtime.Time `json:"updated_at"`
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(chatAliasField("o", "id")).ScanAndCount(&rows, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取客服绑定列表失败")
	}
	list = make([]*sysin.AdminChatOperatorModel, 0, len(rows))
	for _, row := range rows {
		item := &sysin.AdminChatOperatorModel{Id: row.Id, AdminMemberId: row.AdminMemberId, AdminUsername: row.AdminUsername, AdminRealName: row.AdminRealName, TelegramUserId: row.TelegramUserId, TelegramUsername: row.TelegramUsername, DisplayName: row.DisplayName, BindCode: row.BindCode, Remark: row.Remark, Status: row.Status}
		if row.CreatedAt != nil {
			item.CreatedAt = row.CreatedAt.String()
		}
		if row.UpdatedAt != nil {
			item.UpdatedAt = row.UpdatedAt.String()
		}
		list = append(list, item)
	}
	return
}

func (s *sSysChat) AdminSaveOperator(ctx context.Context, in *sysin.AdminChatOperatorSaveInp) (err error) {
	if in == nil {
		return gerror.New("客服信息不能为空")
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	data := g.Map{
		"admin_member_id":   in.AdminMemberId,
		"telegram_user_id":  strings.TrimSpace(in.TelegramUserId),
		"telegram_username": strings.TrimSpace(in.TelegramUsername),
		"display_name":      strings.TrimSpace(in.DisplayName),
		"bind_code":         strings.TrimSpace(in.BindCode),
		"remark":            strings.TrimSpace(in.Remark),
		"status":            status,
		"updated_at":        gtime.Now(),
	}
	if in.Id > 0 {
		_, err = g.DB().Model(chatOperatorTable).Ctx(ctx).Where("id", in.Id).Data(data).Update()
	} else {
		data["created_at"] = gtime.Now()
		_, err = g.DB().Model(chatOperatorTable).Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存客服绑定失败")
	}
	return nil
}

func (s *sSysChat) adminConversationModel(ctx context.Context) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	memberColumns := dao.AdminMember.Columns()
	return g.DB().Model(chatConversationTable+" c").Ctx(ctx).
		LeftJoin(dao.AdminMember.Table()+" u", chatAliasField("u", memberColumns.Id)+"="+chatAliasField("c", "member_id")).
		LeftJoin(dao.ContentProfile.Table()+" p", chatAliasField("p", profileColumns.Id)+"="+chatAliasField("c", "profile_id")).
		LeftJoin(chatMessageTable+" msg", chatAliasField("msg", "conversation_id")+"="+chatAliasField("c", "id")+" AND "+chatAliasField("msg", "deleted_at")+" IS NULL").
		WhereNull(chatAliasField("c", "deleted_at")).
		Group(strings.Join([]string{
			chatAliasField("c", "id"),
			chatAliasField("u", memberColumns.Id),
			chatAliasField("u", memberColumns.Username),
			chatAliasField("u", memberColumns.RealName),
			chatAliasField("u", memberColumns.Mobile),
			chatAliasField("u", memberColumns.Email),
			chatAliasField("u", memberColumns.Avatar),
			chatAliasField("u", memberColumns.Remark),
			chatAliasField("p", profileColumns.Id),
			chatAliasField("p", profileColumns.ProfileNo),
			chatAliasField("p", profileColumns.Title),
			chatAliasField("p", profileColumns.Province),
			chatAliasField("p", profileColumns.City),
			chatAliasField("p", profileColumns.PlainText),
		}, ",")).
		Fields(strings.Join([]string{
			chatAliasField("c", "id"),
			chatAliasField("c", "member_id"),
			chatAliasField("u", memberColumns.Username) + " AS member_username",
			chatAliasField("u", memberColumns.RealName) + " AS member_real_name",
			chatAliasField("u", memberColumns.Mobile) + " AS member_mobile",
			chatAliasField("u", memberColumns.Email) + " AS member_email",
			chatAliasField("u", memberColumns.Avatar) + " AS member_avatar",
			chatAliasField("u", memberColumns.Remark) + " AS member_remark",
			chatAliasField("c", "profile_id"),
			chatAliasField("p", profileColumns.ProfileNo) + " AS profile_no",
			chatAliasField("p", profileColumns.Title) + " AS profile_title",
			chatAliasField("p", profileColumns.Province),
			chatAliasField("p", profileColumns.City),
			chatAliasField("p", profileColumns.PlainText) + " AS profile_text",
			chatAliasField("c", "pocketping_session_id"),
			chatAliasField("c", "tg_chat_id"),
			chatAliasField("c", "tg_message_thread_id"),
			chatAliasField("c", "last_message"),
			chatAliasField("c", "last_message_at"),
			chatAliasField("c", "unread_count"),
			"COUNT(" + chatAliasField("msg", "id") + ") AS message_count",
			chatAliasField("c", "status"),
			chatAliasField("c", "created_at"),
			chatAliasField("c", "updated_at"),
		}, ","))
}

func (s *sSysChat) applyAdminConversationFilter(mod *gdb.Model, in *sysin.AdminChatConversationListInp) *gdb.Model {
	if in.MemberId > 0 {
		mod = mod.Where(chatAliasField("c", "member_id"), in.MemberId)
	}
	if in.ProfileId > 0 {
		mod = mod.Where(chatAliasField("c", "profile_id"), in.ProfileId)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = mod.Where(chatAliasField("c", "status"), status)
	}
	if in.HasTgTopic == 1 {
		mod = mod.WhereGT(chatAliasField("c", "tg_message_thread_id"), 0)
	}
	if in.HasTgTopic == 2 {
		mod = mod.Where(chatAliasField("c", "tg_message_thread_id"), 0)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where(fmt.Sprintf("(%s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ?)",
			chatAliasField("c", "pocketping_session_id"),
			chatAliasField("u", dao.AdminMember.Columns().Username),
			chatAliasField("u", dao.AdminMember.Columns().RealName),
			chatAliasField("u", dao.AdminMember.Columns().Mobile),
			chatAliasField("p", dao.ContentProfile.Columns().ProfileNo),
			chatAliasField("p", dao.ContentProfile.Columns().Title),
		), like, like, like, like, like, like)
	}
	return mod
}

func packAdminChatConversation(row *adminChatConversationRow) *sysin.AdminChatConversationListModel {
	if row == nil {
		return nil
	}
	model := &sysin.AdminChatConversationListModel{
		Id:                row.Id,
		MemberId:          row.MemberId,
		MemberUsername:    row.MemberUsername,
		MemberRealName:    row.MemberRealName,
		MemberMobile:      row.MemberMobile,
		MemberEmail:       row.MemberEmail,
		ProfileId:         row.ProfileId,
		ProfileNo:         row.ProfileNo,
		ProfileTitle:      row.ProfileTitle,
		Province:          row.Province,
		City:              row.City,
		ChatSessionId:     row.ChatSessionId,
		TgChatId:          row.TgChatId,
		TgMessageThreadId: row.TgMessageThreadId,
		LastMessage:       row.LastMessage,
		UnreadCount:       row.UnreadCount,
		MessageCount:      row.MessageCount,
		Status:            row.Status,
	}
	if row.LastMessageAt != nil {
		model.LastMessageAt = row.LastMessageAt.String()
	}
	if row.CreatedAt != nil {
		model.CreatedAt = row.CreatedAt.String()
	}
	if row.UpdatedAt != nil {
		model.UpdatedAt = row.UpdatedAt.String()
	}
	return model
}

func (s *sSysChat) getConversation(ctx context.Context, memberId, profileId int64) (row *chatConversationRow, err error) {
	err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("member_id", memberId).Where("profile_id", profileId).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取聊天会话失败")
	}
	return
}

func (s *sSysChat) ensureGlobalConversation(ctx context.Context, memberId int64) (*chatConversationRow, error) {
	row, err := s.getConversation(ctx, memberId, 0)
	if err != nil {
		return nil, err
	}
	if row != nil {
		if row.PinnedAt == nil {
			_, _ = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"pinned_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
		}
		return row, nil
	}
	binding, err := s.getGlobalBinding(ctx)
	if err != nil {
		return nil, err
	}
	if binding == nil || strings.TrimSpace(binding.TgChatId) == "" || strings.TrimSpace(binding.BotToken) == "" {
		return nil, nil
	}
	now := gtime.Now()
	id, err := g.DB().Model(chatConversationTable).Ctx(ctx).Data(g.Map{
		"member_id":            memberId,
		"profile_id":           0,
		"bot_id":               routeBotId(binding),
		"tg_chat_id":           routeTargetChatId(binding),
		"routing_rule_id":      0,
		"assigned_operator_id": 0,
		"status":               "opened",
		"last_message":         "有问题随时联系悦伴客服",
		"last_message_at":      now,
		"pinned_at":            now,
		"created_at":           now,
		"updated_at":           now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建全局客服会话失败")
	}
	sessionId := chatSessionId(id)
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", id).Data(g.Map{"pocketping_session_id": sessionId, "updated_at": now}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新全局客服会话失败")
	}
	return &chatConversationRow{Id: id, MemberId: memberId, ProfileId: 0, ChatSessionId: sessionId, BotId: routeBotId(binding), TgChatId: routeTargetChatId(binding), Status: "opened", PinnedAt: now}, nil
}

func (s *sSysChat) getConversationById(ctx context.Context, memberId, conversationId int64) (row *chatConversationRow, err error) {
	err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("member_id", memberId).Where("id", conversationId).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取聊天会话失败")
		return
	}
	if row == nil {
		err = gerror.New("聊天会话不存在")
	}
	return
}

func (s *sSysChat) getConversationBySessionId(ctx context.Context, sessionId string) (row *chatConversationRow, err error) {
	err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("pocketping_session_id", sessionId).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取聊天会话失败")
	}
	return
}

func (s *sSysChat) getConversationByTelegramTopic(ctx context.Context, chatID string, topicID int64) (row *chatConversationRow, err error) {
	if strings.TrimSpace(chatID) == "" || topicID <= 0 {
		return nil, nil
	}
	err = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("tg_chat_id", strings.TrimSpace(chatID)).
		Where("tg_message_thread_id", topicID).
		WhereNull("deleted_at").
		Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取Telegram会话失败")
	}
	return
}

func (s *sSysChat) getLatestConversationByTelegramChat(ctx context.Context, chatID string) (row *chatConversationRow, err error) {
	if strings.TrimSpace(chatID) == "" {
		return nil, nil
	}
	err = g.DB().Model(chatConversationTable).Ctx(ctx).
		Where("tg_chat_id", strings.TrimSpace(chatID)).
		WhereNull("deleted_at").
		OrderDesc("updated_at").
		OrderDesc("id").
		Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取Telegram群最近会话失败")
	}
	return
}

func (s *sSysChat) markServiceMessagesRead(ctx context.Context, row *chatConversationRow) error {
	if row == nil {
		return nil
	}
	if err := s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return err
	}
	var messages []*chatMessageRow
	err := g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("conversation_id", row.Id).
		Where("direction", "service").
		Where("status", "unread").
		WhereNull("deleted_at").
		Scan(&messages)
	if err != nil {
		return gerror.Wrap(err, "读取未读客服消息失败")
	}
	if len(messages) == 0 {
		return nil
	}
	now := gtime.Now()
	_, err = g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("conversation_id", row.Id).
		Where("direction", "service").
		Where("status", "unread").
		WhereNull("deleted_at").
		Data(g.Map{"status": "read", "read_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新客服消息已读失败")
	}
	for _, item := range messages {
		if item == nil || item.TgMessageId <= 0 {
			continue
		}
		if reactErr := s.telegramSetMessageReaction(ctx, row, item, "👀"); reactErr != nil {
			g.Log().Warningf(ctx, "设置Telegram已读反应失败 message:%d err:%+v", item.Id, reactErr)
		}
	}
	return nil
}

func (s *sSysChat) markMineMessagesReadByTelegramReply(ctx context.Context, row *chatConversationRow) error {
	if row == nil {
		return nil
	}
	if err := s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return err
	}
	now := gtime.Now()
	_, err := g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("conversation_id", row.Id).
		Where("direction", "mine").
		Where("status", "sent").
		WhereNull("read_at").
		WhereNull("deleted_at").
		Data(g.Map{"status": "read", "read_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新用户消息已读失败")
	}
	return nil
}

func (s *sSysChat) markMineMessageReadById(ctx context.Context, row *chatConversationRow, messageId int64) error {
	if row == nil || messageId <= 0 {
		return nil
	}
	if err := s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return err
	}
	now := gtime.Now()
	_, err := g.DB().Model(chatMessageTable).Ctx(ctx).
		Where("id", messageId).
		Where("conversation_id", row.Id).
		Where("direction", "mine").
		WhereNull("deleted_at").
		Data(g.Map{"status": "read", "read_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新用户消息已读失败")
	}
	return nil
}

func (s *sSysChat) ensureTelegramMessageReadColumns(ctx context.Context) error {
	sqlList := []string{
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_chat_id varchar(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_message_thread_id bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS tg_message_id bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS read_at timestamp`,
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS reply_to_message_id bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE hg_youban_chat_message ADD COLUMN IF NOT EXISTS reactions_json text`,
		`CREATE INDEX IF NOT EXISTS idx_ybcm_tg_message ON hg_youban_chat_message (tg_chat_id, tg_message_id)`,
	}
	if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
		sqlList = []string{
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_chat_id` varchar(128) NOT NULL DEFAULT '' COMMENT 'TG群ID'",
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_message_thread_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG话题ID'",
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `tg_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT 'TG消息ID'",
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `read_at` datetime DEFAULT NULL COMMENT '已读时间'",
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `reply_to_message_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '引用本地消息ID'",
			"ALTER TABLE `hg_youban_chat_message` ADD COLUMN IF NOT EXISTS `reactions_json` text COMMENT '表情反应JSON'",
		}
	}
	for _, sql := range sqlList {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return gerror.Wrap(err, "初始化消息已读字段失败")
		}
	}
	return nil
}

func (s *sSysChat) ensureConversationUserColumns(ctx context.Context) error {
	sqlList := []string{
		`ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS pinned_at timestamp`,
		`ALTER TABLE hg_youban_chat_conversation ADD COLUMN IF NOT EXISTS hidden_before_at timestamp`,
		`CREATE INDEX IF NOT EXISTS idx_ybc_member_pinned_updated ON hg_youban_chat_conversation (member_id, pinned_at, updated_at)`,
	}
	if strings.ToLower(g.DB().GetConfig().Type) != consts.DBPgsql {
		sqlList = []string{
			"ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `pinned_at` datetime DEFAULT NULL COMMENT '置顶时间'",
			"ALTER TABLE `hg_youban_chat_conversation` ADD COLUMN IF NOT EXISTS `hidden_before_at` datetime DEFAULT NULL COMMENT '用户清空记录时间'",
		}
	}
	for _, sql := range sqlList {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			return gerror.Wrap(err, "初始化会话用户字段失败")
		}
	}
	return nil
}

func (s *sSysChat) getBotById(ctx context.Context, id int64) (row *chatBotRow, err error) {
	if id <= 0 {
		return nil, nil
	}
	err = g.DB().Model(chatBotTable).Ctx(ctx).Where("id", id).Where("status", 1).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		err = gerror.Wrap(err, "读取Bot配置失败")
	}
	return
}

func (s *sSysChat) enabledBots(ctx context.Context) (rows []*chatBotRow, err error) {
	err = g.DB().Model(chatBotTable).Ctx(ctx).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "读取启用Bot失败")
	}
	return
}

func (s *sSysChat) packStart(row *chatConversationRow) *sysin.ChatStartModel {
	return &sysin.ChatStartModel{Id: row.Id, ConversationId: row.Id, ContactId: 0, ProfileId: row.ProfileId, Status: row.Status}
}

func (s *sSysChat) getMember(ctx context.Context, memberId int64) (member *entity.AdminMember, err error) {
	err = dao.AdminMember.Ctx(ctx).WherePri(memberId).Scan(&member)
	if err != nil {
		err = gerror.Wrap(err, "读取会员信息失败")
	}
	if member == nil {
		err = gerror.New("会员不存在")
	}
	return
}

func (s *sSysChat) getProfileBrief(ctx context.Context, profileId int64) (profile *profileBrief, err error) {
	cols := dao.ContentProfile.Columns()
	err = dao.ContentProfile.Ctx(ctx).
		Fields(cols.Id, cols.ProfileNo, cols.Title, cols.ChannelId, cols.Province, cols.City, cols.Age, cols.Height, cols.Weight, cols.CupSize, cols.PlainText).
		Where(cols.Id, profileId).
		WhereNull(cols.DeletedAt).
		Scan(&profile)
	if err != nil {
		err = gerror.Wrap(err, "读取女孩资料失败")
	}
	if profile == nil {
		err = gerror.New("女孩资料不存在")
	}
	if err == nil && profile != nil && profile.ChannelId > 0 {
		channelCols := dao.ContentChannel.Columns()
		var channel *entity.ContentChannel
		valueErr := dao.ContentChannel.Ctx(ctx).
			Fields(channelCols.SourceChannelId, channelCols.Username, channelCols.TgChatId).
			Where(channelCols.Id, profile.ChannelId).
			WhereNull(channelCols.DeletedAt).
			Scan(&channel)
		if valueErr != nil {
			err = gerror.Wrap(valueErr, "读取资料来源频道失败")
			return
		}
		if channel != nil {
			profile.SourceChannelId = channel.SourceChannelId
			profile.SourceChannelName = firstNonEmptyString(channel.Username, channel.TgChatId)
		}
	}
	if err == nil && profile != nil {
		profile.SourceMessageId, _ = s.getProfileSourceMessageId(ctx, profile)
	}
	return
}

func (s *sSysChat) getConversationProfileBrief(ctx context.Context, row *chatConversationRow) (*profileBrief, error) {
	if row == nil || row.ProfileId <= 0 {
		return nil, nil
	}
	return s.getProfileBrief(ctx, row.ProfileId)
}

func (s *sSysChat) getProfileSourceMessageId(ctx context.Context, profile *profileBrief) (int64, error) {
	if profile == nil || profile.Id <= 0 {
		return 0, nil
	}
	cols := dao.ContentSourceMap.Columns()
	mod := dao.ContentSourceMap.Ctx(ctx).
		Fields(cols.SourceMessageId).
		Where(cols.ProfileId, profile.Id)
	if profile.SourceChannelId > 0 {
		mod = mod.Where(cols.SourceChannelId, profile.SourceChannelId)
	}
	value, err := mod.OrderDesc(cols.Id).Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料来源消息失败")
	}
	return value.Int64(), nil
}

func (s *sSysChat) memberTelegramTitle(ctx context.Context, member *entity.AdminMember) string {
	name := memberDisplayName(member)
	if member == nil {
		return name
	}
	if member.Id < 0 {
		return fmt.Sprintf("%s [网站用户]", name)
	}
	vip, err := internalService.AdminMember().GetVip(ctx, member.Id)
	if err != nil || vip == nil || !vip.IsVip {
		return fmt.Sprintf("%s [普通用户/会员 0 级]", name)
	}
	return fmt.Sprintf("%s [会员 %d 级]", name, vip.VipLevel)
}

func (s *sSysChat) ensureConversationRoute(ctx context.Context, row *chatConversationRow, binding *chatBindingRow) error {
	if row == nil {
		return nil
	}
	targetChatId := routeTargetChatId(binding)
	botId := routeBotId(binding)
	if row.TgChatId == targetChatId && row.BotId == botId && row.RoutingRuleId == 0 && row.AssignedOperatorId == 0 {
		return nil
	}
	row.TgChatId = targetChatId
	row.BotId = botId
	row.RoutingRuleId = 0
	row.AssignedOperatorId = 0
	_, err := g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{
		"bot_id":               botId,
		"tg_chat_id":           targetChatId,
		"routing_rule_id":      0,
		"assigned_operator_id": 0,
		"updated_at":           gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新聊天路由失败")
	}
	return nil
}

func (s *sSysChat) matchBinding(ctx context.Context, profile *profileBrief) (row *chatBindingRow, err error) {
	if profile == nil {
		return s.getGlobalBinding(ctx)
	}
	channelIds := bindingMatchChannelIds(profile)
	if len(channelIds) > 0 {
		row, err = s.matchBindingByChannelMap(ctx, channelIds)
		if err != nil {
			return nil, err
		}
		if row != nil && strings.TrimSpace(row.TgChatId) != "" {
			return row, nil
		}
	}
	mod := s.bindingModel(ctx)
	if profile.SourceChannelId > 0 && profile.ChannelId > 0 {
		mod = mod.Where("(b.source_channel_id=? OR b.content_channel_id=?)", profile.SourceChannelId, profile.ChannelId)
	} else if profile.SourceChannelId > 0 {
		mod = mod.Where("b.source_channel_id", profile.SourceChannelId)
	} else if profile.ChannelId > 0 {
		mod = mod.Where("b.content_channel_id", profile.ChannelId)
	} else {
		return s.getGlobalBinding(ctx)
	}
	err = mod.OrderDesc("b.updated_at").OrderDesc("b.id").Scan(&row)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "匹配聊天频道绑定失败")
	}
	if row != nil && strings.TrimSpace(row.TgChatId) != "" {
		return row, nil
	}
	return s.getGlobalBinding(ctx)
}

func (s *sSysChat) matchBindingByChannelMap(ctx context.Context, channelIds []int64) (row *chatBindingRow, err error) {
	if len(channelIds) == 0 {
		return nil, nil
	}
	err = s.bindingModel(ctx).
		InnerJoin(chatBindingChannelTable+" bc", "bc.binding_id=b.id").
		WhereIn("bc.channel_id", channelIds).
		OrderDesc("b.updated_at").
		OrderDesc("b.id").
		Scan(&row)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "匹配聊天频道绑定失败")
	}
	return
}

func (s *sSysChat) getGlobalBinding(ctx context.Context) (row *chatBindingRow, err error) {
	err = s.bindingModel(ctx).
		Where("b.bind_type", "global").
		OrderDesc("b.updated_at").
		OrderDesc("b.id").
		Scan(&row)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, gerror.Wrap(err, "读取全局客服群绑定失败")
	}
	return
}

func (s *sSysChat) bindingModel(ctx context.Context) *gdb.Model {
	return g.DB().Model(chatBindingTable+" b").Ctx(ctx).
		LeftJoin(chatBotTable+" bot", "bot.id=b.bot_id AND bot.deleted_at IS NULL").
		Where("b.status", 1).
		WhereNull("b.deleted_at").
		Fields(strings.Join([]string{
			"b.id",
			"b.bind_code",
			"b.bind_type",
			"b.source_channel_id",
			"b.content_channel_id",
			"b.bot_id",
			"bot.bot_name",
			"bot.bot_token",
			"b.tg_chat_id",
			"b.tg_chat_title",
			"b.remark",
			"b.status",
			"b.created_at",
			"b.updated_at",
		}, ","))
}

func routeTargetChatId(binding *chatBindingRow) string {
	if binding != nil && strings.TrimSpace(binding.TgChatId) != "" {
		return strings.TrimSpace(binding.TgChatId)
	}
	return ""
}

func routeBotId(binding *chatBindingRow) int64 {
	if binding == nil {
		return 0
	}
	return binding.BotId
}

func (s *sSysChat) chatSessionPayload(row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) g.Map {
	profileTitle := ""
	if profile != nil {
		profileTitle = strings.TrimSpace(profile.Title)
	}
	if profile != nil && profileTitle == "" {
		profileTitle = profile.ProfileNo
	}
	if profileTitle == "" {
		profileTitle = "全局客服"
	}
	profileId := int64(0)
	profileArea := ""
	if profile != nil {
		profileId = profile.Id
		profileArea = strings.TrimSpace(profile.Province + " " + profile.City)
	}
	return g.Map{
		"id":             row.ChatSessionId,
		"visitorId":      fmt.Sprintf("youban-member-%d", member.Id),
		"createdAt":      time.Now().Format(time.RFC3339),
		"lastActivity":   time.Now().Format(time.RFC3339),
		"operatorOnline": true,
		"identity":       g.Map{"id": fmt.Sprintf("youban-member-%d", member.Id), "name": fmt.Sprintf("%s（%s）", memberDisplayName(member), memberShortId(member.Id)), "email": member.Email},
		"metadata":       g.Map{"pageTitle": profileTitle, "url": fmt.Sprintf("/pages/detail/index?id=%d", profileId), "country": "CN", "city": profileArea},
		"telegramChatId": row.TgChatId,
	}
}

func (s *sSysChat) notifyTelegramSession(ctx context.Context, row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) error {
	botToken, chatID, err := s.telegramTarget(ctx, row)
	if err != nil || botToken == "" || chatID == "" {
		return err
	}
	topicID, err := s.ensureTelegramTopic(ctx, botToken, chatID, row, profile, member)
	if err != nil {
		return gerror.Wrap(err, "创建Telegram会话话题失败")
	}
	visitorName := memberDisplayName(member)
	profileTitle := ""
	if profile != nil {
		profileTitle = strings.TrimSpace(profile.Title)
	}
	if profile != nil && profileTitle == "" {
		profileTitle = profile.ProfileNo
	}
	if profileTitle == "" {
		profileTitle = "全局客服"
	}
	text := telegramSessionNoticeText(ctx, profile, member, visitorName, profileTitle)
	_, err = s.telegramSendMessage(ctx, botToken, chatID, topicID, text)
	if err != nil {
		return gerror.Wrap(err, "发送Telegram会话通知失败")
	}
	return nil
}

func (s *sSysChat) notifyTelegramMessage(ctx context.Context, row *chatConversationRow, profile *profileBrief, member *entity.AdminMember, messageId string, content string, attachments []*sysin.ChatMessageAttachmentModel) error {
	botToken, chatID, err := s.telegramTarget(ctx, row)
	if err != nil || botToken == "" || chatID == "" {
		return err
	}
	needSessionNotice := row.TgMessageThreadId <= 0
	topicID, err := s.ensureTelegramTopic(ctx, botToken, chatID, row, profile, member)
	if err != nil {
		return gerror.Wrap(err, "创建Telegram消息话题失败")
	}
	if needSessionNotice {
		if err = s.sendTelegramSessionNotice(ctx, botToken, chatID, topicID, row, profile, member); err != nil {
			return err
		}
	}
	replyToTelegramID := s.telegramReplyTarget(ctx, row.Id, messageId)
	senderTitle := s.memberTelegramTitle(ctx, member)
	text := fmt.Sprintf("<b>%s</b>：\n%s", senderTitle, strings.TrimSpace(content))
	if strings.TrimSpace(content) == "" && len(attachments) > 0 {
		text = fmt.Sprintf("<b>%s</b>：\n%s", senderTitle, attachmentLastMessage(&sysin.ChatMessageModel{Attachments: attachments}))
	}
	if len(attachments) > 0 {
		var tgMessageID int64
		if tgMessageID, err = s.telegramSendAttachments(ctx, botToken, chatID, topicID, replyToTelegramID, text, attachments); err != nil {
			return gerror.Wrap(err, "发送Telegram附件失败")
		}
		s.persistTelegramMessageTarget(ctx, messageId, chatID, topicID, tgMessageID)
		return nil
	}
	var tgMessageID int64
	tgMessageID, err = s.telegramSendMessageReply(ctx, botToken, chatID, topicID, replyToTelegramID, text)
	if err != nil {
		return gerror.Wrap(err, "发送Telegram消息失败")
	}
	s.persistTelegramMessageTarget(ctx, messageId, chatID, topicID, tgMessageID)
	return nil
}

func (s *sSysChat) telegramReplyTarget(ctx context.Context, conversationID int64, externalMessageID string) int64 {
	var current *chatMessageRow
	if err := g.DB().Model(chatMessageTable).Ctx(ctx).Where("conversation_id", conversationID).Where("pocketping_message_id", externalMessageID).Scan(&current); err != nil || current == nil || current.ReplyToMessageId <= 0 {
		return 0
	}
	value, err := g.DB().Model(chatMessageTable).Ctx(ctx).Where("conversation_id", conversationID).Where("id", current.ReplyToMessageId).Fields("tg_message_id").Value()
	if err != nil {
		return 0
	}
	return value.Int64()
}

func (s *sSysChat) persistTelegramMessageTarget(ctx context.Context, externalMessageID, chatID string, topicID, messageID int64) {
	if messageID <= 0 {
		return
	}
	_, _ = g.DB().Model(chatMessageTable).Ctx(ctx).Where("pocketping_message_id", externalMessageID).Data(g.Map{
		"tg_chat_id": chatID, "tg_message_thread_id": topicID, "tg_message_id": messageID, "updated_at": gtime.Now(),
	}).Update()
}

func (s *sSysChat) sendTelegramSessionNotice(ctx context.Context, botToken, chatID string, topicID int64, row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) error {
	visitorName := memberDisplayName(member)
	profileTitle := ""
	if profile != nil {
		profileTitle = strings.TrimSpace(profile.Title)
	}
	if profile != nil && profileTitle == "" {
		profileTitle = profile.ProfileNo
	}
	if profileTitle == "" {
		profileTitle = "全局客服"
	}
	text := telegramSessionNoticeText(ctx, profile, member, visitorName, profileTitle)
	_, err := s.telegramSendMessage(ctx, botToken, chatID, topicID, text)
	if err != nil {
		return gerror.Wrap(err, "发送Telegram会话通知失败")
	}
	return nil
}

func telegramSessionNoticeText(ctx context.Context, profile *profileBrief, member *entity.AdminMember, visitorName, profileTitle string) string {
	memberId := int64(0)
	if member != nil {
		memberId = member.Id
	}
	lines := []string{
		"<b>新客服会话</b>",
		fmt.Sprintf("用户：%s（%s）", visitorName, memberShortId(memberId)),
		fmt.Sprintf("资料：%s", profileTitle),
	}
	if profile != nil {
		appendProfileLine := func(label string, value interface{}) {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "0" {
				lines = append(lines, fmt.Sprintf("%s：%s", label, text))
			}
		}
		appendProfileLine("省份", profile.Province)
		appendProfileLine("城市", profile.City)
		appendProfileLine("年龄", profile.Age)
		appendProfileLine("身高", profile.Height)
		appendProfileLine("体重", profile.Weight)
		appendProfileLine("罩杯", profile.CupSize)
		area := strings.TrimSpace(profile.Province + " " + profile.City)
		if area != "" {
			lines = append(lines, "地区："+area)
		}
		if link := telegramProfileSourceLink(profile); link != "" {
			lines = append(lines, fmt.Sprintf(`<a href="%s">打开原频道消息</a>`, link))
		}
	}
	return strings.Join(lines, "\n")
}

func telegramProfileSourceLink(profile *profileBrief) string {
	if profile == nil || profile.SourceMessageId <= 0 {
		return ""
	}
	channel := strings.TrimSpace(profile.SourceChannelName)
	if channel == "" {
		return ""
	}
	channel = strings.TrimPrefix(channel, "@")
	if strings.HasPrefix(channel, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(channel, "-100"), profile.SourceMessageId)
	}
	if strings.HasPrefix(channel, "-") {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s/%d", channel, profile.SourceMessageId)
}

func (s *sSysChat) telegramTarget(ctx context.Context, row *chatConversationRow) (botToken string, chatID string, err error) {
	chatID = strings.TrimSpace(row.TgChatId)
	if row.BotId > 0 {
		bot, botErr := s.getBotById(ctx, row.BotId)
		if botErr != nil {
			return "", "", botErr
		}
		if bot != nil {
			botToken = strings.TrimSpace(bot.BotToken)
		}
	}
	if botToken == "" || chatID == "" {
		return "", "", nil
	}
	return botToken, chatID, nil
}

func (s *sSysChat) ensureTelegramTopic(ctx context.Context, botToken, chatID string, row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) (int64, error) {
	if row.TgMessageThreadId > 0 {
		return row.TgMessageThreadId, nil
	}
	lockValue, _ := s.telegramTopicLocks.LoadOrStore(row.Id, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer func() {
		lock.Unlock()
		s.telegramTopicLocks.Delete(row.Id)
	}()
	var persistedThreadID int64
	if err := g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Fields("tg_message_thread_id").Scan(&persistedThreadID); err != nil {
		return 0, gerror.Wrap(err, "读取Telegram话题失败")
	}
	if persistedThreadID > 0 {
		row.TgMessageThreadId = persistedThreadID
		return persistedThreadID, nil
	}
	if enabled, known := s.telegramForumCapability(chatID); known && !enabled {
		return 0, nil
	}
	name := telegramTopicName(row, profile, member)
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return 0, err
	}
	topic, err := bot.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{
		ChatID: chatID,
		Name:   name,
	})
	if err != nil {
		if isTelegramNotForumError(err) {
			s.cacheTelegramForumCapability(chatID, false)
			g.Log().Warningf(ctx, "Telegram群未开启话题，降级发送到普通群 chat:%s conversation:%d", chatID, row.Id)
			return 0, nil
		}
		return 0, err
	}
	if topic == nil || topic.MessageThreadID <= 0 {
		return 0, gerror.New("Telegram 创建话题失败")
	}
	row.TgMessageThreadId = int64(topic.MessageThreadID)
	s.cacheTelegramForumCapability(chatID, true)
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"tg_message_thread_id": topic.MessageThreadID, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "保存Telegram话题失败")
	}
	return int64(topic.MessageThreadID), nil
}

func (s *sSysChat) telegramForumCapability(chatID string) (bool, bool) {
	s.telegramCapabilityMu.RLock()
	capability, ok := s.telegramForumCapabilities[strings.TrimSpace(chatID)]
	s.telegramCapabilityMu.RUnlock()
	if !ok || time.Now().After(capability.expiresAt) {
		return false, false
	}
	return capability.enabled, true
}

func (s *sSysChat) cacheTelegramForumCapability(chatID string, enabled bool) {
	key := strings.TrimSpace(chatID)
	if key == "" {
		return
	}
	s.telegramCapabilityMu.Lock()
	if s.telegramForumCapabilities == nil {
		s.telegramForumCapabilities = make(map[string]telegramForumCapability)
	}
	s.telegramForumCapabilities[key] = telegramForumCapability{enabled: enabled, expiresAt: time.Now().Add(10 * time.Minute)}
	s.telegramCapabilityMu.Unlock()
}

func isTelegramNotForumError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "chat is not a forum") || strings.Contains(message, "not a forum")
}

func (s *sSysChat) telegramSendMessage(ctx context.Context, botToken, chatID string, topicID int64, text string) (int64, error) {
	return s.telegramSendMessageReply(ctx, botToken, chatID, topicID, 0, text)
}

func (s *sSysChat) telegramSendMessageReply(ctx context.Context, botToken, chatID string, topicID, replyToMessageID int64, text string) (int64, error) {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return 0, err
	}
	params := &tgbot.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML}
	if topicID > 0 {
		params.MessageThreadID = int(topicID)
	}
	if replyToMessageID > 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyToMessageID)}
	}
	message, err := bot.SendMessage(ctx, params)
	if err != nil {
		return 0, err
	}
	if message == nil {
		return 0, nil
	}
	return int64(message.ID), nil
}

func (s *sSysChat) telegramSendAttachments(ctx context.Context, botToken, chatID string, topicID, replyToMessageID int64, caption string, attachments []*sysin.ChatMessageAttachmentModel) (int64, error) {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return 0, err
	}
	var firstMessageID int64
	for i, item := range attachments {
		if item == nil {
			continue
		}
		file := telegramInputFile(item)
		if file == nil {
			continue
		}
		itemCaption := caption
		if i > 0 {
			itemCaption = ""
		}
		switch item.FileType {
		case "image":
			params := &tgbot.SendPhotoParams{ChatID: chatID, Photo: file, Caption: itemCaption, ParseMode: models.ParseModeHTML}
			if topicID > 0 {
				params.MessageThreadID = int(topicID)
			}
			if i == 0 && replyToMessageID > 0 {
				params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyToMessageID)}
			}
			if sent, sendErr := bot.SendPhoto(ctx, params); sendErr != nil {
				return 0, sendErr
			} else if firstMessageID == 0 && sent != nil {
				firstMessageID = int64(sent.ID)
			}
		case "video":
			params := &tgbot.SendVideoParams{ChatID: chatID, Video: file, Caption: itemCaption, ParseMode: models.ParseModeHTML, SupportsStreaming: true}
			if topicID > 0 {
				params.MessageThreadID = int(topicID)
			}
			if i == 0 && replyToMessageID > 0 {
				params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyToMessageID)}
			}
			if sent, sendErr := bot.SendVideo(ctx, params); sendErr != nil {
				return 0, sendErr
			} else if firstMessageID == 0 && sent != nil {
				firstMessageID = int64(sent.ID)
			}
		default:
			params := &tgbot.SendDocumentParams{ChatID: chatID, Document: file, Caption: itemCaption, ParseMode: models.ParseModeHTML}
			if topicID > 0 {
				params.MessageThreadID = int(topicID)
			}
			if i == 0 && replyToMessageID > 0 {
				params.ReplyParameters = &models.ReplyParameters{MessageID: int(replyToMessageID)}
			}
			if sent, sendErr := bot.SendDocument(ctx, params); sendErr != nil {
				return 0, sendErr
			} else if firstMessageID == 0 && sent != nil {
				firstMessageID = int64(sent.ID)
			}
		}
	}
	return firstMessageID, nil
}

func telegramInputFile(item *sysin.ChatMessageAttachmentModel) models.InputFile {
	if item == nil {
		return nil
	}
	if len(item.Data) > 0 {
		filename := strings.TrimSpace(item.Name)
		if filename == "" {
			filename = "attachment"
		}
		return &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(item.Data)}
	}
	if url := fallbackString(item.DataUrl, item.FallbackUrl); url != "" {
		return &models.InputFileString{Data: url}
	}
	return nil
}

func (s *sSysChat) telegramBot(ctx context.Context, botToken string) (*tgbot.Bot, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, gerror.New("Telegram Bot Token未配置")
	}
	return gatewayservice.Gateway().Client(ctx, botToken)
}

func (s *sSysChat) telegramBotProfile(ctx context.Context, botToken string) (*models.User, error) {
	profileCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	user, err := gatewayservice.Gateway().Probe(profileCtx, strings.TrimSpace(botToken))
	if err != nil {
		if profileCtx.Err() == context.DeadlineExceeded {
			return nil, gerror.New("连接Telegram超时，请检查服务器网络后重试")
		}
		return nil, err
	}
	if user == nil || !user.IsBot {
		return nil, gerror.New("Token不是有效的Telegram Bot")
	}
	return user, nil
}

func (s *sSysChat) telegramMessageAttachments(ctx context.Context, row *chatConversationRow, msg *sysin.TelegramMessageInp) ([]*sysin.ChatMessageAttachmentModel, error) {
	if msg == nil {
		return nil, nil
	}
	botToken, _, err := s.telegramTarget(ctx, row)
	if err != nil {
		return nil, err
	}
	if botToken == "" {
		return nil, nil
	}
	var files []telegramIncomingFile
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		files = append(files, telegramIncomingFile{FileID: photo.FileId, FileName: fmt.Sprintf("telegram_%d_%d.jpg", msg.Chat.Id, msg.MessageId), FileType: "image"})
	}
	if msg.Video != nil {
		files = append(files, telegramIncomingFile{FileID: msg.Video.FileId, FileName: fallbackString(msg.Video.FileName, fmt.Sprintf("telegram_%d_%d.mp4", msg.Chat.Id, msg.MessageId)), FileType: "video"})
	}
	if msg.Document != nil {
		files = append(files, telegramIncomingFile{FileID: msg.Document.FileId, FileName: fallbackString(msg.Document.FileName, fmt.Sprintf("telegram_%d_%d", msg.Chat.Id, msg.MessageId)), FileType: attachmentTypeByMime(msg.Document.MimeType)})
	}
	if msg.Animation != nil {
		files = append(files, telegramIncomingFile{FileID: msg.Animation.FileId, FileName: fallbackString(msg.Animation.FileName, fmt.Sprintf("telegram_%d_%d.mp4", msg.Chat.Id, msg.MessageId)), FileType: "video"})
	}
	if msg.Sticker != nil {
		ext := ".webp"
		fileType := "image"
		convertTGS := false
		if msg.Sticker.IsVideo {
			ext = ".webm"
			fileType = "video"
		} else if msg.Sticker.IsAnimated {
			ext = ".tgs"
			fileType = "file"
			convertTGS = true
		}
		fileID := msg.Sticker.FileId
		if msg.Sticker.IsAnimated && msg.Sticker.Thumbnail != nil && strings.TrimSpace(msg.Sticker.Thumbnail.FileId) != "" {
			fileID = msg.Sticker.Thumbnail.FileId
			ext = ".jpg"
			fileType = "image"
			convertTGS = false
		}
		files = append(files, telegramIncomingFile{FileID: fileID, FileName: fmt.Sprintf("telegram_sticker_%d_%d%s", msg.Chat.Id, msg.MessageId, ext), FileType: fileType, ConvertTGS: convertTGS})
	}
	customEmojiFiles, err := s.telegramCustomEmojiFiles(ctx, botToken, msg)
	if err != nil {
		g.Log().Warningf(ctx, "读取Telegram自定义表情失败 message:%d err:%+v", msg.MessageId, err)
	} else {
		files = append(files, customEmojiFiles...)
	}
	attachments := make([]*sysin.ChatMessageAttachmentModel, 0, len(files))
	for _, file := range files {
		item, itemErr := s.saveTelegramFileAttachment(ctx, botToken, file)
		if itemErr != nil {
			if file.Optional {
				g.Log().Warningf(ctx, "保存Telegram可选附件失败 message:%d file:%s type:%s convert:%v err:%+v", msg.MessageId, file.FileName, file.FileType, file.ConvertTGS, itemErr)
				continue
			}
			return nil, itemErr
		}
		if item != nil {
			attachments = append(attachments, item)
		}
	}
	return attachments, nil
}

func (s *sSysChat) telegramCustomEmojiFiles(ctx context.Context, botToken string, msg *sysin.TelegramMessageInp) ([]telegramIncomingFile, error) {
	if msg == nil || (len(msg.Entities) == 0 && len(msg.CaptionEntities) == 0) {
		return nil, nil
	}
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, entity := range telegramMessageEntities(msg) {
		if entity == nil || entity.Type != "custom_emoji" || strings.TrimSpace(entity.CustomEmojiId) == "" {
			continue
		}
		id := strings.TrimSpace(entity.CustomEmojiId)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	g.Log().Infof(ctx, "Telegram自定义表情 message:%d ids:%v", msg.MessageId, ids)
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, err
	}
	stickers, err := bot.GetCustomEmojiStickers(ctx, &tgbot.GetCustomEmojiStickersParams{CustomEmojiIDs: ids})
	if err != nil {
		return nil, err
	}
	files := make([]telegramIncomingFile, 0, len(stickers))
	for _, sticker := range stickers {
		if sticker == nil || strings.TrimSpace(sticker.FileID) == "" {
			continue
		}
		ext := ".webp"
		fileType := "image"
		fileID := sticker.FileID
		if sticker.IsVideo {
			ext = ".webm"
			fileType = "video"
		} else if sticker.IsAnimated {
			ext = ".tgs"
			fileType = "file"
			if sticker.Thumbnail != nil && strings.TrimSpace(sticker.Thumbnail.FileID) != "" {
				fileID = sticker.Thumbnail.FileID
				ext = ".jpg"
				fileType = "image"
			}
		}
		files = append(files, telegramIncomingFile{
			FileID:     fileID,
			FileName:   fmt.Sprintf("telegram_custom_emoji_%d_%d%s", msg.Chat.Id, msg.MessageId, ext),
			FileType:   fileType,
			ConvertTGS: ext == ".tgs",
		})
	}
	g.Log().Infof(ctx, "Telegram自定义表情文件 message:%d stickers:%d files:%d", msg.MessageId, len(stickers), len(files))
	return files, nil
}

type telegramIncomingFile struct {
	FileID     string
	FileName   string
	FileType   string
	ConvertTGS bool
	Optional   bool
}

func (s *sSysChat) saveTelegramFileAttachment(ctx context.Context, botToken string, file telegramIncomingFile) (*sysin.ChatMessageAttachmentModel, error) {
	if strings.TrimSpace(file.FileID) == "" {
		return nil, nil
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, err
	}
	data, _, err := telegrammedia.Download(ctx, bot, file.FileID)
	if err != nil {
		return nil, gerror.Wrapf(err, "下载Telegram附件失败 fileId:%s", file.FileID)
	}
	name := fallbackString(file.FileName, "telegram_attachment")
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if file.ConvertTGS {
		converted, convertedName, convertedType, convertErr := convertTelegramTGS(ctx, name, data)
		if convertErr != nil {
			return nil, convertErr
		}
		data = converted
		name = convertedName
		file.FileType = convertedType
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	fileType := normalizeAttachmentType(file.FileType, name, contentType)
	name = ensureAttachmentExt(name, fileType, contentType)
	attachment, err := storager.DoUpload(ctx, storagerKindByFileType(fileType), bytesUploadFile(name, data))
	if err != nil {
		return nil, gerror.Wrapf(err, "保存Telegram附件失败 file:%s type:%s contentType:%s size:%d", name, fileType, contentType, len(data))
	}
	url := storager.LastUrl(ctx, attachment.FileUrl, attachment.Drive)
	return &sysin.ChatMessageAttachmentModel{Id: attachment.Id, Name: attachment.Name, FileType: fileType, DataUrl: url, ThumbUrl: url, FallbackUrl: url}, nil
}

func convertTelegramTGS(ctx context.Context, name string, data []byte) ([]byte, string, string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	gif, err := transformTelegramTGS(timeoutCtx, data, converter.FormatGIF)
	if err == nil && len(gif) > 0 {
		return gif, replaceFileExt(name, ".gif"), "image", nil
	}
	g.Log().Warningf(ctx, "转换Telegram TGS为GIF失败 file:%s err:%+v", name, err)

	png, err := transformTelegramTGS(timeoutCtx, data, converter.FormatPNG)
	if err == nil && len(png) > 0 {
		return png, replaceFileExt(name, ".png"), "image", nil
	}
	return nil, "", "", gerror.Wrap(err, "转换Telegram TGS失败")
}

func transformTelegramTGS(ctx context.Context, data []byte, format converter.OutputFormat) ([]byte, error) {
	var out bytes.Buffer
	options := converter.TGSTransformOptions{
		Format:       format,
		Frame:        converter.FrameFirst,
		ResizeWidth:  256,
		ResizeHeight: 256,
		CacheKey:     fmt.Sprintf("youban_chat_%d", time.Now().UnixNano()),
	}
	if format == converter.FormatGIF {
		options.Frame = converter.FrameAll
	}
	if err := tgs.NewConverter().Transform(ctx, bytes.NewReader(data), &out, options); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func telegramUpdateToWebhookInp(update *models.Update) (*sysin.TelegramWebhookInp, error) {
	if update == nil {
		return nil, nil
	}
	in := &sysin.TelegramWebhookInp{
		UpdateId:      int64(update.ID),
		Message:       telegramSDKMessageToInp(update.Message),
		EditedMessage: telegramSDKMessageToInp(update.EditedMessage),
	}
	if update.MessageReaction != nil {
		reaction := update.MessageReaction
		actorID := int64(0)
		if reaction.User != nil {
			actorID = reaction.User.ID
		}
		emojis := make([]string, 0, len(reaction.NewReaction))
		for _, item := range reaction.NewReaction {
			if item.ReactionTypeEmoji != nil {
				emojis = append(emojis, item.ReactionTypeEmoji.Emoji)
			}
		}
		in.MessageReaction = &sysin.TelegramReactionInp{ChatId: reaction.Chat.ID, MessageId: int64(reaction.MessageID), ActorId: actorID, NewReaction: emojis}
	}
	return in, nil
}

func telegramSDKMessageToInp(msg *models.Message) *sysin.TelegramMessageInp {
	if msg == nil {
		return nil
	}
	return &sysin.TelegramMessageInp{
		MessageId:       int64(msg.ID),
		MessageThreadId: int64(msg.MessageThreadID),
		Text:            msg.Text,
		Caption:         msg.Caption,
		Chat:            telegramSDKChatToInp(&msg.Chat),
		From:            telegramSDKUserToInp(msg.From),
		ReplyTo:         telegramSDKMessageToInp(msg.ReplyToMessage),
		Entities:        telegramSDKEntitiesToInp(msg.Entities),
		CaptionEntities: telegramSDKEntitiesToInp(msg.CaptionEntities),
		Photo:           telegramSDKPhotosToInp(msg.Photo),
		Video:           telegramSDKVideoToInp(msg.Video),
		Document:        telegramSDKDocumentToInp(msg.Document),
		Sticker:         telegramSDKStickerToInp(msg.Sticker),
		Animation:       telegramSDKAnimationToInp(msg.Animation),
	}
}

func telegramSDKChatToInp(chat *models.Chat) *sysin.TelegramChatInp {
	if chat == nil {
		return nil
	}
	return &sysin.TelegramChatInp{Id: chat.ID, Type: string(chat.Type), Title: chat.Title, Username: chat.Username}
}

func telegramSDKUserToInp(user *models.User) *sysin.TelegramUserInp {
	if user == nil {
		return nil
	}
	return &sysin.TelegramUserInp{Id: user.ID, IsBot: user.IsBot, FirstName: user.FirstName, LastName: user.LastName, Username: user.Username}
}

func telegramSDKEntitiesToInp(entities []models.MessageEntity) []*sysin.TelegramEntityInp {
	if len(entities) == 0 {
		return nil
	}
	result := make([]*sysin.TelegramEntityInp, 0, len(entities))
	for _, entity := range entities {
		result = append(result, &sysin.TelegramEntityInp{
			Type:          string(entity.Type),
			Offset:        entity.Offset,
			Length:        entity.Length,
			CustomEmojiId: entity.CustomEmojiID,
		})
	}
	return result
}

func telegramSDKPhotosToInp(photos []models.PhotoSize) []*sysin.TelegramPhotoInp {
	if len(photos) == 0 {
		return nil
	}
	result := make([]*sysin.TelegramPhotoInp, 0, len(photos))
	for _, photo := range photos {
		result = append(result, telegramSDKPhotoToInp(&photo))
	}
	return result
}

func telegramSDKPhotoToInp(photo *models.PhotoSize) *sysin.TelegramPhotoInp {
	if photo == nil {
		return nil
	}
	return &sysin.TelegramPhotoInp{FileId: photo.FileID, FileUniqueId: photo.FileUniqueID, Width: photo.Width, Height: photo.Height, FileSize: int64(photo.FileSize)}
}

func telegramSDKVideoToInp(video *models.Video) *sysin.TelegramFileInp {
	if video == nil {
		return nil
	}
	return &sysin.TelegramFileInp{FileId: video.FileID, FileUniqueId: video.FileUniqueID, FileName: video.FileName, MimeType: video.MimeType, FileSize: video.FileSize, Width: video.Width, Height: video.Height, Duration: video.Duration}
}

func telegramSDKDocumentToInp(document *models.Document) *sysin.TelegramFileInp {
	if document == nil {
		return nil
	}
	return &sysin.TelegramFileInp{FileId: document.FileID, FileUniqueId: document.FileUniqueID, FileName: document.FileName, MimeType: document.MimeType, FileSize: document.FileSize}
}

func telegramSDKAnimationToInp(animation *models.Animation) *sysin.TelegramFileInp {
	if animation == nil {
		return nil
	}
	return &sysin.TelegramFileInp{FileId: animation.FileID, FileUniqueId: animation.FileUniqueID, FileName: animation.FileName, MimeType: animation.MimeType, FileSize: animation.FileSize, Width: animation.Width, Height: animation.Height, Duration: animation.Duration}
}

func telegramSDKStickerToInp(sticker *models.Sticker) *sysin.TelegramStickerInp {
	if sticker == nil {
		return nil
	}
	return &sysin.TelegramStickerInp{
		FileId:       sticker.FileID,
		FileUniqueId: sticker.FileUniqueID,
		Type:         sticker.Type,
		Emoji:        sticker.Emoji,
		SetName:      sticker.SetName,
		FileSize:     int64(sticker.FileSize),
		Width:        sticker.Width,
		Height:       sticker.Height,
		IsAnimated:   sticker.IsAnimated,
		IsVideo:      sticker.IsVideo,
		Thumbnail:    telegramSDKPhotoToInp(sticker.Thumbnail),
	}
}

func (s *sSysChat) saveMessage(ctx context.Context, conversationId int64, externalMessageId, direction, content, contentType, status, senderName string, attachments []*sysin.ChatMessageAttachmentModel) (id int64, inserted bool, err error) {
	attachmentsJson := "[]"
	if len(attachments) > 0 {
		bytes, _ := json.Marshal(attachments)
		attachmentsJson = string(bytes)
	}
	data := g.Map{"conversation_id": conversationId, "pocketping_message_id": externalMessageId, "direction": direction, "content": content, "content_type": contentType, "status": status, "sender_name": senderName, "attachments_json": attachmentsJson, "created_at": gtime.Now(), "updated_at": gtime.Now()}
	id, err = g.DB().Model(chatMessageTable).Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			value, valueErr := g.DB().Model(chatMessageTable).Ctx(ctx).Where("pocketping_message_id", externalMessageId).Fields("id").Value()
			if valueErr == nil {
				return value.Int64(), false, nil
			}
		}
		err = gerror.Wrap(err, "保存聊天消息失败")
		return 0, false, err
	}
	return id, true, nil
}

func (s *sSysChat) saveTelegramInboundMessage(ctx context.Context, row *chatConversationRow, msg *sysin.TelegramMessageInp, externalMessageId, content, contentType, senderName string, attachments []*sysin.ChatMessageAttachmentModel) (id int64, inserted bool, err error) {
	if row == nil || msg == nil || msg.Chat == nil {
		return 0, false, nil
	}
	if err = s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return 0, false, err
	}
	attachmentsJson := "[]"
	if len(attachments) > 0 {
		bytes, _ := json.Marshal(attachments)
		attachmentsJson = string(bytes)
	}
	replyToMessageID := int64(0)
	if msg.ReplyTo != nil && msg.ReplyTo.MessageId > 0 {
		value, valueErr := g.DB().Model(chatMessageTable).Ctx(ctx).
			Where("conversation_id", row.Id).
			Where("tg_chat_id", fmt.Sprintf("%d", msg.Chat.Id)).
			Where("tg_message_id", msg.ReplyTo.MessageId).
			Fields("id").Value()
		if valueErr == nil {
			replyToMessageID = value.Int64()
		}
	}
	data := g.Map{
		"conversation_id":       row.Id,
		"pocketping_message_id": externalMessageId,
		"direction":             "service",
		"content":               content,
		"content_type":          contentType,
		"status":                "unread",
		"sender_name":           senderName,
		"attachments_json":      attachmentsJson,
		"reply_to_message_id":   replyToMessageID,
		"tg_chat_id":            fmt.Sprintf("%d", msg.Chat.Id),
		"tg_message_thread_id":  msg.MessageThreadId,
		"tg_message_id":         msg.MessageId,
		"created_at":            gtime.Now(),
		"updated_at":            gtime.Now(),
	}
	id, err = g.DB().Model(chatMessageTable).Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			value, valueErr := g.DB().Model(chatMessageTable).Ctx(ctx).Where("pocketping_message_id", externalMessageId).Fields("id").Value()
			if valueErr == nil {
				return value.Int64(), false, nil
			}
		}
		err = gerror.Wrap(err, "保存Telegram客服消息失败")
		return 0, false, err
	}
	return id, true, nil
}

func (s *sSysChat) telegramSetMessageReaction(ctx context.Context, row *chatConversationRow, msg *chatMessageRow, emoji string) error {
	if row == nil || msg == nil || msg.TgMessageId <= 0 {
		return nil
	}
	botToken, chatID, err := s.telegramTarget(ctx, row)
	if err != nil || botToken == "" {
		return err
	}
	if strings.TrimSpace(msg.TgChatId) != "" {
		chatID = strings.TrimSpace(msg.TgChatId)
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	reaction := []models.ReactionType(nil)
	if strings.TrimSpace(emoji) != "" {
		reaction = []models.ReactionType{{Type: models.ReactionTypeTypeEmoji, ReactionTypeEmoji: &models.ReactionTypeEmoji{Emoji: emoji}}}
	}
	_, err = bot.SetMessageReaction(ctx, &tgbot.SetMessageReactionParams{
		ChatID:    chatID,
		MessageID: int(msg.TgMessageId),
		Reaction:  reaction,
	})
	return err
}

func packLocalChatMessage(item *chatMessageRow) *sysin.ChatMessageModel {
	attachments := []*sysin.ChatMessageAttachmentModel{}
	if strings.TrimSpace(item.AttachmentsJson) != "" {
		_ = json.Unmarshal([]byte(item.AttachmentsJson), &attachments)
	}
	createdAt := ""
	if item.CreatedAt != nil {
		createdAt = item.CreatedAt.String()
	}
	readAt := ""
	if item.ReadAt != nil {
		readAt = item.ReadAt.String()
	}
	reactions := map[string][]string{}
	if strings.TrimSpace(item.ReactionsJson) != "" {
		_ = json.Unmarshal([]byte(item.ReactionsJson), &reactions)
	}
	return &sysin.ChatMessageModel{Id: item.Id, ClientMessageId: item.ExternalMessageId, ConversationId: item.ConversationId, Direction: item.Direction, Content: item.Content, ContentType: item.ContentType, Status: item.Status, SenderName: item.SenderName, CreatedAt: createdAt, ReadAt: readAt, Attachments: attachments, Reactions: reactions}
}

func packApiChatMessage(item *chatMessageRow) *sysin.ChatMessageModel {
	message := packLocalChatMessage(item)
	message.SenderName = ""
	if message.Direction == "service" && message.ContentType == "image" && isTelegramVisualEmojiMessage(message) {
		message.Content = ""
	}
	return message
}

func isTelegramVisualEmojiMessage(message *sysin.ChatMessageModel) bool {
	if message == nil || !strings.HasPrefix(message.ClientMessageId, "telegram_") {
		return false
	}
	for _, attachment := range message.Attachments {
		if attachment == nil {
			continue
		}
		name := strings.TrimSpace(attachment.Name)
		if strings.HasPrefix(name, "telegram_sticker_") || strings.HasPrefix(name, "telegram_custom_emoji_") {
			return true
		}
	}
	return false
}

func normalizeClientMessageId(clientMessageId string, memberId int64) string {
	clientMessageId = strings.TrimSpace(clientMessageId)
	if clientMessageId != "" {
		return clientMessageId
	}
	return fmt.Sprintf("msg_%d_%d", memberId, time.Now().UnixNano())
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func chatSessionId(conversationId int64) string {
	return fmt.Sprintf("youban-conv-%d", conversationId)
}

func memberShortId(memberId int64) string {
	if memberId < 0 {
		memberId = -memberId
	}
	return fmt.Sprintf("U%05d", memberId%100000)
}

func memberDisplayName(member *entity.AdminMember) string {
	for _, item := range []string{member.RealName, member.Username, member.Mobile, member.Email} {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return "悦伴用户"
}

func attachmentLastMessage(message *sysin.ChatMessageModel) string {
	if strings.TrimSpace(message.Content) != "" {
		return message.Content
	}
	for _, item := range message.Attachments {
		switch item.FileType {
		case "image":
			return "[图片]"
		case "video":
			return "[视频]"
		default:
			return "[附件]"
		}
	}
	return "[附件]"
}

func localUploadAttachment(ctx context.Context, file *ghttp.UploadFile) (*sysin.ChatMessageAttachmentModel, error) {
	opened, err := file.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传文件失败")
	}
	defer opened.Close()
	data, err := io.ReadAll(opened)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传文件失败")
	}
	contentType := ""
	if file.FileHeader != nil && file.FileHeader.Header != nil {
		contentType = file.FileHeader.Header.Get("Content-Type")
	}
	fileType := normalizeAttachmentType("", file.Filename, contentType)
	name := ensureAttachmentExt(file.Filename, fileType, contentType)
	attachment, err := storager.DoUpload(ctx, storagerKindByFileType(fileType), bytesUploadFile(name, data))
	if err != nil {
		return nil, err
	}
	url := storager.LastUrl(ctx, attachment.FileUrl, attachment.Drive)
	return &sysin.ChatMessageAttachmentModel{Id: attachment.Id, Name: attachment.Name, FileType: fileType, FallbackUrl: url, DataUrl: url, ThumbUrl: url, Data: data}, nil
}

func (s *sSysChat) bindTelegramChatByCode(ctx context.Context, code string, msg *sysin.TelegramMessageInp, botId int64) error {
	if msg == nil || msg.Chat == nil || msg.Chat.Id == 0 {
		return nil
	}
	code = strings.TrimSpace(code)
	token, _ := s.telegramTokenForIncoming(ctx, botId)
	var row *chatBindingRow
	err := g.DB().Model(chatBindingTable).Ctx(ctx).
		Where("LOWER(bind_code)=?", strings.ToLower(code)).
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&row)
	if err != nil {
		return gerror.Wrap(err, "读取绑定码失败")
	}
	if row == nil {
		if token != "" {
			_, _ = s.telegramSendMessage(ctx, token, fmt.Sprintf("%d", msg.Chat.Id), msg.MessageThreadId, "绑定码不存在或已禁用："+code)
		}
		g.Log().Warningf(ctx, "Telegram绑定码不存在 bot:%d chat:%d code:%s", botId, msg.Chat.Id, code)
		return nil
	}
	if botId > 0 && row.BotId > 0 && row.BotId != botId {
		if token != "" {
			_, _ = s.telegramSendMessage(ctx, token, fmt.Sprintf("%d", msg.Chat.Id), msg.MessageThreadId, "绑定码与当前Bot不匹配")
		}
		return nil
	}
	data := g.Map{"tg_chat_id": fmt.Sprintf("%d", msg.Chat.Id), "tg_chat_title": telegramChatTitle(msg.Chat), "updated_at": gtime.Now()}
	if botId > 0 {
		data["bot_id"] = botId
	}
	_, err = g.DB().Model(chatBindingTable).Ctx(ctx).
		Where("id", row.Id).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "绑定Telegram群失败")
	}
	if botId > 0 {
		row.BotId = botId
	}
	if token == "" && row.BotId > 0 {
		bot, _ := s.getBotById(ctx, row.BotId)
		if bot != nil {
			token = bot.BotToken
		}
	}
	if token != "" {
		_, _ = s.telegramSendMessage(ctx, token, fmt.Sprintf("%d", msg.Chat.Id), msg.MessageThreadId, "绑定成功："+code)
	}
	return nil
}

func extractTelegramBindCode(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	for i, item := range fields {
		token := strings.TrimSpace(item)
		upper := strings.ToUpper(token)
		if upper == "/BIND" || upper == "BIND" || strings.HasPrefix(upper, "/BIND@") || token == "绑定" {
			if i+1 < len(fields) {
				return strings.TrimSpace(fields[i+1])
			}
			return ""
		}
		if strings.HasPrefix(upper, "@") {
			continue
		}
	}
	return ""
}

func randomBindCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 7)
	for i := range code {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return fallbackBindCode(chars)
		}
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

func fallbackBindCode(chars string) string {
	code := make([]byte, 7)
	seed := time.Now().UnixNano()
	for i := range code {
		seed = seed*1664525 + 1013904223
		if seed < 0 {
			seed = -seed
		}
		code[i] = chars[seed%int64(len(chars))]
	}
	return string(code)
}

func telegramTopicName(row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) string {
	memberName := "用户"
	if member != nil {
		memberName = memberDisplayName(member)
	}
	if profile == nil || row == nil || row.ProfileId <= 0 {
		return truncateTelegramTopicName(memberName)
	}
	profileNo := ""
	if profile != nil {
		profileNo = strings.TrimSpace(profile.ProfileNo)
	}
	if profileNo == "" && row != nil {
		profileNo = fmt.Sprintf("P%d", row.ProfileId)
	}
	name := strings.TrimSpace(profileNo + " " + memberName)
	return truncateTelegramTopicName(name)
}

func truncateTelegramTopicName(name string) string {
	if len([]rune(name)) > 120 {
		return string([]rune(name)[:120])
	}
	return name
}

func telegramChatTitle(chat *sysin.TelegramChatInp) string {
	if chat == nil {
		return ""
	}
	for _, item := range []string{chat.Title, chat.Username} {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return fmt.Sprintf("%d", chat.Id)
}

func telegramUserName(user *sysin.TelegramUserInp) string {
	if user == nil {
		return ""
	}
	if strings.TrimSpace(user.Username) != "" {
		return "@" + strings.TrimSpace(user.Username)
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name != "" {
		return name
	}
	return fmt.Sprintf("%d", user.Id)
}

func telegramBotDisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name != "" {
		return name
	}
	if strings.TrimSpace(user.Username) != "" {
		return "@" + strings.TrimSpace(user.Username)
	}
	return fmt.Sprintf("%d", user.ID)
}

func attachmentTypeByName(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if ext == "" {
		return "file"
	}
	if storager.IsVideoType(ext) {
		return "video"
	}
	if storager.IsImgType(ext) {
		return "image"
	}
	return "file"
}

func normalizeAttachmentType(fileType, name, contentType string) string {
	fileType = strings.ToLower(strings.TrimSpace(fileType))
	if fileType == "image" || fileType == "video" {
		return fileType
	}
	if byName := attachmentTypeByName(name); byName != "file" {
		return byName
	}
	if byMime := attachmentTypeByMime(contentType); byMime != "file" {
		return byMime
	}
	return "file"
}

func ensureAttachmentExt(name, fileType, contentType string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "attachment"
	}
	if strings.TrimSpace(filepath.Ext(name)) != "" {
		return name
	}
	ext := attachmentExtByMime(contentType)
	if ext == "" {
		switch fileType {
		case "image":
			ext = ".jpg"
		case "video":
			ext = ".mp4"
		}
	}
	if ext == "" {
		return name
	}
	return name + ext
}

func attachmentExtByMime(contentType string) string {
	contentType = strings.TrimSpace(strings.Split(strings.ToLower(contentType), ";")[0])
	if contentType == "" {
		return ""
	}
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	}
	exts, err := mime.ExtensionsByType(contentType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	return exts[0]
}

func replaceFileExt(name string, ext string) string {
	if strings.TrimSpace(name) == "" {
		return "telegram_attachment" + ext
	}
	index := strings.LastIndex(name, ".")
	if index <= 0 {
		return name + ext
	}
	return name[:index] + ext
}

func storagerKindByFileName(name string) string {
	switch attachmentTypeByName(name) {
	case "image":
		return storager.KindImg
	case "video":
		return storager.KindVideo
	default:
		return storager.KindOther
	}
}

func storagerKindByFileType(fileType string) string {
	switch fileType {
	case "image":
		return storager.KindImg
	case "video":
		return storager.KindVideo
	default:
		return storager.KindOther
	}
}

func bindingMatchChannelIds(profile *profileBrief) []int64 {
	if profile == nil {
		return nil
	}
	return uniquePositiveInt64s([]int64{profile.SourceChannelId, profile.ChannelId})
}

func appendBindingChannelIds(ids []int64, extra ...int64) []int64 {
	out := make([]int64, 0, len(ids)+len(extra))
	out = append(out, ids...)
	out = append(out, extra...)
	return uniquePositiveInt64s(out)
}

func uniquePositiveInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func attachmentTypeByMime(mime string) string {
	mime = strings.TrimSpace(strings.Split(strings.ToLower(mime), ";")[0])
	if strings.HasPrefix(mime, "image/") {
		return "image"
	}
	if strings.HasPrefix(mime, "video/") {
		return "video"
	}
	return "file"
}

func telegramMessageEntities(msg *sysin.TelegramMessageInp) []*sysin.TelegramEntityInp {
	if msg == nil {
		return nil
	}
	entities := make([]*sysin.TelegramEntityInp, 0, len(msg.Entities)+len(msg.CaptionEntities))
	entities = append(entities, msg.Entities...)
	entities = append(entities, msg.CaptionEntities...)
	return entities
}

func telegramMessageText(msg *sysin.TelegramMessageInp) string {
	if msg == nil {
		return ""
	}
	text := strings.TrimSpace(fallbackString(msg.Text, msg.Caption, stickerText(msg.Sticker)))
	if text != "" {
		if telegramHasCustomEmoji(msg) && !strings.Contains(text, "[表情]") {
			return text + " [表情]"
		}
		return text
	}
	if telegramHasCustomEmoji(msg) {
		return "[表情]"
	}
	return ""
}

func telegramHasCustomEmoji(msg *sysin.TelegramMessageInp) bool {
	for _, entity := range telegramMessageEntities(msg) {
		if entity != nil && entity.Type == "custom_emoji" {
			return true
		}
	}
	return false
}

func telegramHasVisualEmoji(msg *sysin.TelegramMessageInp) bool {
	if msg == nil {
		return false
	}
	return msg.Sticker != nil || telegramHasCustomEmoji(msg)
}

func stickerText(sticker *sysin.TelegramStickerInp) string {
	if sticker == nil {
		return ""
	}
	if strings.TrimSpace(sticker.Emoji) != "" {
		return strings.TrimSpace(sticker.Emoji)
	}
	return "[表情包]"
}

func bytesUploadFile(filename string, data []byte) *ghttp.UploadFile {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write(data)
	_ = writer.Close()
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	_ = request.ParseMultipartForm(int64(len(data)) + 1024)
	if request.MultipartForm == nil || len(request.MultipartForm.File["file"]) == 0 {
		return &ghttp.UploadFile{FileHeader: &multipart.FileHeader{Filename: filename, Size: int64(len(data))}}
	}
	return &ghttp.UploadFile{FileHeader: request.MultipartForm.File["file"][0]}
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "does not exist") || strings.Contains(message, "doesn't exist")
}
