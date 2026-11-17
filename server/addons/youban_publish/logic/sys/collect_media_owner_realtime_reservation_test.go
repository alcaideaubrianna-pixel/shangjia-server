package sys

import "testing"

func TestCollectMediaOwnerRealtimeReservation(t *testing.T) {
	for total := 2; total <= collectMediaMaxOwnerConcurrency; total++ {
		for eventID := int64(1); eventID <= int64(total*2); eventID++ {
			for _, bulk := range []bool{false, true} {
				slots := collectMediaOwnerSlotOrder(eventID, total, bulk)
				want := total
				if bulk {
					want--
				}
				if len(slots) != want {
					t.Fatalf("limit=%d bulk=%t slots=%v", total, bulk, slots)
				}
				seen := map[int]bool{}
				for _, slot := range slots {
					if slot < 0 || slot >= total || seen[slot] {
						t.Fatalf("slot escaped existing key space: %v", slots)
					}
					if bulk && slot == total-1 {
						t.Fatalf("bulk borrowed reserved slot %d", slot)
					}
					seen[slot] = true
				}
				if !bulk && !seen[total-1] {
					t.Fatal("realtime cannot reach reserved slot")
				}
			}
		}
	}
}

func TestCollectMediaOwnerBulkFullLeavesRealtimeCapacity(t *testing.T) {
	// Model the exact slot order used by acquisition, with all existing bulk
	// slots occupied. Realtime can still acquire the last existing slot; another
	// bulk event must defer rather than creating capacity outside the old limit.
	for _, total := range []int{collectMediaDefaultSourceConcurrency, collectMediaDefaultAccountConcurrency} {
		occupied := map[int]bool{}
		claim := func(eventID int64, bulk bool) bool {
			for _, slot := range collectMediaOwnerSlotOrder(eventID, total, bulk) {
				if !occupied[slot] {
					occupied[slot] = true
					return true
				}
			}
			return false
		}
		for id := 1; id < total; id++ {
			if !claim(int64(id), true) {
				t.Fatal("bulk lost non-reserved capacity")
			}
		}
		if claim(100, true) {
			t.Fatal("bulk occupied realtime reservation")
		}
		if !claim(101, false) {
			t.Fatal("realtime blocked behind bulk owner quota")
		}
		if len(occupied) != total {
			t.Fatal("owner total changed")
		}
		if claim(102, false) || claim(103, true) {
			t.Fatal("total owner cap exceeded")
		}
	}
}

func TestCollectMediaOwnerSingleSlotCompatibility(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		for _, limit := range []int{-1, 0, 1} {
			slots := collectMediaOwnerSlotOrder(7, limit, bulk)
			if len(slots) != 1 || slots[0] != 0 {
				t.Fatalf("single-slot history permanently stopped: bulk=%t limit=%d slots=%v", bulk, limit, slots)
			}
		}
	}
}

func TestCollectMediaOwnerReservationNormalizesAndRotates(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		got := collectMediaOwnerSlotOrder(1, 1000, bulk)
		expected := collectMediaMaxOwnerConcurrency
		if bulk {
			expected--
		}
		if len(got) != expected {
			t.Fatal("configured limit escaped maximum")
		}
	}
	first := collectMediaOwnerSlotOrder(1, 4, true)
	second := collectMediaOwnerSlotOrder(2, 4, true)
	if first[0] == second[0] {
		t.Fatal("bulk tasks no longer rotate across available slots")
	}
}
