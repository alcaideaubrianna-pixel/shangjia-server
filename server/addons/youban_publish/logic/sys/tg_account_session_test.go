package sys

import (
	"errors"
	"testing"
)

func TestTelegramAuthKeyDuplicatedIsPermanent(t *testing.T) {
	err := errors.New("rpc error code 406: AUTH_KEY_DUPLICATED")
	if !isTelegramPermanentAccountAuthError(err) {
		t.Fatal("AUTH_KEY_DUPLICATED should expire the TG account session")
	}
	message := telegramPermanentAccountAuthMessage(err)
	if message != "TG账号授权密钥被重复使用，Telegram 已作废该登录态，请重新扫码登录" {
		t.Fatalf("unexpected auth message: %q", message)
	}
}

func TestTelegramJobChannelTableSoftDeleteSupport(t *testing.T) {
	tests := []struct {
		name           string
		operationNo    string
		wantTable      string
		wantSoftDelete bool
	}{
		{name: "regular publish", operationNo: "publish:1", wantTable: publishChannelTable, wantSoftDelete: true},
		{name: "message push", operationNo: "message_push:1", wantTable: publishTgChannelTable, wantSoftDelete: false},
		{name: "message push plan", operationNo: "message_push_plan:1", wantTable: publishTgChannelTable, wantSoftDelete: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, hasSoftDelete := telegramJobChannelTable(tt.operationNo)
			if table != tt.wantTable || hasSoftDelete != tt.wantSoftDelete {
				t.Fatalf("got table=%q softDelete=%v, want table=%q softDelete=%v", table, hasSoftDelete, tt.wantTable, tt.wantSoftDelete)
			}
		})
	}
}
