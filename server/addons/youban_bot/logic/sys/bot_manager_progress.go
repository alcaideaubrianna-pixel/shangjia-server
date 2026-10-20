package sys

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/internal/library/cache"
	"hotgo/utility/sensitive"
)

// Only presentation metadata is cached. Task progress remains in the DB and
// completion notifications still work if Redis loses this optional pointer.
type managerProgressTarget struct {
	MessageID int    `json:"messageId"`
	Text      string `json:"text"`
}

func managerProgressKey(root *entity.YoubanBotManagerTask) string {
	return fmt.Sprintf("%s%d:%d:%s", consts.ManagerProgressPrefix, root.EntryBotId, root.TelegramUserId, root.BatchKey)
}

func managerProgressLoad(ctx context.Context, root *entity.YoubanBotManagerTask) (managerProgressTarget, error) {
	var target managerProgressTarget
	if !cache.Initialized() {
		return target, nil
	}
	value, err := cache.Instance().Get(ctx, managerProgressKey(root))
	if err != nil || value == nil || value.IsNil() {
		return target, err
	}
	return target, value.Scan(&target)
}

func managerProgressSave(ctx context.Context, root *entity.YoubanBotManagerTask, target managerProgressTarget) error {
	if !cache.Initialized() {
		return nil
	}
	return cache.Instance().Set(ctx, managerProgressKey(root), target, 24*time.Hour)
}

func managerSyncProgressText(ctx context.Context, root *entity.YoubanBotManagerTask) (string, error) {
	c := dao.YoubanBotManagerTask.Columns()
	var tasks []*entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.BatchKey, root.BatchKey).Where(c.TenantId, root.TenantId).Where(c.AccountId, root.AccountId).OrderAsc(c.Id).Scan(&tasks); err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "", gerror.New("同步任务不存在，请重新同步")
	}
	channels, err := managerChannels(ctx, &entity.YoubanBotManagerState{TenantId: root.TenantId, AccountId: root.AccountId})
	if err != nil {
		return "", err
	}
	titles := make(map[int64]string, len(channels))
	for _, ch := range channels {
		titles[ch.Id] = ch.Title
	}
	success, failed := 0, 0
	var current, failures []string
	for _, task := range tasks {
		title := html.EscapeString(firstNonEmpty(titles[task.ChannelId], fmt.Sprintf("频道 #%d", task.ChannelId)))
		switch task.Status {
		case "success":
			success++
		case "failed":
			failed++
			if len(failures) < 3 {
				failures = append(failures, title+"："+html.EscapeString(sensitive.Diagnostic(task.ErrorMessage)))
			}
		default:
			if task.Status == "submitted" && len(current) < 3 {
				current = append(current, title)
			}
		}
	}
	done := success + failed
	filled := done * 10 / len(tasks)
	text := fmt.Sprintf("<b>正在同步 %d 个上架频道</b>\n%s%s %d%%\n已处理 %d/%d · 成功 %d · 失败 %d\n", len(tasks), strings.Repeat("🟩", filled), strings.Repeat("⬜", 10-filled), done*100/len(tasks), done, len(tasks), success, failed)
	if done == len(tasks) {
		text = strings.Replace(text, "正在同步", "同步完成：", 1)
	} else if len(current) > 0 {
		text += "进行中／等待协议号：" + strings.Join(current, "、") + "\n"
	} else {
		text += "正在等待协议号调度。\n"
	}
	if len(failures) > 0 {
		text += "\n⚠️ " + strings.Join(failures, "\n⚠️ ")
	}
	return text, nil
}

func (s *sSysBot) managerStartProgress(ctx context.Context, st *entity.YoubanBotManagerState, root *entity.YoubanBotManagerTask) error {
	if root.EntryBotId != st.EntryBotId || root.TelegramUserId != st.TelegramUserId {
		return s.managerSend(ctx, st.EntryBotId, st.TelegramUserId, 0, "此账户已有同步任务，完成后会通知最初发起同步的入口。", managerSyncButtons())
	}
	text, err := managerSyncProgressText(ctx, root)
	if err != nil {
		return err
	}
	target, err := managerProgressLoad(ctx, root)
	if err != nil {
		g.Log().Warningf(ctx, "读取机器人同步进度消息失败 batch:%s err:%s", root.BatchKey, sensitive.Diagnostic(err.Error()))
		target = managerProgressTarget{}
	}
	if target.MessageID > 0 && target.Text == text {
		return nil
	}
	// A dedicated progress message must not overwrite the interactive menu.
	id, err := s.managerSendMessage(ctx, st.EntryBotId, st.TelegramUserId, target.MessageID, text, managerSyncButtons())
	if err != nil && managerProgressMessageMissing(err) {
		id, err = s.managerSendMessage(ctx, st.EntryBotId, st.TelegramUserId, 0, text, managerSyncButtons())
	}
	if err != nil {
		return err
	}
	if err := managerProgressSave(ctx, root, managerProgressTarget{MessageID: id, Text: text}); err != nil {
		g.Log().Warningf(ctx, "保存机器人同步进度消息失败 batch:%s err:%s", root.BatchKey, sensitive.Diagnostic(err.Error()))
	}
	return nil
}

func managerProgressMessageMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "message to edit not found") || strings.Contains(message, "message can't be edited")
}

func (s *sSysBot) managerUpdateSyncProgress(ctx context.Context) error {
	if !cache.Initialized() {
		return nil
	}
	c := dao.YoubanBotManagerTask.Columns()
	var roots []*entity.YoubanBotManagerTask
	first := fmt.Sprintf("%s IN (SELECT MIN(%s) FROM %s WHERE %s='sync' AND %s=0 GROUP BY %s)", c.Id, c.Id, dao.YoubanBotManagerTask.Table(), c.Action, c.Notified, c.BatchKey)
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Where(c.Action, "sync").Where(c.Notified, 0).Where(first).OrderAsc(c.UpdatedAt).OrderAsc(c.Id).Limit(20).Scan(&roots); err != nil {
		return err
	}
	for _, root := range roots {
		// Touch only updated_at: never write a stale status over a worker result.
		if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(root.Id).Data(do.YoubanBotManagerTask{UpdatedAt: gtime.Now()}).Update(); err != nil {
			return err
		}
		st, err := s.managerState(ctx, root.EntryBotId, root.TelegramUserId)
		if err != nil || st.TenantId != root.TenantId || st.AccountId != root.AccountId {
			continue
		}
		target, err := managerProgressLoad(ctx, root)
		if err != nil || target.MessageID == 0 {
			continue
		}
		text, err := managerSyncProgressText(ctx, root)
		if err != nil {
			return err
		}
		if text == target.Text {
			continue
		}
		call, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = s.managerSend(call, root.EntryBotId, root.TelegramUserId, target.MessageID, text, managerSyncButtons())
		cancel()
		if err != nil {
			// Optional progress cannot prevent completion notification or dispatch.
			if managerProgressMessageMissing(err) {
				_, _ = cache.Instance().Remove(ctx, managerProgressKey(root))
			}
			continue
		}
		if err := managerProgressSave(ctx, root, managerProgressTarget{MessageID: target.MessageID, Text: text}); err != nil {
			return err
		}
	}
	return nil
}

func managerPermanentSyncError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	message := strings.ToUpper(err.Error())
	for _, code := range []string{"AUTH_KEY_UNREGISTERED", "AUTH_KEY_INVALID", "SESSION_REVOKED", "SESSION_EXPIRED", "USER_DEACTIVATED"} {
		if strings.Contains(message, code) {
			return "协议号登录已失效，请重新登录后同步", true
		}
	}
	for _, code := range []string{"CHAT_ADMIN_REQUIRED", "CHANNEL_PRIVATE", "CHANNEL_INVALID", "USER_NOT_PARTICIPANT"} {
		if strings.Contains(message, code) {
			return "协议号无法访问频道或没有查看管理员的权限，请检查成员及管理员权限", true
		}
	}
	return sensitive.Diagnostic(err.Error()), false
}
