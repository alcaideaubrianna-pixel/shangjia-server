package sys

import "testing"

func TestTgMessageRepairOperationNoScopesByProfileAndChannel(t *testing.T) {
	first := tgMessageRepairOperationNo(101, 202)
	if first != "message_repair:101:202" {
		t.Fatalf("unexpected repair operation number: %q", first)
	}
	if first == tgMessageRepairOperationNo(101, 203) {
		t.Fatal("repair operation number must distinguish channels")
	}
	if first == tgMessageRepairOperationNo(102, 202) {
		t.Fatal("repair operation number must distinguish profiles")
	}
}
