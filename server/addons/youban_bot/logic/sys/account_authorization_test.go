package sys

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type accountAuthorizationTestService struct {
	ps.ISysPublish
	fail   bool
	tenant int64
	calls  int
}

func (s *accountAuthorizationTestService) AccountCapability(context.Context, string, int64) (*pi.AccountCapabilityModel, error) {
	return &pi.AccountCapabilityModel{TenantId: s.tenant, AccountType: "admin"}, nil
}
func (s *accountAuthorizationTestService) AdminAccountSave(ctx context.Context, in *pi.AccountSaveInp) (*pi.AccountSaveModel, error) {
	s.calls++
	if in.TelegramUserId != "11" || in.Username != "recipient" || in.AccountType != "uploader" || in.ParentId != 0 {
		return nil, fmt.Errorf("invalid creation input %+v", in)
	}
	_, err := g.DB().Exec(ctx, `INSERT INTO authorization_created(id) VALUES(123)`)
	if err != nil {
		return nil, err
	}
	return &pi.AccountSaveModel{Id: 123, Password: "secret-must-not-be-persisted"}, nil
}
func (s *accountAuthorizationTestService) PatchAccountSetting(ctx context.Context, in *pi.AccountSettingPatchInp) (*pi.AccountSettingModel, error) {
	if in.AccountId != 123 || in.Field != "telegramBindingEnabled" || in.Value != "1" {
		return nil, fmt.Errorf("wrong setting patch")
	}
	if s.fail {
		return nil, fmt.Errorf("injected grant failure")
	}
	return &pi.AccountSettingModel{AccountId: 123, TelegramBindingEnabled: 1}, nil
}

func TestAccountAuthorizationConsumption(t *testing.T) {
	for _, scenario := range []string{"success", "wrong_user", "wrong_bot", "wrong_chat", "expired_share", "revoked_share", "wrong_tenant", "bound_user", "setting_failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := relayTestDatabase(t)
			db, _ := gdb.Instance()
			for _, table := range []string{profileSessionTable, accountBindTbl, "authorization_created"} {
				if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
					t.Fatal(err)
				}
			}
			for _, sql := range []string{
				`CREATE TABLE authorization_created(id bigint primary key)`,
				`CREATE TABLE hg_youban_bot_profile_session(id bigint primary key,bot_id bigint,telegram_user_id text,chat_id text,account_id bigint,tenant_id bigint,scene text,step text,profile_id bigint,profile_no text,status text,payload_json text,expires_at timestamp,updated_at timestamp)`,
				`CREATE TABLE hg_youban_bot_account_bind(id integer primary key autoincrement,app text,account_id bigint,telegram_user_id text,telegram_username text,telegram_first_name text,telegram_last_name text,bot_id bigint,status int,created_at timestamp,updated_at timestamp,deleted_at timestamp)`,
				`CREATE UNIQUE INDEX authorization_bind_user ON hg_youban_bot_account_bind(app,telegram_user_id) WHERE status=1 AND deleted_at IS NULL`,
			} {
				if _, err := db.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
				t.Fatal(err)
			}
			expiry := gtime.Now().Add(time.Hour)
			shareStatus := accountShareStatusAvailable
			if scenario == "expired_share" {
				expiry = gtime.Now().Add(-time.Hour)
			}
			if scenario == "revoked_share" {
				shareStatus = "canceled"
			}
			if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(1,7,'99','99',99,1,'account_share',?,'',?)`, shareStatus, expiry); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(2,7,'11','11',99,1,'account_authorize','active','{"shareId":1,"username":"recipient"}',?)`, gtime.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if scenario == "bound_user" {
				if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_account_bind(app,account_id,telegram_user_id,status)VALUES('admin',456,'11',1)`); err != nil {
					t.Fatal(err)
				}
			}
			previous := managerPreviousPublish()
			fake := &accountAuthorizationTestService{ISysPublish: previous, tenant: 1, fail: scenario == "setting_failure"}
			if scenario == "wrong_tenant" {
				fake.tenant = 2
			}
			ps.RegisterSysPublish(fake)
			t.Cleanup(func() { ps.RegisterSysPublish(previous) })
			user := models.User{ID: 11, Username: "recipient"}
			botID := int64(7)
			chatID := "11"
			if scenario == "wrong_user" {
				user.ID = 12
			}
			if scenario == "wrong_bot" {
				botID = 8
			}
			if scenario == "wrong_chat" {
				chatID = "12"
			}
			s := &sSysBot{}
			session := &profileSessionRow{Id: 2}
			result, err := s.consumeAccountAuthorization(ctx, botID, user, chatID, session)
			if scenario == "success" {
				if err != nil || result == nil {
					t.Fatalf("authorize: %+v %v", result, err)
				}
				again, err := s.consumeAccountAuthorization(ctx, botID, user, chatID, session)
				if err != nil || again != nil || fake.calls != 1 {
					t.Fatalf("duplicate created account: %+v %v", again, err)
				}
				bind, err := db.GetOne(ctx, `SELECT account_id FROM hg_youban_bot_account_bind WHERE telegram_user_id='11'`)
				if err != nil || bind["account_id"].Int64() != 123 {
					t.Fatal("binding not persisted", bind, err)
				}
				rows, err := db.GetAll(ctx, `SELECT payload_json,status FROM hg_youban_bot_profile_session`)
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					if row["status"].String() != "completed" || strings.Contains(row["payload_json"].String(), "secret-must-not-be-persisted") || !strings.Contains(row["payload_json"].String(), "123") {
						t.Fatal("invalid authorization audit", row)
					}
				}
			} else {
				if err == nil && result != nil {
					t.Fatal("invalid authorization succeeded")
				}
				count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM authorization_created`)
				if err != nil || count != 0 {
					t.Fatalf("failed authorization left account: %d %v", count, err)
				}
				row, err := db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE id=2`)
				if err != nil || row["status"].String() != "active" {
					t.Fatal("failure consumed recipient session", row, err)
				}
			}
		})
	}
}

func TestAccountAuthorizationShareLifecycle(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	for _, table := range []string{profileSessionTable, accountBindTbl} {
		if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{
		`CREATE TABLE hg_youban_bot_profile_session(id integer primary key autoincrement,bot_id bigint,telegram_user_id text,chat_id text,app text,account_id bigint,tenant_id bigint,scene text,step text,profile_no text,status text,expires_at timestamp,created_at timestamp,updated_at timestamp)`,
		`CREATE TABLE hg_youban_bot_account_bind(id integer primary key,app text,account_id bigint,telegram_user_id text,status int,updated_at timestamp,deleted_at timestamp)`,
		`INSERT INTO hg_youban_bot_account_bind VALUES(1,'api',99,'99',1,NULL,NULL)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	account := &botProfileAccount{AccountId: 99, TenantId: 1}
	s := &sSysBot{}
	first := accountAuthorizationDigest("first-link")
	second := accountAuthorizationDigest("second-link")
	if len(first) != 32 || first == second {
		t.Fatal("invalid token digest")
	}
	if err := s.createAccountAuthorizationShare(ctx, 7, "99", "99", account, first); err != nil {
		t.Fatal(err)
	}
	// Canceling ordinary user input leaves the separately scoped share available.
	if err := s.cancelProfileSessionByIdsSilent(ctx, 7, "99", "99"); err != nil {
		t.Fatal(err)
	}
	row, err := db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE profile_no=?`, first)
	if err != nil || row["status"].String() != accountShareStatusAvailable {
		t.Fatal("navigation revoked share", row, err)
	}
	if err = s.createAccountAuthorizationShare(ctx, 7, "99", "99", account, second); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE profile_no=?`, first)
	if err != nil || row["status"].String() != "canceled" {
		t.Fatal("new link did not revoke old link", row, err)
	}
	if err = s.revokeAccountAuthorizationShares(ctx, 7, "99", &botProfileAccount{AccountId: 99, TenantId: 2}); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE profile_no=?`, second)
	if err != nil || row["status"].String() != accountShareStatusAvailable {
		t.Fatal("cross tenant revocation", row, err)
	}
	if err = s.revokeAccountAuthorizationShares(ctx, 7, "99", account); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT status FROM hg_youban_bot_profile_session WHERE profile_no=?`, second)
	if err != nil || row["status"].String() != "canceled" {
		t.Fatal("explicit revoke failed", row, err)
	}
	if err = s.createAccountAuthorizationShare(ctx, 7, "another-user", "99", account, first); err == nil {
		t.Fatal("changed binding permitted issuance")
	}
}

func TestAccountManagementSessionRetention(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(ctx) })
	for _, sql := range []string{
		"DROP TABLE IF EXISTS " + profileSessionTable,
		`CREATE TABLE hg_youban_bot_profile_session(id integer primary key,scene text,status text,expires_at timestamp)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for id := 1; id <= 205; id++ {
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,'account_share','available',?)`, id, now.AddDate(0, 0, -31)); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id            int
		scene, status string
		expires       time.Time
	}{
		{301, "account_authorize", "completed", now.AddDate(0, 0, -1)},
		{302, "account_share", "available", now.Add(time.Hour)},
		{303, "create", "active", now.AddDate(0, 0, -90)},
		{304, "account_renewal", "completed", now.AddDate(0, 0, -31)},
		{305, "account_managed_reset", "completed", now.AddDate(0, 0, -31)},
		{306, "account_reset", "active", now.AddDate(0, 0, -31)},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session VALUES(?,?,?,?)`, row.id, row.scene, row.status, row.expires); err != nil {
			t.Fatal(err)
		}
	}
	service := &sSysBot{}
	if err := service.cleanupAccountManagementSessions(ctx, now); err != nil {
		t.Fatal(err)
	}
	count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_bot_profile_session`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 11 {
		t.Fatalf("cleanup not bounded to 200 rows: %d", count)
	}
	if err := service.cleanupAccountManagementSessions(ctx, now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetAll(ctx, `SELECT id FROM hg_youban_bot_profile_session ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0]["id"].Int() != 301 || rows[1]["id"].Int() != 302 || rows[2]["id"].Int() != 303 {
		t.Fatalf("cleanup touched fresh audit/live link/media draft: %v", rows)
	}
}
