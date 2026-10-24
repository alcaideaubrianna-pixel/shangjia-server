package sys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	botin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
)

type accountChannelAccessNotification struct {
	Event        collectorin.AccountChannelAccessEvent `json:"event"`
	TGAccountID  int64                                 `json:"tgAccountId"`
	TenantID     int64                                 `json:"tenantId"`
	AccountID    int64                                 `json:"accountId"`
	SourceID     int64                                 `json:"sourceId"`
	SourceTitle  string                                `json:"sourceTitle"`
	Version      string                                `json:"version"`
	PauseVersion string                                `json:"pauseVersion"`
}

type accountChannelAccessSource struct {
	Id        int64
	AccountId int64
	Title     string
	Version   string
}

func (w *accountCollectWorker) HandleAccountRuntimeChannelAccess(ctx context.Context, event collectorin.AccountChannelAccessEvent) error {
	if event.ChannelID <= 0 || event.UserID <= 0 || accountChannelAccessReason(event.Reason) == "" {
		return nil
	}
	owner, err := w.service.notifyTgAccountOwner(ctx, w.tgAccountId, 0)
	if err != nil || owner.Id <= 0 {
		return err
	}
	sources, err := loadAccountChannelAccessSources(ctx, g.DB(), owner.TenantId, w.tgAccountId, event)
	if err != nil {
		return err
	}
	for _, source := range sources {
		payload := &accountChannelAccessNotification{Event: event, TGAccountID: w.tgAccountId, TenantID: owner.TenantId, AccountID: source.AccountId, SourceID: source.Id, SourceTitle: source.Title, Version: source.Version, PauseVersion: time.Now().Format("2006-01-02 15:04:05.999999")}
		id := fmt.Sprintf("account-channel-access:%d:%d:%x", owner.TenantId, source.Id, sha256.Sum256([]byte(source.Version)))
		if err := w.service.enqueueCollectNotification(ctx, collectNotificationPayload{ID: id, ChannelAccess: payload}); err != nil {
			return gerror.Wrap(err, "投递TG账号频道异常通知失败")
		}
		g.Log().Warningf(ctx, "TG账号频道访问异常已入队 tgAccountId:%d tenantId:%d sourceId:%d channelId:%d reason:%s", w.tgAccountId, owner.TenantId, source.Id, event.ChannelID, event.Reason)
	}
	return nil
}

func loadAccountChannelAccessSources(ctx context.Context, db gdb.DB, tenantID, tgAccountID int64, event collectorin.AccountChannelAccessEvent) ([]accountChannelAccessSource, error) {
	c := pdao.YoubanPublishCollectSource.Columns()
	var sources []accountChannelAccessSource
	mod := db.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
		Fields(c.Id, c.AccountId, c.Title, c.UpdatedAt+"::text AS version").
		Where(c.TenantId, tenantID).Where(c.TgAccountId, tgAccountID).
		Where(c.SourceType, sysin.CollectSourceTypeAccount).
		WhereIn(c.SourceChatId, tgChannelCacheLookupIds(strconv.FormatInt(event.ChannelID, 10))).
		Where(c.CollectEnabled, 1).Where(c.Status, 1).WhereNull(c.DeletedAt)
	if event.Date > 0 {
		// Old/replayed updates must not pause a later recreated or edited source.
		mod = mod.WhereLT(c.UpdatedAt, time.Unix(int64(event.Date)+1, 0))
	}
	err := mod.Scan(&sources)
	return sources, gerror.Wrap(err, "读取TG账号频道异常采集源失败")
}

func accountChannelAccessReason(reason string) string {
	switch reason {
	case collectorin.AccountChannelAccessRemoved:
		return "TG账号已被移出频道或被禁止查看消息"
	case collectorin.AccountChannelAccessLeft:
		return "TG账号已不在该频道"
	case collectorin.AccountChannelAccessForbidden:
		return "TG账号已无法访问该频道（Telegram返回频道不可访问状态）"
	default:
		return ""
	}
}

func accountChannelAccessText(owner *tgAccountAuthOwner, notice *accountChannelAccessNotification) string {
	name := firstNonEmpty(owner.DisplayName, fmt.Sprintf("账号%d", owner.Id))
	if username := strings.TrimPrefix(strings.TrimSpace(owner.TelegramUsername), "@"); username != "" {
		name += "（@" + username + "）"
	}
	return fmt.Sprintf("⚠️ 频道采集已暂停\n\nTG号：%s\nTG账号编号：%d\n频道：%s\n频道ID：%d\n采集来源：%s（编号%d）\n原因：%s\n\n该频道的新资料暂时无法采集，其他频道不受影响。请先恢复此TG号的频道访问权限，再在采集配置中重新启用。已有资料保留，历史消息不会自动补采。",
		html.EscapeString(name), owner.Id, html.EscapeString(firstNonEmpty(notice.Event.Title, notice.SourceTitle)), notice.Event.ChannelID, html.EscapeString(notice.SourceTitle), notice.SourceID, html.EscapeString(accountChannelAccessReason(notice.Event.Reason)))
}

func (s *sSysPublish) handleAccountChannelAccessNotification(ctx context.Context, id string, notice *accountChannelAccessNotification) error {
	if notice.TenantID <= 0 || notice.TGAccountID <= 0 || notice.SourceID <= 0 || notice.AccountID <= 0 || notice.Version == "" || notice.PauseVersion == "" || accountChannelAccessReason(notice.Event.Reason) == "" {
		return gerror.New("TG账号频道异常通知归属不完整")
	}
	owner, err := s.notifyTgAccountOwner(ctx, notice.TGAccountID, notice.TenantID)
	if err != nil || owner.Id <= 0 {
		return err
	}
	// The queued pause version is stable across retries. A later manual edit
	// has a different version and must never be overwritten by this incident.
	paused, err := pauseAccountChannelSource(ctx, g.DB(), notice)
	if err != nil || !paused {
		return err
	}
	s.refreshCollectSourceCache(ctx)
	s.refreshAccountCollectSupervisor()
	text := accountChannelAccessText(owner, notice)
	// Reuse configured collection administrators and their per-recipient dedup.
	adminErr := botservice.SysBot().NotifyCollectEvent(ctx, 0, "", id, text, nil)
	key := publishconsts.AccountChannelAccessNotifyKeyPrefix + id
	sent, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return errors.Join(adminErr, err)
	}
	if !sent.IsNil() {
		return adminErr
	}
	if err = s.notifyAccountWithPreference(ctx, notice.TenantID, "channel", &botin.NotifyAccountInp{App: consts.AppApi, AccountId: notice.AccountID, BotStrategy: "official", FallbackBoundBot: true, IgnoreFeatureSwitch: true, RequireDelivery: true, Text: text, ParseMode: "HTML"}); err != nil {
		return errors.Join(adminErr, err)
	}
	if err = cache.Instance().Set(ctx, key, true, 7*24*time.Hour); err != nil {
		return errors.Join(adminErr, err)
	}
	g.Log().Infof(ctx, "TG账号频道异常通知完成 tgAccountId:%d tenantId:%d accountId:%d sourceId:%d channelId:%d reason:%s", notice.TGAccountID, notice.TenantID, notice.AccountID, notice.SourceID, notice.Event.ChannelID, notice.Event.Reason)
	return adminErr
}

func pauseAccountChannelSource(ctx context.Context, db gdb.DB, notice *accountChannelAccessNotification) (bool, error) {
	c := pdao.YoubanPublishCollectSource.Columns()
	paused := false
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		mod := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).
			Where(c.Id, notice.SourceID).Where(c.TenantId, notice.TenantID).Where(c.AccountId, notice.AccountID).
			Where(c.TgAccountId, notice.TGAccountID).Where(c.SourceType, sysin.CollectSourceTypeAccount).
			WhereIn(c.SourceChatId, tgChannelCacheLookupIds(strconv.FormatInt(notice.Event.ChannelID, 10))).WhereNull(c.DeletedAt)
		row, err := mod.Clone().Fields(c.CollectEnabled, c.Status, c.UpdatedAt+"::text AS version").LockUpdate().One()
		if err != nil || row.IsEmpty() {
			return err
		}
		if row[c.Status].Int() != 1 {
			return nil
		}
		if row[c.CollectEnabled].Int() == 0 {
			paused = row["version"].String() == notice.PauseVersion
			return nil
		}
		if row["version"].String() != notice.Version {
			return nil
		}
		_, err = mod.Data(g.Map{c.CollectEnabled: 0, c.UpdatedAt: notice.PauseVersion}).Update()
		paused = err == nil
		return err
	})
	return paused, gerror.Wrap(err, "暂停失去频道访问权限的采集源失败")
}
