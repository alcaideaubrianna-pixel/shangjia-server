package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestProfilePushBatchDayOffset(t *testing.T) {
	for _, tc := range []struct {
		name   string
		size   int
		rank   profilePushBatchRank
		active int64
		want   int64
		ok     bool
	}{
		{"first batch", 70, profilePushBatchRank{AheadInRound: 0, Eligible: 1, InCurrentRound: 1}, 0, 0, true},
		{"last place in batch", 70, profilePushBatchRank{AheadInRound: 69, Eligible: 1, InCurrentRound: 1}, 0, 0, true},
		{"next batch", 70, profilePushBatchRank{AheadInRound: 70, Eligible: 1, InCurrentRound: 1}, 0, 1, true},
		{"new profile waits for next round", 70, profilePushBatchRank{BeforeTarget: 211, RoundLeft: 140, Eligible: 1}, 0, 5, true},
		{"passed cursor", 70, profilePushBatchRank{BeforeTarget: 69, RoundLeft: 71, Eligible: 1}, 0, 2, true},
		{"empty tail wraps same day", 70, profilePushBatchRank{BeforeTarget: 70, RoundLeft: 0, Eligible: 1}, 0, 1, true},
		{"inside running batch", 70, profilePushBatchRank{AheadInRound: 5, Eligible: 1, InCurrentRound: 1}, 70, -1, true},
		{"after running batch", 70, profilePushBatchRank{AheadInRound: 70, Eligible: 1, InCurrentRound: 1}, 70, 0, true},
		{"new round after running batch", 70, profilePushBatchRank{BeforeTarget: 211, RoundLeft: 140, Eligible: 1}, 70, 4, true},
		{"creation order decides membership", 70, profilePushBatchRank{BeforeTarget: 200, AheadInRound: 0, RoundLeft: 140, Eligible: 1, InCurrentRound: 1}, 0, 0, true},
		{"production sample", 70, profilePushBatchRank{BeforeTarget: 251, RoundLeft: 247, Eligible: 1}, 0, 7, true},
		{"not eligible", 70, profilePushBatchRank{}, 0, 0, false},
		{"disabled batch size", 0, profilePushBatchRank{Eligible: 1}, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := profilePushBatchDayOffset(tc.size, tc.rank, tc.active)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("offset=%d ok=%v; want offset=%d ok=%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestProfilePushNextBatchAt(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	now := gtime.New(time.Date(2026, 11, 18, 12, 0, 0, 0, zone))
	future := gtime.New(time.Date(2026, 11, 19, 13, 0, 0, 0, zone))
	past := gtime.New(time.Date(2026, 11, 18, 11, 0, 0, 0, zone))
	for _, tc := range []struct {
		name    string
		planned *gtime.Time
		want    int64
	}{
		{"stored plan", future, future.Unix()},
		{"overdue is immediately due", past, now.Unix()},
		{"missing plan is immediately due", nil, now.Unix()},
		{"database clock represented as UTC", gtime.New(time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)), now.Add(time.Hour).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := profilePushNextBatchAt(tc.planned, now); got.Unix() != tc.want {
				t.Fatalf("got=%s want unix=%d", got, tc.want)
			}
		})
	}
}
