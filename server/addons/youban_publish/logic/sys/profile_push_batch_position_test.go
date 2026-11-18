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
		{"263 queue tail has three days ahead", profilePushBatchRank{Eligible: 1, BeforeTarget: 262}, 0, profileBatchPushPosition{AheadCount: 262}, 3},
		{"production 260 never adds old 247", profilePushBatchRank{Eligible: 1, BeforeTarget: 260}, 0, profileBatchPushPosition{AheadCount: 260}, 3},
		{"unselected removes only unmoved selected", profilePushBatchRank{Eligible: 1, BeforeTarget: 222, SelectedAhead: 30}, 70, profileBatchPushPosition{AheadCount: 192}, 2},
		{"actual snapshot member", profilePushBatchRank{Eligible: 1, BeforeTarget: 200, SelectedAhead: 5, InCurrentRun: true}, 70, profileBatchPushPosition{AheadCount: 5, InCurrentRun: true}, -1},
		{"low rank is not membership", profilePushBatchRank{Eligible: 1, BeforeTarget: 5}, 70, profileBatchPushPosition{AheadCount: 5}, 0},
		{"all selected moved tail already", profilePushBatchRank{Eligible: 1, BeforeTarget: 192}, 70, profileBatchPushPosition{AheadCount: 192}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := profilePushBatchPosition(tc.rank, tc.active); got != tc.want {
				t.Fatalf("position=%+v want=%+v", got, tc.want)
			}
			if days, ok := profilePushBatchDayOffset(70, tc.rank, tc.active); !ok || days != tc.days {
				t.Fatalf("days=%d eligible=%v want=%d", days, ok, tc.days)
			}
		})
	}
}

func TestProfilePushQueueRankPartialSuccess(t *testing.T) {
	// Profiles 1..70 were selected. The first 40 succeeded and moved behind
	// all 193 unselected profiles. Their ordinals no longer match batch_order.
	items := make([]profilePushQueueItem, 0, 263)
	for id := int64(41); id <= 263; id++ {
		ordinal := int64(0)
		if id <= 70 {
			ordinal = id
		}
		items = append(items, profilePushQueueItem{ProfileID: id, Order: id, Ordinal: ordinal, Eligible: true})
	}
	for id := int64(1); id <= 40; id++ {
		items = append(items, profilePushQueueItem{ProfileID: id, Order: 263 + id, Eligible: true})
	}
	for _, tc := range []struct {
		id, ahead int64
		member    bool
	}{
		{41, 0, true}, {70, 29, true}, {71, 0, false}, {263, 192, false}, {1, 193, false}, {40, 232, false},
	} {
		rank := profilePushQueueRank(items, tc.id, 0) // BOT account zero is valid.
		position := profilePushBatchPosition(rank, 70)
		if rank.Eligible != 1 || position.AheadCount != tc.ahead || position.InCurrentRun != tc.member || position.WaitingNextRound {
			t.Fatalf("profile=%d rank=%+v position=%+v", tc.id, rank, position)
		}
	}
	if got := profilePushQueueRank(items, 41, 99); got.Eligible != 0 {
		t.Fatalf("cross-account target: %+v", got)
	}
	items[0].Eligible = false
	if got := profilePushQueueRank(items, 41, 0); got.Eligible != 0 {
		t.Fatalf("disabled target: %+v", got)
	}
	if got := profilePushQueueRank(items, 70, 0); got.SelectedAhead != 28 {
		t.Fatalf("disabled predecessor counted: %+v", got)
	}
}

func TestProfilePushRotationSendingDayGaps(t *testing.T) {
	// Simulate several daily immutable selections and successful tail moves.
	// 263/70 must yield only 3- or 4-sending-day return intervals.
	queue := make([]int, 263)
	for i := range queue {
		queue[i] = i
	}
	last := make(map[int]int)
	for day := 0; day < 20; day++ {
		for _, id := range queue[:70] {
			if previous, ok := last[id]; ok && day-previous != 3 && day-previous != 4 {
				t.Fatalf("id=%d gap=%d", id, day-previous)
			}
			last[id] = day
		}
		queue = append(append([]int{}, queue[70:]...), queue[:70]...)
		items := make([]profilePushQueueItem, len(queue))
		for i, id := range queue {
			items[i] = profilePushQueueItem{ProfileID: int64(id), Eligible: true}
		}
		for i, id := range queue {
			rank := profilePushQueueRank(items, int64(id), 0)
			if offset, ok := profilePushBatchDayOffset(70, rank, 0); !ok || offset != int64(i/70) {
				t.Fatalf("rank=%d offset=%d ok=%v", i, offset, ok)
			}
		}
	}
}
