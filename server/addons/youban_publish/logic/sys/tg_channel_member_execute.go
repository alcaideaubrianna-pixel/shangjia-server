package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const tgChannelMemberPageLimit = 200

type tgChannelMemberRow struct {
	UserId          int64
	AccessHash      string
	DisplayName     string
	FirstName       string
	LastName        string
	Username        string
	Phone           string
	ParticipantRole string
	IsBot           int
	IsPremium       int
}

func (s *sSysPublish) ExecuteChannelMemberSyncTask(ctx context.Context, taskId int64) error {
	task, err := s.channelMemberSyncTaskById(ctx, taskId)
	if err != nil {
		return err
	}
	if channelMemberTaskTerminal(task.Status) {
		return nil
	}
	accountTaskID, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: task.TenantId, AccountID: task.TgAccountId,
		TaskType: collectorin.AccountTaskTypeChannelMemberSync,
		TaskKey:  fmt.Sprintf("channel-member-sync:%d", task.Id), Priority: 50, MaxAttempts: 3,
	})
	if err != nil {
		_ = s.markChannelMemberTaskFailed(ctx, task.Id, err)
		return err
	}
	g.Log().Infof(ctx, "频道成员同步已提交账号任务 taskId:%d accountTaskId:%d", task.Id, accountTaskID)
	return nil
}

func (s *sSysPublish) executeChannelMemberSyncWithClient(ctx context.Context, client *telegram.Client, task *sysin.TgChannelMemberSyncModel) error {
	cache, err := s.tgChannelCacheByChannelId(ctx, task.TenantId, task.TgAccountId, task.ChannelId)
	if err != nil {
		return err
	}
	peer, err := channelMemberInputChannel(cache)
	if err != nil {
		return err
	}
	startAt := gtime.Now()
	seen := map[int64]string{}
	if _, err = client.Self(ctx); err != nil {
		return err
	}
	if err = s.syncChannelMembersByFilter(ctx, client, task, peer, &tg.ChannelParticipantsAdmins{}, seen, tgChannelMemberStageAdmins); err != nil {
		return err
	}
	err = s.syncChannelMembersByFilter(ctx, client, task, peer, &tg.ChannelParticipantsRecent{}, seen, tgChannelMemberStageMembers)
	if err != nil {
		return err
	}
	return s.finishChannelMemberSync(ctx, task, startAt)
}

func (s *sSysPublish) handleChannelMemberSyncAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) error {
	const prefix = "channel-member-sync:"
	id, err := strconv.ParseInt(strings.TrimPrefix(task.TaskKey, prefix), 10, 64)
	if err != nil || id <= 0 || !strings.HasPrefix(task.TaskKey, prefix) || task.AccountID <= 0 {
		return gerror.New("频道成员同步账号任务参数无效")
	}
	model, err := s.channelMemberSyncTaskById(ctx, id)
	if err != nil {
		return err
	}
	if model.TgAccountId != task.AccountID {
		return gerror.New("频道成员同步账号任务账号不一致")
	}
	return s.executeChannelMemberSyncWithClient(ctx, client, model)
}

func channelMemberInputChannel(channel *sysin.ChannelCacheModel) (*tg.InputChannel, error) {
	if channel == nil {
		return nil, gerror.New("频道缓存为空")
	}
	channelId, err := strconv.ParseInt(strings.TrimSpace(channel.ChannelId), 10, 64)
	if err != nil || channelId <= 0 {
		return nil, gerror.New("普通群组暂不支持成员同步，请选择频道或超级群")
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(channel.AccessHash), 10, 64)
	if err != nil || accessHash == 0 {
		return nil, gerror.New("频道AccessHash无效，请先刷新频道缓存")
	}
	return &tg.InputChannel{ChannelID: channelId, AccessHash: accessHash}, nil
}

func (s *sSysPublish) syncChannelMembersByFilter(
	ctx context.Context,
	client *telegram.Client,
	task *sysin.TgChannelMemberSyncModel,
	peer *tg.InputChannel,
	filter tg.ChannelParticipantsFilterClass,
	seen map[int64]string,
	stage string,
) error {
	if err := s.markChannelMemberTaskRunning(ctx, task.Id, stage); err != nil {
		return err
	}
	offset := 0
	total := 0
	done := 0
	for {
		if err := s.ensureChannelMemberTaskNotCanceled(ctx, task.Id); err != nil {
			return err
		}
		page, err := channelMemberParticipantsPage(ctx, client, peer, filter, offset)
		if err != nil {
			return err
		}
		if total == 0 && page.Count > 0 {
			total = page.Count
			if err = s.updateChannelMemberTaskTotal(ctx, task.Id, stage, total); err != nil {
				return err
			}
		}
		rows := channelMemberRows(page, seen)
		if err = s.upsertChannelMemberRows(ctx, task, rows); err != nil {
			return err
		}
		done += len(page.Participants)
		if err = s.updateChannelMemberTaskDone(ctx, task.Id, stage, done, len(rows)); err != nil {
			return err
		}
		if len(page.Participants) < tgChannelMemberPageLimit {
			return nil
		}
		offset += len(page.Participants)
		time.Sleep(600 * time.Millisecond)
	}
}

func channelMemberParticipantsPage(ctx context.Context, client *telegram.Client, peer *tg.InputChannel, filter tg.ChannelParticipantsFilterClass, offset int) (*tg.ChannelsChannelParticipants, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		res, err := client.API().ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
			Channel: peer,
			Filter:  filter,
			Offset:  offset,
			Limit:   tgChannelMemberPageLimit,
			Hash:    0,
		})
		if err == nil {
			page, ok := res.AsModified()
			if !ok || page == nil {
				return &tg.ChannelsChannelParticipants{}, nil
			}
			return page, nil
		}
		lastErr = err
		delay := tgChannelMemberRetryDelay(err)
		if delay <= 0 || attempt == 2 {
			return nil, gerror.Wrap(err, "拉取TG频道成员失败")
		}
		time.Sleep(delay)
	}
	return nil, lastErr
}

func tgChannelMemberRetryDelay(err error) time.Duration {
	if delay, ok := collectMediaFloodWaitDelay(err); ok {
		return delay
	}
	if isTgRepairRetryableErr(err) {
		return 2 * time.Second
	}
	return 0
}

func channelMemberRows(page *tg.ChannelsChannelParticipants, seen map[int64]string) []tgChannelMemberRow {
	if page == nil {
		return nil
	}
	users := channelMemberUsers(page.Users)
	rows := make([]tgChannelMemberRow, 0, len(page.Participants))
	for _, participant := range page.Participants {
		userId, role := channelMemberParticipantRole(participant)
		if userId <= 0 || role == "" {
			continue
		}
		if oldRole, ok := seen[userId]; ok && channelMemberRolePriority(oldRole) >= channelMemberRolePriority(role) {
			continue
		}
		seen[userId] = role
		rows = append(rows, channelMemberRowFromUser(users[userId], userId, role))
	}
	return rows
}

func channelMemberUsers(items []tg.UserClass) map[int64]*tg.User {
	users := make(map[int64]*tg.User, len(items))
	for _, item := range items {
		user, ok := item.(*tg.User)
		if !ok || user.ID <= 0 {
			continue
		}
		users[user.ID] = user
	}
	return users
}

func channelMemberParticipantRole(item tg.ChannelParticipantClass) (int64, string) {
	switch participant := item.(type) {
	case *tg.ChannelParticipantCreator:
		return participant.UserID, "creator"
	case *tg.ChannelParticipantAdmin:
		return participant.UserID, "admin"
	case *tg.ChannelParticipant:
		return participant.UserID, "member"
	case *tg.ChannelParticipantSelf:
		return participant.UserID, "member"
	default:
		return 0, ""
	}
}

func channelMemberRowFromUser(user *tg.User, userId int64, role string) tgChannelMemberRow {
	row := tgChannelMemberRow{UserId: userId, ParticipantRole: role}
	if user == nil {
		row.DisplayName = strconv.FormatInt(userId, 10)
		return row
	}
	accessHash, _ := user.GetAccessHash()
	firstName, _ := user.GetFirstName()
	lastName, _ := user.GetLastName()
	username, _ := user.GetUsername()
	phone, _ := user.GetPhone()
	row.AccessHash = strconv.FormatInt(accessHash, 10)
	row.FirstName = strings.TrimSpace(firstName)
	row.LastName = strings.TrimSpace(lastName)
	row.Username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	row.Phone = strings.TrimSpace(phone)
	row.DisplayName = strings.TrimSpace(strings.Join([]string{row.FirstName, row.LastName}, " "))
	if row.DisplayName == "" {
		row.DisplayName = firstNonEmpty(row.Username, strconv.FormatInt(user.ID, 10))
	}
	row.IsBot = boolToInt(user.Bot)
	row.IsPremium = boolToInt(user.Premium)
	return row
}

func channelMemberRolePriority(role string) int {
	switch role {
	case "creator":
		return 3
	case "admin":
		return 2
	case "member":
		return 1
	default:
		return 0
	}
}

func (s *sSysPublish) upsertChannelMemberRows(ctx context.Context, task *sysin.TgChannelMemberSyncModel, rows []tgChannelMemberRow) error {
	if len(rows) == 0 {
		return nil
	}
	now := gtime.Now()
	data := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		data = append(data, g.Map{
			"tenant_id":         task.TenantId,
			"tg_account_id":     task.TgAccountId,
			"channel_id":        task.ChannelId,
			"user_id":           row.UserId,
			"access_hash":       row.AccessHash,
			"display_name":      row.DisplayName,
			"first_name":        row.FirstName,
			"last_name":         row.LastName,
			"username":          row.Username,
			"phone":             row.Phone,
			"participant_role":  row.ParticipantRole,
			"is_bot":            row.IsBot,
			"is_premium":        row.IsPremium,
			"status":            1,
			"last_sync_task_id": task.Id,
			"last_synced_at":    now,
			"created_at":        now,
			"updated_at":        now,
		})
	}
	_, err := g.DB().Model(publishTgChannelMemberTable).Safe().Ctx(ctx).
		Data(data).
		Batch(100).
		OnConflict("tenant_id,tg_account_id,channel_id,user_id").
		OnDuplicateEx("id,tenant_id,tg_account_id,channel_id,user_id,created_at").
		Save()
	return gerror.Wrap(err, "保存TG频道成员缓存失败")
}

func (s *sSysPublish) finishChannelMemberSync(ctx context.Context, task *sysin.TgChannelMemberSyncModel, startAt *gtime.Time) error {
	result, err := g.DB().Model(publishTgChannelMemberTable).Safe().Ctx(ctx).
		Where("tenant_id", task.TenantId).
		Where("tg_account_id", task.TgAccountId).
		Where("channel_id", task.ChannelId).
		WhereLT("updated_at", startAt).
		Data(g.Map{"status": 2, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "标记失效频道成员失败")
	}
	removed, _ := result.RowsAffected()
	_, err = g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", task.Id).Data(g.Map{
		"status":        tgChannelMemberStatusSuccess,
		"stage":         tgChannelMemberStageFinish,
		"removed_count": removed,
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "完成频道成员同步任务失败")
}

func (s *sSysPublish) channelMemberSyncTaskById(ctx context.Context, id int64) (*sysin.TgChannelMemberSyncModel, error) {
	row, err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", id).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道成员同步任务失败")
	}
	task := channelMemberTaskFromRecord(row)
	if task == nil || task.Id <= 0 {
		return nil, gerror.New("频道成员同步任务不存在")
	}
	return task, nil
}

func (s *sSysPublish) ensureChannelMemberTaskNotCanceled(ctx context.Context, id int64) error {
	value, err := g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", id).Fields("status").Value()
	if err != nil {
		return gerror.Wrap(err, "检查频道成员同步任务状态失败")
	}
	if value.String() == tgChannelMemberStatusCanceled {
		return gerror.New("频道成员同步任务已取消")
	}
	return nil
}

func (s *sSysPublish) updateChannelMemberTaskTotal(ctx context.Context, id int64, stage string, total int) error {
	data := g.Map{"updated_at": gtime.Now()}
	if stage == tgChannelMemberStageAdmins {
		data["admin_total"] = total
	} else {
		data["member_total"] = total
	}
	return s.updateChannelMemberTaskProgress(ctx, id, data)
}

func (s *sSysPublish) updateChannelMemberTaskDone(ctx context.Context, id int64, stage string, done int, upserted int) error {
	data := g.Map{"updated_at": gtime.Now(), "upserted_count": gdb.Raw("upserted_count+" + strconv.Itoa(upserted))}
	if stage == tgChannelMemberStageAdmins {
		data["admin_done"] = done
	} else {
		data["member_done"] = done
	}
	return s.updateChannelMemberTaskProgress(ctx, id, data)
}

func (s *sSysPublish) updateChannelMemberTaskProgress(ctx context.Context, id int64, data g.Map) error {
	task, err := s.channelMemberSyncTaskById(ctx, id)
	if err != nil {
		return err
	}
	done := task.AdminDone
	total := task.AdminTotal
	if value, ok := data["admin_done"]; ok {
		done = value.(int)
	}
	if value, ok := data["admin_total"]; ok {
		total = value.(int)
	}
	memberDone := task.MemberDone
	memberTotal := task.MemberTotal
	if value, ok := data["member_done"]; ok {
		memberDone = value.(int)
	}
	if value, ok := data["member_total"]; ok {
		memberTotal = value.(int)
	}
	data["progress_done"] = done + memberDone
	data["progress_total"] = total + memberTotal
	_, err = g.DB().Model(publishTgChannelMemberTaskTable).Safe().Ctx(ctx).Where("id", id).Data(data).Update()
	return gerror.Wrap(err, "更新频道成员同步进度失败")
}
