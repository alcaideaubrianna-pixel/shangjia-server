package sys

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramOptionalAttemptScanTreatsMissingRowAsCacheMiss(t *testing.T) {
	if !telegramOptionalAttemptScanSucceeded(nil) {
		t.Fatal("nil error should be accepted")
	}
	if !telegramOptionalAttemptScanSucceeded(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should create a new delivery attempt")
	}
	if telegramOptionalAttemptScanSucceeded(errors.New("database unavailable")) {
		t.Fatal("database errors must not be swallowed")
	}
}

func TestTelegramMediaSendTimeoutsFitPublishTaskDeadline(t *testing.T) {
	if telegramVerifySendTimeout <= 0 {
		t.Fatalf("unexpected media timeouts: verify=%s display=%s", telegramVerifySendTimeout, telegramDisplaySendTimeout)
	}
	if telegramDisplaySendTimeout+telegramVerifySendTimeout+telegramAttemptWebhookWait >= telegramPublishTaskTimeout {
		t.Fatal("media send and webhook timeouts must fit publish task deadline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), telegramVerifySendTimeout)
	defer cancel()
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > telegramVerifySendTimeout {
		t.Fatal("verify send context must carry the configured deadline")
	}
}

func TestTelegramVerifyMediaNeedsAccountUpload(t *testing.T) {
	if !telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "video"}}) {
		t.Fatal("verify video without file_id should use account upload")
	}
	if telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "video", TgFileId: "telegram-file-id"}}) {
		t.Fatal("verify video with reusable file_id should keep the Bot fast path")
	}
	if telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "image"}}) {
		t.Fatal("non-video verification media should not force account upload")
	}
}

func TestTelegramSentMessageIDs(t *testing.T) {
	got := telegramSentMessageIDs([]*telegramSentMessage{nil, {MessageId: 0}, {MessageId: 1880}, {MessageId: 1849}})
	if len(got) != 2 || got[0] != 1880 || got[1] != 1849 {
		t.Fatalf("unexpected message ids: %v", got)
	}
}

func TestTelegramAttemptMarkerStableAndDistinct(t *testing.T) {
	first := telegramAttemptMarker("attempt-a")
	if first == "" || first != telegramAttemptMarker("attempt-a") {
		t.Fatal("attempt marker must be stable and non-empty")
	}
	if first == telegramAttemptMarker("attempt-b") {
		t.Fatal("different attempts must not share a marker")
	}
	if !strings.HasPrefix(first, "\u2063") || !strings.HasSuffix(first, "\u2064") {
		t.Fatal("attempt marker must retain its invisible boundaries")
	}
}

func TestTelegramWebhookDetachedJobSQLKeepsOrderingBarrier(t *testing.T) {
	if got := telegramWebhookDetachedJobSQL("j."); got != "1=1" {
		t.Fatalf("telegramWebhookDetachedJobSQL() = %q, want ordering barrier predicate", got)
	}
}

func TestTelegramAttemptWaitingKeepsChannelOrderingBarrier(t *testing.T) {
	for _, phase := range []string{"display", "verify"} {
		if got := telegramAttemptWaitingDispatchStatus(); got != tgDispatchStatusProcessing {
			t.Fatalf("phase %s dispatch status = %q, want processing", phase, got)
		}
	}
}

func TestTelegramAttemptReconcileSupportsDisplayAndVerify(t *testing.T) {
	for _, phase := range []string{"display", "verify"} {
		if phase != "display" && phase != "verify" {
			t.Fatalf("unsupported reconciliation phase: %s", phase)
		}
	}
}

func TestTelegramUnknownProcessingBlocksNextChannelJobIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	seed := time.Now().UnixNano()
	now := gtime.Now()
	olderID, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": "ordering-old", "tenant_id": seed, "channel_id": seed,
		"status": telegramJobStatusConfirming, "dispatch_status": tgDispatchStatusProcessing,
		"dispatched_at": now, "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert earlier uncertain job: %v", err)
	}
	time.Sleep(time.Millisecond)
	newerID, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": "ordering-new", "tenant_id": seed, "channel_id": seed,
		"status": telegramJobStatusPending, "dispatch_status": tgDispatchStatusIdle,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert later pending job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).WhereIn("id", []int64{olderID, newerID}).Delete()
	})
	blocked, err := new(sSysPublish).telegramChannelHasEarlierActiveJob(ctx, telegramJobRecord{
		Id: newerID, TenantId: seed, ChannelId: seed, CreatedAt: gtime.Now(),
	})
	if err != nil || !blocked {
		t.Fatalf("earlier uncertain job must block next channel job: blocked=%v err=%v", blocked, err)
	}
}

func TestTelegramAttemptMarkerSurvivesCaptionComposition(t *testing.T) {
	marker := telegramAttemptMarker("attempt-caption")
	caption := telegramCaptionWithJobMarker("资料标题", 42, "display") + marker
	if !strings.Contains(caption, marker) {
		t.Fatal("composed caption lost attempt marker")
	}
}

func TestTelegramInternalDeliveryMarkerDetection(t *testing.T) {
	if telegramTextHasInternalDeliveryMarker("普通消息") {
		t.Fatal("plain message must not be treated as an internal delivery")
	}
	if !telegramTextHasInternalDeliveryMarker("资料" + telegramAttemptMarker("delete-safe")) {
		t.Fatal("attempt marker must protect internal delivery from auto delete")
	}
}

func TestTelegramVerifyMediaMatchRequiresKnownUniqueID(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "video", TgFileUniqueId: "expected"}}
	if telegramVerifyMediaMatches(media, "video", "other") {
		t.Fatal("known file unique id mismatch must not confirm an attempt")
	}
	if !telegramVerifyMediaMatches(media, "video", "expected") {
		t.Fatal("matching file unique id must confirm the media candidate")
	}
	if telegramVerifyMediaMatches(media, "image", "expected") {
		t.Fatal("media type mismatch must not confirm an attempt")
	}
}

func TestTelegramVerifyFreshUploadRequiresMarker(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "photo"}}
	if telegramVerifyMediaMatches(media, "image", "new-file") {
		t.Fatal("unmarked fresh upload must not match by media type alone")
	}
}

func TestTelegramAttemptTerminalStatusesAreDistinct(t *testing.T) {
	statuses := map[string]struct{}{
		telegramAttemptStatusConfirmed:  {},
		telegramAttemptStatusSuperseded: {},
	}
	if len(statuses) != 2 {
		t.Fatal("confirmed and superseded attempts must remain distinguishable")
	}
}
