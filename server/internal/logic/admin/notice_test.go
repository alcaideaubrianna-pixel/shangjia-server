package admin

import "testing"

func TestNoticeTelegramText(t *testing.T) {
	got := noticeTelegramText("会员通知", "<p>会员已开通</p><p>有效期 30 天</p>")
	want := "<b>会员通知</b>\n\n会员已开通\n有效期 30 天"
	if got != want {
		t.Fatalf("noticeTelegramText() = %q, want %q", got, want)
	}
}

func TestNoticeTelegramTextEscapesHTML(t *testing.T) {
	got := noticeTelegramText("<通知>", "Tom & Jerry")
	want := "<b>&lt;通知&gt;</b>\n\nTom &amp; Jerry"
	if got != want {
		t.Fatalf("noticeTelegramText() = %q, want %q", got, want)
	}
}
