package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

const telegramFairTestAt = "2026-11-17 18:00:00.000001"
const telegramFairTestFence = "2026-11-17 19:00:00.000000"

func telegramFairTestCandidate(id, tenant, account, channel int64) telegramOutboxFairCandidate {
	return telegramOutboxFairCandidate{
		telegramJobOutboxRecord: telegramJobOutboxRecord{
			Id: id, JobId: id, TenantId: tenant, AccountId: account,
			ChannelId: channel, AvailableAt: gtime.New(telegramFairTestAt),
		},
		CursorAt: telegramFairTestAt,
	}
}

// Model the ordered keyset/fence contract without a database or Redis.
func telegramFairTestPage(rows []telegramOutboxFairCandidate, calls *int) telegramOutboxFairPage {
	return func(_ context.Context, cursor telegramOutboxFairCursor, limit int) ([]telegramOutboxFairCandidate, error) {
		*calls++
		var page []telegramOutboxFairCandidate
		for _, row := range rows {
			if row.CursorAt > cursor.Fence ||
				(row.CursorAt < cursor.At || (row.CursorAt == cursor.At && row.Id <= cursor.ID)) {
				continue
			}
			page = append(page, row)
			if len(page) == limit {
				break
			}
		}
		return page, nil
	}
}

func TestTelegramOutboxFairScanCrossesPagesAndRounds(t *testing.T) {
	var candidates []telegramOutboxFairCandidate
	for id := int64(1); id <= 64; id++ {
		candidates = append(candidates, telegramFairTestCandidate(id, 1, 1, 1))
	}
	candidates = append(candidates, telegramFairTestCandidate(65, 1, 2, 2))
	calls := 0
	page := telegramFairTestPage(candidates, &calls)
	cursor := telegramOutboxFairCursor{Fence: telegramFairTestFence}
	rows, next, err := scanTelegramOutboxFairPages(context.Background(), cursor, 8, page)
	if err != nil || calls != 4 || len(rows) != 1 || next.ID != 64 || next.Fence != cursor.Fence {
		t.Fatalf("first round rows=%v cursor=%+v calls=%d err=%v", rows, next, calls, err)
	}
	// Even though 63 candidates were skipped by quota, their scan progress
	// survives. The next round can reach a different channel past four pages.
	rows, next, err = scanTelegramOutboxFairPages(context.Background(), next, 8, page)
	if err != nil || len(rows) != 1 || rows[0].Id != 65 || next != (telegramOutboxFairCursor{}) {
		t.Fatalf("second round rows=%v cursor=%+v err=%v", rows, next, err)
	}
}

func TestTelegramOutboxFairScanQuotasSpanPagesAndTenantScopes(t *testing.T) {
	var candidates []telegramOutboxFairCandidate
	for id := int64(1); id <= 32; id++ {
		candidates = append(candidates, telegramFairTestCandidate(id, 1, 1, 1))
	}
	// With limit=16 account quota is two. This second page must remember
	// account 1 and channel 1 selections from the first page.
	candidates = append(candidates,
		telegramFairTestCandidate(33, 1, 1, 2),
		telegramFairTestCandidate(34, 1, 1, 3),
		telegramFairTestCandidate(35, 2, 1, 1),
		telegramFairTestCandidate(36, 1, 2, 3),
	)
	calls := 0
	rows, _, err := scanTelegramOutboxFairPages(context.Background(),
		telegramOutboxFairCursor{Fence: telegramFairTestFence}, 16, telegramFairTestPage(candidates, &calls))
	if err != nil || calls != 2 || len(rows) != 4 {
		t.Fatalf("rows=%v calls=%d err=%v", rows, calls, err)
	}
	for i, id := range []int64{1, 33, 35, 36} {
		if rows[i].Id != id {
			t.Fatalf("selection[%d]=%d want %d", i, rows[i].Id, id)
		}
	}
}

func TestTelegramOutboxFairScanOldJobsAndFailedDeliveryReturnAfterWrap(t *testing.T) {
	// Both candidates have waited an hour, well past the old ten-minute
	// cutoff. A delivery failure leaves candidate 1 in the supplied set.
	candidates := []telegramOutboxFairCandidate{
		telegramFairTestCandidate(1, 1, 1, 1),
		telegramFairTestCandidate(2, 1, 1, 1),
	}
	calls := 0
	page := telegramFairTestPage(candidates, &calls)
	for round := 0; round < 2; round++ {
		rows, next, err := scanTelegramOutboxFairPages(context.Background(),
			telegramOutboxFairCursor{Fence: telegramFairTestFence}, 8, page)
		if err != nil || len(rows) != 1 || rows[0].Id != 1 || next != (telegramOutboxFairCursor{}) {
			t.Fatalf("round %d rows=%v next=%+v err=%v", round, rows, next, err)
		}
	}
	// After the failed job is successfully delivered and disappears, the
	// same-channel candidate skipped earlier must become selectable.
	candidates = candidates[1:]
	rows, _, err := scanTelegramOutboxFairPages(context.Background(),
		telegramOutboxFairCursor{Fence: telegramFairTestFence}, 8, telegramFairTestPage(candidates, &calls))
	if err != nil || len(rows) != 1 || rows[0].Id != 2 {
		t.Fatalf("skipped candidate did not return: rows=%v err=%v", rows, err)
	}
}

func TestTelegramOutboxFairScanPageFailureDiscardsProgress(t *testing.T) {
	original := telegramOutboxFairCursor{Fence: telegramFairTestFence}
	failure := errors.New("page query failed")
	calls := 0
	page := func(_ context.Context, _ telegramOutboxFairCursor, size int) ([]telegramOutboxFairCandidate, error) {
		calls++
		if calls == 2 {
			return nil, failure
		}
		var rows []telegramOutboxFairCandidate
		for id := 1; id <= size; id++ {
			rows = append(rows, telegramFairTestCandidate(int64(id), 1, 1, 1))
		}
		return rows, nil
	}
	rows, next, err := scanTelegramOutboxFairPages(context.Background(), original, 8, page)
	if !errors.Is(err, failure) || len(rows) != 0 || next != original {
		t.Fatalf("failed scan committed progress: rows=%v next=%+v err=%v", rows, next, err)
	}
}

func TestTelegramOutboxFairScanFenceAndFullBatchCursor(t *testing.T) {
	var candidates []telegramOutboxFairCandidate
	for id := int64(1); id <= 5; id++ {
		candidates = append(candidates, telegramFairTestCandidate(id, 1, id, id))
	}
	future := telegramFairTestCandidate(6, 1, 6, 6)
	future.CursorAt = "2026-11-17 20:00:00.000000"
	candidates = append(candidates, future)
	calls := 0
	page := telegramFairTestPage(candidates, &calls)
	rows, next, err := scanTelegramOutboxFairPages(context.Background(),
		telegramOutboxFairCursor{Fence: telegramFairTestFence}, 2, page)
	if err != nil || len(rows) != 2 || next.ID != 2 {
		t.Fatalf("full batch skipped unread rows: rows=%v next=%+v err=%v", rows, next, err)
	}
	rows, next, err = scanTelegramOutboxFairPages(context.Background(), next, 8, page)
	if err != nil || len(rows) != 3 || next != (telegramOutboxFairCursor{}) {
		t.Fatalf("fence/tail: rows=%v next=%+v err=%v", rows, next, err)
	}
}

func TestTelegramOutboxFairScanDeferredFailureWaitsForNextFence(t *testing.T) {
	// Model a caller moving a failed Outbox's available_at past this sweep's
	// fence. The pagination helper does not own the failure/backoff mutation.
	retry := telegramFairTestCandidate(1, 1, 1, 1)
	retry.CursorAt = "2026-11-17 19:01:00.000000"
	calls := 0
	page := telegramFairTestPage([]telegramOutboxFairCandidate{retry}, &calls)
	rows, next, err := scanTelegramOutboxFairPages(context.Background(),
		telegramOutboxFairCursor{Fence: telegramFairTestFence}, 8, page)
	if err != nil || len(rows) != 0 || next != (telegramOutboxFairCursor{}) {
		t.Fatalf("retry escaped fixed fence: rows=%v next=%+v err=%v", rows, next, err)
	}
	rows, next, err = scanTelegramOutboxFairPages(context.Background(),
		telegramOutboxFairCursor{Fence: "2026-11-17 19:02:00.000000"}, 8, page)
	if err != nil || len(rows) != 1 || rows[0].Id != 1 || next != (telegramOutboxFairCursor{}) {
		t.Fatalf("deferred retry was lost: rows=%v next=%+v err=%v", rows, next, err)
	}
}

func TestTelegramOutboxFairScanCancelledBudgetDoesNotQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	original := telegramOutboxFairCursor{Fence: telegramFairTestFence}
	rows, next, err := scanTelegramOutboxFairPages(ctx, original, 8, telegramFairTestPage(nil, &calls))
	if !errors.Is(err, context.Canceled) || calls != 0 || len(rows) != 0 || next != original {
		t.Fatalf("cancelled scan progressed: rows=%v next=%+v calls=%d err=%v", rows, next, calls, err)
	}
}
