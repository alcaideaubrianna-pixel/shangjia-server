package sys

import "testing"

func TestDuplicateVerificationRepairOperationNoIsStable(t *testing.T) {
	first := duplicateVerificationRepairOperationNo(652900, 4598733)
	second := duplicateVerificationRepairOperationNo(652900, 4598733)
	if first != "verify-repair:4598733:profile:652900" {
		t.Fatalf("unexpected operation no: %s", first)
	}
	if second != first {
		t.Fatalf("operation no must be stable: first=%s second=%s", first, second)
	}
	if !isManualProfilePublishOperation(first) {
		t.Fatal("verification repair must replace the incomplete channel publish")
	}
}

func TestDuplicateVerificationRepairOperationNoChangesByEvent(t *testing.T) {
	first := duplicateVerificationRepairOperationNo(652900, 4598733)
	second := duplicateVerificationRepairOperationNo(652900, 4598734)
	if first == second {
		t.Fatalf("different collection events must not share an operation: %s", first)
	}
}

func TestAIOpsRepublishReplacesIncompletePublish(t *testing.T) {
	if !isManualProfilePublishOperation("ai-republish:652900:1791600000000000000") {
		t.Fatal("AI operations republish must replace the incomplete channel publish")
	}
}
