package sys

import (
	"bytes"
	"encoding/json"
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

func TestDuplicateImageSignatureFallsBackToPHash(t *testing.T) {
	left, ok := duplicateImageSignature([]duplicateImageRow{{PerceptualHash: "d87a07c29151f7c5"}, {PerceptualHash: "cc2af3518c676c93"}})
	right, rightOK := duplicateImageSignature([]duplicateImageRow{{PerceptualHash: "CC2AF3518C676C93"}, {PerceptualHash: "d87a07c29151f7c5"}})
	if !ok || !rightOK || left != right {
		t.Fatalf("pHash fallback must be complete and order independent: %q != %q", left, right)
	}
}

func TestDuplicateProfileSignatureUsesNormalizedText(t *testing.T) {
	left, ok := duplicateProfileSignature("介绍费：7888\nB2", nil)
	right, rightOK := duplicateProfileSignature("介绍费：7888\u200b\nB2", nil)
	if !ok || !rightOK || left != right || len(left) < len("text:") || left[:len("text:")] != "text:" {
		t.Fatalf("normalized text must produce the same signature: %q != %q", left, right)
	}
}

func TestProfilePHashSetsMatchIgnoresOrderAndAllowsReencodingDistance(t *testing.T) {
	left := []string{"d87a07c29151f7c5", "cc2af3518c676c93", "dd0670e46e58e9a6"}
	right := []string{"f81e726076528fa6", "d87a07c3912ff2c1", "cc34b3798cd44e69"}
	if !profilePHashSetsMatch(left, right, 20) {
		t.Fatal("expected the re-encoded image set to match within distance 20")
	}
	if profilePHashSetsMatch(left, right[:2], 20) {
		t.Fatal("different image counts must not match")
	}
}

func TestProfilePHashSetsMatchRejectsPartialImageOverlap(t *testing.T) {
	left := []string{"0000000000000000", "ffffffffffffffff"}
	right := []string{"0000000000000001", "0000000000000002"}
	if profilePHashSetsMatch(left, right, collectProfilePHashDuplicateThreshold) {
		t.Fatal("多图资料只有一张相似时不应判定为整套重复")
	}
}

func TestCollectPHashCandidateRecallUsesLSHRange(t *testing.T) {
	if collectProfilePHashCandidateThreshold > 12 {
		t.Fatalf("实时采集候选阈值 %d 会绕过 LSH 索引", collectProfilePHashCandidateThreshold)
	}
	if collectProfilePHashCandidateThreshold > collectProfilePHashDuplicateThreshold {
		t.Fatalf("候选阈值 %d 不应大于最终校验阈值 %d", collectProfilePHashCandidateThreshold, collectProfilePHashDuplicateThreshold)
	}
}

func TestDuplicateScanChunkSizeStaysBelowPostgresParameterLimit(t *testing.T) {
	if duplicateScanChunkSize <= 0 || duplicateScanChunkSize > 5000 {
		t.Fatalf("重复扫描分批大小 %d 可能产生超量 SQL 参数", duplicateScanChunkSize)
	}
}

func TestLegacyPHashQueryHasResourceLimits(t *testing.T) {
	if cap(mediaPHashLegacyQuerySlots) != 1 {
		t.Fatalf("旧索引查询并发 = %d, want 1", cap(mediaPHashLegacyQuerySlots))
	}
	if mediaPHashLegacyStatementTimeout == "" {
		t.Fatal("旧索引查询必须设置 statement_timeout")
	}
}

func TestProfilePHashGroupRejectsTransitiveSimilarityChain(t *testing.T) {
	sets := map[int64][]string{
		1: {"0000000000000000"},
		2: {"00000000000fffff"},
	}
	third := []string{"000000ffffffffff"}
	if !profilePHashSetsMatch(sets[1], sets[2], 20) || !profilePHashSetsMatch(sets[2], third, 20) {
		t.Fatal("test fixture must form an A~B~C similarity chain")
	}
	if profilePHashSetsMatch(sets[1], third, 20) {
		t.Fatal("test fixture endpoints must not be similar")
	}
	if profilePHashGroupAccepts(third, []int64{1, 2}, sets, 20) {
		t.Fatal("transitive similarity must not merge profiles that do not all match")
	}
}

func TestDuplicateScanCacheLifetimes(t *testing.T) {
	if duplicateScanSessionTTL != 24*time.Hour {
		t.Fatalf("扫描任务有效期 = %s, want 24h", duplicateScanSessionTTL)
	}
	if duplicateScanSessionTTL <= duplicateScanResultTTL {
		t.Fatal("运行中任务索引必须覆盖完整扫描周期")
	}
	if duplicateScanResultTTL > 30*time.Minute {
		t.Fatalf("扫描结果复用索引有效期过长: %s", duplicateScanResultTTL)
	}
}

func TestPHashCandidateCacheIsShortAndBounded(t *testing.T) {
	if mediaPHashBucketResultTTL > 10*time.Minute {
		t.Fatalf("pHash candidate cache TTL is too long: %s", mediaPHashBucketResultTTL)
	}
	if mediaPHashBucketMaxCachedRows > 2000 {
		t.Fatalf("pHash candidate cache row limit is too large: %d", mediaPHashBucketMaxCachedRows)
	}
}

func TestDuplicateScanSessionRejectsPreviousAlgorithm(t *testing.T) {
	account := &sysin.AccountModel{Id: 7, TenantId: 6}
	session := newDuplicateScanSession(account)
	if err := validateDuplicateScanSessionOwner(session, account); err != nil {
		t.Fatalf("current scan session must remain valid: %v", err)
	}
	session.AlgorithmVersion--
	if err := validateDuplicateScanSessionOwner(session, account); err == nil {
		t.Fatal("previous scan algorithm session must be rejected")
	}
}

func TestDuplicateScanSessionCompressionRoundTrip(t *testing.T) {
	session := &duplicateScanSession{
		AlgorithmVersion: duplicateScanAlgorithmVersion,
		AdminAccountId:   7,
		TenantId:         6,
		ScanCursor:       100000,
		ScannedTotal:     100000,
	}
	plain, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := encodeDuplicateScanSession(session)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeDuplicateScanSession(compressed)
	if err != nil || decoded.AdminAccountId != session.AdminAccountId || decoded.ScanCursor != session.ScanCursor {
		t.Fatalf("compressed session round trip failed: session=%#v err=%v", decoded, err)
	}
	legacy, err := decodeDuplicateScanSession(plain)
	if err != nil || legacy.TenantId != session.TenantId {
		t.Fatalf("legacy JSON session must remain readable: session=%#v err=%v", legacy, err)
	}
}

func TestDuplicateScanMetadataStaysBounded(t *testing.T) {
	session := newDuplicateScanSession(&sysin.AccountModel{Id: 7, TenantId: 6})
	session.ScannedTotal = 100000
	data, err := encodeDuplicateScanSession(duplicateScanMetadata(session))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 1024 {
		t.Fatalf("progress metadata must stay bounded for 100k profiles: %d bytes", len(data))
	}
	for _, forbidden := range []string{"signatureIds", "pHashBuckets", "pHashSets", "pHashGroups"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("session metadata must not contain %s", forbidden)
		}
	}
}

func TestDuplicateScanWorkGroupKeepsNewestProfile(t *testing.T) {
	group := &duplicateScanWorkGroup{KeepProfileId: 10, KeepCreatedAt: 100, MemberIds: []int64{10}}
	group.addMember(20, 90)
	group.addMember(30, 110)
	group.addMember(30, 110)
	if group.KeepProfileId != 30 || len(group.MemberIds) != 3 || group.MemberIds[0] != 30 {
		t.Fatalf("unexpected work group ordering: %#v", group)
	}
}

func TestDuplicateScanTaskHasBoundedRuntime(t *testing.T) {
	if duplicateScanPagesPerTask <= 0 || duplicateScanPagesPerTask > 20 {
		t.Fatalf("pages per task must stay bounded: %d", duplicateScanPagesPerTask)
	}
}

func TestDuplicatePHashScanBucketsBoundLookupCost(t *testing.T) {
	keys := duplicatePHashBucketKeys([]string{"d87a07c29151f7c5"}, true)
	if len(keys) != 8*37 {
		t.Fatalf("unexpected scan LSH probe count: %d", len(keys))
	}
	exact := duplicatePHashBucketKeys([]string{"d87a07c29151f7c5"}, false)
	if len(exact) != 8 {
		t.Fatalf("unexpected exact scan bucket count: %d", len(exact))
	}
}

func TestScheduledBatchCycleRunUsesScheduledAt(t *testing.T) {
	if scheduledBatchCycleRun(cycleRunRecord{Stage: "producing"}) {
		t.Fatal("手动循环不应识别为定时批次")
	}
	if !scheduledBatchCycleRun(cycleRunRecord{Stage: "producing", ScheduledAt: gtime.Now()}) {
		t.Fatal("批次 stage 被覆盖后仍应通过 scheduled_at 识别")
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
	if err := validateDuplicateCleanupState([]int64{1}, candidates, profiles, map[int64]string{1: "changed", 2: "same"}, nil); err == nil {
		t.Fatal("expected changed target signature to be rejected")
	}
}

func TestValidateDuplicateCleanupStateRejectsMissingKeep(t *testing.T) {
	candidates := map[int64]duplicateScanCandidate{1: {ProfileId: 1, KeepProfileId: 2, Signature: "same"}}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{1: {Id: 1}}
	if err := validateDuplicateCleanupState([]int64{1}, candidates, profiles, map[int64]string{1: "same"}, nil); err == nil {
		t.Fatal("expected missing retained profile to be rejected")
	}
}

func TestValidateDuplicateCleanupStateKeepsTextAndPHashIndependent(t *testing.T) {
	keepTime := gtime.New(time.Unix(20, 0))
	targetTime := gtime.New(time.Unix(10, 0))
	candidates := map[int64]duplicateScanCandidate{
		1: {ProfileId: 1, KeepProfileId: 3, Signature: "text:same"},
		2: {ProfileId: 2, KeepProfileId: 3, Signature: "phash:3"},
	}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{
		1: {Id: 1, CreatedAt: targetTime},
		2: {Id: 2, CreatedAt: targetTime},
		3: {Id: 3, CreatedAt: keepTime},
	}
	signatures := map[int64]string{1: "text:same", 2: "text:other", 3: "text:same"}
	if err := validateDuplicateCleanupState([]int64{1, 2}, candidates, profiles, signatures, map[int64]bool{2: true}); err != nil {
		t.Fatalf("text and pHash candidates sharing a keep must validate independently: %v", err)
	}
}

func TestValidateDuplicateCleanupStateRequiresNewerKeep(t *testing.T) {
	sameTime := gtime.New(time.Unix(10, 0))
	candidates := map[int64]duplicateScanCandidate{2: {ProfileId: 2, KeepProfileId: 1, Signature: "same"}}
	profiles := map[int64]*sysin.AdminNoteDuplicateItemModel{
		1: {Id: 1, CreatedAt: sameTime},
		2: {Id: 2, CreatedAt: sameTime},
	}
	if err := validateDuplicateCleanupState([]int64{2}, candidates, profiles, map[int64]string{1: "same", 2: "same"}, nil); err == nil {
		t.Fatal("expected older retained profile to be rejected")
	}
}

func TestValidateDuplicateScanSessionOwner(t *testing.T) {
	session := &duplicateScanSession{AlgorithmVersion: duplicateScanAlgorithmVersion, TenantId: 7, AdminAccountId: 8}
	if err := validateDuplicateScanSessionOwner(session, &sysin.AccountModel{TenantId: 7, Id: 8}); err != nil {
		t.Fatalf("expected matching owner: %v", err)
	}
	if err := validateDuplicateScanSessionOwner(session, &sysin.AccountModel{TenantId: 7, Id: 9}); err == nil {
		t.Fatal("expected another admin account to be rejected")
	}
}

func TestDuplicateScanPreviousAlgorithmCanOnlyRestartForOwner(t *testing.T) {
	owner := &sysin.AccountModel{TenantId: 7, Id: 8}
	session := &duplicateScanSession{AlgorithmVersion: duplicateScanAlgorithmVersion - 1, TenantId: 7, AdminAccountId: 8}
	if !duplicateScanSessionOwnedBy(session, owner) {
		t.Fatal("previous algorithm session must remain identifiable by its owner")
	}
	if duplicateScanSessionOwnedBy(session, &sysin.AccountModel{TenantId: 7, Id: 9}) {
		t.Fatal("another account must not restart an owned scan session")
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
