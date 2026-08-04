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
