package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	_ "hotgo/addons/youban_bot/logic/sys"
	"hotgo/addons/youban_publish/model/input/sysin"
	_ "hotgo/internal/logic/sys"
	"hotgo/internal/model"
)

func TestInviteConcurrentRegisterIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	code := integrationInviteCode()
	prefix := strings.ToLower(code)
	insertIntegrationInvite(t, ctx, code)
	defer deleteIntegrationRegisterData(t, ctx, code, prefix)

	publish := &sSysPublish{}
	start := make(chan struct{})
	type registerResult struct {
		data *accountRegisterTxResult
		err  error
	}
	resultsCh := make(chan registerResult, 2)
	var waitGroup sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			input := &sysin.AccountRegisterInp{
				Username:   fmt.Sprintf("%s_%d", prefix, index),
				Password:   "InviteTest123!",
				Name:       fmt.Sprintf("%s_%d", prefix, index),
				InviteCode: code,
			}
			if err := input.Filter(ctx); err != nil {
				resultsCh <- registerResult{err: err}
				return
			}
			data, err := publish.registerAccountWithInvite(ctx, input)
			resultsCh <- registerResult{data: data, err: err}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(resultsCh)

	successCount := 0
	failureCount := 0
	var success *accountRegisterTxResult
	for result := range resultsCh {
		if result.err == nil {
			successCount++
			success = result.data
			continue
		}
		if !strings.Contains(result.err.Error(), "邀请码已使用或已失效") {
			t.Fatalf("unexpected concurrent register error: %+v", result.err)
		}
		failureCount++
	}
	if successCount != 1 || failureCount != 1 {
		t.Fatalf("success=%d failure=%d, want 1/1", successCount, failureCount)
	}
	if success == nil || success.Tenant == nil || success.Tenant.Id <= 0 || success.AccountId <= 0 {
		t.Fatalf("successful registration result is incomplete: %+v", success)
	}
	var inviteRelation struct {
		Status        string `json:"status"`
		UsedTenantId  int64  `json:"used_tenant_id"`
		UsedAccountId int64  `json:"used_account_id"`
	}
	if err := g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx).Where("code", code).Scan(&inviteRelation); err != nil {
		t.Fatalf("read consumed invite: %+v", err)
	}
	if inviteRelation.Status != registerInviteStatusUsed || inviteRelation.UsedTenantId != success.Tenant.Id || inviteRelation.UsedAccountId != success.AccountId {
		t.Fatalf("invite relation = %+v, registration = %+v", inviteRelation, success)
	}
}

func TestInviteConsumeRollbackIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	code := integrationInviteCode()
	insertIntegrationInvite(t, ctx, code)
	defer deleteIntegrationInvite(t, ctx, code)

	publish := &sSysPublish{}
	rollbackErr := errors.New("rollback invite integration test")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		invite, err := publish.validateRegisterInviteCodeTx(ctx, tx, code, true)
		if err != nil {
			return err
		}
		if err = publish.markRegisterInviteUsedTx(ctx, tx, invite.Id, 900010, 910010, "integration_rollback"); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("transaction error = %+v", err)
	}
	invite, err := publish.validateRegisterInviteCode(ctx, code)
	if err != nil {
		t.Fatalf("invite should remain active after rollback: %+v", err)
	}
	if invite.Status != registerInviteStatusActive {
		t.Fatalf("invite status = %s, want active", invite.Status)
	}
}

func TestInstantRegisterAutoBindsTelegramIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	code := integrationInviteCode()
	prefix := strings.ToLower(code)
	telegramUserId := fmt.Sprintf("self_%d", time.Now().UnixNano())
	now := gtime.Now()
	_, err := g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx).Data(g.Map{
		"code": code, "source": registerInviteSourceSelf, "inviter_app": registerBindAppApi,
		"registration_telegram_user_id":    telegramUserId,
		"registration_telegram_username":   "instant_register",
		"registration_telegram_first_name": "Instant",
		"registration_telegram_last_name":  "Register",
		"registration_bot_id":              1,
		"status":                           registerInviteStatusActive, "expires_at": now.Add(10 * time.Minute), "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		t.Fatalf("insert instant register invite: %+v", err)
	}
	defer func() {
		_, _ = g.DB().Exec(ctx, "DELETE FROM "+botAccountBindTable+" WHERE telegram_user_id=?", telegramUserId)
		deleteIntegrationRegisterData(t, ctx, code, prefix)
	}()

	registration, err := (&sSysPublish{}).registerAccountWithInvite(ctx, &sysin.AccountRegisterInp{
		Username: prefix + "_self", Password: "InviteTest123!", Name: prefix + "_self", InviteCode: code,
	})
	if err != nil {
		t.Fatalf("instant register: %+v", err)
	}
	if registration.Binding == nil || registration.Binding.AccountId != registration.AccountId || registration.Binding.TelegramUserId != telegramUserId {
		t.Fatalf("binding event=%+v registration=%+v", registration.Binding, registration)
	}
	var binding struct {
		AccountId      int64  `json:"account_id"`
		TelegramUserId string `json:"telegram_user_id"`
		Status         int    `json:"status"`
	}
	if err = g.DB().Model(botAccountBindTable).Safe().Ctx(ctx).
		Where("app", registerBindAppApi).Where("telegram_user_id", telegramUserId).Scan(&binding); err != nil {
		t.Fatalf("read instant register binding: %+v", err)
	}
	if binding.AccountId != registration.AccountId || binding.TelegramUserId != telegramUserId || binding.Status != 1 {
		t.Fatalf("binding=%+v registration=%+v", binding, registration)
	}
}

func TestInviteRewardUsesFirstTelegramBindTimeIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	accountId := time.Now().UnixNano()%1_000_000_000 + 8_000_000_000
	telegramUserId := fmt.Sprintf("integration_%d", accountId)
	createdAt := gtime.NewFromStr("2026-07-01 10:00:00")
	updatedAt := gtime.NewFromStr("2026-08-01 10:00:00")
	_, err := g.DB().Model("hg_youban_bot_account_bind").Safe().Ctx(ctx).Data(g.Map{
		"app":                 "api",
		"account_id":          accountId,
		"telegram_user_id":    telegramUserId,
		"telegram_username":   telegramUserId,
		"telegram_first_name": "integration",
		"status":              1,
		"created_at":          createdAt,
		"updated_at":          updatedAt,
	}).Insert()
	if err != nil {
		t.Fatalf("insert Telegram binding fixture: %+v", err)
	}
	defer func() {
		if _, deleteErr := g.DB().Exec(ctx, "DELETE FROM hg_youban_bot_account_bind WHERE app='api' AND account_id=?", accountId); deleteErr != nil {
			t.Errorf("delete Telegram binding fixture: %+v", deleteErr)
		}
	}()

	boundAt, err := (&sSysPublish{}).tenantVipAccountFirstBoundAt(ctx, accountId, updatedAt)
	if err != nil {
		t.Fatalf("read first Telegram bind time: %+v", err)
	}
	if boundAt == nil || boundAt.Format("Y-m-d H:i:s") != "2026-07-01 10:00:00" {
		t.Fatalf("first bound time = %v, want 2026-07-01 10:00:00", boundAt)
	}
}

func TestInviteBindEligibilityUsesFirstBindTimeIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	code := integrationInviteCode()
	accountId := time.Now().UnixNano()%1_000_000_000 + 7_000_000_000
	inviterTenantId := accountId + 1
	usedTenantId := accountId + 2
	username := strings.ToLower(code)
	telegramUserId := fmt.Sprintf("eligibility_%d", accountId)
	createdAt := gtime.NewFromStr("2026-07-01 10:00:00")
	updatedAt := gtime.NewFromStr("2026-08-01 10:00:00")
	_, err := g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).Data(g.Map{
		"id":           accountId,
		"tenant_id":    usedTenantId,
		"merchant_id":  usedTenantId,
		"account_type": "admin",
		"username":     username,
		"nickname":     username,
		"status":       1,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	}).Insert()
	if err != nil {
		t.Fatalf("insert eligibility account: %+v", err)
	}
	defer func() {
		_, _ = g.DB().Exec(ctx, "DELETE FROM hg_youban_bot_account_bind WHERE app='api' AND account_id=?", accountId)
		_, _ = g.DB().Exec(ctx, "DELETE FROM hg_youban_bot_invite_code WHERE code=?", code)
		_, _ = g.DB().Exec(ctx, "DELETE FROM hg_youban_publish_account WHERE id=?", accountId)
	}()
	_, err = g.DB().Model("hg_youban_bot_account_bind").Safe().Ctx(ctx).Data(g.Map{
		"app":                 "api",
		"account_id":          accountId,
		"telegram_user_id":    telegramUserId,
		"telegram_username":   telegramUserId,
		"telegram_first_name": "integration",
		"status":              1,
		"created_at":          createdAt,
		"updated_at":          updatedAt,
	}).Insert()
	if err != nil {
		t.Fatalf("insert eligibility binding: %+v", err)
	}
	_, err = g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx).Data(g.Map{
		"code":               code,
		"source":             "integration",
		"inviter_app":        "api",
		"inviter_tenant_id":  inviterTenantId,
		"inviter_account_id": inviterTenantId,
		"inviter_username":   "integration",
		"inviter_nickname":   "integration",
		"used_tenant_id":     usedTenantId,
		"used_account_id":    accountId,
		"used_username":      username,
		"status":             registerInviteStatusUsed,
		"used_at":            createdAt,
		"expires_at":         updatedAt,
		"created_at":         createdAt,
		"updated_at":         updatedAt,
	}).Insert()
	if err != nil {
		t.Fatalf("insert eligibility invite: %+v", err)
	}

	handler := activityAdminHandlerByCode(tenantVipEventInviteBindGift)
	cfg := &model.YoubanPublishVipActivityConfig{InviteBindGiftEnabled: true, InviteBindGiftDays: 3, InviteBindGiftEnabledAt: "2026-07-15 00:00:00"}
	count, err := (&sSysPublish{}).activityEligibleCount(ctx, handler, cfg, inviterTenantId)
	if err != nil {
		t.Fatalf("count invite bind eligibility: %+v", err)
	}
	if count != 0 {
		t.Fatalf("rebind after activity start must not count as first bind, count=%d", count)
	}
	cfg.InviteBindGiftEnabledAt = "2026-06-15 00:00:00"
	count, err = (&sSysPublish{}).activityEligibleCount(ctx, handler, cfg, inviterTenantId)
	if err != nil {
		t.Fatalf("count eligible first bind: %+v", err)
	}
	if count != 1 {
		t.Fatalf("first bind after activity start should be eligible, count=%d", count)
	}
}

func TestInviteRewardsApplyOnceIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_INVITE_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_INVITE_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	code := integrationInviteCode()
	seed := time.Now().UnixNano()%1_000_000_000 + 6_000_000_000
	inviterTenantId := seed
	inviterAccountId := seed + 1
	invitedTenantId := seed + 2
	invitedAccountId := seed + 3
	now := gtime.Now()
	_, err := g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx).Data(g.Map{
		"code":               code,
		"source":             "integration",
		"inviter_app":        "api",
		"inviter_tenant_id":  inviterTenantId,
		"inviter_account_id": inviterAccountId,
		"inviter_username":   "integration",
		"inviter_nickname":   "integration",
		"used_tenant_id":     invitedTenantId,
		"used_account_id":    invitedAccountId,
		"used_username":      strings.ToLower(code),
		"status":             registerInviteStatusUsed,
		"used_at":            now,
		"expires_at":         now.Add(time.Hour),
		"created_at":         now,
		"updated_at":         now,
	}).Insert()
	if err != nil {
		t.Fatalf("insert reward invite fixture: %+v", err)
	}
	defer deleteIntegrationRewardData(t, ctx, code, inviterTenantId)

	publish := &sSysPublish{}
	cfg := &model.YoubanPublishVipActivityConfig{
		InviteBindGiftEnabled:   true,
		InviteBindGiftDays:      3,
		InviteBindGiftEnabledAt: now.Add(-time.Hour).Format("Y-m-d H:i:s"),
		InviteFirstPayEnabled:   true,
		InviteFirstPayDays:      30,
		InviteFirstPayEnabledAt: now.Add(-time.Hour).Format("Y-m-d H:i:s"),
	}
	account := &sysin.AccountModel{Id: invitedAccountId, TenantId: invitedTenantId}
	for index := 0; index < 2; index++ {
		if err = publish.applyTenantVipInviteBindGift(ctx, account, now, cfg); err != nil {
			t.Fatalf("apply invite bind reward: %+v", err)
		}
		if err = publish.rewardInviterVip(ctx, invitedTenantId, invitedAccountId, now, cfg); err != nil {
			t.Fatalf("apply invite first-pay reward: %+v", err)
		}
	}

	var summary struct {
		Count int `json:"count"`
		Days  int `json:"days"`
	}
	if err = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
		Fields("COUNT(*) AS count,COALESCE(SUM(change_days),0) AS days").
		Where("tenant_id", inviterTenantId).
		WhereIn("event_type", []string{tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay}).
		Scan(&summary); err != nil {
		t.Fatalf("read invite reward events: %+v", err)
	}
	if summary.Count != 2 || summary.Days != 33 {
		t.Fatalf("reward events count=%d days=%d, want count=2 days=33", summary.Count, summary.Days)
	}

	var vip struct {
		ExpiredAt *gtime.Time `json:"expired_at"`
	}
	if err = g.DB().Model("hg_youban_publish_tenant_vip").Safe().Ctx(ctx).
		Fields("expired_at").Where("tenant_id", inviterTenantId).Scan(&vip); err != nil {
		t.Fatalf("read inviter vip: %+v", err)
	}
	if vip.ExpiredAt == nil {
		t.Fatal("inviter vip expiration was not created")
	}
	wantExpiredAt := now.AddDate(0, 0, 33).Format("Y-m-d H:i:s")
	if vip.ExpiredAt.Format("Y-m-d H:i:s") != wantExpiredAt {
		t.Fatalf("inviter vip expired_at=%s, want %s", vip.ExpiredAt.Format("Y-m-d H:i:s"), wantExpiredAt)
	}
}

func integrationInviteCode() string {
	return fmt.Sprintf("IT%012d", time.Now().UnixNano()%1_000_000_000_000)
}

func insertIntegrationInvite(t *testing.T, ctx context.Context, code string) {
	t.Helper()
	now := gtime.Now()
	_, err := g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx).Data(g.Map{
		"code":               code,
		"source":             "integration",
		"inviter_app":        "api",
		"inviter_tenant_id":  999001,
		"inviter_account_id": 999001,
		"inviter_username":   "integration",
		"inviter_nickname":   "integration",
		"status":             registerInviteStatusActive,
		"expires_at":         now.Add(10 * time.Minute),
		"created_at":         now,
		"updated_at":         now,
	}).Insert()
	if err != nil {
		t.Fatalf("insert integration invite: %+v", err)
	}
}

func deleteIntegrationInvite(t *testing.T, ctx context.Context, code string) {
	t.Helper()
	if _, err := g.DB().Exec(ctx, "DELETE FROM "+botInviteCodeTable+" WHERE code=?", code); err != nil {
		t.Errorf("delete integration invite: %+v", err)
	}
}

func deleteIntegrationRegisterData(t *testing.T, ctx context.Context, code, prefix string) {
	t.Helper()
	if _, err := g.DB().Exec(ctx, "DELETE FROM hg_youban_publish_account WHERE username LIKE ?", prefix+"_%"); err != nil {
		t.Errorf("delete integration accounts: %+v", err)
	}
	if _, err := g.DB().Exec(ctx, "DELETE FROM hg_youban_publish_tenant WHERE name LIKE ?", prefix+"_%"); err != nil {
		t.Errorf("delete integration tenants: %+v", err)
	}
	deleteIntegrationInvite(t, ctx, code)
}

func deleteIntegrationRewardData(t *testing.T, ctx context.Context, code string, tenantId int64) {
	t.Helper()
	statements := []string{
		"DELETE FROM hg_youban_publish_tenant_vip_event WHERE tenant_id=?",
		"DELETE FROM hg_youban_publish_tenant_vip_log WHERE tenant_id=?",
		"DELETE FROM hg_youban_publish_tenant_vip WHERE tenant_id=?",
		"DELETE FROM hg_youban_publish_activity_generation WHERE tenant_id=?",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement, tenantId); err != nil {
			t.Errorf("delete integration reward data: %+v", err)
		}
	}
	deleteIntegrationInvite(t, ctx, code)
}
