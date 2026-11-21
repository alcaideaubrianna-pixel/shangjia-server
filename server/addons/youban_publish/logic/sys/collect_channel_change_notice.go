package sys

import (
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectChannelChangeNoticeCategory = "collect_channel_change"

type collectChannelChange struct {
	TenantID    int64
	ActorID     int64
	SourceID    int64
	ActorType   string
	ChannelID   string
	ChannelName string
	Action      string
	Before      string
	After       string
	EventID     string
}

// collectChannelChangeRecipients is used both at write time and before delivery.
func collectChannelChangeRecipients(ctx context.Context, tenantID int64) *gdb.Model {
	account := pdao.YoubanPublishAccount
	c := account.Columns()
	return account.Ctx(ctx).As("a").
		InnerJoin(publishAccountSettingTable+" s", "s.account_id=a.id AND s.tenant_id=a.tenant_id AND s.deleted_at IS NULL").
		Where("a."+c.TenantId, tenantID).
		Where("a."+c.Status, 1).
		Where("a."+c.AccountType, sysin.PublishAccountTypeUploader).
		WhereNull("a."+c.DeletedAt).
		Where("s.collect_push_enabled", 1)
}

// enqueueCollectChannelChange persists one outbox row per authorized account.
// Call with the business transaction context when the write and notification must commit together.
func enqueueCollectChannelChange(ctx context.Context, change collectChannelChange) error {
	change.ChannelID = canonicalCollectSourceChatID(change.ChannelID)
	if change.TenantID <= 0 || change.SourceID <= 0 || change.ChannelID == "" || strings.TrimSpace(change.Action) == "" {
		return gerror.New("频道变更通知缺少目标信息")
	}
	if change.ActorType != "system" && (change.ActorID <= 0 || change.ActorType == "") {
		return gerror.New("频道变更通知缺少操作人")
	}
	if change.EventID == "" {
		change.EventID = guid.S()
	}
	var ids []struct{ Id int64 }
	if err := collectChannelChangeRecipients(ctx, change.TenantID).Fields("a.id").Distinct().Scan(&ids); err != nil {
		return gerror.Wrap(err, "读取频道通知接收账户失败")
	}
	if len(ids) == 0 {
		return nil
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s:%s", change.TenantID, change.SourceID, change.ChannelID, change.Action, change.EventID)))
	key := fmt.Sprintf("collect-channel:%x", digest)
	body := collectChannelChangeText(change)
	notices := make([]do.YoubanPublishAccountNotice, 0, len(ids))
	for _, id := range ids {
		notices = append(notices, do.YoubanPublishAccountNotice{
			TenantId: change.TenantID, AccountId: id.Id, Category: collectChannelChangeNoticeCategory,
			EventKey: key, TextContent: body, Status: "collect_pending",
		})
	}
	_, err := pdao.YoubanPublishAccountNotice.Ctx(ctx).Data(notices).
		Batch(200).OnConflict("event_key,account_id").OnDuplicate("event_key").Save()
	return gerror.Wrap(err, "保存频道变更通知失败")
}

func collectChannelChangeText(change collectChannelChange) string {
	actor := "系统发现"
	if change.ActorType != "system" {
		label := "上架账户"
		if change.ActorType == sysin.PublishAccountTypeAdmin {
			label = "管理员"
		}
		actor = fmt.Sprintf("%s ID：%d", label, change.ActorID)
	}
	text := fmt.Sprintf("<b>采集频道变更</b>\n频道：%s\n频道 ID：%s\n采集源 ID：%d\n操作：%s\n操作人：%s\n时间：%s",
		html.EscapeString(firstNonEmpty(change.ChannelName, change.ChannelID)), html.EscapeString(change.ChannelID),
		change.SourceID, html.EscapeString(change.Action), actor,
		time.Now().In(time.FixedZone("UTC+8", 8*60*60)).Format("2006-01-02 15:04:05 -07:00"))
	if change.Before != "" || change.After != "" {
		text += fmt.Sprintf("\n变更前：%s\n变更后：%s",
			html.EscapeString(firstNonEmpty(change.Before, "空")), html.EscapeString(firstNonEmpty(change.After, "空")))
	}
	return text
}

func (s *sSysPublish) deliverCollectChannelChangeNotice(ctx context.Context, row *entity.YoubanPublishAccountNotice) (skipped bool, err error) {
	allowed, err := collectChannelChangeRecipients(ctx, row.TenantId).Where("a.id", row.AccountId).Exist()
	if err != nil {
		return false, err
	}
	if !allowed {
		return true, nil
	}
	err = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
		NoticeId: row.Id, AccountId: row.AccountId, App: "api", Text: row.TextContent, ParseMode: "HTML",
		BotStrategy: "official", IgnoreFeatureSwitch: true, RequireDelivery: true,
	})
	if err != nil {
		switch strings.TrimSpace(err.Error()) {
		case "当前账号未绑定Telegram", "没有可用的Telegram通知会话":
			return true, nil
		}
	}
	return false, err
}
