package sys

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestCycleBatchSnapshotLocalDay(t *testing.T) {
	// UTC November 18 evening is already November 19 in Beijing.
	start, end := batchCycleLocalDay(time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC))
	if start.Format("2006-01-02 15:04 -0700") != "2026-11-19 00:00 +0800" || end.Sub(start) != 24*time.Hour {
		t.Fatalf("Beijing day boundaries: %v %v", start, end)
	}
}

func TestCycleBatchSnapshotIdentityAndOrder(t *testing.T) {
	candidates := []batchRotationCandidate{
		{ProfileID: 900, AccountID: 0, Order: 10},
		{ProfileID: 2, AccountID: 7, Order: 11},
		{ProfileID: 41, AccountID: 7, Order: 12},
	}
	items, err := newCycleBatchSnapshot(3, 4, 70, candidates)
	if err != nil || len(items) != 3 {
		t.Fatalf("short batch must not wrap: %v %+v", err, items)
	}
	for i, item := range items {
		if item.Ordinal != int64(i+1) || item.ProfileID != candidates[i].ProfileID || item.BatchOrder != candidates[i].Order {
			t.Fatalf("ordinal follows queue order, not profile ID: %+v", items)
		}
	}
	candidates[1] = candidates[0]
	if _, err := newCycleBatchSnapshot(3, 4, 70, candidates); err == nil {
		t.Fatal("duplicate daily profile must be rejected")
	}
}

func TestCycleBatchSnapshotPreparationIsolation(t *testing.T) {
	errs := []error{errProfileCycleMediaPending, errors.New("媒体校验失败"), errPublishProfileUnavailable, nil}
	want := []cycleBatchItemState{cycleBatchItemDeferred, cycleBatchItemFailed, cycleBatchItemSkipped, cycleBatchItemAdmitted}
	var got []cycleBatchItemState
	for _, err := range errs {
		got = append(got, cycleBatchPreparationState(err))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("each slot must retain its own result: %v", got)
	}
}

func TestCycleBatchSnapshotTracksReusedJobs(t *testing.T) {
	items := []cycleBatchDeliveryItem{
		{Ordinal: 1, ItemState: cycleBatchItemAdmitted, JobID: 91, ActualID: 91, JobStatus: JobStatusSending},
		{Ordinal: 2, ItemState: cycleBatchItemDeferred},
		{Ordinal: 3, ItemState: cycleBatchItemFailed},
		{Ordinal: 4, ItemState: cycleBatchItemAdmitted, JobID: 94, ActualID: 94, JobStatus: JobStatusSent},
	}
	pending, sent, failed := aggregateCycleBatchDelivery(items)
	if pending != 1 || sent != 1 || failed != 2 {
		t.Fatalf("active reused job must hold run open: %d %d %d", pending, sent, failed)
	}
	items[0].JobStatus = JobStatusSent
	pending, sent, failed = aggregateCycleBatchDelivery(items)
	if pending != 0 || sent != 2 || failed != 2 {
		t.Fatalf("terminal reused job must finish without new send: %d %d %d", pending, sent, failed)
	}
	items[0].ActualID = 0
	pending, _, _ = aggregateCycleBatchDelivery(items)
	if pending != 1 {
		t.Fatal("missing or mismatched job cannot finish run")
	}
}
