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
