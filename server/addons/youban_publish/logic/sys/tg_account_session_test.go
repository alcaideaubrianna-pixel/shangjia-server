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
