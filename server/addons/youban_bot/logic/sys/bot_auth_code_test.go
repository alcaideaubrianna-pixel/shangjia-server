package sys

import (
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestAuthCodeRowStatusModel(t *testing.T) {
	expiresAt := gtime.New("2026-08-03 19:27:03")
	row := &authCodeRow{
		Id:               21,
		Code:             "951510",
		Scene:            "login",
		App:              "api",
		AccountId:        1,
		TelegramUserId:   "8379260686",
		TelegramUsername: "Durovdse",
		LoginToken:       "login-token",
		Status:           "authorized",
		ExpiresAt:        expiresAt,
	}

	got := row.statusModel()
	if got.AccountId != row.AccountId {
		t.Fatalf("AccountId=%d, want %d", got.AccountId, row.AccountId)
	}
	if got.AccountId == row.Id {
		t.Fatalf("AccountId incorrectly used auth code row id %d", row.Id)
	}
	if got.Token != row.LoginToken {
		t.Fatalf("Token=%q, want %q", got.Token, row.LoginToken)
	}
	if got.ExpiresAt != expiresAt {
		t.Fatalf("ExpiresAt=%v, want %v", got.ExpiresAt, expiresAt)
	}
}
