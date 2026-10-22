package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	ci "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pub "hotgo/addons/youban_publish/service"
)

const managerTaskType = ci.AccountTaskTypeBotManager
const managerPageSize = 10

func managerIDs(raw string) []int64 {
	var ids []int64
	_ = json.Unmarshal([]byte(raw), &ids)
	return uniqueIds(ids)
}
func managerJSON(v interface{}) string { raw, _ := json.Marshal(v); return string(raw) }

func (s *sSysBot) managerState(ctx context.Context, entryBotID, userID int64) (*entity.YoubanBotManagerState, error) {
	account, err := s.relayBoundAccount(ctx, userID)
	if err != nil {
		return nil, gerror.New("请先绑定上架账号后使用机器人管理，发送 /bind 完成绑定。")
	}
	if account.TenantId <= 0 || account.AccountId <= 0 {
		return nil, gerror.New("账户绑定无效")
	}
	c := dao.YoubanBotManagerState.Columns()
	_, err = dao.YoubanBotManagerState.Ctx(ctx).Data(do.YoubanBotManagerState{TenantId: account.TenantId, AccountId: account.AccountId, EntryBotId: entryBotID, TelegramUserId: userID}).OnConflict(c.EntryBotId, c.TelegramUserId).InsertIgnore()
	if err != nil {
		return nil, gerror.Wrap(err, "机器人管理尚未初始化，请联系管理员执行数据库升级")
	}
	var state *entity.YoubanBotManagerState
	err = dao.YoubanBotManagerState.Ctx(ctx).Where(c.EntryBotId, entryBotID).Where(c.TelegramUserId, userID).Scan(&state)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, gerror.New("机器人管理状态不存在")
	}
	if state.TenantId != account.TenantId || state.AccountId != account.AccountId {
		err = dao.YoubanBotManagerState.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(state.Id).Data(do.YoubanBotManagerState{TenantId: account.TenantId, AccountId: account.AccountId, SelectedChannelIds: "[]", SelectionSaved: 0, InputMode: "", Keyword: "", DraftBotId: 0, Revision: state.Revision + 1}).Update(); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE "+dao.YoubanBotManagerState.Table()+" SET "+c.SyncedAt+"=NULL WHERE "+c.Id+"=?", state.Id)
			return err
		})
		if err != nil {
			return nil, err
		}
		return s.managerState(ctx, entryBotID, userID)
	}
	return state, nil
}

func managerChannels(ctx context.Context, st *entity.YoubanBotManagerState) ([]*entity.YoubanBotManagedChannel, error) {
	c := dao.YoubanBotManagedChannel.Columns()
	var rows []*entity.YoubanBotManagedChannel
	err := dao.YoubanBotManagedChannel.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).OrderAsc(c.Id).Scan(&rows)
	return rows, err
}
func managerBot(ctx context.Context, st *entity.YoubanBotManagerState, id int64) (*entity.YoubanBotManaged, error) {
	c := dao.YoubanBotManaged.Columns()
	var row *entity.YoubanBotManaged
	err := dao.YoubanBotManaged.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.Id, id).Scan(&row)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, gerror.New("机器人不存在或无权管理")
	}
	return row, nil
}

// Only show actual channel memberships, not the account's credential inventory.
func managerChannelBotsModel(ctx context.Context, st *entity.YoubanBotManagerState) *gdb.Model {
	bc, mc, cc := dao.YoubanBotManaged.Columns(), dao.YoubanBotManagedMember.Columns(), dao.YoubanBotManagedChannel.Columns()
	return dao.YoubanBotManaged.Ctx(ctx).Where(bc.TenantId, st.TenantId).Where(bc.AccountId, st.AccountId).
		Where("EXISTS (SELECT 1 FROM "+dao.YoubanBotManagedMember.Table()+" m JOIN "+dao.YoubanBotManagedChannel.Table()+" ch ON ch."+cc.Id+"=m."+mc.ChannelId+
			" WHERE m."+mc.ManagedBotId+"="+dao.YoubanBotManaged.Table()+"."+bc.Id+" AND m."+mc.TenantId+"=? AND m."+mc.AccountId+"=? AND ch."+cc.TenantId+"=? AND ch."+cc.AccountId+"=? AND ch."+cc.SyncStatus+"<>'unavailable' AND m."+mc.MemberStatus+" IN ('admin','member'))", st.TenantId, st.AccountId, st.TenantId, st.AccountId)
}

// Full syncs are durable and deduplicated; missing channel data may request one.
func (s *sSysBot) managerSync(ctx context.Context, st *entity.YoubanBotManagerState) error {
	return dao.YoubanBotManagerState.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var locked entity.YoubanBotManagerState
		if err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).LockUpdate().Scan(&locked); err != nil {
			return err
		}
		if locked.TenantId != st.TenantId || locked.AccountId != st.AccountId {
			return gerror.New("账户绑定已变化，请重新打开机器人管理")
		}
		tc := dao.YoubanBotManagerTask.Columns()
		n, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(tc.TenantId, st.TenantId).Where(tc.AccountId, st.AccountId).Where(tc.Action, "sync").WhereIn(tc.Status, []string{"pending", "submitted"}).Count()
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		channels, bots, err := pub.SysPublish().BotManagerCatalog(ctx, st.TenantId, st.AccountId)
		if err != nil {
			return err
		}
		bc := dao.YoubanBotManaged.Columns()
		for _, b := range bots {
			identity, err := strconv.ParseInt(strings.Split(b.BotToken, ":")[0], 10, 64)
			if err != nil || identity <= 0 {
				continue
			}
			// Catalog snapshots must not overwrite a Token rotated concurrently.
			// Existing publishing credentials are read live when mutating a Bot.
			_, err = dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: st.TenantId, AccountId: st.AccountId, TelegramBotId: identity, BotName: b.BotName, BotUsername: b.BotUsername, BotToken: b.BotToken, PublishBotId: b.Id, Status: 1}).OnConflict(bc.TenantId, bc.AccountId, bc.TelegramBotId).OnDuplicate(bc.BotName, bc.BotUsername, bc.PublishBotId).Save()
			if err != nil {
				return err
			}
			// Discovery can create identity-only rows before a Token is configured.
			// Fill only an empty credential; concurrent rotations remain untouched.
			if _, err := dao.YoubanBotManaged.Ctx(ctx).Where(bc.TenantId, st.TenantId).Where(bc.AccountId, st.AccountId).
				Where(bc.TelegramBotId, identity).Where(bc.BotToken, "").Data(do.YoubanBotManaged{BotToken: b.BotToken}).Update(); err != nil {
				return err
			}
		}
		cc := dao.YoubanBotManagedChannel.Columns()
		batch := guid.S()
		// Removed catalog entries remain visible as unavailable, never candidates.
		if _, err := dao.YoubanBotManagedChannel.Ctx(ctx).Where(cc.TenantId, st.TenantId).Where(cc.AccountId, st.AccountId).Data(do.YoubanBotManagedChannel{SyncStatus: "unavailable", CanManage: 0}).Update(); err != nil {
			return err
		}
		for _, c := range channels {
			url := ""
			if c.ChannelUsername != "" {
				url = "https://t.me/" + strings.TrimPrefix(c.ChannelUsername, "@")
			}
			_, err = dao.YoubanBotManagedChannel.Ctx(ctx).Data(do.YoubanBotManagedChannel{TenantId: st.TenantId, AccountId: st.AccountId, PublishChannelId: c.Id, TgAccountId: c.TgAccountId, ChatId: c.TargetChatId, Title: c.ChannelTitle, AccessHash: c.AccessHash, VisitUrl: url, SyncStatus: "unknown"}).OnConflict(cc.TenantId, cc.AccountId, cc.PublishChannelId).OnDuplicate(cc.TgAccountId, cc.ChatId, cc.Title, cc.AccessHash, cc.VisitUrl, cc.SyncStatus).Save()
			if err != nil {
				return err
			}
			var ch entity.YoubanBotManagedChannel
			if err := dao.YoubanBotManagedChannel.Ctx(ctx).Where(cc.TenantId, st.TenantId).Where(cc.AccountId, st.AccountId).Where(cc.PublishChannelId, c.Id).Scan(&ch); err != nil {
				return err
			}
			if _, err := managerNewTask(ctx, st, batch, "sync", 0, ch.Id, "{}"); err != nil {
				return err
			}
			if c.AccountError != "" || c.AccessHash == "" || c.AccessHash == "0" {
				reason := c.AccountError
				if reason == "" {
					reason = "频道缓存缺少访问凭据，请先刷新协议号频道缓存"
				}
				if _, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(tc.BatchKey, batch).Where(tc.ChannelId, ch.Id).Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: reason}).Update(); err != nil {
					return err
				}
				if _, err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(ch.Id).Data(do.YoubanBotManagedChannel{SyncStatus: "failed", ErrorMessage: reason}).Update(); err != nil {
					return err
				}
			}
		}
		if len(channels) == 0 {
			_, err = dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{SyncedAt: gtime.Now()}).Update()
		}
		return err
	})
}

func (s *sSysBot) managerSyncFeedback(ctx context.Context, st *entity.YoubanBotManagerState, msgID int) error {
	if err := s.managerSync(ctx, st); err != nil {
		return err
	}
	cc := dao.YoubanBotManagedChannel.Columns()
	available, err := dao.YoubanBotManagedChannel.Ctx(ctx).Where(cc.TenantId, st.TenantId).Where(cc.AccountId, st.AccountId).WhereNot(cc.SyncStatus, "unavailable").Count()
	if err != nil {
		return err
	}
	if available == 0 {
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "没有可同步的上架频道，请先配置上架频道并绑定可访问频道的 Telegram 账号。", managerSyncButtons())
	}
	c := dao.YoubanBotManagerTask.Columns()
	var root *entity.YoubanBotManagerTask
	err = dao.YoubanBotManagerTask.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.Action, "sync").OrderDesc(c.Id).Limit(1).Scan(&root)
	if err != nil {
		return err
	}
	if root == nil {
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, msgID, "没有可同步的上架频道，请先配置上架频道并绑定可访问频道的 Telegram 账号。", managerSyncButtons())
	}
	return s.managerStartProgress(ctx, st, root)
}

func managerNewTask(ctx context.Context, st *entity.YoubanBotManagerState, batch, action string, botID, channelID int64, rights string) (int64, error) {
	return dao.YoubanBotManagerTask.Ctx(ctx).Data(do.YoubanBotManagerTask{TenantId: st.TenantId, AccountId: st.AccountId, EntryBotId: st.EntryBotId, TelegramUserId: st.TelegramUserId, BatchKey: batch, Action: action, ManagedBotId: botID, ChannelId: channelID, RightsJson: rights, Status: "pending"}).InsertAndGetId()
}

func managerSelected(st *entity.YoubanBotManagerState, channels []*entity.YoubanBotManagedChannel) []int64 {
	previous := map[int64]bool{}
	for _, id := range managerIDs(st.SelectedChannelIds) {
		previous[id] = true
	}
	ids := []int64{}
	for _, c := range channels {
		if c.SyncStatus != "unavailable" && (st.SelectionSaved == 0 || previous[c.Id]) {
			ids = append(ids, c.Id)
		}
	}
	return ids
}

func managerCandidateLess(a, b *entity.YoubanBotManagedChannel, reserved map[int64]int) bool {
	x, y := a.BotCount+reserved[a.Id], b.BotCount+reserved[b.Id]
	if x != y {
		return x < y
	}
	x, y = managerRemaining(a), managerRemaining(b)
	if x != y {
		return x > y
	}
	if a.LastAssignedAt == nil && b.LastAssignedAt != nil {
		return true
	}
	if a.LastAssignedAt != nil && b.LastAssignedAt == nil {
		return false
	}
	if a.LastAssignedAt != nil && b.LastAssignedAt != nil && !a.LastAssignedAt.Equal(b.LastAssignedAt) {
		return a.LastAssignedAt.Before(b.LastAssignedAt)
	}
	return a.Id < b.Id
}
func managerRemaining(c *entity.YoubanBotManagedChannel) int {
	if c.AdminLimit <= 0 || c.AdminCount < 0 {
		return -1
	}
	return c.AdminLimit - c.AdminCount
}
func (s *sSysBot) managerAllocate(ctx context.Context, st *entity.YoubanBotManagerState, revision int64, batch ...string) (int64, error) {
	var taskID int64
	err := dao.YoubanBotManagerState.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current entity.YoubanBotManagerState
		if err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.TenantId != st.TenantId || current.AccountId != st.AccountId || current.Revision != revision || current.DraftBotId <= 0 {
			return gerror.New("此确认按钮已失效，请重新选择机器人")
		}
		b, err := managerBot(ctx, &current, current.DraftBotId)
		if err != nil {
			return err
		}
		if b.BotToken == "" {
			return gerror.New("请先配置机器人Token")
		}
		cc := dao.YoubanBotManagedChannel.Columns()
		var channels []*entity.YoubanBotManagedChannel
		// Lock in stable order to serialize reservations across multiple entry Bots/users.
		if err := dao.YoubanBotManagedChannel.Ctx(ctx).Where(cc.TenantId, st.TenantId).Where(cc.AccountId, st.AccountId).OrderAsc(cc.Id).LockUpdate().Scan(&channels); err != nil {
			return err
		}
		selected := map[int64]bool{}
		for _, id := range managerSelected(&current, channels) {
			selected[id] = true
		}
		tc := dao.YoubanBotManagerTask.Columns()
		var pending []*entity.YoubanBotManagerTask
		if err := dao.YoubanBotManagerTask.Ctx(ctx).Where(tc.TenantId, st.TenantId).Where(tc.AccountId, st.AccountId).Where(tc.Action, "attach").WhereIn(tc.Status, []string{"pending", "submitted"}).Scan(&pending); err != nil {
			return err
		}
		reserved := map[int64]int{}
		for _, p := range pending {
			if p.ManagedBotId == b.Id {
				return gerror.New("该机器人已有添加任务，请等待完成")
			}
			reserved[p.ChannelId]++
		}
		var members []*entity.YoubanBotManagedMember
		mc := dao.YoubanBotManagedMember.Columns()
		if err := dao.YoubanBotManagedMember.Ctx(ctx).Where(mc.ManagedBotId, b.Id).Where(mc.TenantId, st.TenantId).Where(mc.AccountId, st.AccountId).Where(mc.MemberStatus, "admin").Scan(&members); err != nil {
			return err
		}
		existing := map[int64]bool{}
		for _, m := range members {
			existing[m.ChannelId] = true
		}
		candidates := []*entity.YoubanBotManagedChannel{}
		for _, c := range channels {
			fresh := c.CanManage == 1 && c.SyncStatus == "success" && c.AdminCount >= 0 && c.LastSyncedAt != nil && time.Since(c.LastSyncedAt.Time) < 30*time.Minute
			if selected[c.Id] && fresh && (existing[c.Id] || c.AdminLimit <= 0 || c.AdminCount+reserved[c.Id] < c.AdminLimit) {
				candidates = append(candidates, c)
			}
		}
		if len(candidates) == 0 {
			return gerror.New("没有可分配的频道，请同步状态并确认管理权限和管理员名额")
		}
		sort.SliceStable(candidates, func(i, j int) bool { return managerCandidateLess(candidates[i], candidates[j], reserved) })
		chosen := candidates[0]
		for _, m := range members {
			if selected[m.ChannelId] {
				for _, c := range candidates {
					if c.Id == m.ChannelId {
						chosen = c
					}
				}
				break
			}
		}
		if !existing[chosen.Id] && chosen.AdminLimit > 0 && chosen.AdminCount+reserved[chosen.Id] >= chosen.AdminLimit {
			return gerror.New("频道管理员名额已被其他任务预留，请刷新后重试")
		}
		batchKey := guid.S()
		if len(batch) > 0 && batch[0] != "" {
			batchKey = batch[0]
		}
		taskID, err = managerNewTask(ctx, &current, batchKey, "attach", b.Id, chosen.Id, managerJSON(managerDefaultRights()))
		if err != nil {
			return err
		}
		if _, err = dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(chosen.Id).Data(do.YoubanBotManagedChannel{LastAssignedAt: gtime.Now()}).Update(); err != nil {
			return err
		}
		_, err = dao.YoubanBotManagerState.Ctx(ctx).WherePri(current.Id).Data(do.YoubanBotManagerState{SelectionSaved: 1, SelectedChannelIds: managerJSON(managerSelected(&current, channels)), DraftBotId: 0, Revision: current.Revision + 1, InputMode: ""}).Update()
		return err
	})
	return taskID, err
}

func managerTaskKey(id int64) string { return fmt.Sprintf("bot-manager:%d", id) }

func managerRetry(ctx context.Context, st *entity.YoubanBotManagerState, id int64) (int64, error) {
	var next int64
	err := dao.YoubanBotManagerTask.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotManagerTask.Columns()
		var row *entity.YoubanBotManagerTask
		if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.EntryBotId, st.EntryBotId).Where(c.TelegramUserId, st.TelegramUserId).LockUpdate().Scan(&row); err != nil {
			return err
		}
		if row == nil || row.Status != "failed" || row.Action == "sync" {
			return gerror.New("此任务不能重试，请重新打开机器人管理")
		}
		if _, err := managerBot(ctx, st, row.ManagedBotId); err != nil {
			return err
		}
		n, err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.TenantId, st.TenantId).Where(c.AccountId, st.AccountId).Where(c.ManagedBotId, row.ManagedBotId).Where(c.ChannelId, row.ChannelId).WhereGT(c.Id, id).Count()
		if err != nil {
			return err
		}
		if n > 0 {
			return gerror.New("已有后续操作，请查看最新任务，避免重复执行旧操作")
		}
		next, err = managerNewTask(ctx, st, guid.S(), row.Action, row.ManagedBotId, row.ChannelId, row.RightsJson)
		return err
	})
	return next, err
}
