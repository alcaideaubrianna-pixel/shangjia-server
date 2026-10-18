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
