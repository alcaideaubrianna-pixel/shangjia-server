package sys

import "testing"

func TestProfileMediaSyncLockKeySerializesByProfile(t *testing.T) {
	first := profileMediaSyncLockKey(99)
	if first == profileMediaSyncLockKey(100) {
		t.Fatal("different profiles must not share a media sync lock")
	}
}
