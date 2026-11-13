package sys

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfileMediaRecoveryUsesContentProfileForProfileNo(t *testing.T) {
	raw, err := os.ReadFile("profile_media_operation.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "cp.profile_no=o.profile_no") {
		t.Fatal("media recovery must resolve profile_no through hg_content_profile")
	}
	if strings.Contains(source, " p.profile_no=o.profile_no") {
		t.Fatal("profile_state does not contain profile_no; recovery must not join it directly")
	}
}

func TestProfileMediaOperationPayloadRoundTrip(t *testing.T) {
	want := profileMediaOperationInput{
		TenantId:  7,
		AccountId: 9,
		ProfileNo: "A12345",
		TraceId:   "trace-1",
		DisplayMedia: []*sysin.MessageTemplateMediaInp{{
			MediaType: "image", TgFileId: "file-1", TgFileUniqueId: "unique-1", AssetHash: "md5-1", SortIndex: 2,
		}},
		VerifyMedia: []*sysin.MessageTemplateMediaInp{{
			MediaType: "video", TgFileId: "file-2", TgFileUniqueId: "unique-2", StoragePath: "media/video.mp4", SortIndex: 1,
		}},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got profileMediaOperationInput
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.TenantId != want.TenantId || got.AccountId != want.AccountId || got.ProfileNo != want.ProfileNo || got.TraceId != want.TraceId {
		t.Fatalf("operation metadata changed after round trip: got=%+v want=%+v", got, want)
	}
	if len(got.DisplayMedia) != 1 || got.DisplayMedia[0].TgFileUniqueId != "unique-1" || got.DisplayMedia[0].AssetHash != "md5-1" {
		t.Fatalf("display media payload was not preserved: %+v", got.DisplayMedia)
	}
	if len(got.VerifyMedia) != 1 || got.VerifyMedia[0].MediaType != "video" || got.VerifyMedia[0].StoragePath != "media/video.mp4" {
		t.Fatalf("verify media payload was not preserved: %+v", got.VerifyMedia)
	}
}
