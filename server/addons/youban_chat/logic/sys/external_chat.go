package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_chat/model/input/sysin"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/storager"
	"hotgo/internal/model/entity"
)

const externalVisitorTable = "hg_youban_chat_visitor"

func (s *sSysChat) ensureExternalAdminColumns(ctx context.Context) error {
	for _, statement := range []string{"ALTER TABLE hg_youban_chat_bot ADD COLUMN IF NOT EXISTS app_id VARCHAR(128) NOT NULL DEFAULT ''", "ALTER TABLE hg_youban_chat_binding ADD COLUMN IF NOT EXISTS app_id VARCHAR(128) NOT NULL DEFAULT ''"} {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysChat) ExternalAdminBots(ctx context.Context, in *sysin.ExternalAdminListInp) (*sysin.ExternalAdminBotListModel, error) {
	if in == nil || strings.TrimSpace(in.AppId) == "" {
		return nil, gerror.New("开放应用身份无效")
	}
	if err := s.ensureExternalAdminColumns(ctx); err != nil {
		return nil, gerror.Wrap(err, "初始化租户聊天配置失败")
	}
	page, size := in.Page, in.PerPage
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	mod := g.DB().Model(chatBotTable).Ctx(ctx).Where("app_id", in.AppId).WhereNull("deleted_at")
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ?)", like, like)
	}
	var rows []*chatBotRow
	total := 0
	if err := mod.Page(page, size).OrderDesc("id").ScanAndCount(&rows, &total, false); err != nil {
		return nil, gerror.Wrap(err, "读取Bot列表失败")
	}
	res := &sysin.ExternalAdminBotListModel{List: make([]*sysin.ExternalAdminBotModel, 0, len(rows)), Total: total}
	for _, row := range rows {
		hint := ""
		if len(row.BotToken) >= 4 {
			hint = "****" + row.BotToken[len(row.BotToken)-4:]
		}
		var binding *chatBindingRow
		if err := g.DB().Model(chatBindingTable).Ctx(ctx).
			Where("app_id", in.AppId).
			Where("bot_id", row.Id).
			WhereNull("deleted_at").
			OrderDesc("id").
			Scan(&binding); err != nil {
			return nil, gerror.Wrap(err, "读取Bot绑定码失败")
		}
		if binding == nil {
			now := gtime.Now()
			code := randomBindCode()
			bindingId, err := g.DB().Model(chatBindingTable).Ctx(ctx).Data(g.Map{"app_id": in.AppId, "bind_code": code, "bind_type": "global", "bot_id": row.Id, "tg_chat_id": "", "tg_chat_title": "", "remark": row.Remark, "status": 1, "created_at": now, "updated_at": now}).InsertAndGetId()
			if err != nil {
				return nil, gerror.Wrap(err, "生成Bot绑定码失败")
			}
			binding = &chatBindingRow{Id: bindingId, AppId: in.AppId, BotId: row.Id, BindCode: code, BindType: "global", Status: 1}
		}
		item := &sysin.ExternalAdminBotModel{Id: row.Id, BotName: row.BotName, BotUsername: row.BotUsername, TokenHint: hint, Remark: row.Remark, Status: row.Status}
		if binding != nil {
			item.BindingId = binding.Id
			item.BindCode = binding.BindCode
			item.TgChatId = binding.TgChatId
			item.TgChatTitle = binding.TgChatTitle
			item.IsBound = strings.TrimSpace(binding.TgChatId) != ""
		}
		res.List = append(res.List, item)
	}
	return res, nil
}

func (s *sSysChat) ExternalAdminSaveBot(ctx context.Context, in *sysin.ExternalAdminBotSaveInp) error {
	if in == nil || strings.TrimSpace(in.AppId) == "" {
		return gerror.New("开放应用身份无效")
	}
	if err := s.ensureExternalAdminColumns(ctx); err != nil {
		return err
	}
	token := strings.TrimSpace(in.BotToken)
	profile, err := s.telegramBotProfile(ctx, token)
	if err != nil {
		return gerror.Wrap(err, "校验Bot Token失败")
	}
	now := gtime.Now()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		botId, insertErr := tx.Model(chatBotTable).Ctx(ctx).Data(g.Map{"app_id": in.AppId, "bot_name": telegramBotDisplayName(profile), "bot_username": strings.TrimPrefix(profile.Username, "@"), "bot_token": token, "remark": strings.TrimSpace(in.Remark), "status": 1, "created_at": now, "updated_at": now}).InsertAndGetId()
		if insertErr != nil {
			return gerror.Wrap(insertErr, "保存Bot失败")
		}
		_, insertErr = tx.Model(chatBindingTable).Ctx(ctx).Data(g.Map{"app_id": in.AppId, "bind_code": randomBindCode(), "bind_type": "global", "bot_id": botId, "tg_chat_id": "", "tg_chat_title": "", "remark": strings.TrimSpace(in.Remark), "status": 1, "created_at": now, "updated_at": now}).Insert()
		return gerror.Wrap(insertErr, "生成Bot绑定码失败")
	})
	if err != nil {
		return err
	}
	if err = gatewayservice.Gateway().Refresh(ctx); err != nil {
		g.Log().Warningf(ctx, "刷新Telegram Gateway失败 bot:%s err:%+v", profile.Username, err)
	}
	return nil
}

func (s *sSysChat) externalAdminBot(ctx context.Context, appId string, id int64) (*chatBotRow, error) {
	if strings.TrimSpace(appId) == "" || id <= 0 {
		return nil, gerror.New("Bot ID无效")
	}
	var row *chatBotRow
	err := g.DB().Model(chatBotTable).Ctx(ctx).Where("app_id", appId).Where("id", id).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Bot失败")
	}
	if row == nil {
		return nil, gerror.New("Bot不存在或无权访问")
	}
	return row, nil
}

func (s *sSysChat) ExternalAdminDeleteBot(ctx context.Context, in *sysin.ExternalAdminBotActionInp) error {
	if in == nil {
		return gerror.New("Bot ID无效")
	}
	row, err := s.externalAdminBot(ctx, in.AppId, in.Id)
	if err != nil {
		return err
	}
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err = tx.Model(chatBindingTable).Ctx(ctx).Where("app_id", in.AppId).Where("bot_id", row.Id).WhereNull("deleted_at").Data(g.Map{"status": 0, "deleted_at": now, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "停用Bot群组绑定失败")
		}
		_, err = tx.Model(chatBotTable).Ctx(ctx).Where("app_id", in.AppId).Where("id", row.Id).Data(g.Map{"status": 0, "deleted_at": now, "updated_at": now}).Update()
		return gerror.Wrap(err, "删除Bot失败")
	})
}

func (s *sSysChat) ExternalAdminRotateBotBindingCode(ctx context.Context, in *sysin.ExternalAdminBotActionInp) error {
	if in == nil {
		return gerror.New("Bot ID无效")
	}
	row, err := s.externalAdminBot(ctx, in.AppId, in.Id)
	if err != nil {
		return err
	}
	now := gtime.Now()
	result, err := g.DB().Model(chatBindingTable).Ctx(ctx).Where("app_id", in.AppId).Where("bot_id", row.Id).WhereNull("deleted_at").Data(g.Map{"bind_code": randomBindCode(), "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "刷新Bot绑定码失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = g.DB().Model(chatBindingTable).Ctx(ctx).Data(g.Map{"app_id": in.AppId, "bind_code": randomBindCode(), "bind_type": "global", "bot_id": row.Id, "tg_chat_id": "", "tg_chat_title": "", "remark": row.Remark, "status": 1, "created_at": now, "updated_at": now}).Insert()
	return gerror.Wrap(err, "生成Bot绑定码失败")
}

func (s *sSysChat) externalAdminConversation(ctx context.Context, appId string, id int64) (*chatConversationRow, error) {
	var row *chatConversationRow
	err := g.DB().Model(chatConversationTable+" c").Ctx(ctx).InnerJoin(externalVisitorTable+" v", "c.member_id=-v.id").Where("v.app_id", strings.TrimSpace(appId)).Where("c.id", id).WhereNull("c.deleted_at").Fields("c.*").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取租户会话失败")
	}
	if row == nil {
		return nil, gerror.New("会话不存在或无权访问")
	}
	return row, nil
}

func (s *sSysChat) ExternalAdminConversations(ctx context.Context, in *sysin.ExternalAdminConversationInp) (*sysin.ExternalAdminConversationListModel, error) {
	if in == nil || strings.TrimSpace(in.AppId) == "" {
		return nil, gerror.New("开放应用身份无效")
	}
	page, size := in.Page, in.PerPage
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	// Use explicit aliases: scanning c.* into an embedded struct can silently
	// leave numeric fields at zero with some database drivers.
	mod := g.DB().Model(chatConversationTable+" c").Ctx(ctx).InnerJoin(externalVisitorTable+" v", "c.member_id=-v.id").Where("v.app_id", in.AppId).WhereNull("c.deleted_at").Fields("c.id AS id,c.profile_id AS profile_id,c.status AS status,c.unread_count AS unread_count,c.last_message AS last_message,c.last_message_at AS last_message_at,v.name AS visitor_name")
	var rows []gdb.Record
	total := 0
	if err := mod.Page(page, size).OrderDesc("c.updated_at").ScanAndCount(&rows, &total, false); err != nil {
		return nil, gerror.Wrap(err, "读取租户会话列表失败")
	}
	res := &sysin.ExternalAdminConversationListModel{List: make([]*sysin.ExternalConversationModel, 0, len(rows)), Total: total}
	for _, item := range rows {
		id := gconv.Int64(item["id"])
		profileId := gconv.Int64(item["profile_id"])
		name := gconv.String(item["visitor_name"])
		if profileId > 0 {
			name = fmt.Sprintf("%s · 资料 %d", name, profileId)
		}
		model := &sysin.ExternalConversationModel{Id: id, ConversationId: id, ProfileId: profileId, Name: name, Status: gconv.String(item["status"]), UnreadCount: gconv.Int(item["unread_count"]), LastMessage: gconv.String(item["last_message"]), CanDelete: true}
		if value := item["last_message_at"]; value != nil {
			model.LastMessageAt = gconv.String(value)
		}
		res.List = append(res.List, model)
	}
	return res, nil
}

func (s *sSysChat) ExternalAdminMessages(ctx context.Context, in *sysin.ExternalAdminConversationInp) (*sysin.ChatMessagesModel, error) {
	row, err := s.externalAdminConversation(ctx, in.AppId, in.ConversationId)
	if err != nil {
		return nil, err
	}
	mod := g.DB().Model(chatMessageTable).Ctx(ctx).Where("conversation_id", row.Id).WhereNull("deleted_at")
	if row.HiddenBeforeAt != nil {
		mod = mod.WhereGT("created_at", row.HiddenBeforeAt)
	}
	var rows []*chatMessageRow
	if err = mod.OrderAsc("id").Limit(500).Scan(&rows); err != nil {
		return nil, err
	}
	list := make([]*sysin.ChatMessageModel, 0, len(rows))
	for _, item := range rows {
		list = append(list, packApiChatMessage(item))
	}
	return &sysin.ChatMessagesModel{List: list}, nil
}

func (s *sSysChat) ExternalAdminClear(ctx context.Context, in *sysin.ExternalAdminConversationInp) error {
	row, err := s.externalAdminConversation(ctx, in.AppId, in.ConversationId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"hidden_before_at": gtime.Now(), "unread_count": 0, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysChat) ExternalAdminDelete(ctx context.Context, in *sysin.ExternalAdminConversationInp) error {
	row, err := s.externalAdminConversation(ctx, in.AppId, in.ConversationId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"deleted_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysChat) ExternalConversations(ctx context.Context, in *sysin.ExternalConversationsInp) (*sysin.ExternalConversationsModel, error) {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	var rows []*chatConversationRow
	if err = s.ensureExternalConversationColumns(ctx); err != nil {
		return nil, err
	}
	err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("member_id", externalOwnerId(visitor.Id)).WhereNull("deleted_at").WhereNull("user_hidden_at").OrderDesc("pinned_at").OrderDesc("updated_at").Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取客服会话失败")
	}
	res := &sysin.ExternalConversationsModel{List: make([]*sysin.ExternalConversationModel, 0, len(rows))}
	for _, row := range rows {
		name := "在线客服"
		if row.ProfileId > 0 {
			name = fmt.Sprintf("在线客服 · 资料 %d", row.ProfileId)
		}
		item := &sysin.ExternalConversationModel{Id: row.Id, ConversationId: row.Id, ProfileId: row.ProfileId, Name: name, Status: row.Status, UnreadCount: row.UnreadCount, LastMessage: row.LastMessage, IsPinned: row.PinnedAt != nil, CanDelete: true}
		if row.LastMessageAt != nil {
			item.LastMessageAt = row.LastMessageAt.String()
		}
		res.List = append(res.List, item)
	}
	return res, nil
}

func (s *sSysChat) ExternalPin(ctx context.Context, in *sysin.ExternalConversationActionInp) error {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return err
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return err
	}
	data := g.Map{"pinned_at": nil, "updated_at": gtime.Now()}
	if in.Pinned || row.ProfileId == 0 {
		data["pinned_at"] = gtime.Now()
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(data).Update()
	return err
}

func (s *sSysChat) ExternalDelete(ctx context.Context, in *sysin.ExternalConversationActionInp) error {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return err
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"user_hidden_at": gtime.Now(), "unread_count": 0, "updated_at": gtime.Now()}).Update()
	return err
}

type externalVisitorRow struct {
	Id             int64       `json:"id"`
	AppId          string      `json:"app_id"`
	ExternalUserId string      `json:"external_user_id"`
	Name           string      `json:"name"`
	Email          string      `json:"email"`
	AvatarUrl      string      `json:"avatar_url"`
	UpdatedAt      *gtime.Time `json:"updated_at"`
}

func (s *sSysChat) ExternalSession(ctx context.Context, in *sysin.ExternalSessionInp) (*sysin.ChatStartModel, error) {
	if err := s.ensureExternalSchema(ctx); err != nil {
		return nil, err
	}
	visitor, member, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	ownerId := externalOwnerId(visitor.Id)
	var profile *profileBrief
	if in.ProfileId > 0 {
		profile, err = s.getProfileBrief(ctx, in.ProfileId)
		if err != nil {
			return nil, err
		}
	}
	row, err := s.getConversation(ctx, ownerId, in.ProfileId)
	if err != nil {
		return nil, err
	}
	created := row == nil
	if created {
		row, err = s.deletedExternalConversation(ctx, ownerId, in.ProfileId)
		if err != nil {
			return nil, err
		}
	}
	binding, err := s.matchExternalBinding(ctx, in.Visitor.AppId, profile)
	if err != nil {
		return nil, err
	}
	if binding == nil || strings.TrimSpace(binding.TgChatId) == "" || strings.TrimSpace(binding.BotToken) == "" {
		return nil, gerror.New("客服群尚未绑定")
	}
	if row == nil {
		now := gtime.Now()
		id, insertErr := g.DB().Model(chatConversationTable).Ctx(ctx).Data(g.Map{
			"member_id": ownerId, "profile_id": in.ProfileId, "bot_id": routeBotId(binding),
			"tg_chat_id": routeTargetChatId(binding), "status": "opened", "last_message": "会话已创建",
			"last_message_at": now, "created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if insertErr != nil {
			return nil, gerror.Wrap(insertErr, "创建外部访客会话失败")
		}
		row = &chatConversationRow{Id: id, MemberId: ownerId, ProfileId: in.ProfileId, BotId: routeBotId(binding), TgChatId: routeTargetChatId(binding), Status: "opened"}
		row.ChatSessionId = chatSessionId(id)
		_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", id).Data(g.Map{"pocketping_session_id": row.ChatSessionId, "updated_at": now}).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "保存外部访客会话标识失败")
		}
	} else if err = s.ensureConversationRoute(ctx, row, binding); err != nil {
		return nil, err
	}
	_, _ = g.DB().Model(chatConversationTable).Ctx(ctx).Unscoped().Where("id", row.Id).Data(g.Map{"deleted_at": nil, "user_hidden_at": nil, "status": "opened", "updated_at": gtime.Now()}).Update()
	if created {
		go s.initializeExternalTelegramSession(row, profile, member)
	}
	return s.packStart(row), nil
}

func (s *sSysChat) initializeExternalTelegramSession(row *chatConversationRow, profile *profileBrief, member *entity.AdminMember) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.notifyTelegramSession(ctx, row, profile, member); err != nil {
		g.Log().Warningf(ctx, "异步初始化Telegram会话失败 conversation:%d err:%+v", row.Id, err)
	}
}

func (s *sSysChat) matchExternalBinding(ctx context.Context, appId string, profile *profileBrief) (*chatBindingRow, error) {
	base := func() *gdb.Model {
		return s.bindingModel(ctx).Where("b.app_id", strings.TrimSpace(appId))
	}
	var row *chatBindingRow
	if profile != nil && (profile.SourceChannelId > 0 || profile.ChannelId > 0) {
		mod := base().Where("b.bind_type", "channel")
		if profile.SourceChannelId > 0 && profile.ChannelId > 0 {
			mod = mod.Where("(b.source_channel_id=? OR b.content_channel_id=?)", profile.SourceChannelId, profile.ChannelId)
		} else if profile.SourceChannelId > 0 {
			mod = mod.Where("b.source_channel_id", profile.SourceChannelId)
		} else {
			mod = mod.Where("b.content_channel_id", profile.ChannelId)
		}
		if err := mod.OrderDesc("b.updated_at").OrderDesc("b.id").Scan(&row); err != nil {
			return nil, gerror.Wrap(err, "匹配租户客服群绑定失败")
		}
		if row != nil && strings.TrimSpace(row.TgChatId) != "" {
			return row, nil
		}
	}
	row = nil
	err := base().Where("b.bind_type", "global").OrderDesc("b.updated_at").OrderDesc("b.id").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取租户默认客服群绑定失败")
	}
	return row, nil
}

func (s *sSysChat) ExternalSend(ctx context.Context, in *sysin.ExternalMessageInp) (*sysin.ChatSendModel, error) {
	visitor, member, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil, gerror.New("消息不能为空")
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return nil, err
	}
	if err = s.validateExternalReply(ctx, row.Id, in.ReplyToMessageId); err != nil {
		return nil, err
	}
	messageId := normalizeClientMessageId(in.ClientMessageId, externalOwnerId(visitor.Id))
	id, inserted, err := s.saveMessage(ctx, row.Id, messageId, "mine", content, "text", "sent", memberDisplayName(member), nil)
	if err != nil {
		return nil, err
	}
	if in.ReplyToMessageId > 0 {
		_, err = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", id).Data(g.Map{"reply_to_message_id": in.ReplyToMessageId}).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "保存引用消息失败")
		}
	}
	if !inserted {
		return &sysin.ChatSendModel{MessageId: id, ClientMessageId: messageId, Status: "sent"}, nil
	}
	profile, err := s.getConversationProfileBrief(ctx, row)
	if err == nil {
		go s.notifyExternalMessageAsync(row, profile, member, messageId, content, nil, id)
	}
	s.updateExternalConversation(ctx, row.Id, content)
	return &sysin.ChatSendModel{MessageId: id, ClientMessageId: messageId, Status: "sent"}, nil
}

func (s *sSysChat) ExternalMessages(ctx context.Context, in *sysin.ExternalMessagesInp) (*sysin.ChatMessagesModel, error) {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return nil, err
	}
	if err = s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return nil, err
	}
	mod := g.DB().Model(chatMessageTable).Ctx(ctx).Where("conversation_id", row.Id).
		WhereGTE("created_at", gtime.Now().AddDate(0, 0, -7)).WhereNull("deleted_at")
	if in.AfterId > 0 {
		mod = mod.WhereGT("id", in.AfterId)
	}
	var rows []*chatMessageRow
	if err = mod.OrderAsc("id").Limit(200).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取聊天消息失败")
	}
	list := make([]*sysin.ChatMessageModel, 0, len(rows))
	for _, item := range rows {
		message := packApiChatMessage(item)
		if item.ReplyToMessageId > 0 {
			var reply *chatMessageRow
			if scanErr := g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", item.ReplyToMessageId).Where("conversation_id", row.Id).Scan(&reply); scanErr == nil && reply != nil {
				message.Reply = packApiChatMessage(reply)
			}
		}
		list = append(list, message)
	}
	return &sysin.ChatMessagesModel{List: list}, nil
}

func (s *sSysChat) ExternalFile(ctx context.Context, in *sysin.ExternalFileInp) (*sysin.ChatUploadModel, error) {
	visitor, member, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(in.ContentBase64)
	if err != nil || len(data) == 0 {
		return nil, gerror.New("文件内容格式不正确")
	}
	if len(data) > 25*1024*1024 {
		return nil, gerror.New("单个文件不能超过25MB")
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return nil, err
	}
	if err = s.validateExternalReply(ctx, row.Id, in.ReplyToMessageId); err != nil {
		return nil, err
	}
	attachment, err := externalUploadAttachment(ctx, in.FileName, in.MimeType, data)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(in.Content)
	messageId := normalizeClientMessageId(in.ClientMessageId, externalOwnerId(visitor.Id))
	id, inserted, err := s.saveMessage(ctx, row.Id, messageId, "mine", content, attachment.FileType, "sent", memberDisplayName(member), []*sysin.ChatMessageAttachmentModel{attachment})
	if err != nil {
		return nil, err
	}
	if in.ReplyToMessageId > 0 {
		_, err = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", id).Data(g.Map{"reply_to_message_id": in.ReplyToMessageId}).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "保存引用消息失败")
		}
	}
	if !inserted {
		return &sysin.ChatUploadModel{Message: &sysin.ChatMessageModel{Id: id, ConversationId: row.Id, Direction: "mine", Content: content, ContentType: attachment.FileType, Status: "sent", Attachments: []*sysin.ChatMessageAttachmentModel{attachment}}}, nil
	}
	profile, err := s.getConversationProfileBrief(ctx, row)
	if err == nil {
		go s.notifyExternalMessageAsync(row, profile, member, messageId, content, []*sysin.ChatMessageAttachmentModel{attachment}, id)
	}
	message := &sysin.ChatMessageModel{Id: id, ClientMessageId: messageId, ConversationId: row.Id, Direction: "mine", Content: content, ContentType: attachment.FileType, Status: "sent", SenderName: memberDisplayName(member), CreatedAt: gtime.Now().String(), Attachments: []*sysin.ChatMessageAttachmentModel{attachment}}
	s.updateExternalConversation(ctx, row.Id, attachmentLastMessage(message))
	return &sysin.ChatUploadModel{Message: message}, nil
}

func (s *sSysChat) notifyExternalMessageAsync(row *chatConversationRow, profile *profileBrief, member *entity.AdminMember, messageId, content string, attachments []*sysin.ChatMessageAttachmentModel, localId int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.notifyTelegramMessage(ctx, row, profile, member, messageId, content, attachments); err != nil {
		g.Log().Warningf(ctx, "异步发送Telegram消息失败 message:%d err:%+v", localId, err)
		_, _ = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", localId).Data(g.Map{"status": "failed", "updated_at": gtime.Now()}).Update()
	}
}

func (s *sSysChat) ExternalRead(ctx context.Context, in *sysin.ExternalReadInp) error {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return err
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", row.Id).Data(g.Map{"unread_count": 0, "updated_at": gtime.Now()}).Update()
	if err == nil {
		err = s.markServiceMessagesRead(ctx, row)
	}
	return err
}

func (s *sSysChat) ExternalUnread(ctx context.Context, in *sysin.ExternalUnreadInp) (*sysin.ChatUnreadModel, error) {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return nil, err
	}
	value, err := g.DB().Model(chatConversationTable).Ctx(ctx).Where("member_id", externalOwnerId(visitor.Id)).WhereNull("deleted_at").Fields("COALESCE(SUM(unread_count),0)").Value()
	if err != nil {
		return nil, gerror.Wrap(err, "获取聊天未读数失败")
	}
	return &sysin.ChatUnreadModel{UnreadCount: value.Int()}, nil
}

func (s *sSysChat) ExternalReaction(ctx context.Context, in *sysin.ExternalReactionInp) error {
	visitor, _, err := s.externalActor(ctx, &in.Visitor)
	if err != nil {
		return err
	}
	row, err := s.getConversationById(ctx, externalOwnerId(visitor.Id), in.ConversationId)
	if err != nil {
		return err
	}
	if err = s.ensureTelegramMessageReadColumns(ctx); err != nil {
		return err
	}
	var message *chatMessageRow
	if err = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", in.MessageId).Where("conversation_id", row.Id).WhereNull("deleted_at").Scan(&message); err != nil {
		return gerror.Wrap(err, "读取反应消息失败")
	}
	if message == nil {
		return gerror.New("消息不存在")
	}
	reactions := map[string][]string{}
	_ = json.Unmarshal([]byte(message.ReactionsJson), &reactions)
	actor := in.Visitor.ExternalUserId
	values := reactions[in.Emoji]
	filtered := make([]string, 0, len(values)+1)
	for _, value := range values {
		if value != actor {
			filtered = append(filtered, value)
		}
	}
	if !in.Remove {
		filtered = append(filtered, actor)
	}
	if len(filtered) == 0 {
		delete(reactions, in.Emoji)
	} else {
		reactions[in.Emoji] = filtered
	}
	encoded, _ := json.Marshal(reactions)
	if _, err = g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", message.Id).Data(g.Map{"reactions_json": string(encoded), "updated_at": gtime.Now()}).Update(); err != nil {
		return gerror.Wrap(err, "保存消息反应失败")
	}
	if message.TgMessageId > 0 {
		emoji := in.Emoji
		if in.Remove {
			emoji = ""
		}
		if err = s.telegramSetMessageReaction(ctx, row, message, emoji); err != nil {
			return gerror.Wrap(err, "同步Telegram反应失败")
		}
	}
	return nil
}

func (s *sSysChat) validateExternalReply(ctx context.Context, conversationId, messageId int64) error {
	if messageId <= 0 {
		return nil
	}
	count, err := g.DB().Model(chatMessageTable).Ctx(ctx).Where("id", messageId).Where("conversation_id", conversationId).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "校验引用消息失败")
	}
	if count == 0 {
		return gerror.New(fmt.Sprintf("引用消息不存在: %d", messageId))
	}
	return nil
}

func (s *sSysChat) externalActor(ctx context.Context, in *sysin.ExternalVisitorInp) (*externalVisitorRow, *entity.AdminMember, error) {
	appId := strings.TrimSpace(in.AppId)
	externalId := strings.TrimSpace(in.ExternalUserId)
	if appId == "" || externalId == "" {
		return nil, nil, gerror.New("外部访客身份不完整")
	}
	now := gtime.Now()
	_, err := g.DB().Model(externalVisitorTable).Ctx(ctx).Data(g.Map{"app_id": appId, "external_user_id": externalId, "name": strings.TrimSpace(in.Name), "email": strings.TrimSpace(in.Email), "avatar_url": strings.TrimSpace(in.AvatarUrl), "created_at": now, "updated_at": now}).OnConflict("app_id,external_user_id").OnDuplicate("name,email,avatar_url,updated_at").Save()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "保存外部访客失败")
	}
	var visitor *externalVisitorRow
	if err = g.DB().Model(externalVisitorTable).Ctx(ctx).Where("app_id", appId).Where("external_user_id", externalId).Scan(&visitor); err != nil {
		return nil, nil, gerror.Wrap(err, "读取外部访客失败")
	}
	if visitor == nil {
		return nil, nil, gerror.New("外部访客不存在")
	}
	member := &entity.AdminMember{Id: externalOwnerId(visitor.Id), RealName: visitor.Name, Username: "网站用户 " + externalId, Email: visitor.Email, Avatar: visitor.AvatarUrl}
	return visitor, member, nil
}

func ensureExternalVisitorTable(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, `CREATE TABLE IF NOT EXISTS hg_youban_chat_visitor (
id BIGSERIAL PRIMARY KEY, app_id VARCHAR(128) NOT NULL, external_user_id VARCHAR(128) NOT NULL,
name VARCHAR(128) NOT NULL DEFAULT '', email VARCHAR(255) NOT NULL DEFAULT '', avatar_url VARCHAR(500) NOT NULL DEFAULT '',
created_at TIMESTAMP, updated_at TIMESTAMP)`)
	if err != nil && strings.Contains(strings.ToLower(g.DB().GetConfig().Type), "mysql") {
		_, err = g.DB().Exec(ctx, "CREATE TABLE IF NOT EXISTS hg_youban_chat_visitor (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, app_id VARCHAR(128) NOT NULL, external_user_id VARCHAR(128) NOT NULL, name VARCHAR(128) NOT NULL DEFAULT '', email VARCHAR(255) NOT NULL DEFAULT '', avatar_url VARCHAR(500) NOT NULL DEFAULT '', created_at DATETIME, updated_at DATETIME)")
	}
	if err != nil {
		return err
	}
	_, _ = g.DB().Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS uk_ybcv_app_user ON hg_youban_chat_visitor (app_id, external_user_id)")
	return nil
}

func (s *sSysChat) ensureExternalConversationColumns(ctx context.Context) error {
	if err := s.ensureConversationUserColumns(ctx); err != nil {
		return err
	}
	return ensureExternalConversationHiddenColumn(ctx)
}

func (s *sSysChat) ensureExternalSchema(ctx context.Context) error {
	s.externalSchemaMu.Lock()
	defer s.externalSchemaMu.Unlock()
	if s.externalSchemaReady {
		return nil
	}
	if err := ensureExternalVisitorTable(ctx); err != nil {
		return gerror.Wrap(err, "初始化外部访客表失败")
	}
	if err := s.ensureExternalConversationColumns(ctx); err != nil {
		return err
	}
	s.externalSchemaReady = true
	return nil
}

func externalOwnerId(visitorId int64) int64 { return -visitorId }

func (s *sSysChat) updateExternalConversation(ctx context.Context, id int64, lastMessage string) {
	_, _ = g.DB().Model(chatConversationTable).Ctx(ctx).Where("id", id).Data(g.Map{"last_message": lastMessage, "last_message_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
}

func externalUploadAttachment(ctx context.Context, fileName, mimeType string, data []byte) (*sysin.ChatMessageAttachmentModel, error) {
	fileType := normalizeAttachmentType("", fileName, mimeType)
	if fileType == "video" {
		var err error
		fileName, mimeType, data, err = normalizeBrowserVideo(ctx, fileName, mimeType, data)
		if err != nil {
			return nil, err
		}
	}
	name := ensureAttachmentExt(fileName, fileType, mimeType)
	attachment, err := storager.DoUpload(ctx, storagerKindByFileType(fileType), bytesUploadFile(name, data))
	if err != nil {
		return nil, err
	}
	url := storager.LastUrl(ctx, attachment.FileUrl, attachment.Drive)
	return &sysin.ChatMessageAttachmentModel{Id: attachment.Id, Name: attachment.Name, FileType: fileType, FallbackUrl: url, DataUrl: url, ThumbUrl: url, Data: data}, nil
}
