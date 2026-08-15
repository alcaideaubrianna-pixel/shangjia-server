package sys

import (
	"errors"
	"testing"
	"time"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestIsInvalidTelegramMediaReference(t *testing.T) {
	if !isInvalidTelegramMediaReference(errors.New("bad request: wrong file identifier/HTTP URL specified")) {
		t.Fatal("expected invalid Telegram media reference to be detected")
	}
	if isInvalidTelegramMediaReference(errors.New("request timeout")) {
		t.Fatal("unexpectedly classified timeout as invalid media reference")
	}
}

func TestProfileMediaGroupIdleWaitUsesLastMessageTime(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{CreatedAt: now.Add(-10 * time.Minute).Unix(), UpdatedAt: now.Add(-time.Second).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != time.Second {
		t.Fatalf("unexpected idle wait: got %s, want %s", wait, time.Second)
	}
}

func TestProfileMediaGroupIdleWaitReturnsReadyAfterDebounce(t *testing.T) {
	now := time.Date(2026, 8, 9, 16, 0, 0, 0, time.Local)
	pending := profilePendingMediaGroup{UpdatedAt: now.Add(-profileMediaGroupDebounce).UnixNano()}
	if wait := profileMediaGroupIdleWait(pending, now); wait != 0 {
		t.Fatalf("expected media group to be ready, got wait %s", wait)
	}
}

func TestProfileShouldSendVerifyPrompt(t *testing.T) {
	tests := []struct {
		name   string
		step   string
		status string
		draft  *profileCreateDraft
		want   bool
	}{
		{name: "waiting for verify", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{}, want: true},
		{name: "display group pending", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{PendingGroup: &profilePendingMediaGroup{}}, want: false},
		{name: "verify already received", step: "waiting_verify", status: profileSessionStatusActive, draft: &profileCreateDraft{PendingVerify: &profilePendingMediaGroup{}}, want: false},
		{name: "still collecting display", step: "waiting_display", status: profileSessionStatusActive, draft: &profileCreateDraft{}, want: false},
		{name: "session completed", step: "waiting_verify", status: profileSessionStatusCompleted, draft: &profileCreateDraft{}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := profileShouldSendVerifyPrompt(test.step, test.status, test.draft); got != test.want {
				t.Fatalf("profileShouldSendVerifyPrompt() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestProfileGroupMediaKeepsTelegramOrderAndDropsDuplicates(t *testing.T) {
	first := &publishsysin.MessageTemplateMediaInp{MediaType: "image", TgFileId: "first"}
	second := &publishsysin.MessageTemplateMediaInp{MediaType: "video", TgFileId: "second"}
	media := appendProfileGroupMedia(nil, 102, []*publishsysin.MessageTemplateMediaInp{second})
	media = appendProfileGroupMedia(media, 101, []*publishsysin.MessageTemplateMediaInp{first})
	media = appendProfileGroupMedia(media, 101, []*publishsysin.MessageTemplateMediaInp{first})
	media = orderedProfileGroupMedia(media)
	if len(media) != 2 {
		t.Fatalf("unexpected media count: got %d, want 2", len(media))
	}
	if media[0].TgFileId != "first" || media[0].SortIndex != 1 {
		t.Fatalf("unexpected first media: %#v", media[0])
	}
	if media[1].TgFileId != "second" || media[1].SortIndex != 2 {
		t.Fatalf("unexpected second media: %#v", media[1])
	}
}

func TestProfileMediaGroupPurposeUsesFirstAndSecondGroup(t *testing.T) {
	draft := &profileCreateDraft{
		PendingGroupId: "display-group",
		PendingGroup:   &profilePendingMediaGroup{GroupId: "display-group", Purpose: "display", Text: "正文"},
	}
	if got := profileMediaGroupPurpose("waiting_display", draft, "display-group"); got != "display" {
		t.Fatalf("first group purpose = %q, want display", got)
	}
	if got := profileMediaGroupPurpose("waiting_display", draft, "verify-group"); got != "verify_pending" {
		t.Fatalf("second group purpose = %q, want verify_pending", got)
	}
	if got := profileMediaGroupPurpose("waiting_verify", draft, "verify-group"); got != "verify" {
		t.Fatalf("waiting verify purpose = %q, want verify", got)
	}
}
