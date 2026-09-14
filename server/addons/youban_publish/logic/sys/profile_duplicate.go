package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"

	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
)

const (
	duplicateScanChunkSize  = 500
	duplicateScanBatchSize  = 500
	duplicateScanSessionTTL = 30 * time.Minute
	duplicateScanResultTTL  = 10 * time.Minute
)

type duplicateImageRow struct {
	Id        int64  `orm:"id"`
	ProfileId int64  `orm:"profile_id"`
	Md5       string `orm:"md5"`
}

type duplicateScanCandidate struct {
	KeepProfileId int64  `json:"keepProfileId"`
	ProfileId     int64  `json:"profileId"`
	Signature     string `json:"signature"`
}

type duplicateScanSession struct {
	AdminAccountId  int64              `json:"adminAccountId"`
	CandidateTotal  int                `json:"candidateTotal"`
	ChunkCount      int                `json:"chunkCount"`
	DuplicateTotal  int                `json:"duplicateTotal"`
	GroupTotal      int                `json:"groupTotal"`
	IncompleteTotal int                `json:"incompleteTotal"`
	ScanCursor      int64              `json:"scanCursor"`
	ScannedTotal    int                `json:"scannedTotal"`
	SignatureIds    map[string][]int64 `json:"signatureIds,omitempty"`
	TenantId        int64              `json:"tenantId"`
	ResultCacheKey  string             `json:"resultCacheKey,omitempty"`
}

func (s *sSysPublish) AdminNoteDuplicateScan(ctx context.Context, in *sysin.AdminNoteDuplicateScanInp) (*sysin.AdminNoteDuplicateScanModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		in = &sysin.AdminNoteDuplicateScanInp{}
	}
	var session *duplicateScanSession
	if strings.TrimSpace(in.ScanToken) == "" {
		resultCacheKey := duplicateScanResultCacheKey(account, &in.NoteListInp)
		if cached := loadCachedDuplicateScanResult(ctx, resultCacheKey); cached != "" {
			if cachedSession, cacheErr := loadDuplicateScanSession(ctx, cached); cacheErr == nil && validateDuplicateScanSessionOwner(cachedSession, account) == nil {
				g.Log().Infof(ctx, "复用重复资料扫描任务 scanTaskId:%s tenantId:%d accountId:%d", cached, account.TenantId, account.Id)
				if cachedSession.ChunkCount > 0 {
					result, resultErr := s.duplicateScanBatchResult(ctx, cached, cachedSession, 0)
					if result != nil {
						result.ScanComplete = true
						result.ScanCursor = cachedSession.ScanCursor
						result.ScannedTotal = cachedSession.ScannedTotal
					}
					return result, resultErr
				}
				return duplicateScanProgressResult(cached, cachedSession, true), nil
			}
			_, _ = cache.Instance().Remove(ctx, resultCacheKey)
		}
		in.ScanToken = guid.S()
		session = newDuplicateScanSession(account)
		session.ResultCacheKey = resultCacheKey
		if err = cache.Instance().Set(ctx, duplicateScanSessionKey(in.ScanToken), session, duplicateScanSessionTTL); err != nil {
			return nil, gerror.Wrap(err, "创建重复资料扫描会话失败")
		}
		g.Log().Infof(ctx, "重复资料扫描任务已创建 scanTaskId:%s tenantId:%d accountId:%d", in.ScanToken, account.TenantId, account.Id)
		return duplicateScanProgressResult(in.ScanToken, session, false), nil
	} else {
		session, err = loadDuplicateScanSession(ctx, in.ScanToken)
		if err != nil {
			return nil, err
		}
		if err = validateDuplicateScanSessionOwner(session, account); err != nil {
			return nil, err
		}
		if in.ScanCursor != session.ScanCursor {
			return nil, gerror.New("扫描游标已失效，请重新开始扫描")
		}
	}
	ids, err := s.duplicateScanProfileIdPage(ctx, &in.NoteListInp, account, session.ScanCursor)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		if err = s.appendDuplicateScanSignatures(ctx, session, ids); err != nil {
			g.Log().Warningf(ctx, "重复资料扫描分片失败 scanTaskId:%s cursor:%d err:%+v", in.ScanToken, session.ScanCursor, err)
			return nil, err
		}
		session.ScanCursor = ids[len(ids)-1]
		session.ScannedTotal += len(ids)
	}
	complete := len(ids) < duplicateScanChunkSize
	g.Log().Infof(ctx, "重复资料扫描分片完成 scanTaskId:%s scanned:%d batch:%d cursor:%d duplicate:%d incomplete:%d complete:%t",
		in.ScanToken, session.ScannedTotal, len(ids), session.ScanCursor, session.DuplicateTotal, session.IncompleteTotal, complete)
	if !complete {
		if err = cache.Instance().Set(ctx, duplicateScanSessionKey(in.ScanToken), session, duplicateScanSessionTTL); err != nil {
			return nil, gerror.Wrap(err, "保存重复资料扫描进度失败")
		}
		return duplicateScanProgressResult(in.ScanToken, session, false), nil
	}
	return s.finishDuplicateScan(ctx, in.ScanToken, session)
}

func newDuplicateScanSession(account *sysin.AccountModel) *duplicateScanSession {
	return &duplicateScanSession{AdminAccountId: account.Id, SignatureIds: make(map[string][]int64), TenantId: account.TenantId}
}

func duplicateScanResultCacheKey(account *sysin.AccountModel, in *sysin.NoteListInp) string {
	if account == nil {
		return ""
	}
	normalized := sysin.NoteListInp{}
	if in != nil {
		normalized.ProfileListInp = in.ProfileListInp
		normalized.Page = 0
		normalized.PerPage = 0
		normalized.Pagination = false
	}
	payload, _ := json.Marshal(normalized)
	return fmt.Sprintf("%s%d:%d:%s", consts.DuplicateScanResultKeyPrefix, account.TenantId, account.Id, collectHash(string(payload)))
}

func loadCachedDuplicateScanResult(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value.IsNil() {
		return ""
	}
	return strings.TrimSpace(value.String())
}

func cacheDuplicateScanResult(ctx context.Context, token string, session *duplicateScanSession) {
	if session == nil || session.ResultCacheKey == "" {
		return
	}
	_ = cache.Instance().Set(ctx, session.ResultCacheKey, token, duplicateScanResultTTL)
}

func (s *sSysPublish) duplicateScanProfileIdPage(ctx context.Context, in *sysin.NoteListInp, account *sysin.AccountModel, cursor int64) ([]int64, error) {
	scope, err := s.adminProfileVisibleScope(ctx, account, &in.ProfileListInp)
	if err != nil {
		return nil, err
	}
	if err = s.restrictDuplicateScanToEditableAccounts(ctx, scope, account); err != nil {
		return nil, err
	}
	if err = s.ensureAdminProfileScopeTenants(ctx, scope); err != nil {
		return nil, err
	}
	if scope.Strict && len(scope.AccountIds) == 0 {
		return []int64{}, nil
	}
	mod := noteIndexModel(ctx).LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
	mod = applyNoteIndexFilters(applyNoteIndexScope(mod, scope.TenantId, scope.TenantIds, scope.AccountIds, &in.ProfileListInp), &in.ProfileListInp)
	if cursor > 0 {
		mod = mod.WhereLT("i.profile_id", cursor)
	}
	var rows []struct {
		Id int64 `orm:"id"`
	}
	if err = mod.Fields("i.profile_id AS id").Group("i.profile_id").OrderDesc("i.profile_id").Limit(duplicateScanChunkSize).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取待扫描资料失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return ids, nil
}

func (s *sSysPublish) restrictDuplicateScanToEditableAccounts(ctx context.Context, scope *adminProfileVisibleScope, account *sysin.AccountModel) error {
	if scope == nil || account == nil {
		return gerror.New("当前账号无管理权限")
	}
	editableAccountIds := []int64{account.Id}
	if account.AccountType == sysin.PublishAccountTypeAdmin {
		var err error
		editableAccountIds, err = s.adminManagedAccountIds(ctx, account)
		if err != nil {
			return err
		}
	}
	scope.AccountIds = intersectInt64(scope.AccountIds, editableAccountIds)
	scope.TenantId = account.TenantId
	scope.TenantIds = []int64{account.TenantId}
	scope.Strict = true
	return nil
}

func (s *sSysPublish) appendDuplicateScanSignatures(ctx context.Context, session *duplicateScanSession, ids []int64) error {
	var rows []duplicateImageRow
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Fields("id,profile_id,md5").WhereIn("profile_id", ids).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).Where("purpose IS NULL OR purpose='' OR purpose='display'").
		OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取重复资料图片失败")
	}
	mediaByProfile := make(map[int64][]duplicateImageRow, len(ids))
	for _, row := range rows {
		mediaByProfile[row.ProfileId] = append(mediaByProfile[row.ProfileId], row)
	}
	if session.SignatureIds == nil {
		session.SignatureIds = make(map[string][]int64)
	}
	for _, profileId := range ids {
		signature, complete := duplicateImageSignature(mediaByProfile[profileId])
		if !complete {
			session.IncompleteTotal++
			continue
		}
		current := session.SignatureIds[signature]
		if len(current) == 1 {
			session.GroupTotal++
		}
		if len(current) >= 1 {
			session.DuplicateTotal++
		}
		session.SignatureIds[signature] = append(current, profileId)
	}
	return nil
}

func (s *sSysPublish) finishDuplicateScan(ctx context.Context, token string, session *duplicateScanSession) (*sysin.AdminNoteDuplicateScanModel, error) {
	session.CandidateTotal = session.ScannedTotal
	groupIds := session.SignatureIds
	duplicateIds := make([]int64, 0)
	for _, ids := range groupIds {
		if len(ids) > 1 {
			duplicateIds = append(duplicateIds, ids...)
		}
	}
	if len(duplicateIds) == 0 {
		session.SignatureIds = nil
		_ = cache.Instance().Set(ctx, duplicateScanSessionKey(token), session, duplicateScanSessionTTL)
		cacheDuplicateScanResult(ctx, token, session)
		g.Log().Infof(ctx, "重复资料扫描任务完成 scanTaskId:%s scanned:%d groups:0 duplicate:0 incomplete:%d", token, session.ScannedTotal, session.IncompleteTotal)
		return duplicateScanProgressResult(token, session, true), nil
	}
	columns := dao.ContentProfile.Columns()
	profiles := make([]*sysin.AdminNoteDuplicateItemModel, 0, len(duplicateIds))
	for start := 0; start < len(duplicateIds); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(duplicateIds) {
			end = len(duplicateIds)
		}
		var rows []*sysin.AdminNoteDuplicateItemModel
		err := dao.ContentProfile.Ctx(ctx).
			Fields(columns.Id, columns.SourceNoteUuid+" AS uuid", columns.ProfileNo, columns.Title, columns.CreatedAt).
			WhereIn(columns.Id, duplicateIds[start:end]).WhereNull(columns.DeletedAt).Scan(&rows)
		if err != nil {
			return nil, gerror.Wrap(err, "读取重复资料详情失败")
		}
		profiles = append(profiles, rows...)
	}
	profileById := make(map[int64]*sysin.AdminNoteDuplicateItemModel, len(profiles))
	for _, profile := range profiles {
		profileById[profile.Id] = profile
	}

	allGroups := make([]*sysin.AdminNoteDuplicateGroupModel, 0, len(groupIds))
	for signature, ids := range groupIds {
		if len(ids) < 2 {
			continue
		}
		items := make([]*sysin.AdminNoteDuplicateItemModel, 0, len(ids))
		for _, id := range ids {
			if item := profileById[id]; item != nil {
				items = append(items, item)
			}
		}
		if len(items) < 2 {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			left, right := items[i], items[j]
			leftTime, rightTime := int64(0), int64(0)
			if left.CreatedAt != nil {
				leftTime = left.CreatedAt.Time.UnixNano()
			}
			if right.CreatedAt != nil {
				rightTime = right.CreatedAt.Time.UnixNano()
			}
			if leftTime == rightTime {
				return left.Id > right.Id
			}
			return leftTime > rightTime
		})
		duplicates := append([]*sysin.AdminNoteDuplicateItemModel(nil), items[1:]...)
		allGroups = append(allGroups, &sysin.AdminNoteDuplicateGroupModel{Signature: signature, Keep: items[0], Duplicates: duplicates})
	}
	sort.Slice(allGroups, func(i, j int) bool { return allGroups[i].Keep.Id > allGroups[j].Keep.Id })
	session.GroupTotal = len(allGroups)
	candidates := duplicateScanCandidates(allGroups)
	if len(candidates) == 0 {
		return duplicateScanProgressResult(token, session, true), nil
	}
	session.ChunkCount = (len(candidates) + duplicateScanBatchSize - 1) / duplicateScanBatchSize
	session.DuplicateTotal = len(candidates)
	session.SignatureIds = nil
	if err := saveDuplicateScanSession(ctx, token, session, candidates); err != nil {
		return nil, err
	}
	cacheDuplicateScanResult(ctx, token, session)
	g.Log().Infof(ctx, "重复资料扫描任务完成 scanTaskId:%s scanned:%d groups:%d duplicate:%d incomplete:%d",
		token, session.ScannedTotal, session.GroupTotal, session.DuplicateTotal, session.IncompleteTotal)
	result, err := s.duplicateScanBatchResult(ctx, token, session, 0)
	if result != nil {
		result.ScanComplete = true
		result.ScannedTotal = session.ScannedTotal
		result.ScanCursor = session.ScanCursor
	}
	return result, err
}

func duplicateScanProgressResult(token string, session *duplicateScanSession, complete bool) *sysin.AdminNoteDuplicateScanModel {
	return &sysin.AdminNoteDuplicateScanModel{
		Groups: []*sysin.AdminNoteDuplicateGroupModel{}, CandidateTotal: session.CandidateTotal,
		IncompleteTotal: session.IncompleteTotal, ScanToken: token, ScanCursor: session.ScanCursor,
		ScannedTotal: session.ScannedTotal, ScanComplete: complete,
		GroupTotal: session.GroupTotal, DuplicateTotal: session.DuplicateTotal,
	}
}

func (s *sSysPublish) AdminNoteDuplicateBatch(ctx context.Context, in *sysin.AdminNoteDuplicateBatchInp) (*sysin.AdminNoteDuplicateScanModel, error) {
	if in == nil {
		return nil, gerror.New("扫描会话凭证不能为空")
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	session, err := loadDuplicateScanSession(ctx, in.ScanToken)
	if err != nil {
		return nil, err
	}
	if err = validateDuplicateScanSessionOwner(session, account); err != nil {
		return nil, err
	}
	return s.duplicateScanBatchResult(ctx, in.ScanToken, session, in.Cursor)
}

func (s *sSysPublish) AdminNoteDuplicateCleanup(ctx context.Context, in *sysin.AdminNoteDuplicateCleanupInp) (*sysin.AdminNoteDuplicateCleanupModel, error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要删除的重复资料")
	}
	if len(in.Ids) > 10 {
		return nil, gerror.New("单次只能删除1到10条重复资料")
	}
	ids := uniqueIds(in.Ids)
	if len(ids) != len(in.Ids) {
		return nil, gerror.New("待删除资料ID不能重复")
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	session, err := loadDuplicateScanSession(ctx, in.ScanToken)
	if err != nil {
		return nil, err
	}
	if err = validateDuplicateScanSessionOwner(session, account); err != nil {
		return nil, err
	}
	g.Log().Infof(ctx, "重复资料清理批次开始 scanTaskId:%s cursor:%d idCount:%d", in.ScanToken, in.Cursor, len(ids))
	candidates, err := loadDuplicateScanCandidates(ctx, in.ScanToken, session, in.Cursor)
	if err != nil {
		return nil, err
	}
	candidateById := make(map[int64]duplicateScanCandidate, len(candidates))
	validationIds := make([]int64, 0, len(ids)*2)
	for _, candidate := range candidates {
		candidateById[candidate.ProfileId] = candidate
	}
	for _, id := range ids {
		candidate, ok := candidateById[id]
		if !ok {
			return nil, gerror.New("待删除资料不属于当前扫描批次，请重新扫描")
		}
		validationIds = append(validationIds, id, candidate.KeepProfileId)
	}
	profiles, signatures, err := s.loadDuplicateValidationState(ctx, uniqueIds(validationIds), account.TenantId)
	if err != nil {
		return nil, err
	}
	if err = validateDuplicateCleanupState(ids, candidateById, profiles, signatures); err != nil {
		return nil, err
	}
	if _, err = s.updateProfileStatus(ctx, &sysin.ProfileStatusInp{Ids: ids, Status: 2}, account.TenantId, 0); err != nil {
		return nil, err
	}
	if err = s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: ids}, account.TenantId, 0); err != nil {
		return nil, err
	}
	if session.ResultCacheKey != "" {
		_, _ = cache.Instance().Remove(ctx, session.ResultCacheKey)
	}
	return &sysin.AdminNoteDuplicateCleanupModel{DeletedIds: ids}, nil
}

func validateDuplicateScanSessionOwner(session *duplicateScanSession, account *sysin.AccountModel) error {
	if session == nil || account == nil || session.TenantId != account.TenantId || session.AdminAccountId != account.Id {
		return gerror.New("无权访问该重复资料扫描结果")
	}
	return nil
}

func validateDuplicateCleanupState(ids []int64, candidates map[int64]duplicateScanCandidate, profiles map[int64]*sysin.AdminNoteDuplicateItemModel, signatures map[int64]string) error {
	for _, id := range ids {
		candidate, ok := candidates[id]
		if !ok {
			return gerror.New("待删除资料不属于当前扫描批次，请重新扫描")
		}
		target, keep := profiles[id], profiles[candidate.KeepProfileId]
		if target == nil || keep == nil || signatures[id] != candidate.Signature || signatures[candidate.KeepProfileId] != candidate.Signature || !duplicateProfileNewer(keep, target) {
			return gerror.New("重复资料已发生变化，为避免误删，请重新扫描后再试")
		}
	}
	return nil
}

func duplicateScanCandidates(groups []*sysin.AdminNoteDuplicateGroupModel) []duplicateScanCandidate {
	result := make([]duplicateScanCandidate, 0)
	for _, group := range groups {
		if group == nil || group.Keep == nil {
			continue
		}
		for _, item := range group.Duplicates {
			if item != nil && item.Id > 0 {
				result = append(result, duplicateScanCandidate{KeepProfileId: group.Keep.Id, ProfileId: item.Id, Signature: group.Signature})
			}
		}
	}
	return result
}

func duplicateScanSessionKey(token string) string {
	return consts.DuplicateScanSessionKeyPrefix + strings.TrimSpace(token)
}

func duplicateScanBatchKey(token string, cursor int) string {
	return fmt.Sprintf("%s%s:%d", consts.DuplicateScanBatchKeyPrefix, strings.TrimSpace(token), cursor)
}

func saveDuplicateScanSession(ctx context.Context, token string, session *duplicateScanSession, candidates []duplicateScanCandidate) error {
	writtenKeys := make([]string, 0, session.ChunkCount)
	for cursor, start := 0, 0; start < len(candidates); cursor, start = cursor+1, start+duplicateScanBatchSize {
		end := start + duplicateScanBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		key := duplicateScanBatchKey(token, cursor)
		if err := cache.Instance().Set(ctx, key, candidates[start:end], duplicateScanSessionTTL); err != nil {
			removeDuplicateScanKeys(ctx, writtenKeys)
			return gerror.Wrap(err, "保存重复资料扫描批次失败")
		}
		writtenKeys = append(writtenKeys, key)
	}
	if err := cache.Instance().Set(ctx, duplicateScanSessionKey(token), session, duplicateScanSessionTTL); err != nil {
		removeDuplicateScanKeys(ctx, writtenKeys)
		return gerror.Wrap(err, "保存重复资料扫描会话失败")
	}
	return nil
}

func removeDuplicateScanKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		_, _ = cache.Instance().Remove(ctx, key)
	}
}

func loadDuplicateScanSession(ctx context.Context, token string) (*duplicateScanSession, error) {
	if strings.TrimSpace(token) == "" {
		return nil, gerror.New("扫描会话凭证不能为空")
	}
	value, err := cache.Instance().Get(ctx, duplicateScanSessionKey(token))
	if err != nil {
		return nil, gerror.Wrap(err, "读取重复资料扫描会话失败")
	}
	if value.IsNil() {
		return nil, gerror.New("重复资料扫描结果已过期，请重新扫描")
	}
	var session *duplicateScanSession
	if err = value.Scan(&session); err != nil || session == nil {
		return nil, gerror.New("重复资料扫描结果无效，请重新扫描")
	}
	_ = cache.Instance().Set(ctx, duplicateScanSessionKey(token), session, duplicateScanSessionTTL)
	return session, nil
}

func loadDuplicateScanCandidates(ctx context.Context, token string, session *duplicateScanSession, cursor int) ([]duplicateScanCandidate, error) {
	if session == nil || cursor < 0 || cursor >= session.ChunkCount {
		return nil, gerror.New("重复资料扫描批次不存在，请重新扫描")
	}
	value, err := cache.Instance().Get(ctx, duplicateScanBatchKey(token, cursor))
	if err != nil {
		return nil, gerror.Wrap(err, "读取重复资料扫描批次失败")
	}
	if value.IsNil() {
		return nil, gerror.New("重复资料扫描批次已过期，请重新扫描")
	}
	var candidates []duplicateScanCandidate
	if err = value.Scan(&candidates); err != nil || len(candidates) == 0 {
		return nil, gerror.New("重复资料扫描批次无效，请重新扫描")
	}
	_ = cache.Instance().Set(ctx, duplicateScanBatchKey(token, cursor), candidates, duplicateScanSessionTTL)
	return candidates, nil
}

func (s *sSysPublish) duplicateScanBatchResult(ctx context.Context, token string, session *duplicateScanSession, cursor int) (*sysin.AdminNoteDuplicateScanModel, error) {
	candidates, err := loadDuplicateScanCandidates(ctx, token, session, cursor)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(candidates)*2)
	for _, candidate := range candidates {
		ids = append(ids, candidate.ProfileId, candidate.KeepProfileId)
	}
	profiles, err := s.loadDuplicateProfiles(ctx, uniqueIds(ids))
	if err != nil {
		return nil, err
	}
	groupsByKey := make(map[string]*sysin.AdminNoteDuplicateGroupModel)
	groups := make([]*sysin.AdminNoteDuplicateGroupModel, 0)
	for _, candidate := range candidates {
		target, keep := profiles[candidate.ProfileId], profiles[candidate.KeepProfileId]
		if target == nil || keep == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d", candidate.Signature, candidate.KeepProfileId)
		group := groupsByKey[key]
		if group == nil {
			group = &sysin.AdminNoteDuplicateGroupModel{Signature: candidate.Signature, Keep: keep, Duplicates: []*sysin.AdminNoteDuplicateItemModel{}}
			groupsByKey[key] = group
			groups = append(groups, group)
		}
		group.Duplicates = append(group.Duplicates, target)
	}
	nextCursor := 0
	hasMore := cursor+1 < session.ChunkCount
	if hasMore {
		nextCursor = cursor + 1
	}
	return &sysin.AdminNoteDuplicateScanModel{
		Groups: groups, GroupTotal: session.GroupTotal, DuplicateTotal: session.DuplicateTotal,
		BatchTotal: len(candidates), CandidateTotal: session.CandidateTotal, IncompleteTotal: session.IncompleteTotal,
		HasMore: hasMore, ScanToken: token, Cursor: cursor, NextCursor: nextCursor,
	}, nil
}

func (s *sSysPublish) loadDuplicateProfiles(ctx context.Context, ids []int64) (map[int64]*sysin.AdminNoteDuplicateItemModel, error) {
	columns := dao.ContentProfile.Columns()
	result := make(map[int64]*sysin.AdminNoteDuplicateItemModel, len(ids))
	for start := 0; start < len(ids); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		var rows []*sysin.AdminNoteDuplicateItemModel
		if err := dao.ContentProfile.Ctx(ctx).Fields(columns.Id, columns.SourceNoteUuid+" AS uuid", columns.ProfileNo, columns.Title, columns.CreatedAt).
			WhereIn(columns.Id, ids[start:end]).WhereNull(columns.DeletedAt).Scan(&rows); err != nil {
			return nil, gerror.Wrap(err, "读取重复资料详情失败")
		}
		for _, row := range rows {
			result[row.Id] = row
		}
	}
	return result, nil
}

func (s *sSysPublish) loadDuplicateValidationState(ctx context.Context, ids []int64, tenantId int64) (map[int64]*sysin.AdminNoteDuplicateItemModel, map[int64]string, error) {
	allowedIds, err := s.allowedProfileIds(ctx, ids, tenantId, 0)
	if err != nil {
		return nil, nil, err
	}
	if len(allowedIds) != len(ids) {
		g.Log().Warningf(ctx, "重复资料清理权限校验失败 tenantId:%d requestedIds:%v allowedIds:%v", tenantId, ids, allowedIds)
		return nil, nil, gerror.New("重复资料已不存在或无权操作，请重新扫描")
	}
	profiles, err := s.loadDuplicateProfiles(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	mediaByProfile := make(map[int64][]duplicateImageRow, len(ids))
	for start := 0; start < len(ids); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		var rows []duplicateImageRow
		if err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Fields("id,profile_id,md5").WhereIn("profile_id", ids[start:end]).
			WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).Where("purpose IS NULL OR purpose='' OR purpose='display'").
			OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
			return nil, nil, gerror.Wrap(err, "校验重复资料图片失败")
		}
		for _, row := range rows {
			mediaByProfile[row.ProfileId] = append(mediaByProfile[row.ProfileId], row)
		}
	}
	signatures := make(map[int64]string, len(ids))
	for _, id := range ids {
		if signature, complete := duplicateImageSignature(mediaByProfile[id]); complete {
			signatures[id] = signature
		}
	}
	return profiles, signatures, nil
}

func duplicateProfileNewer(left, right *sysin.AdminNoteDuplicateItemModel) bool {
	if left == nil || right == nil {
		return false
	}
	leftTime, rightTime := int64(0), int64(0)
	if left.CreatedAt != nil {
		leftTime = left.CreatedAt.Time.UnixNano()
	}
	if right.CreatedAt != nil {
		rightTime = right.CreatedAt.Time.UnixNano()
	}
	if leftTime == rightTime {
		return left.Id > right.Id
	}
	return leftTime > rightTime
}

func duplicateScanBatch(groups []*sysin.AdminNoteDuplicateGroupModel, limit int) []*sysin.AdminNoteDuplicateGroupModel {
	if limit <= 0 {
		return []*sysin.AdminNoteDuplicateGroupModel{}
	}
	result := make([]*sysin.AdminNoteDuplicateGroupModel, 0)
	remaining := limit
	for _, group := range groups {
		if group == nil || group.Keep == nil || len(group.Duplicates) == 0 || remaining == 0 {
			continue
		}
		count := len(group.Duplicates)
		if count > remaining {
			count = remaining
		}
		result = append(result, &sysin.AdminNoteDuplicateGroupModel{
			Signature:  group.Signature,
			Keep:       group.Keep,
			Duplicates: append([]*sysin.AdminNoteDuplicateItemModel(nil), group.Duplicates[:count]...),
		})
		remaining -= count
	}
	return result
}

func (s *sSysPublish) loadDuplicateScanMedia(ctx context.Context, groups []*sysin.AdminNoteDuplicateGroupModel) error {
	profileIds := make([]int64, 0)
	items := make(map[int64]*sysin.AdminNoteDuplicateItemModel)
	for _, group := range groups {
		if group == nil || group.Keep == nil {
			continue
		}
		groupItems := append([]*sysin.AdminNoteDuplicateItemModel{group.Keep}, group.Duplicates...)
		for _, item := range groupItems {
			if item == nil || item.Id <= 0 {
				continue
			}
			item.Media = []*sysin.AdminNoteDuplicateMediaModel{}
			items[item.Id] = item
			profileIds = append(profileIds, item.Id)
		}
	}
	if len(profileIds) == 0 {
		return nil
	}
	var rows []struct {
		Id        int64  `orm:"id"`
		ProfileId int64  `orm:"profile_id"`
		FileUrl   string `orm:"file_url"`
	}
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,profile_id,file_url").WhereIn("profile_id", uniqueIds(profileIds)).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).
		Where("purpose IS NULL OR purpose='' OR purpose='display'").
		OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取重复资料图片预览失败")
	}
	for _, row := range rows {
		if item := items[row.ProfileId]; item != nil {
			item.Media = append(item.Media, &sysin.AdminNoteDuplicateMediaModel{Id: row.Id, FileUrl: row.FileUrl})
		}
	}
	return nil
}

func duplicateImageSignature(rows []duplicateImageRow) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row.Md5))
		if value == "" {
			return "", false
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return collectHash(strings.Join(values, "|")), true
}
