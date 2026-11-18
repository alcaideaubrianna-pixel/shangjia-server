package sys

import (
	"database/sql"
	"errors"
	"testing"
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

func TestSelectBotCollectChannelAliasUsesEnabledApprovedRelation(t *testing.T) {
	candidates := []botCollectChannelCandidate{
		{Id: 24092, TelegramChatId: "-1003715905169", Title: "historical rejected alias"},
		{Id: 1845, TelegramChatId: "3715905169", Title: "enabled alias"},
	}
	relations := []botCollectChannelRelation{
		{Id: 1, ChannelId: 24092, SourceId: 351, CollectEnabled: 0, Status: "disabled", ApprovalStatus: "rejected"},
		{Id: 2, ChannelId: 1845, SourceId: 351, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"},
	}

	got := selectBotCollectChannelForAccount(candidates, relations, "-1003715905169")
	if got.Id != 1845 {
		t.Fatalf("selected channel id=%d, want enabled+approved alias id=1845", got.Id)
	}
}

func TestSelectBotCollectChannelDoesNotLetDisabledAliasOverrideEnabledAlias(t *testing.T) {
	candidates := []botCollectChannelCandidate{
		{Id: 24092, TelegramChatId: "3715905169", Title: "disabled alias"},
		{Id: 1845, TelegramChatId: "-1003715905169", Title: "approved alias"},
	}
	relations := []botCollectChannelRelation{
		{Id: 1, ChannelId: 24092, SourceId: 351, CollectEnabled: 0, Status: "disabled", ApprovalStatus: "rejected"},
		{Id: 2, ChannelId: 1845, SourceId: 351, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"},
	}

	got := selectBotCollectChannelForAccount(candidates, relations, "3715905169")
	if got.Id != 1845 {
		t.Fatalf("disabled alias overrode enabled alias: selected channel id=%d", got.Id)
	}
}
