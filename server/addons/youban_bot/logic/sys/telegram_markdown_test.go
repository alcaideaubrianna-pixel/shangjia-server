package sys

import (
	"strings"
	"testing"
)

func TestTelegramMarkdownToHTMLSupportsMultipleAdminDomains(t *testing.T) {
	markdown := "**请选择管理后台：**\n\n- [主域名](https://admin.example.com)\n- [备用域名](https://backup.example.com)"
	actual, err := telegramMarkdownToHTML(markdown)
	if err != nil {
		t.Fatalf("telegramMarkdownToHTML() error = %v", err)
	}
	for _, expected := range []string{"<b>请选择管理后台：</b>", `href="https://admin.example.com"`, `href="https://backup.example.com"`} {
		if !strings.Contains(actual, expected) {
			t.Fatalf("telegramMarkdownToHTML() = %q, want contains %q", actual, expected)
		}
	}
}

func TestTelegramMarkdownToHTMLEscapesUnsafeHTML(t *testing.T) {
	actual, err := telegramMarkdownToHTML("<script>alert(1)</script>\n\n[后台](https://admin.example.com)")
	if err != nil {
		t.Fatalf("telegramMarkdownToHTML() error = %v", err)
	}
	if strings.Contains(actual, "<script") {
		t.Fatalf("telegramMarkdownToHTML() kept unsafe HTML: %q", actual)
	}
}
