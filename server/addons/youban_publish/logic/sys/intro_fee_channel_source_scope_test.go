package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestIntroFeeChannelSourceScopeEvidence(t *testing.T) {
	const chat = "3736052878"
	base := introFeeChannelSourceScope{Id: 351, ChatId: "-1004326200245", BotId: 64}
	account := introFeeChannelSourceEvidence{
		TenantID: 78, AccountID: 615, SourceID: 351, ChatID: "-1003736052878",
		Status: sysin.CollectAccountChannelDisabled,
	}
	bot := introFeeChannelSourceEvidence{
		TenantID: 78, BotID: 64, ChatID: "-1003736052878", Status: sysin.CollectBotChannelDiscovered,
	}
	for _, tt := range []struct {
		name     string
		source   introFeeChannelSourceScope
		ruleBot  int64
		evidence introFeeChannelSourceEvidenceSet
		want     bool
	}{
		{"configured_alias", introFeeChannelSourceScope{Id: 351, ChatId: "-1003736052878"}, 0, introFeeChannelSourceEvidenceSet{}, true},
		{"unrelated_configured_channel", base, 0, introFeeChannelSourceEvidenceSet{}, false},
		{"empty_configuration_not_wildcard", introFeeChannelSourceScope{Id: 351, BotId: 64}, 0, introFeeChannelSourceEvidenceSet{}, false},
		{"disabled_relation_still_owned", base, 64, introFeeChannelSourceEvidenceSet{AccountChannels: []introFeeChannelSourceEvidence{account}}, true},
		{"bot_discovered_dynamic_channel", base, 64, introFeeChannelSourceEvidenceSet{BotChannels: []introFeeChannelSourceEvidence{bot}}, true},
		{"rule_without_bot_uses_source_bot_evidence", base, 0, introFeeChannelSourceEvidenceSet{BotChannels: []introFeeChannelSourceEvidence{bot}}, true},
		{"bot_cache_dynamic_channel", base, 64, introFeeChannelSourceEvidenceSet{BotCache: []introFeeChannelSourceEvidence{bot}}, true},
		{"rule_bot_mismatch", base, 65, introFeeChannelSourceEvidenceSet{BotCache: []introFeeChannelSourceEvidence{bot}}, false},
		{"source_without_bot_cannot_borrow_cache", introFeeChannelSourceScope{Id: 351}, 0, introFeeChannelSourceEvidenceSet{BotCache: []introFeeChannelSourceEvidence{bot}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, alias := range []string{chat, "-100" + chat} {
				got := introFeeChannelSourceHasBinding(78, 615, tt.ruleBot, alias, tt.source, tt.evidence)
				if got != tt.want {
					t.Fatalf("chat=%s got=%v want=%v", alias, got, tt.want)
				}
			}
		})
	}
	for _, mutation := range []string{"tenant", "account", "source", "chat", "deleted"} {
		t.Run("account_scope_"+mutation, func(t *testing.T) {
			row := account
			switch mutation {
			case "tenant":
				row.TenantID++
			case "account":
				row.AccountID++
			case "source":
				row.SourceID++
			case "chat":
				row.ChatID = "-100999"
			case "deleted":
				row.Deleted = true
			}
			if introFeeChannelSourceHasBinding(78, 615, 64, chat, base,
				introFeeChannelSourceEvidenceSet{AccountChannels: []introFeeChannelSourceEvidence{row}}) {
				t.Fatal("foreign/deleted account relation established source ownership")
			}
		})
	}
	for _, kind := range []string{"relation", "cache"} {
		for _, mutation := range []string{"tenant", "bot", "chat", "deleted"} {
			t.Run(kind+"_scope_"+mutation, func(t *testing.T) {
				row := bot
				switch mutation {
				case "tenant":
					row.TenantID++
				case "bot":
					row.BotID++
				case "chat":
					row.ChatID = "-100999"
				case "deleted":
					row.Deleted = true
				}
				var evidence introFeeChannelSourceEvidenceSet
				if kind == "relation" {
					evidence.BotChannels = []introFeeChannelSourceEvidence{row}
				} else {
					evidence.BotCache = []introFeeChannelSourceEvidence{row}
				}
				if introFeeChannelSourceHasBinding(78, 615, 64, chat, base, evidence) {
					t.Fatal("foreign/deleted Bot evidence established ownership")
				}
			})
		}
	}
	t.Run("removed_alias_blocks_stale_cache_and_conflicting_alias", func(t *testing.T) {
		removed := bot
		removed.Status, removed.ChatID = sysin.CollectBotChannelRemoved, chat
		evidence := introFeeChannelSourceEvidenceSet{
			BotChannels: []introFeeChannelSourceEvidence{removed},
			BotCache:    []introFeeChannelSourceEvidence{bot},
		}
		if introFeeChannelSourceHasBinding(78, 615, 64, chat, base, evidence) {
			t.Fatal("stale cache bypassed explicit Bot removal")
		}
		evidence.BotChannels = append(evidence.BotChannels, bot)
		if introFeeChannelSourceHasBinding(78, 615, 64, chat, base, evidence) {
			t.Fatal("contradictory aliases bypassed explicit removal")
		}
		// A pre-existing source-specific relation remains editable; this is
		// ownership resolution, not permission to collect from a removed Bot.
		evidence.AccountChannels = []introFeeChannelSourceEvidence{account}
		if !introFeeChannelSourceHasBinding(78, 615, 64, chat, base, evidence) {
			t.Fatal("existing disabled relation can no longer be toggled")
		}
	})
	t.Run("removed_other_scope_does_not_override_valid_cache", func(t *testing.T) {
		for _, foreign := range []string{"tenant", "bot", "chat", "deleted"} {
			removed := bot
			removed.Status = sysin.CollectBotChannelRemoved
			switch foreign {
			case "tenant":
				removed.TenantID++
			case "bot":
				removed.BotID++
			case "chat":
				removed.ChatID = "999"
			case "deleted":
				removed.Deleted = true
			}
			if !introFeeChannelSourceHasBinding(78, 615, 64, chat, base, introFeeChannelSourceEvidenceSet{
				BotChannels: []introFeeChannelSourceEvidence{removed}, BotCache: []introFeeChannelSourceEvidence{bot},
			}) {
				t.Fatalf("foreign removal %s overrode valid cache", foreign)
			}
		}
	})
	t.Run("source_a_relation_does_not_bind_source_b", func(t *testing.T) {
		other := introFeeChannelSourceScope{Id: 352, BotId: 65}
		evidence := introFeeChannelSourceEvidenceSet{AccountChannels: []introFeeChannelSourceEvidence{account}}
		if !introFeeChannelSourceHasBinding(78, 615, 0, chat, base, evidence) ||
			introFeeChannelSourceHasBinding(78, 615, 0, chat, other, evidence) {
			t.Fatal("source-specific evidence escaped its source")
		}
	})
	t.Run("invalid_scope", func(t *testing.T) {
		for _, scope := range [][3]int64{{0, 615, 351}, {78, 0, 351}, {78, 615, 0}} {
			source := introFeeChannelSourceScope{Id: scope[2], ChatId: chat}
			if introFeeChannelSourceHasBinding(scope[0], scope[1], 0, chat, source, introFeeChannelSourceEvidenceSet{}) {
				t.Fatal("invalid identity accepted")
			}
		}
		if introFeeChannelSourceHasBinding(78, 615, 0, "", base, introFeeChannelSourceEvidenceSet{}) {
			t.Fatal("empty target accepted")
		}
	})
}
