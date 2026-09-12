package sys

import (
	"testing"

	"github.com/hibiken/asynq"
)

func TestDecodeProfileProjectionQueuePayload(t *testing.T) {
	payload := []byte(`{"profileId":12,"tenantId":3,"accountId":7,"mediaChanged":true,"removedMediaIds":[8],"oldFingerprints":[{"channelId":9,"layer":"text","signature":"abc","itemTotal":1,"signatureCount":1}]}`)
	got, err := decodeProfileProjectionQueuePayload(asynq.NewTask(tgTaskTypeProfileProjection, payload))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got.ProfileId != 12 || got.TenantId != 3 || got.AccountId != 7 || !got.MediaChanged {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if len(got.OldFingerprints) != 1 || got.OldFingerprints[0].ChannelID != 9 {
		t.Fatalf("unexpected fingerprints: %+v", got.OldFingerprints)
	}
}

func TestDecodeProfileProjectionQueuePayloadRejectsInvalidTask(t *testing.T) {
	if _, err := decodeProfileProjectionQueuePayload(asynq.NewTask(tgTaskTypeProfileProjection, []byte(`{"profileId":12}`))); err == nil {
		t.Fatal("expected invalid payload error")
	}
}
