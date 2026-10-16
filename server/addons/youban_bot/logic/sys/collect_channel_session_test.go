package sys

import (
	"strings"
	"testing"
)

func TestCollectChannelSuffixSessionBinding(t *testing.T) {
	session := &profileSessionRow{Id: 42, Scene: "collect_channel_add", Status: profileSessionStatusActive, Step: "confirming", PayloadJson: `{"confirmationToken":"draft-new","draft":"新后缀"}`}
	for _, tc := range []struct {
		name, callback string
		want           bool
	}{
		{"current", "cm:channelsuffixconfirm:42:draft-new", true},
		{"old session", "cm:channelsuffixconfirm:41:draft-new", false},
		{"old draft", "cm:channelsuffixconfirm:42:draft-old", false},
		{"legacy", "cm:channelsuffixconfirm", false},
		{"invalid id", "cm:channelsuffixconfirm:x:draft-new", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectChannelSuffixSessionMatches(session, strings.Split(tc.callback, ":"), true); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	session.Step = "waiting_value"
	parts := strings.Split("cm:channelsuffixcancel:42:draft-new", ":")
	if collectChannelSuffixSessionMatches(session, parts, true) {
		t.Fatal("waiting session allowed save")
	}
	if !collectChannelSuffixSessionMatches(session, parts, false) {
		t.Fatal("current cancel rejected")
	}
	session.Status = profileSessionStatusCanceled
	if collectChannelSuffixSessionMatches(session, parts, false) {
		t.Fatal("completed session accepted")
	}
}
