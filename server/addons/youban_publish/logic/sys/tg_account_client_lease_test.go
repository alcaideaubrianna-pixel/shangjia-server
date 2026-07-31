package sys

import "testing"

func TestTelegramAccountClientLeaseKey(t *testing.T) {
	if got, want := telegramAccountClientLeaseKey(12), "youban_publish:tg:account-client:12"; got != want {
		t.Fatalf("lease key = %q, want %q", got, want)
	}
	if telegramAccountClientLeaseKey(12) == telegramAccountClientLeaseKey(13) {
		t.Fatal("different Telegram accounts must not share a client lease")
	}
}
