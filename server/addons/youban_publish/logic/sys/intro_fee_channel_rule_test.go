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

func TestIntroFeeConfiguredSourceMatchesChannel(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sourceChatID string
		channelID    string
		want         bool
	}{
		{name: "full and short channel aliases", sourceChatID: "-1004294797657", channelID: "4294797657", want: true},
		{name: "short and full channel aliases", sourceChatID: "4294797657", channelID: "-1004294797657", want: true},
		{name: "different configured channel", sourceChatID: "-1003856373273", channelID: "3736052878"},
		{name: "no configured channel", channelID: "4294797657"},
		{name: "both empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := introFeeConfiguredSourceMatchesChannel(tc.sourceChatID, tc.channelID); got != tc.want {
				t.Fatalf("configured channel match=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestIntroFeeConfiguredSourceCannotAuthorizeUnrelatedChannel(t *testing.T) {
	// The configured-channel predicate deliberately returns false for the
	// production-shaped dynamic channel. The save path must instead resolve
	// the real source/channel relation or Bot membership before authorizing it.
	if introFeeConfiguredSourceMatchesChannel("-1003856373273", "3736052878") {
		t.Fatal("a different configured channel must not authorize this channel")
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
