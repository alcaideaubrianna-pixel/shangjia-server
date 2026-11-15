package sysin

import "testing"

func TestPublishTaskStatusUnpublishingIsValid(t *testing.T) {
	if !PublishTaskStatusUnpublishing.Valid() {
		t.Fatal("unpublishing must be a valid durable publish task status")
	}
	if PublishTaskStatusUnpublishing.String() != "unpublishing" {
		t.Fatalf("unexpected unpublishing value: %q", PublishTaskStatusUnpublishing.String())
	}
}
