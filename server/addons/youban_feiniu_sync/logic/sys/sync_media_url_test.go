package sys

import "testing"

func TestNormalizeTelegramContentStoragePathLocal(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "content path", raw: "telegram/content/prod/persistent/image/a.jpg", want: "telegram/content/prod/persistent/image/a.jpg"},
		{name: "content URL", raw: "https://img.yuebanby.com/telegram/content/prod/persistent/image/a.jpg", want: "telegram/content/prod/persistent/image/a.jpg"},
		{name: "resource URL rejected", raw: "https://img.yuebanby.com/telegram/resource/5938241648133344942?kind=origin", want: ""},
		{name: "local path rejected", raw: "vf_admin/upload_path/telegram_lazy_media/1/file.jpg", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTelegramContentStoragePathLocal(test.raw); got != test.want {
				t.Fatalf("normalizeTelegramContentStoragePathLocal(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}
