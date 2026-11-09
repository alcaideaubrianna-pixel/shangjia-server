package sys

import "testing"

func TestIntroFeeChannelDisplayScorePrefersNamedChannel(t *testing.T) {
	emptyHistorical := introFeeChannelDisplayScore("", "", "")
	namedCanonical := introFeeChannelDisplayScore("阿水一群", "uushsh886", "")
	if namedCanonical <= emptyHistorical {
		t.Fatalf("named channel score = %d, empty historical score = %d", namedCanonical, emptyHistorical)
	}
}

func TestIntroFeeChannelDisplayScoreUsesAvailableMetadata(t *testing.T) {
	titleOnly := introFeeChannelDisplayScore("阿水一群", "", "")
	usernameOnly := introFeeChannelDisplayScore("", "uushsh886", "")
	inviteOnly := introFeeChannelDisplayScore("", "", "https://t.me/+invite")
	if !(titleOnly > usernameOnly && usernameOnly > inviteOnly) {
		t.Fatalf("unexpected display scores: title=%d username=%d invite=%d", titleOnly, usernameOnly, inviteOnly)
	}
}

func TestIntroFeeCollectionSourceMatchesChannel(t *testing.T) {
	tests := []struct {
		name         string
		sourceChatID string
		sourceBotID  int64
		channelID    string
		ruleBotID    int64
		want         bool
	}{
		{name: "direct channel source", sourceChatID: "-1004294797657", channelID: "4294797657", want: true},
		{name: "global scope accepts bound aggregate bot", sourceBotID: 64, channelID: "4294797657", want: true},
		{name: "explicit scope accepts same aggregate bot", sourceBotID: 64, channelID: "4294797657", ruleBotID: 64, want: true},
		{name: "explicit scope rejects another aggregate bot", sourceBotID: 65, channelID: "4294797657", ruleBotID: 64, want: false},
		{name: "empty non bot source is not aggregate", channelID: "4294797657", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := introFeeCollectionSourceMatchesChannel(test.sourceChatID, test.sourceBotID, test.channelID, test.ruleBotID)
			if got != test.want {
				t.Fatalf("introFeeCollectionSourceMatchesChannel() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIntroFeeAggregateBotSourceMatchesCanonicalCacheChannel(t *testing.T) {
	candidates := map[string]struct{}{"3998084277": {}}
	if !introFeeAggregateBotSourceMatchesChannel(candidates, "-1003998084277") {
		t.Fatal("aggregate BOT source should match the canonical cached channel")
	}
	if introFeeAggregateBotSourceMatchesChannel(candidates, "-1001234567890") {
		t.Fatal("aggregate BOT source must not match an uncached channel")
	}
}

func TestIntroFeeOnlineSource351FixtureIsBound(t *testing.T) {
	// Sanitized production-shaped fixture captured from source 351/rule 338.
	const (
		tenantID      int64 = 78
		accountID     int64 = 615
		ruleID        int64 = 338
		sourceID      int64 = 351
		botID         int64 = 64
		channelChatID       = "-1003998084277"
	)
	if tenantID != 78 || accountID != 615 || ruleID != 338 || sourceID != 351 || botID != 64 {
		t.Fatal("online fixture was changed unexpectedly")
	}
	if !introFeeCollectionSourceMatchesChannel("", botID, channelChatID, 0) {
		t.Fatal("aggregate source 351 should match the Bot-owned channel")
	}
	if !introFeeAggregateBotSourceMatchesChannel(map[string]struct{}{canonicalCollectSourceChatID(channelChatID): {}}, channelChatID) {
		t.Fatal("Bot cache ownership should bind the canonical channel")
	}
}

func TestCollectDiscoveryNotificationKeyCanonicalizesChat(t *testing.T) {
	first := collectDiscoveryNotificationKey(78, 351, "-1003998084277")
	second := collectDiscoveryNotificationKey(78, 351, "3998084277")
	if first != second {
		t.Fatalf("discovery notification key is not canonical: %q != %q", first, second)
	}
}

func TestShouldQueueCollectDiscoveryNotification(t *testing.T) {
	for _, test := range []struct {
		name       string
		relation   bool
		sent       bool
		wantQueued bool
	}{
		{name: "new approved relation", relation: true, sent: false, wantQueued: true},
		{name: "existing relation missed notification", relation: true, sent: false, wantQueued: true},
		{name: "already notified", relation: true, sent: true, wantQueued: false},
		{name: "disabled relation", relation: false, sent: false, wantQueued: false},
		{name: "rejected relation", relation: false, sent: true, wantQueued: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldQueueCollectDiscoveryNotification(test.relation, test.sent); got != test.wantQueued {
				t.Fatalf("queue=%t want=%t", got, test.wantQueued)
			}
		})
	}
}
