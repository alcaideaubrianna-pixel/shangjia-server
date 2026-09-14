package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"

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

func TestValidateDuplicateCleanupStateRejectsChangedTargetSignature(t *testing.T) {
	keepTime := gtime.New(time.Unix(20, 0))
	targetTime := gtime.New(time.Unix(10, 0))
	candidates := map[int64]duplicateScanCandidate{1: {ProfileId: 1, KeepProfileId: 2, Signature: "same"}}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{
		1: {Id: 1, CreatedAt: targetTime},
		2: {Id: 2, CreatedAt: keepTime},
	}
	if err := validateDuplicateCleanupState([]int64{1}, candidates, profiles, map[int64]string{1: "changed", 2: "same"}); err == nil {
		t.Fatal("expected changed target signature to be rejected")
	}
}

func TestValidateDuplicateCleanupStateRejectsMissingKeep(t *testing.T) {
	candidates := map[int64]duplicateScanCandidate{1: {ProfileId: 1, KeepProfileId: 2, Signature: "same"}}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{1: {Id: 1}}
	if err := validateDuplicateCleanupState([]int64{1}, candidates, profiles, map[int64]string{1: "same"}); err == nil {
		t.Fatal("expected missing retained profile to be rejected")
	}
}

func TestValidateDuplicateCleanupStateRequiresNewerKeep(t *testing.T) {
	sameTime := gtime.New(time.Unix(10, 0))
	candidates := map[int64]duplicateScanCandidate{2: {ProfileId: 2, KeepProfileId: 1, Signature: "same"}}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{
		1: {Id: 1, CreatedAt: sameTime},
		2: {Id: 2, CreatedAt: sameTime},
	}
	if err := validateDuplicateCleanupState([]int64{2}, candidates, profiles, map[int64]string{1: "same", 2: "same"}); err == nil {
		t.Fatal("expected older retained profile to be rejected")
	}
}

func TestValidateDuplicateScanSessionOwner(t *testing.T) {
	session := &duplicateScanSession{TenantId: 7, AdminAccountId: 8}
	if err := validateDuplicateScanSessionOwner(session, &sysin.AccountModel{TenantId: 7, Id: 8}); err != nil {
		t.Fatalf("expected matching owner: %v", err)
	}
	if err := validateDuplicateScanSessionOwner(session, &sysin.AccountModel{TenantId: 7, Id: 9}); err == nil {
		t.Fatal("expected another admin account to be rejected")
	}
}

func TestDuplicateScanResultCacheKeyIgnoresPagination(t *testing.T) {
	account := &sysin.AccountModel{TenantId: 7, Id: 8}
	left := &sysin.NoteListInp{ProfileListInp: sysin.ProfileListInp{Keyword: "test"}}
	left.Page, left.PerPage, left.Pagination = 1, 20, true
	right := &sysin.NoteListInp{ProfileListInp: sysin.ProfileListInp{Keyword: "test"}}
	right.Page, right.PerPage, right.Pagination = 9, 500, false
	if duplicateScanResultCacheKey(account, left) != duplicateScanResultCacheKey(account, right) {
		t.Fatal("pagination must not produce another duplicate scan cache entry")
	}
	right.Keyword = "other"
	if duplicateScanResultCacheKey(account, left) == duplicateScanResultCacheKey(account, right) {
		t.Fatal("different filters must not share duplicate scan cache entries")
	}
	if duplicateScanResultCacheKey(account, left) == duplicateScanResultCacheKey(&sysin.AccountModel{TenantId: 7, Id: 9}, left) {
		t.Fatal("different admin accounts must not share duplicate scan cache entries")
	}
}

func TestRestrictDuplicateScanToEditableAccounts(t *testing.T) {
	s := &sSysPublish{}
	scope := &adminProfileVisibleScope{AccountIds: []int64{8, 9}, TenantId: 7, TenantIds: []int64{7, 10}}
	account := &sysin.AccountModel{Id: 8, TenantId: 7, AccountType: sysin.PublishAccountTypeUploader}
	if err := s.restrictDuplicateScanToEditableAccounts(t.Context(), scope, account); err != nil {
		t.Fatalf("restrict duplicate scan scope: %v", err)
	}
	if len(scope.AccountIds) != 1 || scope.AccountIds[0] != 8 {
		t.Fatalf("unexpected editable accounts: %#v", scope.AccountIds)
	}
	if len(scope.TenantIds) != 1 || scope.TenantIds[0] != 7 || !scope.Strict {
		t.Fatalf("unexpected editable tenant scope: %#v", scope)
	}
}
