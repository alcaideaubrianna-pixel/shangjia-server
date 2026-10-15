package sys

import (
	"os"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestAIOpsRepairConfirmTokenIsStableAndBoundToPlan(t *testing.T) {
	previous := os.Getenv("YOUBAN_AI_OPS_TOKEN")
	t.Setenv("YOUBAN_AI_OPS_TOKEN", "test-secret")
	t.Cleanup(func() { _ = os.Setenv("YOUBAN_AI_OPS_TOKEN", previous) })
	expires := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	first, err := aiOpsRepairConfirmToken("plan-1", `{"eventIds":[1]}`, expires)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := aiOpsRepairConfirmToken("plan-1", `{"eventIds":[1]}`, expires)
	if first == "" || first != second {
		t.Fatalf("confirmation token is not stable: %q %q", first, second)
	}
	changed, _ := aiOpsRepairConfirmToken("plan-2", `{"eventIds":[1]}`, expires)
	if first == changed {
		t.Fatal("confirmation token must be bound to the plan")
	}
}

func TestAIOpsRepairConfirmTokenRequiresSecret(t *testing.T) {
	t.Setenv("YOUBAN_AI_OPS_TOKEN", "")
	if _, err := aiOpsRepairConfirmToken("plan", `{}`, time.Now()); err == nil {
		t.Fatal("missing secret must be rejected")
	}
}

func TestMissingCollectProcessOutboxError(t *testing.T) {
	for _, message := range []string{
		`relation "hg_youban_publish_collect_process_outbox" does not exist`,
		`Table 'db.hg_youban_publish_collect_process_outbox' doesn't exist`,
		`no such table: hg_youban_publish_collect_process_outbox`,
	} {
		if !isMissingCollectProcessOutboxError(assertionError(message)) {
			t.Fatalf("missing-table error not recognized: %s", message)
		}
	}
	if isMissingCollectProcessOutboxError(assertionError("connection refused")) {
		t.Fatal("non-schema database errors must not be ignored")
	}
}

func TestCollectProcessOutboxRecoveryBatchSizeSupportsLargeBacklog(t *testing.T) {
	got := collectProcessOutboxRecoveryBatchSize(t.Context())
	if got < 1000 || got > 10000 {
		t.Fatalf("unexpected recovery batch size: %d", got)
	}
}

func TestAIOpsRepairActionsIncludeProfileAndVerificationOperations(t *testing.T) {
	for _, action := range []string{
		"repair_missing_verification", "offline_profiles", "republish_profiles", "delete_profiles",
	} {
		if _, ok := aiOpsRepairActions[action]; !ok {
			t.Fatalf("missing controlled operation %q", action)
		}
	}
}

func TestPreviewMissingVerificationRejectsUnboundedRange(t *testing.T) {
	service := NewSysPublish()
	_, _, _, err := service.previewAIOpsMissingVerificationEvents(t.Context(), &sysin.AIOpsRepairPlanInp{
		StartAt: "2026-10-15T00:00:00+08:00",
		EndAt:   "2026-10-18T00:00:00+08:00",
	}, 100)
	if err == nil {
		t.Fatal("range over 48 hours must be rejected before querying the database")
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }
