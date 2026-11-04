package sys

import (
	"database/sql"
	"errors"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectOptionalRecordScanSucceeded(t *testing.T) {
	if !collectOptionalRecordScanSucceeded(nil) {
		t.Fatal("nil error should be accepted")
	}
	if !collectOptionalRecordScanSucceeded(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should enter the record creation path")
	}
	if collectOptionalRecordScanSucceeded(errors.New("database unavailable")) {
		t.Fatal("unexpected database errors must be returned")
	}
}

func TestBotCollectChannelRelationEnabledAcceptsLegacyApproval(t *testing.T) {
	if !botCollectChannelRelationEnabled(1, "approved", "approved") {
		t.Fatal("legacy approved relation should remain enabled")
	}
	if !botCollectChannelRelationEnabled(1, sysin.CollectAccountChannelEnabled, "pending") {
		t.Fatal("current enabled relation should remain enabled")
	}
	if botCollectChannelRelationEnabled(0, "approved", "approved") {
		t.Fatal("disabled collection must not be enabled by approval status")
	}
}

func TestBotCollectChannelDefaultEnabled(t *testing.T) {
	for _, tc := range []struct {
		status, approval string
		enabled          int
		want             bool
	}{
		{"", "", 0, true}, {"pending", "pending", 0, true},
		{"disabled", "approved", 1, false}, {"enabled", "rejected", 1, false},
		{"enabled", "approved", 1, true}, {"enabled", "approved", 0, false},
	} {
		if got := botCollectChannelRelationEnabled(tc.enabled, tc.status, tc.approval); got != tc.want {
			t.Fatalf("%+v: got %v", tc, got)
		}
	}
}

func TestSelectBotCollectChannelPrefersExactCanonicalChatID(t *testing.T) {
	candidates := []botCollectChannelCandidate{
		{Id: 33803, TelegramChatId: "3739452626", Title: "legacy rejected channel"},
		{Id: 97373, TelegramChatId: "-1003739452626", Title: "canonical approved channel"},
	}

	got := selectBotCollectChannel(candidates, "-1003739452626")
	if got.Id != 97373 {
		t.Fatalf("selected channel id=%d, want canonical channel id=97373", got.Id)
	}
}

func TestSelectBotCollectChannelFallsBackToLegacyChatID(t *testing.T) {
	candidates := []botCollectChannelCandidate{{Id: 33803, TelegramChatId: "3739452626"}}

	got := selectBotCollectChannel(candidates, "-1003739452626")
	if got.Id != 33803 {
		t.Fatalf("selected channel id=%d, want legacy fallback channel id=33803", got.Id)
	}
}

func TestSelectBotCollectChannelUsesCurrentAccountsApprovedMapping(t *testing.T) {
	candidates := []botCollectChannelCandidate{
		{Id: 33803, TelegramChatId: "3739452626", Title: "legacy"},
		{Id: 97373, TelegramChatId: "-1003739452626", Title: "canonical"},
	}
	relations := []botCollectChannelRelation{
		{Id: 1, ChannelId: 33803, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"},
		{Id: 2, ChannelId: 97373, CollectEnabled: 0, Status: "disabled", ApprovalStatus: "rejected"},
	}

	got := selectBotCollectChannelForAccount(candidates, relations, "-1003739452626")
	if got.Id != 33803 {
		t.Fatalf("selected channel id=%d, want the legacy mapping approved for this account", got.Id)
	}
}

func TestSelectBotCollectChannelPrefersCanonicalWhenAccountApprovesBoth(t *testing.T) {
	candidates := []botCollectChannelCandidate{
		{Id: 33803, TelegramChatId: "3739452626"},
		{Id: 97373, TelegramChatId: "-1003739452626"},
	}
	relations := []botCollectChannelRelation{
		{Id: 1, ChannelId: 33803, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"},
		{Id: 2, ChannelId: 97373, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"},
	}

	got := selectBotCollectChannelForAccount(candidates, relations, "-1003739452626")
	if got.Id != 97373 {
		t.Fatalf("selected channel id=%d, want canonical mapping when both are approved", got.Id)
	}
}
