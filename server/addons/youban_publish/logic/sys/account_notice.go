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
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Both web/admin and Bot account deletion persist notices in this transaction.
// No network operation occurs until a scheduler observes the committed row.
func (s *sSysPublish) deleteAccountsWithNotices(ctx context.Context, operator *sysin.AccountModel, ids []int64, tenantID int64) error {
	if operator == nil || operator.Id <= 0 {
		return gerror.New("操作账户不能为空")
	}
	affectedTenants := map[int64]bool{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		cols := pdao.YoubanPublishAccount.Columns()
		mod := pdao.YoubanPublishAccount.Ctx(ctx).WhereIn(cols.Id, ids).WhereNull(cols.DeletedAt)
		if tenantID > 0 {
			mod = mod.Where(cols.TenantId, tenantID).Where(cols.ParentId, operator.Id)
		}
		rows, err := mod.Clone().OrderAsc(cols.Id).LockUpdate().Fields(cols.Id, cols.TenantId, cols.ParentId, cols.AccountType, cols.Username, cols.Nickname).All()
		if err != nil {
			return gerror.Wrap(err, "读取待删除账户失败")
		}
		if tenantID > 0 && len(rows) != len(ids) {
			return gerror.New("账户归属或状态已改变，请刷新后重试")
		}
		operation := guid.S()
		for _, row := range rows {
			id := row[cols.Id].Int64()
			ownerTenant := row[cols.TenantId].Int64()
			if _, err = pdao.YoubanPublishAccount.Ctx(ctx).Where(cols.Id, id).Where(cols.TenantId, ownerTenant).Data(g.Map{cols.DeletedBy: operator.Id, cols.DeletedAt: gtime.Now()}).Update(); err != nil {
				return gerror.Wrap(err, "删除上架账户失败")
			}
			affectedTenants[ownerTenant] = true
			receiver := row[cols.ParentId].Int64()
			if row[cols.AccountType].String() == sysin.PublishAccountTypeAdmin {
				receiver = id
			}
			if receiver <= 0 {
				continue
			}
			// Never send an event to a stale parent reference in another tenant.
			parent, err := pdao.YoubanPublishAccount.Ctx(ctx).Unscoped().Where(cols.Id, receiver).Where(cols.TenantId, ownerTenant).Where(cols.AccountType, sysin.PublishAccountTypeAdmin).One()
			if err != nil {
				return err
			}
			if parent.IsEmpty() {
				continue
			}
			text := accountDeletedNoticeText(id, row[cols.Username].String(), row[cols.Nickname].String(), operator)
			notice := do.YoubanPublishAccountNotice{TenantId: ownerTenant, AccountId: receiver, EventKey: fmt.Sprintf("account-delete:%d:%s", id, operation), Category: "account", TextContent: text}
			if _, err = pdao.YoubanPublishAccountNotice.Ctx(ctx).Data(notice).Insert(); err != nil {
				return gerror.Wrap(err, "保存账户删除通知失败")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for tenant := range affectedTenants {
		_ = bumpAccountVisibilityVersion(ctx, tenant)
	}
	return nil
}

func accountDeletedNoticeText(id int64, username, nickname string, operator *sysin.AccountModel) string {
	name := strings.TrimSpace(nickname)
	if name == "" {
		name = username
	}
	actor := strings.TrimSpace(operator.Nickname)
	if actor == "" {
		actor = operator.Username
	}
	return fmt.Sprintf("<b>上架账户已删除</b>\n\n账户：%s\n用户名：<code>%s</code>\n账户 ID：%d\n操作人：%s（ID：%d）\n\n该账户已无法登录。如非本人操作，请及时联系管理员核实。", noticeName(name), noticeName(username), id, noticeName(actor), operator.Id)
}
func noticeName(value string) string {
	runes := []rune(value)
	if len(runes) > 150 {
		value = string(runes[:150]) + "…"
	}
	return html.EscapeString(value)
}
func accountNoticeDue() string {
	return "((status IN ('pending','failed') AND (next_retry_at IS NULL OR next_retry_at<=?)) OR (status='sending' AND lease_until<=?))"
}

func (s *sSysPublish) runAccountNotices(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			call, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := s.processAccountNotices(call, time.Now(), 20)
			cancel()
			if err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "处理账户通知失败 err:%+v", err)
			}
		}
	}
}
func (s *sSysPublish) processAccountNotices(ctx context.Context, now time.Time, limit int) error {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cols := pdao.YoubanPublishAccountNotice.Columns()
	var rows []*entity.YoubanPublishAccountNotice
	err := pdao.YoubanPublishAccountNotice.Ctx(ctx).Where(accountNoticeDue(), now, now).WhereLT(cols.Attempts, 5).WhereGTE(cols.CreatedAt, now.AddDate(0, 0, -30)).OrderAsc(cols.Id).Limit(limit).Scan(&rows)
	if err != nil {
		return err
	}
	var firstError error
	for _, row := range rows {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = s.deliverAccountNotice(ctx, row, now); err != nil {
			g.Log().Warningf(ctx, "账户通知投递失败 noticeId:%d tenantId:%d accountId:%d err:%+v", row.Id, row.TenantId, row.AccountId, err)
			if firstError == nil {
				firstError = err
			}
		}
	}
	// Terminal and abandoned notices have a finite retention window.
	if _, err = pdao.YoubanPublishAccountNotice.Ctx(ctx).WhereLT(cols.CreatedAt, now.AddDate(0, 0, -30)).Delete(); err != nil {
		return err
	}
	return firstError
}
func (s *sSysPublish) deliverAccountNotice(ctx context.Context, row *entity.YoubanPublishAccountNotice, now time.Time) error {
	cols := pdao.YoubanPublishAccountNotice.Columns()
	token := guid.S()
	result, err := pdao.YoubanPublishAccountNotice.Ctx(ctx).Where(cols.Id, row.Id).Where(accountNoticeDue(), now, now).WhereLT(cols.Attempts, 5).Data(do.YoubanPublishAccountNotice{Status: "sending", LeaseToken: token, LeaseUntil: gtime.New(now.Add(2 * time.Minute))}).Update()
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	if err = pdao.YoubanPublishAccountNotice.Ctx(ctx).Where(cols.Id, row.Id).Where(cols.LeaseToken, token).Scan(row); err != nil {
		return err
	}
	enabled, err := s.accountNotificationEnabled(ctx, row.TenantId, row.AccountId, row.Category)
	if err == nil && !enabled {
		return s.finishAccountNotice(ctx, row.Id, token, "skipped", "")
	}
	if err == nil {
		// Resolve the tenant before using the Bot binding. A moved account must not
		// receive another tenant's pending event. Deleted admins may receive their
		// own deletion warning through their existing binding.
		accountCols := pdao.YoubanPublishAccount.Columns()
		target, targetErr := pdao.YoubanPublishAccount.Ctx(ctx).Unscoped().Where(accountCols.Id, row.AccountId).Where(accountCols.TenantId, row.TenantId).Where(accountCols.AccountType, sysin.PublishAccountTypeAdmin).One()
		err = targetErr
		if err == nil && target.IsEmpty() {
			return s.finishAccountNotice(ctx, row.Id, token, "skipped", "账户归属已改变")
		}
	}
	if err == nil {
		err = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{NoticeId: row.Id, App: "api", AccountId: row.AccountId, Text: row.TextContent, ParseMode: "HTML", BotStrategy: "bound", IgnoreFeatureSwitch: true, RequireDelivery: true})
	}
	if err == nil {
		return s.finishAccountNotice(ctx, row.Id, token, "sent", "")
	}
	cleanup := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		cleanup, cancel = context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
	}
	_, persistErr := pdao.YoubanPublishAccountNotice.Ctx(cleanup).Where(cols.Id, row.Id).Where(cols.LeaseToken, token).Where(cols.Status, "sending").Data(g.Map{cols.Status: "failed", cols.Attempts: row.Attempts + 1, cols.NextRetryAt: now.Add(time.Duration(row.Attempts+1) * time.Minute), cols.LeaseUntil: nil, cols.ErrorMessage: boundDailyReportError(err.Error())}).Update()
	if persistErr != nil {
		return gerror.Wrap(persistErr, "保存账户通知重试状态失败")
	}
	return err
}
func (s *sSysPublish) finishAccountNotice(ctx context.Context, id int64, token, status, message string) error {
	cols := pdao.YoubanPublishAccountNotice.Columns()
	_, err := pdao.YoubanPublishAccountNotice.Ctx(ctx).Where(cols.Id, id).Where(cols.LeaseToken, token).Where(cols.Status, "sending").Data(g.Map{cols.Status: status, cols.LeaseUntil: nil, cols.NextRetryAt: nil, cols.ErrorMessage: message}).Update()
	return err
}

// Persist only when the current publish operation actually transitions to failed.
// The caller owns the state transaction; notification failure rolls back together
// with the projection so a later retry can safely repeat the transition.
func (s *sSysPublish) enqueueProfileFailureNoticeTx(ctx context.Context, tx gdb.TX, tenantID, accountID, profileID int64, operationNo string) error {
	c := pdao.YoubanPublishAccount.Columns()
	account, err := tx.Model(pdao.YoubanPublishAccount.Table()).Ctx(ctx).Where(c.Id, accountID).Where(c.TenantId, tenantID).One()
	if err != nil {
		return gerror.Wrap(err, "读取失败任务所属账户失败")
	}
	if account.IsEmpty() {
		return nil
	}
	receiver := accountID
	if account[c.AccountType].String() != sysin.PublishAccountTypeAdmin {
		receiver = account[c.ParentId].Int64()
	}
	if receiver <= 0 {
		return nil
	}
	admin, err := tx.Model(pdao.YoubanPublishAccount.Table()).Ctx(ctx).Where(c.Id, receiver).Where(c.TenantId, tenantID).Where(c.AccountType, sysin.PublishAccountTypeAdmin).One()
	if err != nil {
		return gerror.Wrap(err, "读取任务失败通知接收者失败")
	}
	if admin.IsEmpty() {
		return nil
	}
	digest := sha256.Sum256([]byte(operationNo))
	notice := do.YoubanPublishAccountNotice{
		TenantId: tenantID, AccountId: receiver, Category: "task",
		EventKey:    fmt.Sprintf("publish-failed:%d:%d:%x", tenantID, profileID, digest[:16]),
		TextContent: fmt.Sprintf("<b>资料上架失败</b>\n\n资料 ID：%d\n所属账户：%s（ID：%d）\n操作编号：<code>%s</code>\n\n本次上架已记录为失败，请在后台发布记录查看具体原因。其他资料继续处理；此通知不会触发重新上架。", profileID, noticeName(firstNonEmpty(account[c.Nickname].String(), account[c.Username].String())), accountID, noticeName(operationNo)),
	}
	_, err = tx.Model(pdao.YoubanPublishAccountNotice.Table()).Ctx(ctx).Data(notice).OnConflict("event_key,account_id").OnDuplicate("event_key").Save()
	return gerror.Wrap(err, "保存任务失败通知失败")
}
