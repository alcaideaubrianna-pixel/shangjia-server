package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"testing"
	"time"
)

func TestUploaderCreateDoesNotBindCreator(t *testing.T) {
	in := uploaderCreateInput("uploader_1")
	if in.AccountType != "uploader" || in.Status != 1 || in.Username != "uploader_1" {
		t.Fatalf("invalid creation input: %+v", in)
	}
	if in.TelegramUserId != "" || in.TelegramUsername != "" || in.Id != 0 || in.TenantId != 0 || in.ParentId != 0 {
		t.Fatalf("creation must leave identity and ownership to authorized services: %+v", in)
	}
}

func TestAccountCreationUsername(t *testing.T) {
	for _, input := range []string{"abc", "Uploader_123", "123"} {
		if !accountUsernamePattern.MatchString(input) {
			t.Errorf("valid username rejected: %q", input)
		}
	}
	for _, input := range []string{"ab", "abc def", "@name", "<b>name</b>", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		if accountUsernamePattern.MatchString(input) {
			t.Errorf("invalid username accepted: %q", input)
		}
	}
}

type accountCreationTestService struct {
	ps.ISysPublish
	calls int
	fail  bool
}

func (s *accountCreationTestService) AdminAccountSave(ctx context.Context, in *pi.AccountSaveInp) (*pi.AccountSaveModel, error) {
	s.calls++
	if in.TelegramUserId != "" || in.AccountType != "uploader" {
		return nil, fmt.Errorf("unexpected identity")
	}
	if s.fail {
		return nil, fmt.Errorf("injected save failure")
	}
	return &pi.AccountSaveModel{Id: 123, Password: "generated"}, nil
}

func TestAccountCreationSessionAtomicity(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,account_id bigint,status text,expires_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &accountCreationTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	account := &botProfileAccount{AccountId: 99, TenantId: 1, AccountType: "admin"}
	for _, id := range []int64{1, 2} {
		_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,99,'active',?,NULL)`, id, gtime.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &sSysBot{}
	session := &profileSessionRow{Id: 1, AccountId: 99}
	first, err := s.createUploaderFromSession(ctx, session, account, "new_user")
	if err != nil || first == nil {
		t.Fatalf("create: %v %v", first, err)
	}
	again, err := s.createUploaderFromSession(ctx, session, account, "new_user")
	if err != nil || again != nil || fake.calls != 1 {
		t.Fatalf("duplicate consumed session: calls=%d result=%v err=%v", fake.calls, again, err)
	}
	fake.fail = true
	_, err = s.createUploaderFromSession(ctx, &profileSessionRow{Id: 2, AccountId: 99}, account, "failed_user")
	if err == nil {
		t.Fatal("save failure lost")
	}
	row, err := db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE id=2`)
	if err != nil || row["status"].String() != "active" {
		t.Fatalf("failed save consumed session: %v %v", row, err)
	}
}

func (s *accountCreationTestService) MyAccountResetPassword(context.Context) (*pi.AccountSaveModel, error) {
	s.calls++
	if s.fail {
		return nil, fmt.Errorf("injected reset failure")
	}
	return &pi.AccountSaveModel{Id: 99, Password: "generated"}, nil
}

func TestOwnPasswordResetSessionAtomicity(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,account_id bigint,tenant_id bigint,scene text,profile_id bigint DEFAULT 0,status text,expires_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &accountCreationTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	for _, id := range []int64{1, 2, 3} {
		_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,99,1,'account_reset',0,'active',?,NULL)`, id, gtime.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &sSysBot{}
	account := &botProfileAccount{AccountId: 99, TenantId: 1, AccountType: "admin"}
	result, err := s.resetOwnAccountFromSession(ctx, &profileSessionRow{Id: 1}, account)
	if err != nil || result == nil {
		t.Fatalf("reset failed: %v %v", result, err)
	}
	result, err = s.resetOwnAccountFromSession(ctx, &profileSessionRow{Id: 1}, account)
	if err != nil || result != nil || fake.calls != 1 {
		t.Fatalf("duplicate reset calls=%d err=%v", fake.calls, err)
	}
	other := *account
	other.TenantId = 2
	result, err = s.resetOwnAccountFromSession(ctx, &profileSessionRow{Id: 3}, &other)
	if err != nil || result != nil || fake.calls != 1 {
		t.Fatal("cross tenant reset allowed")
	}
	fake.fail = true
	_, err = s.resetOwnAccountFromSession(ctx, &profileSessionRow{Id: 2}, account)
	if err == nil {
		t.Fatal("reset failure swallowed")
	}
	row, err := db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE id=2`)
	if err != nil || row["status"].String() != "active" {
		t.Fatalf("failed reset consumed confirmation: %v %v", row, err)
	}
}

func (s *accountCreationTestService) AdminAccountResetPassword(ctx context.Context, in *pi.AccountResetPasswordInp) (*pi.AccountSaveModel, error) {
	if in.Id != 123 {
		return nil, fmt.Errorf("unexpected target")
	}
	return s.MyAccountResetPassword(ctx)
}

func TestManagedPasswordResetConfirmation(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	_, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,account_id bigint,tenant_id bigint,scene text,profile_id bigint,status text,expires_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &accountCreationTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(1,99,1,'account_managed_reset',123,'active',?,NULL)`, gtime.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s := &sSysBot{}
	account := &botProfileAccount{AccountId: 99, TenantId: 1}
	session := &profileSessionRow{Id: 1, Scene: "account_managed_reset", ProfileId: 124}
	result, err := s.resetOwnAccountFromSession(ctx, session, account)
	if err != nil || result != nil || fake.calls != 0 {
		t.Fatal("modified target consumed confirmation")
	}
	session.ProfileId = 123
	result, err = s.resetOwnAccountFromSession(ctx, session, account)
	if err != nil || result == nil || fake.calls != 1 {
		t.Fatalf("reset: %v %v", result, err)
	}
	result, err = s.resetOwnAccountFromSession(ctx, session, account)
	if err != nil || result != nil || fake.calls != 1 {
		t.Fatal("duplicate confirmation reset password")
	}
}

func (s *accountCreationTestService) AdminAccountDelete(ctx context.Context, in *pi.AccountDeleteInp) error {
	if len(in.Ids) != 1 || in.Ids[0] != 123 {
		return fmt.Errorf("unexpected deletion target")
	}
	s.calls++
	if s.fail {
		return fmt.Errorf("injected deletion failure")
	}
	return nil
}

func TestManagedAccountDeletionConfirmation(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `DROP TABLE IF EXISTS hg_youban_bot_profile_session`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,account_id bigint,tenant_id bigint,scene text,profile_id bigint,status text,expires_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	fake := &accountCreationTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	for _, id := range []int64{1, 2, 3} {
		expires := gtime.Now().Add(time.Hour)
		if id == 3 {
			expires = gtime.Now().Add(-time.Hour)
		}
		_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,99,1,'account_delete',123,'active',?,NULL)`, id, expires)
		if err != nil {
			t.Fatal(err)
		}
	}
	s := &sSysBot{}
	account := &botProfileAccount{AccountId: 99, TenantId: 1}
	for _, tc := range []struct{ target, tenant, id int64 }{{124, 1, 1}, {123, 2, 1}, {123, 1, 3}} {
		err = s.deleteManagedAccountFromSession(ctx, &profileSessionRow{Id: tc.id, ProfileId: tc.target}, &botProfileAccount{AccountId: 99, TenantId: tc.tenant})
		if err != nil || fake.calls != 0 {
			t.Fatalf("invalid confirmation invoked deletion: %v", err)
		}
	}
	session := &profileSessionRow{Id: 1, ProfileId: 123}
	if err = s.deleteManagedAccountFromSession(ctx, session, account); err != nil {
		t.Fatal(err)
	}
	if err = s.deleteManagedAccountFromSession(ctx, session, account); err != nil || fake.calls != 1 {
		t.Fatalf("duplicate deletion: calls=%d %v", fake.calls, err)
	}
	fake.fail = true
	if err = s.deleteManagedAccountFromSession(ctx, &profileSessionRow{Id: 2, ProfileId: 123}, account); err == nil {
		t.Fatal("failure swallowed")
	}
	row, err := db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE id=2`)
	if err != nil || row["status"].String() != "active" {
		t.Fatalf("failed deletion consumed confirmation: %v %v", row, err)
	}
}
