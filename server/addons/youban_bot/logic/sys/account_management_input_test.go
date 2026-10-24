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
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,account_id bigint,tenant_id bigint,scene text,status text,expires_at timestamp,updated_at timestamp)`)
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
		_, err = db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,99,1,'account_reset','active',?,NULL)`, id, gtime.Now().Add(time.Hour))
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
