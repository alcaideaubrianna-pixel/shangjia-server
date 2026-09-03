package sys

import "testing"

func TestCollectEventCleanupStatusesKeepProcessedDedupeHistory(t *testing.T) {
	for _, status := range collectEventCleanupStatuses {
		if status == "processed" {
			t.Fatal("processed collect events contain persistent dedupe data and must not be cleaned")
		}
	}
}
