package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestDuplicateImageSignature(t *testing.T) {
	left, ok := duplicateImageSignature([]duplicateImageRow{{Md5: "B"}, {Md5: "a"}, {Md5: "a"}})
	if !ok {
		t.Fatal("expected complete image signature")
	}
	right, ok := duplicateImageSignature([]duplicateImageRow{{Md5: "A"}, {Md5: "b"}, {Md5: "a"}})
	if !ok || left != right {
		t.Fatalf("image order must not affect signature: %q != %q", left, right)
	}
	different, ok := duplicateImageSignature([]duplicateImageRow{{Md5: "a"}, {Md5: "b"}})
	if !ok || different == left {
		t.Fatal("image multiplicity must affect signature")
	}
}

func TestDuplicateImageSignatureRejectsIncompleteProfiles(t *testing.T) {
	for _, rows := range [][]duplicateImageRow{nil, {{Md5: "a"}, {Md5: " "}}} {
		if _, ok := duplicateImageSignature(rows); ok {
			t.Fatalf("expected incomplete signature for %#v", rows)
		}
	}
}

func TestDuplicateScanBatchLimitsReturnedDeleteTargets(t *testing.T) {
	groups := []*sysin.AdminNoteDuplicateGroupModel{
		{Signature: "a", Keep: &sysin.AdminNoteDuplicateItemModel{Id: 9}, Duplicates: []*sysin.AdminNoteDuplicateItemModel{{Id: 8}, {Id: 7}}},
		{Signature: "b", Keep: &sysin.AdminNoteDuplicateItemModel{Id: 6}, Duplicates: []*sysin.AdminNoteDuplicateItemModel{{Id: 5}, {Id: 4}}},
	}
	batch := duplicateScanBatch(groups, 3)
	if len(batch) != 2 || len(batch[0].Duplicates) != 2 || len(batch[1].Duplicates) != 1 {
		t.Fatalf("unexpected batch: %#v", batch)
	}
	if batch[0].Keep.Id != 9 || batch[1].Keep.Id != 6 {
		t.Fatal("batch must retain each group's newest profile")
	}
}
