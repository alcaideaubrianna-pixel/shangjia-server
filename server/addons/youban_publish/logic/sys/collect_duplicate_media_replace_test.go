package sys

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectDuplicateReasonMarksMediaReplacement(t *testing.T) {
	duplicate := &profileFingerprintDuplicateError{ProfileID: 12, ChannelID: 34, Layer: "text_hash"}
	plain := collectDuplicateReason(duplicate, false)
	replaced := collectDuplicateReason(duplicate, true)
	if strings.Contains(plain, "媒体已覆盖") {
		t.Fatalf("plain duplicate reason contains replacement marker: %s", plain)
	}
	if !strings.Contains(replaced, "媒体覆盖") || !strings.Contains(replaced, "profileId:12") {
		t.Fatalf("replacement reason missing audit fields: %s", replaced)
	}
}

func TestDecideDuplicateMediaReplace(t *testing.T) {
	const eventID int64 = 42
	tests := []struct {
		name   string
		marker int64
		want   duplicateMediaReplaceDecision
	}{
		{name: "never replaced", marker: 0, want: duplicateMediaReplaceAllowed},
		{name: "same event retries publish", marker: eventID, want: duplicateMediaReplaceRetryPublish},
		{name: "another event cannot replace again", marker: 41, want: duplicateMediaReplaceAlreadyUsed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideDuplicateMediaReplace(tt.marker, eventID); got != tt.want {
				t.Fatalf("decision=%d, want=%d", got, tt.want)
			}
		})
	}
}

func TestDuplicateMediaSnapshotsEqual(t *testing.T) {
	oldRows := gdb.Result{
		gdb.Record{"media_type": g.NewVar("image"), "purpose": g.NewVar("display"), "md5": g.NewVar("AA")},
		gdb.Record{"media_type": g.NewVar("video"), "purpose": g.NewVar("display"), "md5": g.NewVar("BB")},
	}
	same := []collectPreparedMedia{
		{MediaType: "image", Purpose: "display", MD5: "aa"},
		{MediaType: "video", Purpose: "display", MD5: "bb"},
	}
	reordered := []collectPreparedMedia{same[1], same[0]}
	changed := append([]collectPreparedMedia(nil), same...)
	changed[1].MD5 = "cc"
	oldDisplay := storedDuplicateMediaByPurpose(oldRows, collectMaterialRoleDisplay)
	if !duplicateMediaSnapshotsEqual(oldDisplay, preparedDuplicateMediaByPurpose(same, collectMaterialRoleDisplay)) {
		t.Fatal("identical display media must not be replaced")
	}
	if duplicateMediaSnapshotsEqual(oldDisplay, preparedDuplicateMediaByPurpose(reordered, collectMaterialRoleDisplay)) {
		t.Fatal("media order changes the published cover/order and must be replaced")
	}
	if duplicateMediaSnapshotsEqual(oldDisplay, preparedDuplicateMediaByPurpose(changed, collectMaterialRoleDisplay)) {
		t.Fatal("changed media must be replaced")
	}
}

func TestMissingPreparedVerificationKeepsStoredVerification(t *testing.T) {
	oldRows := gdb.Result{
		gdb.Record{"media_type": g.NewVar("image"), "purpose": g.NewVar("display"), "md5": g.NewVar("display")},
		gdb.Record{"media_type": g.NewVar("video"), "purpose": g.NewVar("verify"), "md5": g.NewVar("verify")},
	}
	prepared := []collectPreparedMedia{{MediaType: "image", Purpose: "display", MD5: "new-display"}}
	plan := buildDuplicateMediaReplacePlan(oldRows, prepared)
	if !plan.DisplayChanged {
		t.Fatal("changed display media must be replaced")
	}
	if plan.VerifyChanged {
		t.Fatal("missing prepared verification must retain stored verification")
	}
}

func TestPreparedVerificationOnlyReplacesWhenChanged(t *testing.T) {
	oldRows := gdb.Result{
		gdb.Record{"media_type": g.NewVar("image"), "purpose": g.NewVar("display"), "md5": g.NewVar("display")},
		gdb.Record{"media_type": g.NewVar("video"), "purpose": g.NewVar("verify"), "md5": g.NewVar("verify")},
	}
	same := []collectPreparedMedia{
		{MediaType: "image", Purpose: "display", MD5: "display"},
		{MediaType: "video", Purpose: "verify", MD5: "verify"},
	}
	if plan := buildDuplicateMediaReplacePlan(oldRows, same); plan.DisplayChanged || plan.VerifyChanged {
		t.Fatalf("identical media unexpectedly changed: %#v", plan)
	}
	changed := append([]collectPreparedMedia(nil), same...)
	changed[1].MD5 = "new-verify"
	if plan := buildDuplicateMediaReplacePlan(oldRows, changed); plan.DisplayChanged || !plan.VerifyChanged {
		t.Fatalf("verification-only change plan=%#v", plan)
	}
}
