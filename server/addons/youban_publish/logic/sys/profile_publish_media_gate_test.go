package sys

import "testing"

func TestProfileMediaOperationPendingStatuses(t *testing.T) {
	statuses := profileMediaOperationPendingStatuses()
	if len(statuses) != 2 || statuses[0] != "pending" || statuses[1] != "processing" {
		t.Fatalf("unexpected pending media statuses: %#v", statuses)
	}
}
