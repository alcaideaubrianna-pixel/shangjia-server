package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	ci "hotgo/addons/telegram_collector/model/input/sysin"
	cs "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
	"hotgo/utility/sensitive"
)

type managerRights struct {
	Post   bool `json:"post"`
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

func managerDefaultRights() managerRights { return managerRights{true, true, true} }
func (r managerRights) telegram() tg.ChatAdminRights {
	return tg.ChatAdminRights{PostMessages: r.Post, EditMessages: r.Edit, DeleteMessages: r.Delete, Other: true}
}

type botManagerAccountHandler struct{ bot *sSysBot }

type managerFloodWait struct {
	error
	delay time.Duration
}

func (e managerFloodWait) AccountTaskRetryDelay() time.Duration { return e.delay }

func registerBotManagerAccountHandler(bot *sSysBot) {
	cs.RegisterAccountTaskHandler(managerTaskType, &botManagerAccountHandler{bot: bot})
	cs.RegisterAccountTaskPolicy(managerTaskType, cs.AccountTaskPolicy{PendingTTL: 20 * time.Minute})
}
func (h *botManagerAccountHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *ci.AccountTask) (*ci.AccountMediaDownloadResult, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(task.TaskKey, "bot-manager:"), 10, 64)
	if err != nil || id <= 0 || task.TaskKey != managerTaskKey(id) {
		return nil, gerror.New("机器人管理任务参数无效")
	}
	var row *entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row); err != nil {
		return nil, err
	}
	if row == nil || row.TenantId != task.TenantID {
		return nil, gerror.New("机器人管理任务归属无效")
	}
	if row.Status == "success" {
		return &ci.AccountMediaDownloadResult{}, nil
	}
	if row.Status == "failed" {
		return nil, gerror.New("机器人管理任务已终止")
	}
	if waiting, err := managerHasPredecessor(ctx, row); err != nil {
		return nil, err
	} else if waiting {
		return nil, managerFloodWait{gerror.New("同一机器人频道的前序操作尚未完成"), 10 * time.Second}
	}
	st, err := h.bot.managerState(ctx, row.EntryBotId, row.TelegramUserId)
	if err != nil {
		return nil, err
	}
	if st.TenantId != row.TenantId || st.AccountId != row.AccountId {
		return nil, gerror.New("账户绑定已改变，停止旧任务")
	}
	var ch *entity.YoubanBotManagedChannel
	if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(row.ChannelId).Scan(&ch); err != nil {
		return nil, err
	}
	if ch == nil || ch.TenantId != row.TenantId || ch.AccountId != row.AccountId || ch.TgAccountId != task.AccountID {
		return nil, gerror.New("频道归属不一致")
	}
	// Re-read live configuration before using a cached access hash/account.
	catalog, _, err := ps.SysPublish().BotManagerCatalog(ctx, row.TenantId, row.AccountId)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, c := range catalog {
		if c.Id == ch.PublishChannelId && c.TgAccountId == task.AccountID {
			if c.AccountError != "" {
				if row.Action == "sync" {
					_, saveErr := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).WhereIn(dao.YoubanBotManagerTask.Columns().Status, []string{"pending", "submitted"}).Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: c.AccountError}).Update()
					if saveErr != nil {
						return nil, saveErr
					}
				}
				return nil, gerror.New(c.AccountError)
			}
			valid = true
			break
		}
	}
	if !valid {
		return nil, gerror.New("频道已移除或无权管理")
	}
	peer, err := managerInputChannel(ch)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if row.Action == "sync" {
		err = h.bot.managerSyncChannel(ctx, client.API(), st, ch, peer, row.Id)
	} else {
		err = h.bot.managerMutateChannel(ctx, client.API(), st, row, ch, peer)
	}
	fields := g.Map{"taskId": row.Id, "action": row.Action, "tenantId": row.TenantId, "accountId": row.AccountId, "channelId": ch.PublishChannelId, "managedBotId": row.ManagedBotId, "durationMs": time.Since(started).Milliseconds()}
	if err != nil {
		fields["error"] = sensitive.Diagnostic(err.Error())
		g.Log().Warning(ctx, "机器人管理任务执行失败", fields)
		if row.Action == "sync" {
			reason, permanent := managerPermanentSyncError(err)
			if _, saveErr := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(ch.Id).Data(do.YoubanBotManagedChannel{SyncStatus: "failed", ErrorMessage: reason}).Update(); saveErr != nil {
				return nil, saveErr
			}
			if permanent {
				if _, saveErr := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).WhereIn(dao.YoubanBotManagerTask.Columns().Status, []string{"pending", "submitted"}).Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: reason}).Update(); saveErr != nil {
					return nil, saveErr
				}
			}
		}
		if delay, ok := tgerr.AsFloodWait(err); ok {
			return nil, managerFloodWait{err, delay}
		}
		return nil, err
	}
	_, err = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).WhereIn(dao.YoubanBotManagerTask.Columns().Status, []string{"pending", "submitted"}).Data(do.YoubanBotManagerTask{Status: "success", ErrorMessage: ""}).Update()
	g.Log().Info(ctx, "机器人管理任务完成", fields)
	return &ci.AccountMediaDownloadResult{}, err
}

func managerInputChannel(ch *entity.YoubanBotManagedChannel) (*tg.InputChannel, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(ch.ChatId, "-100"), 10, 64)
	if err != nil || id <= 0 {
		return nil, gerror.New("仅支持频道和超级群")
	}
	hash, err := strconv.ParseInt(ch.AccessHash, 10, 64)
	if err != nil || hash == 0 {
		return nil, gerror.New("频道信息不完整，请先在小灰机刷新频道缓存")
	}
	return &tg.InputChannel{ChannelID: id, AccessHash: hash}, nil
}

func (s *sSysBot) managerSyncChannel(ctx context.Context, client *tg.Client, st *entity.YoubanBotManagerState, ch *entity.YoubanBotManagedChannel, peer *tg.InputChannel, taskIDs ...int64) error {
	full, err := client.ChannelsGetFullChannel(ctx, peer)
	if err != nil {
		return err
	}
	info, ok := full.FullChat.(*tg.ChannelFull)
	if !ok {
		return gerror.New("未返回完整频道资料")
	}
	adminCount := -1
	if count, ok := info.GetAdminsCount(); ok {
		adminCount = count
	}
	canManage := 0
	for _, c := range full.Chats {
		if channel, ok := c.(*tg.Channel); ok && channel.ID == peer.ChannelID {
			rights, _ := channel.GetAdminRights()
			if channel.Creator || rights.AddAdmins {
				canManage = 1
			}
		}
	}
	// Telegram does not expose an admin limit in every layer/config. Zero means
	// unknown, never a fabricated capacity. ADMINS_TOO_MUCH remains authoritative.
	limit := 0
	if cfg, e := client.HelpGetAppConfig(ctx, 0); e == nil {
		if modified, ok := cfg.(*tg.HelpAppConfig); ok {
			if object, ok := modified.Config.(*tg.JSONObject); ok {
				for _, v := range object.Value {
					if v.Key == "chat_admins_max" {
						if n, ok := v.Value.(*tg.JSONNumber); ok {
							limit = int(n.Value)
						}
					}
				}
			}
		}
	}
	url := ch.VisitUrl
	if invite, ok := info.GetExportedInvite(); ok {
		if v, ok := invite.(*tg.ChatInviteExported); ok && v.Link != "" {
			url = v.Link
		}
	}
	// Read complete pages before changing DB memberships; failed pages never
	// mark old members removed.
	users := map[int64]*tg.User{}
	roles := map[int64]string{}
	rights := map[int64]managerRights{}
	for _, filter := range []tg.ChannelParticipantsFilterClass{&tg.ChannelParticipantsAdmins{}, &tg.ChannelParticipantsBots{}} {
		for offset := 0; ; {
			result, err := client.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{Channel: peer, Filter: filter, Offset: offset, Limit: 100})
			if err != nil {
				return err
			}
			page, ok := result.(*tg.ChannelsChannelParticipants)
			if !ok {
				return gerror.New("频道成员同步未返回完整数据")
			}
			for _, item := range page.Users {
				if u, ok := item.(*tg.User); ok && u.Bot {
					users[u.ID] = u
				}
			}
			for _, p := range page.Participants {
				switch p := p.(type) {
				case *tg.ChannelParticipantAdmin:
					roles[p.UserID] = "admin"
					rights[p.UserID] = managerRights{p.AdminRights.PostMessages, p.AdminRights.EditMessages, p.AdminRights.DeleteMessages}
				case *tg.ChannelParticipant:
					if roles[p.UserID] == "" {
						roles[p.UserID] = "member"
					}
				}
			}
			if len(page.Participants) == 0 && offset < page.Count {
				return gerror.New("频道成员分页数据不完整，请重新同步")
			}
			offset += len(page.Participants)
			if offset >= page.Count || len(page.Participants) == 0 {
				break
			}
		}
	}
	return dao.YoubanBotManagedMember.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if len(taskIDs) > 0 {
			var active *entity.YoubanBotManagerTask
			if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(taskIDs[0]).LockUpdate().Scan(&active); err != nil {
				return err
			}
			if active == nil || (active.Status != "pending" && active.Status != "submitted") {
				return gerror.New("同步任务已结束，忽略迟到的频道数据")
			}
		}
		bc := dao.YoubanBotManaged.Columns()
		mc := dao.YoubanBotManagedMember.Columns()
		seen := []int64{}
		for id, u := range users {
			_, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: st.TenantId, AccountId: st.AccountId, TelegramBotId: id, BotName: strings.TrimSpace(u.FirstName + " " + u.LastName), BotUsername: u.Username, Status: 1}).OnConflict(bc.TenantId, bc.AccountId, bc.TelegramBotId).OnDuplicate(bc.BotName, bc.BotUsername).Save()
			if err != nil {
				return err
			}
			// Restore a formerly deleted discovery row if Telegram still reports it.
			// Credentials are left untouched; discovery never invents a Token.
			if _, err := tx.Exec("UPDATE "+dao.YoubanBotManaged.Table()+" SET "+bc.DeletedAt+"=NULL WHERE "+bc.TenantId+"=? AND "+bc.AccountId+"=? AND "+bc.TelegramBotId+"=?", st.TenantId, st.AccountId, id); err != nil {
				return err
			}
			var b entity.YoubanBotManaged
			if err := dao.YoubanBotManaged.Ctx(ctx).Where(bc.TenantId, st.TenantId).Where(bc.AccountId, st.AccountId).Where(bc.TelegramBotId, id).Scan(&b); err != nil {
				return err
			}
			seen = append(seen, b.Id)
			_, err = dao.YoubanBotManagedMember.Ctx(ctx).Data(do.YoubanBotManagedMember{TenantId: st.TenantId, AccountId: st.AccountId, ManagedBotId: b.Id, ChannelId: ch.Id, MemberStatus: firstNonEmpty(roles[id], "member"), RightsJson: managerJSON(rights[id]), LastSyncedAt: gtime.Now()}).OnConflict(mc.ManagedBotId, mc.ChannelId).OnDuplicate(mc.MemberStatus, mc.RightsJson, mc.LastSyncedAt).Save()
			if err != nil {
				return err
			}
		}
		q := dao.YoubanBotManagedMember.Ctx(ctx).Where(mc.ChannelId, ch.Id).Where(mc.TenantId, st.TenantId).Where(mc.AccountId, st.AccountId)
		if len(seen) > 0 {
			q = q.WhereNotIn(mc.ManagedBotId, seen)
		}
		if _, err := q.Data(do.YoubanBotManagedMember{MemberStatus: "removed", LastSyncedAt: gtime.Now()}).Update(); err != nil {
			return err
		}
		_, err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(ch.Id).Data(do.YoubanBotManagedChannel{AdminCount: adminCount, AdminLimit: limit, BotCount: len(users), CanManage: canManage, SyncStatus: "success", ErrorMessage: "", VisitUrl: url, LastSyncedAt: gtime.Now()}).Update()
		return err
	})
}

func (s *sSysBot) managerMutateChannel(ctx context.Context, client *tg.Client, st *entity.YoubanBotManagerState, task *entity.YoubanBotManagerTask, ch *entity.YoubanBotManagedChannel, peer *tg.InputChannel) error {
	b, err := managerBot(ctx, st, task.ManagedBotId)
	if err != nil {
		return err
	}
	if b.PublishBotId > 0 && task.Action != "remove" {
		_, bots, err := ps.SysPublish().BotManagerCatalog(ctx, st.TenantId, st.AccountId)
		if err != nil {
			return err
		}
		found := false
		for _, current := range bots {
			if current.Id == b.PublishBotId {
				if !strings.HasPrefix(current.BotToken, strconv.FormatInt(b.TelegramBotId, 10)+":") {
					return gerror.New("发送机器人身份已变化，请重新同步")
				}
				b.BotToken = current.BotToken
				found = true
				break
			}
		}
		if !found {
			return gerror.New("发送机器人已停用或删除，请重新配置")
		}
	}
	resolved, err := client.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: b.BotUsername})
	if err != nil {
		return err
	}
	var botUser *tg.User
	for _, u := range resolved.Users {
		if v, ok := u.(*tg.User); ok && v.Bot && (b.TelegramBotId <= 0 || v.ID == b.TelegramBotId) {
			botUser = v
		}
	}
	if botUser == nil {
		return gerror.New("机器人用户名身份已变化，请重新同步")
	}
	if b.TelegramBotId <= 0 {
		if _, err := dao.YoubanBotManaged.Ctx(ctx).WherePri(b.Id).Data(do.YoubanBotManaged{TelegramBotId: botUser.ID, BotName: strings.TrimSpace(botUser.FirstName + " " + botUser.LastName)}).Update(); err != nil {
			return err
		}
		b.TelegramBotId = botUser.ID
		if b.BotName == "" {
			b.BotName = strings.TrimSpace(botUser.FirstName + " " + botUser.LastName)
		}
	}
	hash, ok := botUser.GetAccessHash()
	if !ok {
		return gerror.New("机器人身份信息不完整")
	}
	u := &tg.InputUser{UserID: b.TelegramBotId, AccessHash: hash}
	var r managerRights
	if err := json.Unmarshal([]byte(task.RightsJson), &r); err != nil {
		return err
	}
	projection := &pi.BotManagerProjection{TenantId: st.TenantId, AccountId: st.AccountId, ChannelId: ch.PublishChannelId, BotId: b.PublishBotId, BotName: b.BotName, BotUsername: b.BotUsername, BotToken: b.BotToken, Remove: task.Action == "remove"}
	if task.Action == "remove" {
		return ps.SysPublish().BotManagerRemoveFromChannel(ctx, projection, func() error {
			// Replays after a timeout must also accept an already absent Bot.
			_, err := client.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{Channel: peer, Participant: &tg.InputPeerUser{UserID: u.UserID, AccessHash: u.AccessHash}})
			if tgerr.Is(err, "USER_NOT_PARTICIPANT") {
				return nil
			}
			if err != nil {
				return err
			}
			_, err = client.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{Channel: peer, Participant: &tg.InputPeerUser{UserID: u.UserID, AccessHash: u.AccessHash}, BannedRights: tg.ChatBannedRights{ViewMessages: true}})
			if err != nil {
				return err
			}
			s.managerRefreshAfterMutation(ctx, client, st, ch, peer)
			return nil
		})
	}
	if task.Action != "attach" && task.Action != "rights" {
		return gerror.New("不支持的机器人管理操作")
	}
	if task.Action == "attach" && b.BotToken == "" {
		return gerror.New("尚未配置机器人Token")
	}
	if task.Action == "rights" && (!r.Post || !r.Edit || !r.Delete) {
		return ps.SysPublish().BotManagerProtectChannel(ctx, projection, func() error {
			return s.managerSetRights(ctx, client, st, b, ch, peer, u, r, projection)
		})
	}
	return s.managerSetRights(ctx, client, st, b, ch, peer, u, r, projection)
}

func (s *sSysBot) managerSetRights(ctx context.Context, client *tg.Client, st *entity.YoubanBotManagerState, b *entity.YoubanBotManaged, ch *entity.YoubanBotManagedChannel, peer *tg.InputChannel, u *tg.InputUser, r managerRights, projection *pi.BotManagerProjection) error {
	// Fail closed before revoking remote permission. A failed RPC leaves this
	// channel unbound until the user restores posting permission explicitly.
	projection.Remove = !r.Post
	if projection.Remove && (b.PublishBotId > 0 || b.BotToken != "") {
		if _, err := ps.SysPublish().BotManagerApplyBinding(ctx, projection); err != nil {
			return err
		}
	}
	rights := r.telegram()
	previous, err := client.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{Channel: peer, Participant: &tg.InputPeerUser{UserID: u.UserID, AccessHash: u.AccessHash}})
	if err == nil {
		if admin, ok := previous.Participant.(*tg.ChannelParticipantAdmin); ok {
			rights = admin.AdminRights
			rights.PostMessages, rights.EditMessages, rights.DeleteMessages = r.Post, r.Edit, r.Delete
		}
	} else if !tgerr.Is(err, "USER_NOT_PARTICIPANT") {
		return err
	}
	// Telegram validates current privileges and can_edit. Never trust DB flags.
	if _, err := client.ChannelsEditAdmin(ctx, &tg.ChannelsEditAdminRequest{Channel: peer, UserID: u, AdminRights: rights, Rank: "资料推送"}); err != nil {
		return err
	}
	participant, err := client.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{Channel: peer, Participant: &tg.InputPeerUser{UserID: u.UserID, AccessHash: u.AccessHash}})
	if err != nil {
		return err
	}
	admin, ok := participant.Participant.(*tg.ChannelParticipantAdmin)
	if !ok || admin.AdminRights.PostMessages != r.Post || admin.AdminRights.EditMessages != r.Edit || admin.AdminRights.DeleteMessages != r.Delete {
		return gerror.New("机器人权限复查未通过")
	}
	if !projection.Remove {
		// Managing a discovered Bot's Telegram rights needs no Token. Do not
		// manufacture a publishing credential/binding for somebody else's Bot.
		if b.BotToken == "" && b.PublishBotId == 0 {
			s.managerRefreshAfterMutation(ctx, client, st, ch, peer)
			return nil
		}
		id, err := ps.SysPublish().BotManagerApplyBinding(ctx, projection)
		if err != nil {
			return err
		}
		if _, err := dao.YoubanBotManaged.Ctx(ctx).WherePri(b.Id).Data(do.YoubanBotManaged{PublishBotId: id}).Update(); err != nil {
			return err
		}
	}
	s.managerRefreshAfterMutation(ctx, client, st, ch, peer)
	return nil
}

func (s *sSysBot) managerRefreshAfterMutation(ctx context.Context, client *tg.Client, st *entity.YoubanBotManagerState, ch *entity.YoubanBotManagedChannel, peer *tg.InputChannel) {
	if err := s.managerSyncChannel(ctx, client, st, ch, peer); err != nil {
		g.Log().Warningf(ctx, "机器人管理操作已完成，频道缓存刷新失败 channelId:%d err:%s", ch.Id, sensitive.Diagnostic(err.Error()))
		_, _ = dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(ch.Id).Data(do.YoubanBotManagedChannel{SyncStatus: "failed", ErrorMessage: sensitive.Diagnostic(err.Error())}).Update()
	}
}

// Serialize all mutations, including automatic AccountTask retries. A later
// intent cannot execute while an older operation may still change Telegram.
func managerHasPredecessor(ctx context.Context, task *entity.YoubanBotManagerTask) (bool, error) {
	if task.Action == "sync" {
		return false, nil
	}
	c := dao.YoubanBotManagerTask.Columns()
	n, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.TenantId, task.TenantId).Where(c.AccountId, task.AccountId).
		Where(c.ManagedBotId, task.ManagedBotId).Where(c.ChannelId, task.ChannelId).WhereLT(c.Id, task.Id).
		WhereNot(c.Action, "sync").WhereIn(c.Status, []string{"pending", "submitted"}).Count()
	return n > 0, err
}

func (s *sSysBot) startBotManagerRuntime(parent context.Context) {
	s.managerMu.Lock()
	defer s.managerMu.Unlock()
	if s.managerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.managerCancel = cancel
	s.managerDone = make(chan struct{})
	done := s.managerDone
	go func() {
		defer close(done)
		notificationsDone := make(chan struct{})
		go func() {
			defer close(notificationsDone)
			s.runManagerNotifications(ctx)
		}()
		defer func() { <-notificationsDone }()
		ticker := time.NewTicker(5 * time.Second)
		var retryAfter time.Time
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().Before(retryAfter) {
					continue
				}
				call, cancel := context.WithTimeout(ctx, 4*time.Second)
				err := s.managerTick(call)
				cancel()
				if err != nil {
					retryAfter = time.Now().Add(time.Minute)
					g.Log().Warningf(ctx, "机器人管理任务维护失败 err:%s", sensitive.Diagnostic(err.Error()))
				}
			}
		}
	}()
}
func (s *sSysBot) stopBotManagerRuntime() {
	s.managerMu.Lock()
	cancel, done := s.managerCancel, s.managerDone
	s.managerCancel = nil
	s.managerMu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *sSysBot) managerTick(ctx context.Context) error {
	tc := dao.YoubanBotManagerTask.Columns()
	var rows []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WhereIn(tc.Status, []string{"pending", "submitted"}).OrderAsc(tc.UpdatedAt).OrderAsc(tc.Id).Limit(20).Scan(&rows); err != nil {
		return err
	}
	for _, row := range rows {
		// Rotate waiting jobs so a blocked account cannot starve other accounts.
		if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, row.Status).Data(do.YoubanBotManagerTask{Status: row.Status}).Update(); err != nil {
			return err
		}
		waitLimit := 20 * time.Minute
		if row.Action == "sync" {
			waitLimit = 2 * time.Minute
		}
		if row.AccountTaskId == 0 && row.CreatedAt != nil && time.Since(row.CreatedAt.Time) > waitLimit {
			if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, "pending").Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: fmt.Sprintf("管理任务等待超过%d分钟，请检查账户登录后重试", int(waitLimit.Minutes()))}).Update(); err != nil {
				return err
			}
			continue
		}
		if row.AccountTaskId == 0 {
			if waiting, err := managerHasPredecessor(ctx, row); err != nil {
				return err
			} else if waiting {
				continue
			}
			var ch entity.YoubanBotManagedChannel
			if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(row.ChannelId).Scan(&ch); err != nil {
				return err
			}
			next := time.Now().Add(2 * time.Second)
			id, err := cs.AccountTasks().Submit(ctx, &ci.AccountTaskSubmit{TenantID: row.TenantId, AccountID: ch.TgAccountId, TaskType: managerTaskType, TaskKey: managerTaskKey(row.Id), Priority: -200, MaxAttempts: 3, NextRunAt: &next})
			if err != nil {
				// Keep a durable diagnostic and rotate this row; one bad account
				// must not prevent the other channels from being submitted.
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if _, updateErr := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, "pending").Data(do.YoubanBotManagerTask{ErrorMessage: sensitive.Diagnostic(err.Error())}).Update(); updateErr != nil {
					return updateErr
				}
				g.Log().Warningf(ctx, "机器人管理任务提交失败 taskId:%d channelId:%d tgAccountId:%d err:%s", row.Id, ch.PublishChannelId, ch.TgAccountId, sensitive.Diagnostic(err.Error()))
				continue
			}
			if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, "pending").Data(do.YoubanBotManagerTask{AccountTaskId: id, Status: "submitted", ErrorMessage: ""}).Update(); err != nil {
				return err
			}
		} else {
			task, err := cs.AccountTasks().Get(ctx, row.AccountTaskId)
			if err != nil {
				return err
			}
			if task == nil || task.Status == ci.AccountTaskStatusDead || task.Status == ci.AccountTaskStatusCancelled {
				reason := "账号任务已失效"
				if task != nil {
					reason = sensitive.Diagnostic(task.ErrorMessage)
				}
				if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, "submitted").Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: reason}).Update(); err != nil {
					return err
				}
			}
			if row.Action == "sync" && task != nil && row.CreatedAt != nil && time.Since(row.CreatedAt.Time) > 2*time.Minute {
				// This is a read-only sync: abandon its manager row safely, even
				// if the protocol worker later picks up the stale Account task.
				if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(row.Id).Where(tc.Status, "submitted").Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: "协议号两分钟内未能完成同步，请检查是否在线、频道权限或任务繁忙后重试"}).Update(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *sSysBot) managerNotifyCompleted(ctx context.Context) error {
	c := dao.YoubanBotManagerTask.Columns()
	table := dao.YoubanBotManagerTask.Table()
	firstInBatch := fmt.Sprintf("%s IN (SELECT MIN(%s) FROM %s WHERE %s=0 GROUP BY %s)", c.Id, c.Id, table, c.Notified, c.BatchKey)
	noActive := fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s active WHERE active.%s=%s.%s AND active.%s IN ('pending','submitted'))", table, c.BatchKey, table, c.BatchKey, c.Status)
	var roots []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.Notified, 0).WhereIn(c.Status, []string{"success", "failed"}).Where(firstInBatch).Where(noActive).OrderAsc(c.UpdatedAt).OrderAsc(c.Id).Limit(20).Scan(&roots); err != nil {
		return err
	}
	for _, root := range roots {
		// Rotate one root per batch, including batches currently in backoff.
		if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(root.Id).Data(do.YoubanBotManagerTask{Status: root.Status}).Update(); err != nil {
			return err
		}
		key := consts.ManagerNotificationRetryPrefix + root.BatchKey
		if cache.Initialized() {
			cooldown, err := cache.Instance().Get(ctx, key)
			if err == nil && cooldown != nil && !cooldown.IsNil() {
				continue
			}
		}
		call, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := s.managerNotifyBatch(call, root)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			g.Log().Warningf(ctx, "机器人管理批次通知失败 batch:%s err:%s", root.BatchKey, sensitive.Diagnostic(err.Error()))
			if cache.Initialized() {
				_ = cache.Instance().Set(ctx, key, true, time.Minute)
			}
		}
	}
	return nil
}

func (s *sSysBot) managerNotifyBatch(ctx context.Context, root *entity.YoubanBotManagerTask) error {
	c := dao.YoubanBotManagerTask.Columns()
	var tasks []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.BatchKey, root.BatchKey).Scan(&tasks); err != nil {
		return err
	}
	complete, success := true, 0
	for _, t := range tasks {
		if t.Status == "pending" || t.Status == "submitted" {
			complete = false
		}
		if t.Status == "success" {
			success++
		}
	}
	if !complete {
		return nil
	}
	st, err := s.managerState(ctx, root.EntryBotId, root.TelegramUserId)
	if err != nil {
		return err
	}
	if st.TenantId != root.TenantId || st.AccountId != root.AccountId {
		_, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.BatchKey, root.BatchKey).Data(do.YoubanBotManagerTask{Notified: 1}).Update()
		return err
	}
	if root.Action == "sync" && success == len(tasks) {
		if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{SyncedAt: gtime.Now()}).Update(); err != nil {
			return err
		}
	}
	text := fmt.Sprintf("机器人管理处理完成：成功 %d 个，失败 %d 个。\n", success, len(tasks)-success)
	if root.Action == "sync" {
		text, err = managerSyncProgressText(ctx, root)
		if err != nil {
			return err
		}
		n, err := managerChannelBotsModel(ctx, st).Count()
		if err != nil {
			return err
		}
		text += fmt.Sprintf("\n当前上架频道共发现 %d 个不同机器人（不要求持有 Token）。\n", n)
	}
	for _, t := range tasks {
		if root.Action == "attach" && t.Status == "success" {
			b, botErr := managerBot(ctx, st, t.ManagedBotId)
			var ch entity.YoubanBotManagedChannel
			chErr := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(t.ChannelId).Scan(&ch)
			if botErr == nil && chErr == nil {
				text += fmt.Sprintf("@%s → %s\n", html.EscapeString(b.BotUsername), html.EscapeString(ch.Title))
			}
		}
		if t.Status == "failed" && root.Action != "sync" {
			name := fmt.Sprintf("频道记录 #%d", t.ChannelId)
			if b, e := managerBot(ctx, st, t.ManagedBotId); e == nil {
				name = "@" + b.BotUsername
			}
			text += fmt.Sprintf("%s：%s\n", html.EscapeString(name), html.EscapeString(sensitive.Diagnostic(t.ErrorMessage)))
		}
		if len(text) > 2500 {
			break
		}
	}
	target := managerProgressTarget{}
	if root.Action == "sync" {
		// Cache is presentation-only; a missing pointer falls back to a normal notification.
		target, _ = managerProgressLoad(ctx, root)
	}
	notifyErr := s.managerSend(ctx, root.EntryBotId, root.TelegramUserId, target.MessageID, text, managerSyncButtons())
	if notifyErr != nil && target.MessageID > 0 && managerProgressMessageMissing(notifyErr) {
		notifyErr = s.managerSend(ctx, root.EntryBotId, root.TelegramUserId, 0, text, managerSyncButtons())
	}
	if err := notifyErr; err != nil {
		if isBroadcastBlocked(err) {
			_, updateErr := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.BatchKey, root.BatchKey).Data(do.YoubanBotManagerTask{Notified: 1}).Update()
			return updateErr
		}
		return err
	}
	if root.Action == "sync" {
		if err := s.managerList(ctx, st, 0, 1); err != nil {
			return err
		}
	}
	if _, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.BatchKey, root.BatchKey).Data(do.YoubanBotManagerTask{Notified: 1}).Update(); err != nil {
		return err
	}
	return nil
}

func (s *sSysBot) runManagerNotifications(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			call, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := s.managerNotifyCompleted(call)
			cancel()
			if err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "机器人管理通知维护失败 err:%s", sensitive.Diagnostic(err.Error()))
			}
			progressCtx, progressCancel := context.WithTimeout(ctx, 10*time.Second)
			if err := s.managerUpdateSyncProgress(progressCtx); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "机器人同步进度更新失败 err:%s", sensitive.Diagnostic(err.Error()))
			}
			progressCancel()
		}
	}
}
