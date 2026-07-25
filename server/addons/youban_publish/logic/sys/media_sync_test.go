package sys

import "testing"

func TestTaskMediaSyncLockKeySerializesByProfile(t *testing.T) {
	first := taskMediaSyncLockKey(10, 99)
	second := taskMediaSyncLockKey(11, 99)
	if first != second {
		t.Fatalf("same profile must share a media sync lock: %q != %q", first, second)
	}
	if first == taskMediaSyncLockKey(10, 100) {
		t.Fatal("different profiles must not share a media sync lock")
	}
}
