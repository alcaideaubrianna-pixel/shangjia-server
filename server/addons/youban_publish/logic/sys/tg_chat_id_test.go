package sys

import "testing"

func TestNormalizeTelegramChannelChatID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"123456789012", "-100123456789012"},
		{"-100123456789012", "-100123456789012"},
		{"12345", "12345"},
		{"@example", "@example"},
	}
	for _, tt := range tests {
		if got := normalizeTelegramChannelChatID(tt.in); got != tt.want {
			t.Fatalf("normalizeTelegramChannelChatID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTelegramChannelLookupIDsAcceptBothForms(t *testing.T) {
	for _, input := range []string{"123456789012", "-100123456789012"} {
		got := tgChannelCacheLookupIds(input)
		if len(got) != 2 || !containsChatID(got, "123456789012") || !containsChatID(got, "-100123456789012") {
			t.Fatalf("tgChannelCacheLookupIds(%q) = %#v", input, got)
		}
		if canonicalCollectSourceChatID(input) != "123456789012" {
			t.Fatalf("canonicalCollectSourceChatID(%q) = %q", input, canonicalCollectSourceChatID(input))
		}
	}
}

func containsChatID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
