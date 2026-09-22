package sys

import "testing"

func TestTelegramLocalFileURL(t *testing.T) {
	got, err := telegramLocalFileURL(
		"/var/lib/telegram-bot-api/123:token/videos/file name.mp4",
		"http://10.3.4.11:18083/",
	)
	if err != nil {
		t.Fatalf("telegramLocalFileURL() error = %v", err)
	}
	want := "http://10.3.4.11:18083/123:token/videos/file%20name.mp4"
	if got != want {
		t.Fatalf("telegramLocalFileURL() = %q, want %q", got, want)
	}
}

func TestTelegramLocalFileURLRejectsUnsupportedPath(t *testing.T) {
	if _, err := telegramLocalFileURL("/tmp/file.mp4", "http://10.3.4.11:18083"); err == nil {
		t.Fatal("telegramLocalFileURL() error = nil")
	}
}
