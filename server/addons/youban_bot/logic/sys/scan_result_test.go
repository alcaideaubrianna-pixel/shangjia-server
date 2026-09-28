package sys

import (
	"testing"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
)

func TestScanResultButtonRowIncludesSourceLink(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{
		Id: 528043, ProfileNo: "K99559", CollectSourceUrl: "https://t.me/bikebaoyang/6457",
	}}
	row := scanResultButtonRow("QYDDENFLY0", note, "K99559")
	if len(row) != 2 {
		t.Fatalf("button count = %d, want 2", len(row))
	}
	if row[0].CallbackData != "scan:view:QYDDENFLY0:528043" {
		t.Fatalf("view callback = %q", row[0].CallbackData)
	}
	if row[1].Text != "来源频道 >" || row[1].URL != note.CollectSourceUrl {
		t.Fatalf("source button = %#v", row[1])
	}
}

func TestScanResultButtonRowOmitsMissingSourceLink(t *testing.T) {
	note := &publishsysin.NoteModel{ProfileModel: publishsysin.ProfileModel{Id: 1, ProfileNo: "A10001"}}
	row := scanResultButtonRow("TOKEN", note, "A10001")
	if len(row) != 1 {
		t.Fatalf("button count = %d, want 1", len(row))
	}
}
