package sys

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTelegramShutdownHandoffOwnedCheckpointLocalPostgres(t *testing.T) {
	db := missingReadyLocalDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, "CREATE TABLE "+publishTgJobTable+
		" (id bigint PRIMARY KEY,channel_id bigint,status text,last_dispatch_error text)"); err != nil {
		t.Fatal(err)
	}
	const channelID, jobID int64 = 701674, 703342143
	if _, err := db.Exec(ctx, "INSERT INTO "+publishTgJobTable+" VALUES(?,?,'unknown','')", jobID, channelID); err != nil {
		t.Fatal(err)
	}
	if err := enqueueTelegramChannelJob(ctx, channelID, jobID, 10, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	claim, err := claimTelegramChannelJob(ctx, channelID)
	if err != nil || claim.JobID != jobID {
		t.Fatalf("claim=%+v err=%v", claim, err)
	}
	stale := claim
	stale.Token = "obsolete-token"
	publisher := new(sSysPublish)
	publisher.persistTelegramShutdownHandoff(ctx, stale)
	marker, err := db.GetValue(ctx, "SELECT last_dispatch_error FROM "+publishTgJobTable+" WHERE id=?", jobID)
	if err != nil || marker.String() != "" {
		t.Fatalf("stale owner wrote handoff checkpoint: %v err=%v", marker, err)
	}
	publisher.persistTelegramShutdownHandoff(ctx, claim)
	marker, err = db.GetValue(ctx, "SELECT last_dispatch_error FROM "+publishTgJobTable+" WHERE id=?", jobID)
	if err != nil || !strings.HasPrefix(marker.String(), telegramShutdownHandoffPrefix) {
		t.Fatalf("owned checkpoint missing: %v err=%v", marker, err)
	}
	first := marker.String()
	publisher.persistTelegramShutdownHandoff(ctx, claim)
	marker, err = db.GetValue(ctx, "SELECT last_dispatch_error FROM "+publishTgJobTable+" WHERE id=?", jobID)
	if err != nil || marker.String() != first {
		t.Fatal("repeated handoff extended the confirmation deadline")
	}
	// Persisting the checkpoint must not ACK or remove the original claim.
	if ok, err := settleTelegramChannelClaim(ctx, claim, false, 10, time.Now()); err != nil || !ok {
		t.Fatalf("checkpoint changed claim ownership: ok=%v err=%v", ok, err)
	}
}
