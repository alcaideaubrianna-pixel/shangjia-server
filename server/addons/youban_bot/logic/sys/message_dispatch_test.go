package sys

import "testing"

func TestBotListenerBindCode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "exact", text: "OXOEABLO", want: "OXOEABLO"},
		{name: "lowercase", text: "oxoeablo", want: "OXOEABLO"},
		{name: "surrounded", text: "绑定 OXOEABLO 到群聊", want: "OXOEABLO"},
		{name: "profile number", text: "G35535", want: ""},
		{name: "short suffix", text: "OX12345", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := botListenerBindCode(tt.text); got != tt.want {
				t.Fatalf("botListenerBindCode(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestListenerBindCodeIsNotProfileCommand(t *testing.T) {
	if looksLikeProfileCommand("OXOEABLO") {
		t.Fatal("listener bind code must not be handled as profile command")
	}
}

func TestLooksLikeProfileSearchIdentifier(t *testing.T) {
	for _, value := range []string{"M48574", "m48574", "编号：M48574", "001", "天空001"} {
		if !looksLikeProfileSearchIdentifier(value) {
			t.Fatalf("%q should be recognized as profile search identifier", value)
		}
	}
	for _, value := range []string{"", "普通关键词", "12"} {
		if looksLikeProfileSearchIdentifier(value) {
			t.Fatalf("%q should not be recognized as profile search identifier", value)
		}
	}
}

func TestListenerBindHandlerRunsBeforeProfileHandler(t *testing.T) {
	bindIndex := -1
	profileIndex := -1
	for index, handler := range botMessageHandlers {
		switch handler.(type) {
		case publishListenerBindMessageHandler:
			bindIndex = index
		case profileManageMessageHandler:
			profileIndex = index
		}
	}
	if bindIndex < 0 || profileIndex < 0 {
		t.Fatalf("message handlers missing: bind=%d profile=%d", bindIndex, profileIndex)
	}
	if bindIndex >= profileIndex {
		t.Fatalf("listener bind handler must run before profile handler: bind=%d profile=%d", bindIndex, profileIndex)
	}
}

func TestBotAllowedUpdatesMatch(t *testing.T) {
	allowed := botAllowedUpdates()
	if !botAllowedUpdatesMatch(allowed) {
		t.Fatal("configured updates should match")
	}
	reversed := append([]string(nil), allowed...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	if !botAllowedUpdatesMatch(reversed) {
		t.Fatal("update order should not affect matching")
	}
	if botAllowedUpdatesMatch(allowed[:len(allowed)-1]) {
		t.Fatal("missing update should not match")
	}
	withDuplicate := append(append([]string(nil), allowed...), allowed[0])
	if botAllowedUpdatesMatch(withDuplicate) {
		t.Fatal("duplicate update should not match")
	}
}
