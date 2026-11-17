package sys

import "testing"

func TestProfilePushBatchPositionExplainsSameSchedule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rank   profilePushBatchRank
		active int64
		want   profileBatchPushPosition
		days   int64
	}{
		{
			name: "three_daily_batches_ahead",
			rank: profilePushBatchRank{Eligible: 1, InCurrentRound: 1, AheadInRound: 210},
			want: profileBatchPushPosition{AheadCount: 210},
			days: 3,
		},
		{
			name: "production_profile_waits_for_two_round_segments",
			rank: profilePushBatchRank{Eligible: 1, BeforeTarget: 260, RoundLeft: 247},
			want: profileBatchPushPosition{AheadCount: 507, RoundRemaining: 247, WaitingNextRound: true},
			days: 7,
		},
		{
			name:   "active_run_removed_from_future_wait",
			rank:   profilePushBatchRank{Eligible: 1, BeforeTarget: 260, RoundLeft: 247},
			active: 70,
			want:   profileBatchPushPosition{AheadCount: 437, RoundRemaining: 177, WaitingNextRound: true},
			days:   6,
		},
		{
			name:   "estimated_inside_active_run",
			rank:   profilePushBatchRank{Eligible: 1, InCurrentRound: 1, AheadInRound: 5},
			active: 70,
			want:   profileBatchPushPosition{AheadCount: 5, InCurrentRun: true},
			days:   -1,
		},
		{
			name:   "next_batch_has_no_predecessors",
			rank:   profilePushBatchRank{Eligible: 1, InCurrentRound: 1, AheadInRound: 70},
			active: 70,
			want:   profileBatchPushPosition{},
		},
		{
			name:   "expired_tail_does_not_create_negative_counts",
			rank:   profilePushBatchRank{Eligible: 1, BeforeTarget: 4, RoundLeft: 5},
			active: 70,
			want:   profileBatchPushPosition{AheadCount: 4, WaitingNextRound: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := profilePushBatchPosition(tc.rank, tc.active)
			if got != tc.want {
				t.Fatalf("position=%+v want=%+v", got, tc.want)
			}
			if days, ok := profilePushBatchDayOffset(70, tc.rank, tc.active); !ok || days != tc.days {
				t.Fatalf("date offset=%d eligible=%v want=%d", days, ok, tc.days)
			}
		})
	}
}
