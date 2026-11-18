package sys

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProfileDownCleanupContextOwnerScope(t *testing.T) {
	jobs := []telegramResubmitJob{
		{Id: 12, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 11, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 11, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 21, TenantId: 1, AccountId: 2, ProfileId: 4, ChannelId: 10},
		{Id: 31, TenantId: 1, AccountId: 0, ProfileId: 3, ChannelId: 10},
		{Id: 41, TenantId: 2, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 51, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 20},
		{Id: 61, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 30},
	}
	policies := map[int64]bool{10: false, 20: true}
	payloads, err := buildProfileDownCleanupPayloads(jobs, policies, " unpublish:scope ")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int64][]int64{
		11: {11, 12},
		12: {11, 12},
		21: {21},
		31: {31},
		41: {41},
	}
	if len(payloads) != len(expected) {
		t.Fatalf("preserved or unconfigured jobs entered cleanup: %+v", payloads)
	}
	for jobID, ids := range expected {
		payload := payloads[jobID]
		if payload.JobId != jobID || payload.CleanupOperationNo != "unpublish:scope" ||
			!reflect.DeepEqual(payload.CleanupJobIds, ids) {
			t.Fatalf("job %d received another owner's context: %+v", jobID, payload)
		}
	}
	// Query order and duplicate rows cannot change an immutable intent's set.
	reversed := append([]telegramResubmitJob(nil), jobs...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	again, err := buildProfileDownCleanupPayloads(reversed, policies, "unpublish:scope")
	if err != nil || !reflect.DeepEqual(payloads, again) {
		t.Fatalf("scan order changed cleanup context: %+v, err=%v", again, err)
	}
	payloads[11].CleanupJobIds[0] = 999
	if payloads[12].CleanupJobIds[0] != 11 {
		t.Fatal("one job's payload aliases a sibling's immutable set")
	}
}

func TestProfileDownCleanupContextRetryKeepsOriginalSet(t *testing.T) {
	jobs := []telegramResubmitJob{
		{Id: 11, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 12, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 20},
	}
	first, err := buildProfileDownCleanupPayloads(jobs, map[int64]bool{10: false, 20: false}, "unpublish:retry")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(first[12])
	if err != nil {
		t.Fatal(err)
	}
	intent := telegramCleanupIntent{
		Id: 91, JobId: 12, EventType: telegramOutboxCleanupPrefix + "manual",
		OperationNo: "unpublish:retry", Payload: string(body),
	}
	// Job 11 has already disappeared from the active-message scan. The same
	// case also covers a sibling channel becoming preserved or unconfigured.
	retry, err := buildProfileDownCleanupPayloads(jobs[1:], map[int64]bool{20: false}, "unpublish:retry")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoreProfileDownCleanupPayload(retry[12], intent)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CleanupOutboxId != 91 || restored.JobId != 12 ||
		!reflect.DeepEqual(restored.CleanupJobIds, first[12].CleanupJobIds) {
		t.Fatalf("retry shrank or rebound persisted context: %+v", restored)
	}
	if !equalTelegramCleanupJobIDs(restored.CleanupJobIds, first[12].CleanupJobIds) ||
		restored.ForceCompletion != first[12].ForceCompletion {
		t.Fatal("retry would still conflict with ensureTelegramCleanupIntent")
	}
	if intent.Payload != string(body) {
		t.Fatal("retry rewrote the persisted intent")
	}
}

func TestProfileDownCleanupContextRetainsLegacyPayload(t *testing.T) {
	candidate := tgQueuePayload{JobId: 12, CleanupOperationNo: "unpublish:legacy", CleanupJobIds: []int64{11, 12}}
	for _, body := range []string{"", `{"jobId":12,"cleanupOperationNo":"unpublish:legacy","forceCompletion":true}`} {
		intent := telegramCleanupIntent{
			Id: 92, JobId: 12, EventType: telegramOutboxCleanupPrefix + "manual",
			OperationNo: "unpublish:legacy", Payload: body,
		}
		payload, err := restoreProfileDownCleanupPayload(candidate, intent)
		if err != nil {
			t.Fatal(err)
		}
		if payload.CleanupOutboxId != 92 || len(payload.CleanupJobIds) != 0 ||
			payload.ForceCompletion != (body != "") {
			t.Fatalf("legacy payload was upgraded or lost context: %+v", payload)
		}
	}
}

func TestProfileDownCleanupContextRejectsWrongIntent(t *testing.T) {
	candidate := tgQueuePayload{JobId: 12, CleanupOperationNo: "unpublish:A", CleanupJobIds: []int64{12}}
	valid := telegramCleanupIntent{
		Id: 93, JobId: 12, EventType: telegramOutboxCleanupPrefix + "manual",
		OperationNo: "unpublish:A", Payload: `{"jobId":12,"cleanupOperationNo":"unpublish:A"}`,
	}
	cases := map[string]telegramCleanupIntent{}
	row := valid
	row.Id = 0
	cases["missing_identity"] = row
	row = valid
	row.JobId = 13
	cases["other_job"] = row
	row = valid
	row.OperationNo = "unpublish:B"
	cases["other_operation"] = row
	row = valid
	row.EventType = telegramOutboxCleanupPrefix + "cycle"
	cases["other_event"] = row
	row = valid
	row.Payload = `{"jobId":13}`
	cases["payload_job_mismatch"] = row
	row = valid
	row.Payload = `{"cleanupOperationNo":"unpublish:B"}`
	cases["payload_operation_mismatch"] = row
	row = valid
	row.Payload = `{broken`
	cases["corrupt_payload"] = row
	for name, intent := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := restoreProfileDownCleanupPayload(candidate, intent); err == nil {
				t.Fatal("invalid persisted context accepted")
			}
		})
	}
}

func TestProfileDownCleanupContextRejectsConflictingOwner(t *testing.T) {
	jobs := []telegramResubmitJob{
		{Id: 11, TenantId: 1, AccountId: 2, ProfileId: 3, ChannelId: 10},
		{Id: 11, TenantId: 1, AccountId: 2, ProfileId: 4, ChannelId: 10},
	}
	if _, err := buildProfileDownCleanupPayloads(jobs, map[int64]bool{10: false}, "unpublish:A"); err == nil {
		t.Fatal("one job ID was accepted under conflicting owners")
	}
	if _, err := buildProfileDownCleanupPayloads(jobs[:1], map[int64]bool{10: false}, " "); err == nil {
		t.Fatal("missing operation was accepted")
	}
}
