package sys

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const (
	duplicateScanChunkSize        = 500
	duplicateScanBatchSize        = 500
	duplicateScanSessionTTL       = 24 * time.Hour
	duplicateScanResultTTL        = 15 * time.Minute
	duplicateScanAlgorithmVersion = 5
	duplicateScanPagesPerTask     = 20
	duplicateScanStartLockTTL     = 10 * time.Second
)

type duplicateImageRow struct {
	Id             int64  `orm:"id"`
	ProfileId      int64  `orm:"profile_id"`
	Md5            string `orm:"md5"`
	PerceptualHash string `orm:"perceptual_hash"`
}

type duplicateProfileRow struct {
	Id        int64       `orm:"id"`
	PlainText string      `orm:"plain_text"`
	CreatedAt *gtime.Time `orm:"created_at"`
}

type duplicateScanCandidate struct {
	KeepProfileId int64  `json:"keepProfileId"`
	ProfileId     int64  `json:"profileId"`
	Signature     string `json:"signature"`
}

type duplicateScanSession struct {
	AlgorithmVersion int    `json:"algorithmVersion"`
	AdminAccountId   int64  `json:"adminAccountId"`
	CandidateTotal   int    `json:"candidateTotal"`
	ChunkCount       int    `json:"chunkCount"`
	DuplicateTotal   int    `json:"duplicateTotal"`
	GroupTotal       int    `json:"groupTotal"`
	IncompleteTotal  int    `json:"incompleteTotal"`
	ScanCursor       int64  `json:"scanCursor"`
	ScannedTotal     int    `json:"scannedTotal"`
	TenantId         int64  `json:"tenantId"`
	ResultCacheKey   string `json:"resultCacheKey,omitempty"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
}

type duplicateScanWorkGroup struct {
	KeepProfileId int64              `json:"keepProfileId"`
	KeepCreatedAt int64              `json:"keepCreatedAt"`
	MemberIds     []int64            `json:"memberIds"`
	PHashSets     map[int64][]string `json:"pHashSets,omitempty"`
}

type duplicateScanQueuePayload struct {
	Token   string                          `json:"token"`
	Account sysin.AccountModel              `json:"account"`
	Input   sysin.AdminNoteDuplicateScanInp `json:"input"`
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
		return s.startDuplicateScan(ctx, account, in)
	}
	session, err = loadDuplicateScanSession(ctx, in.ScanToken)
	if err != nil {
		return nil, err
	}
	if err = validateDuplicateScanSessionOwner(session, account); err != nil {
		if duplicateScanSessionOwnedBy(session, account) && session.AlgorithmVersion != duplicateScanAlgorithmVersion {
			g.Log().Info(ctx, "重复资料扫描算法升级，自动创建新任务", g.Map{
				"oldScanTaskId": in.ScanToken, "oldVersion": session.AlgorithmVersion,
				"newVersion": duplicateScanAlgorithmVersion, "tenantId": account.TenantId, "accountId": account.Id,
			})
			_, _ = cache.Instance().Remove(ctx, duplicateScanSessionKey(in.ScanToken))
			in.ScanToken = ""
			return s.startDuplicateScan(ctx, account, in)
		}
		return nil, err
	}
	if session.Status == "completed" && session.ChunkCount > 0 {
		return s.duplicateScanBatchResult(ctx, in.ScanToken, session, 0)
	}
	return duplicateScanProgressResult(in.ScanToken, session, session.Status == "completed"), nil
}

func (s *sSysPublish) startDuplicateScan(ctx context.Context, account *sysin.AccountModel, in *sysin.AdminNoteDuplicateScanInp) (*sysin.AdminNoteDuplicateScanModel, error) {
	resultCacheKey := duplicateScanResultCacheKey(account, &in.NoteListInp)
	startLock := lock.NewConfig(duplicateScanStartLockTTL, 100*time.Millisecond).Mutex(resultCacheKey + ":start")
	if err := startLock.Lock(ctx); err != nil {
		return nil, gerror.Wrap(err, "等待重复资料扫描任务创建失败")
	}
	defer func() { _ = startLock.Unlock(context.Background()) }()

	if cached := loadCachedDuplicateScanResult(ctx, resultCacheKey); cached != "" {
		if cachedSession, cacheErr := loadDuplicateScanSession(ctx, cached); cacheErr == nil && validateDuplicateScanSessionOwner(cachedSession, account) == nil {
			if cachedSession.Status == "failed" {
				_, _ = cache.Instance().Remove(ctx, resultCacheKey)
			} else {
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
				return duplicateScanProgressResult(cached, cachedSession, cachedSession.Status == "completed"), nil
			}
		}
		_, _ = cache.Instance().Remove(ctx, resultCacheKey)
	}
	in.ScanToken = guid.S()
	session := newDuplicateScanSession(account)
	session.ResultCacheKey = resultCacheKey
	session.Status = "queued"
	if err := saveDuplicateScanProgress(ctx, in.ScanToken, session); err != nil {
		return nil, gerror.Wrap(err, "创建重复资料扫描会话失败")
	}
	if err := s.enqueueDuplicateScan(ctx, in.ScanToken, account, in); err != nil {
		_, _ = cache.Instance().Remove(ctx, duplicateScanSessionKey(in.ScanToken))
		return nil, err
	}
	// Register queued/running scans too, so repeated start requests reuse one task.
	cacheDuplicateScanResult(ctx, in.ScanToken, session)
	g.Log().Infof(ctx, "重复资料扫描任务已创建 scanTaskId:%s tenantId:%d accountId:%d", in.ScanToken, account.TenantId, account.Id)
	return duplicateScanProgressResult(in.ScanToken, session, false), nil
}

func (s *sSysPublish) runDuplicateScan(ctx context.Context, token string, account *sysin.AccountModel, in *sysin.AdminNoteDuplicateScanInp) error {
	if duplicateScanTaskSuperseded(ctx, token, account, in) {
		return nil
	}
	session, err := loadDuplicateScanSession(ctx, token)
	if err != nil {
		return err
	}
	if err = validateDuplicateScanSessionOwner(session, account); err != nil {
		return err
	}
	if session.Status == "completed" {
		return nil
	}
	session.Status = "running"
	session.Error = ""
	if err = saveDuplicateScanProgress(ctx, token, session); err != nil {
		return err
	}
	for page := 0; page < duplicateScanPagesPerTask; page++ {
		if duplicateScanTaskSuperseded(ctx, token, account, in) {
			return nil
		}
		ids, pageErr := s.duplicateScanProfileIdPage(ctx, &in.NoteListInp, account, session.ScanCursor)
		err = pageErr
		if err != nil {
			return err
		}
		if len(ids) > 0 {
			if err = s.appendDuplicateScanSignatures(ctx, token, session, ids); err != nil {
				return err
			}
			session.ScanCursor = ids[len(ids)-1]
			session.ScannedTotal += len(ids)
		}
		if len(ids) < duplicateScanChunkSize {
			_, err = s.finishDuplicateScan(ctx, token, session)
			return err
		}
		if err = saveDuplicateScanProgress(ctx, token, session); err != nil {
			return gerror.Wrap(err, "保存重复资料扫描进度失败")
		}
	}
	session.Status = "queued"
	if err = saveDuplicateScanProgress(ctx, token, session); err != nil {
		return err
	}
	return s.enqueueDuplicateScanContinuation(ctx, token, account, in)
}

func duplicateScanTaskSuperseded(ctx context.Context, token string, account *sysin.AccountModel, in *sysin.AdminNoteDuplicateScanInp) bool {
	if account == nil || in == nil {
		return false
	}
	activeToken := loadCachedDuplicateScanResult(ctx, duplicateScanResultCacheKey(account, &in.NoteListInp))
	return activeToken != "" && activeToken != strings.TrimSpace(token)
}

func duplicateScanMetadata(session *duplicateScanSession) *duplicateScanSession {
	copy := *session
	return &copy
}

func (s *sSysPublish) enqueueDuplicateScan(ctx context.Context, token string, account *sysin.AccountModel, in *sysin.AdminNoteDuplicateScanInp) error {
	payload := duplicateScanQueuePayload{Token: token, Account: *account, Input: *in}
	payload.Input.ScanToken = ""
	payload.Input.ScanCursor = 0
	body, err := json.Marshal(payload)
	if err != nil {
		return gerror.Wrap(err, "编码重复资料扫描任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeDuplicateScan, body),
		asynq.Queue(tgQueueNameDuplicateScan), asynq.TaskID(token+":0"), asynq.MaxRetry(3), asynq.Timeout(10*time.Minute))
	if err != nil {
		return gerror.Wrap(err, "提交重复资料扫描任务失败")
	}
	return nil
}

func (s *sSysPublish) enqueueDuplicateScanContinuation(ctx context.Context, token string, account *sysin.AccountModel, in *sysin.AdminNoteDuplicateScanInp) error {
	payload := duplicateScanQueuePayload{Token: token, Account: *account, Input: *in}
	payload.Input.ScanToken = ""
	payload.Input.ScanCursor = 0
	body, err := json.Marshal(payload)
	if err != nil {
		return gerror.Wrap(err, "编码重复资料扫描续跑任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeDuplicateScan, body),
		asynq.Queue(tgQueueNameDuplicateScan), asynq.TaskID(fmt.Sprintf("%s:%d", token, time.Now().UnixNano())),
		asynq.MaxRetry(3), asynq.Timeout(10*time.Minute))
	if err != nil {
		return gerror.Wrap(err, "提交重复资料扫描续跑任务失败")
	}
	return nil
}

func newDuplicateScanSession(account *sysin.AccountModel) *duplicateScanSession {
	return &duplicateScanSession{
		AlgorithmVersion: duplicateScanAlgorithmVersion, AdminAccountId: account.Id, TenantId: account.TenantId, Status: "queued",
	}
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
	ttl := duplicateScanSessionTTL
	if session.Status == "completed" {
		ttl = duplicateScanResultTTL
	}
	_ = cache.Instance().Set(ctx, session.ResultCacheKey, token, ttl)
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

func (s *sSysPublish) appendDuplicateScanSignatures(ctx context.Context, token string, session *duplicateScanSession, ids []int64) error {
	var rows []duplicateImageRow
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Fields("id,profile_id,md5,perceptual_hash").WhereIn("profile_id", ids).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).Where("purpose IS NULL OR purpose='' OR purpose='display'").
		OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取重复资料图片失败")
	}
	mediaByProfile := make(map[int64][]duplicateImageRow, len(ids))
	for _, row := range rows {
		mediaByProfile[row.ProfileId] = append(mediaByProfile[row.ProfileId], row)
	}
	profileRows := make([]duplicateProfileRow, 0, len(ids))
	if err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields(dao.ContentProfile.Columns().Id, dao.ContentProfile.Columns().PlainText, dao.ContentProfile.Columns().CreatedAt).
		WhereIn(dao.ContentProfile.Columns().Id, ids).WhereNull(dao.ContentProfile.Columns().DeletedAt).
		Scan(&profileRows); err != nil {
		return gerror.Wrap(err, "读取重复资料正文失败")
	}
	textByProfile := make(map[int64]string, len(profileRows))
	createdByProfile := make(map[int64]int64, len(profileRows))
	for _, row := range profileRows {
		textByProfile[row.Id] = row.PlainText
		if row.CreatedAt != nil {
			createdByProfile[row.Id] = row.CreatedAt.Time.UnixNano()
		}
	}
	work := newDuplicateScanWorkCache(ctx, token)
	if err := work.preloadProcessed(ids); err != nil {
		return err
	}
	lookupKeys := make([]string, 0)
	for _, profileId := range ids {
		if values, complete := duplicateImagePHashes(mediaByProfile[profileId]); complete {
			lookupKeys = append(lookupKeys, duplicatePHashBucketKeys(values, true)...)
		}
	}
	if err := work.preloadBuckets(uniqueStrings(lookupKeys)); err != nil {
		return err
	}
	groupSignatures := make([]string, 0, len(ids))
	for _, profileId := range ids {
		if signature, complete := duplicateProfileSignature(textByProfile[profileId], mediaByProfile[profileId]); complete {
			groupSignatures = append(groupSignatures, signature)
		}
	}
	for _, signatures := range work.buckets {
		groupSignatures = append(groupSignatures, signatures...)
	}
	if err := work.preloadGroups(uniqueStrings(groupSignatures)); err != nil {
		return err
	}
	for _, profileId := range ids {
		processed, err := work.profileProcessed(profileId)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		signature, complete := duplicateProfileSignature(textByProfile[profileId], mediaByProfile[profileId])
		if !complete {
			session.IncompleteTotal++
			work.markProfileProcessed(profileId)
			continue
		}
		group, err := work.group(signature)
		if err != nil {
			return err
		}
		if group == nil {
			group = &duplicateScanWorkGroup{KeepProfileId: profileId, KeepCreatedAt: createdByProfile[profileId], MemberIds: []int64{profileId}}
			work.setGroup(signature, group)
		} else {
			group.addMember(profileId, createdByProfile[profileId])
			work.setGroup(signature, group)
		}
		if err = work.appendPHash(profileId, createdByProfile[profileId], mediaByProfile[profileId]); err != nil {
			return err
		}
		work.markProfileProcessed(profileId)
	}
	return work.save()
}

func (s *sSysPublish) finishDuplicateScan(ctx context.Context, token string, session *duplicateScanSession) (*sysin.AdminNoteDuplicateScanModel, error) {
	session.CandidateTotal = session.ScannedTotal
	session.GroupTotal = 0
	writer := newDuplicateScanCandidateWriter(ctx, token)
	retainedKey := duplicateScanWorkHashKey(token, "retained")
	emittedKey := duplicateScanWorkHashKey(token, "emitted")
	_, _ = cache.Instance().Remove(ctx, retainedKey, emittedKey)
	retainedWriter := newDuplicateScanMarkerWriter(ctx, retainedKey)
	if err := rangeDuplicateScanWorkGroups(ctx, token, func(signature string, group *duplicateScanWorkGroup) error {
		if !strings.HasPrefix(signature, "phash:") && group != nil && len(group.MemberIds) > 1 {
			return retainedWriter.append(group.KeepProfileId)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := retainedWriter.close(); err != nil {
		return nil, err
	}
	if err := rangeDuplicateScanWorkGroups(ctx, token, func(signature string, group *duplicateScanWorkGroup) error {
		if group == nil || len(group.MemberIds) < 2 {
			return nil
		}
		session.GroupTotal++
		members := group.MemberIds[1:]
		for start := 0; start < len(members); start += duplicateScanBatchSize {
			end := start + duplicateScanBatchSize
			if end > len(members) {
				end = len(members)
			}
			fields := make([]string, 0, end-start)
			for _, profileId := range members[start:end] {
				fields = append(fields, fmt.Sprint(profileId))
			}
			retained, err := cache.HashGetMany(ctx, retainedKey, fields)
			if err != nil {
				return gerror.Wrap(err, "读取重复资料保留标记失败")
			}
			emitted, err := cache.HashGetMany(ctx, emittedKey, fields)
			if err != nil {
				return gerror.Wrap(err, "读取重复资料候选标记失败")
			}
			newlyEmitted := make(map[string]any)
			for index, profileId := range members[start:end] {
				if (index < len(retained) && !retained[index].IsNil()) || (index < len(emitted) && !emitted[index].IsNil()) {
					continue
				}
				if appendErr := writer.append(duplicateScanCandidate{KeepProfileId: group.KeepProfileId, ProfileId: profileId, Signature: signature}); appendErr != nil {
					return appendErr
				}
				newlyEmitted[fields[index]] = 1
			}
			if err = cache.HashSetMany(ctx, emittedKey, newlyEmitted, duplicateScanSessionTTL); err != nil {
				return gerror.Wrap(err, "保存重复资料候选标记失败")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := writer.close(); err != nil {
		return nil, err
	}
	session.ChunkCount = writer.chunkCount
	session.DuplicateTotal = writer.total
	session.Status = "completed"
	if err := saveDuplicateScanProgress(ctx, token, session); err != nil {
		return nil, err
	}
	cacheDuplicateScanResult(ctx, token, session)
	g.Log().Infof(ctx, "重复资料扫描任务完成 scanTaskId:%s scanned:%d groups:%d duplicate:%d incomplete:%d",
		token, session.ScannedTotal, session.GroupTotal, session.DuplicateTotal, session.IncompleteTotal)
	if session.ChunkCount == 0 {
		return duplicateScanProgressResult(token, session, true), nil
	}
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
		ScanStatus: session.Status, ScanError: session.Error,
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
	phashValidated, err := applyDuplicatePHashValidation(ctx, ids, candidateById)
	if err != nil {
		return nil, err
	}
	if err = validateDuplicateCleanupState(ids, candidateById, profiles, signatures, phashValidated); err != nil {
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

func applyDuplicatePHashValidation(ctx context.Context, ids []int64, candidates map[int64]duplicateScanCandidate) (map[int64]bool, error) {
	validated := make(map[int64]bool)
	validationIds := make([]int64, 0, len(ids)*2)
	for _, id := range ids {
		candidate, ok := candidates[id]
		if ok && strings.HasPrefix(candidate.Signature, "phash:") {
			validationIds = append(validationIds, id, candidate.KeepProfileId)
		}
	}
	validationIds = uniqueIds(validationIds)
	if len(validationIds) == 0 {
		return validated, nil
	}
	var rows []duplicateImageRow
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,profile_id,perceptual_hash").WhereIn("profile_id", validationIds).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).
		Where("purpose IS NULL OR purpose='' OR purpose='display'").
		OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "校验相似资料图片指纹失败")
	}
	byProfile := make(map[int64][]duplicateImageRow, len(validationIds))
	for _, row := range rows {
		byProfile[row.ProfileId] = append(byProfile[row.ProfileId], row)
	}
	for _, id := range ids {
		candidate, ok := candidates[id]
		if !ok || !strings.HasPrefix(candidate.Signature, "phash:") {
			continue
		}
		left, leftOK := duplicateImagePHashes(byProfile[id])
		right, rightOK := duplicateImagePHashes(byProfile[candidate.KeepProfileId])
		if leftOK && rightOK && profilePHashSetsMatch(left, right, collectProfilePHashDuplicateThreshold) {
			validated[id] = true
		}
	}
	return validated, nil
}

func validateDuplicateScanSessionOwner(session *duplicateScanSession, account *sysin.AccountModel) error {
	if !duplicateScanSessionOwnedBy(session, account) {
		return gerror.New("无权访问该重复资料扫描结果")
	}
	if session.AlgorithmVersion != duplicateScanAlgorithmVersion {
		return gerror.New("重复资料扫描算法已更新，请重新扫描")
	}
	return nil
}

func duplicateScanSessionOwnedBy(session *duplicateScanSession, account *sysin.AccountModel) bool {
	return session != nil && account != nil && session.TenantId == account.TenantId && session.AdminAccountId == account.Id
}

func validateDuplicateCleanupState(ids []int64, candidates map[int64]duplicateScanCandidate, profiles map[int64]*sysin.AdminNoteDuplicateItemModel, signatures map[int64]string, phashValidated map[int64]bool) error {
	for _, id := range ids {
		candidate, ok := candidates[id]
		if !ok {
			return gerror.New("待删除资料不属于当前扫描批次，请重新扫描")
		}
		target, keep := profiles[id], profiles[candidate.KeepProfileId]
		signatureValid := phashValidated[id]
		if !strings.HasPrefix(candidate.Signature, "phash:") {
			signatureValid = signatures[id] == candidate.Signature && signatures[candidate.KeepProfileId] == candidate.Signature
		}
		if target == nil || keep == nil || !signatureValid || !duplicateProfileNewer(keep, target) {
			return gerror.New("重复资料已发生变化，为避免误删，请重新扫描后再试")
		}
	}
	return nil
}

func duplicateScanSessionKey(token string) string {
	return consts.DuplicateScanSessionKeyPrefix + strings.TrimSpace(token)
}

func duplicateScanBatchKey(token string, cursor int) string {
	return fmt.Sprintf("%s%s:%d", consts.DuplicateScanBatchKeyPrefix, strings.TrimSpace(token), cursor)
}

type duplicateScanCandidateWriter struct {
	ctx        context.Context
	token      string
	buffer     []duplicateScanCandidate
	chunkCount int
	total      int
}

type duplicateScanMarkerWriter struct {
	ctx    context.Context
	key    string
	fields map[string]any
}

func newDuplicateScanMarkerWriter(ctx context.Context, key string) *duplicateScanMarkerWriter {
	return &duplicateScanMarkerWriter{ctx: ctx, key: key, fields: make(map[string]any, duplicateScanBatchSize)}
}

func (w *duplicateScanMarkerWriter) append(profileId int64) error {
	w.fields[fmt.Sprint(profileId)] = 1
	if len(w.fields) < duplicateScanBatchSize {
		return nil
	}
	return w.flush()
}

func (w *duplicateScanMarkerWriter) flush() error {
	if err := cache.HashSetMany(w.ctx, w.key, w.fields, duplicateScanSessionTTL); err != nil {
		return gerror.Wrap(err, "保存重复资料保留标记失败")
	}
	w.fields = make(map[string]any, duplicateScanBatchSize)
	return nil
}

func (w *duplicateScanMarkerWriter) close() error { return w.flush() }

func newDuplicateScanCandidateWriter(ctx context.Context, token string) *duplicateScanCandidateWriter {
	return &duplicateScanCandidateWriter{ctx: ctx, token: token, buffer: make([]duplicateScanCandidate, 0, duplicateScanBatchSize)}
}

func (w *duplicateScanCandidateWriter) append(candidate duplicateScanCandidate) error {
	w.buffer = append(w.buffer, candidate)
	w.total++
	if len(w.buffer) < duplicateScanBatchSize {
		return nil
	}
	return w.flush()
}

func (w *duplicateScanCandidateWriter) flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	batch := append([]duplicateScanCandidate(nil), w.buffer...)
	if err := cache.Instance().Set(w.ctx, duplicateScanBatchKey(w.token, w.chunkCount), batch, duplicateScanSessionTTL); err != nil {
		return gerror.Wrap(err, "保存重复资料扫描批次失败")
	}
	w.chunkCount++
	w.buffer = w.buffer[:0]
	return nil
}

func (w *duplicateScanCandidateWriter) close() error { return w.flush() }

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
	session, err := decodeDuplicateScanSession(value.Bytes())
	if err != nil || session == nil {
		return nil, gerror.New("重复资料扫描结果无效，请重新扫描")
	}
	return session, nil
}

func saveDuplicateScanProgress(ctx context.Context, token string, session *duplicateScanSession) error {
	data, err := encodeDuplicateScanSession(session)
	if err != nil {
		return err
	}
	return cache.Instance().Set(ctx, duplicateScanSessionKey(token), data, duplicateScanSessionTTL)
}

func encodeDuplicateScanSession(session *duplicateScanSession) ([]byte, error) {
	data, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err = writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func decodeDuplicateScanSession(data []byte) (*duplicateScanSession, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		var session duplicateScanSession
		if err = json.NewDecoder(reader).Decode(&session); err != nil {
			return nil, err
		}
		return &session, nil
	}
	var session duplicateScanSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
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
		ScanComplete: session.Status == "completed", ScanStatus: session.Status, ScanError: session.Error,
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
		if err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Fields("id,profile_id,md5,perceptual_hash").WhereIn("profile_id", ids[start:end]).
			WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).Where("purpose IS NULL OR purpose='' OR purpose='display'").
			OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
			return nil, nil, gerror.Wrap(err, "校验重复资料图片失败")
		}
		for _, row := range rows {
			mediaByProfile[row.ProfileId] = append(mediaByProfile[row.ProfileId], row)
		}
	}
	var textRows []duplicateProfileRow
	if err = g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields(dao.ContentProfile.Columns().Id, dao.ContentProfile.Columns().PlainText).
		WhereIn(dao.ContentProfile.Columns().Id, ids).WhereNull(dao.ContentProfile.Columns().DeletedAt).
		Scan(&textRows); err != nil {
		return nil, nil, gerror.Wrap(err, "校验重复资料正文失败")
	}
	textByProfile := make(map[int64]string, len(textRows))
	for _, row := range textRows {
		textByProfile[row.Id] = row.PlainText
	}
	signatures := make(map[int64]string, len(ids))
	for _, id := range ids {
		if signature, complete := duplicateProfileSignature(textByProfile[id], mediaByProfile[id]); complete {
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

func duplicateImageSignature(rows []duplicateImageRow) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row.Md5))
		if value == "" {
			value = strings.ToLower(strings.TrimSpace(row.PerceptualHash))
		}
		if value == "" {
			return "", false
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return collectHash(strings.Join(values, "|")), true
}

func duplicateProfileSignature(text string, rows []duplicateImageRow) (string, bool) {
	normalized := normalizeCollectText(normalizeCollectKeywordText(text))
	if normalized != "" {
		return "text:" + collectHash(normalized), true
	}
	imageSignature, ok := duplicateImageSignature(rows)
	if !ok {
		return "", false
	}
	return "image:" + imageSignature, true
}

func duplicateImagePHashes(rows []duplicateImageRow) ([]string, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row.PerceptualHash))
		if _, ok := parseUploadPHash(value); !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

type duplicateScanWorkCache struct {
	ctx            context.Context
	token          string
	groups         map[string]*duplicateScanWorkGroup
	groupLoaded    map[string]bool
	buckets        map[string][]string
	processed      map[int64]bool
	dirtyGroups    map[string]any
	dirtyBuckets   map[string]any
	dirtyProcessed map[string]any
}

func newDuplicateScanWorkCache(ctx context.Context, token string) *duplicateScanWorkCache {
	return &duplicateScanWorkCache{
		ctx: ctx, token: token, groups: make(map[string]*duplicateScanWorkGroup), groupLoaded: make(map[string]bool),
		buckets: make(map[string][]string), processed: make(map[int64]bool), dirtyGroups: make(map[string]any),
		dirtyBuckets: make(map[string]any), dirtyProcessed: make(map[string]any),
	}
}

func (w *duplicateScanWorkCache) preloadBuckets(fields []string) error {
	values, err := cache.HashGetMany(w.ctx, duplicateScanWorkHashKey(w.token, "buckets"), fields)
	if err != nil {
		return gerror.Wrap(err, "读取重复资料扫描图片索引失败")
	}
	for index, field := range fields {
		if index >= len(values) || values[index].IsNil() {
			continue
		}
		var signatures []string
		if _, err = decodeDuplicateScanWorkValue(values[index].Bytes(), &signatures); err != nil {
			return gerror.Wrap(err, "解析重复资料扫描图片索引失败")
		}
		w.buckets[field] = signatures
	}
	return nil
}

func (w *duplicateScanWorkCache) preloadProcessed(ids []int64) error {
	fields := make([]string, 0, len(ids))
	for _, id := range ids {
		fields = append(fields, fmt.Sprint(id))
	}
	values, err := cache.HashGetMany(w.ctx, duplicateScanWorkHashKey(w.token, "processed"), fields)
	if err != nil {
		return gerror.Wrap(err, "读取重复资料扫描断点失败")
	}
	for index, id := range ids {
		w.processed[id] = index < len(values) && !values[index].IsNil()
	}
	return nil
}

func (w *duplicateScanWorkCache) preloadGroups(signatures []string) error {
	values, err := cache.HashGetMany(w.ctx, duplicateScanWorkHashKey(w.token, "groups"), signatures)
	if err != nil {
		return gerror.Wrap(err, "读取重复资料扫描分组失败")
	}
	for index, signature := range signatures {
		w.groupLoaded[signature] = true
		if index >= len(values) || values[index].IsNil() {
			continue
		}
		var group duplicateScanWorkGroup
		present, decodeErr := decodeDuplicateScanWorkValue(values[index].Bytes(), &group)
		if decodeErr != nil {
			err = decodeErr
			return gerror.Wrap(err, "解析重复资料扫描分组失败")
		}
		if !present {
			continue
		}
		w.groups[signature] = &group
	}
	return nil
}

func duplicateScanWorkHashKey(token, kind string) string {
	return fmt.Sprintf("%s%s:%s", consts.DuplicateScanWorkKeyPrefix, strings.TrimSpace(token), kind)
}

func rangeDuplicateScanWorkGroups(ctx context.Context, token string, visit func(string, *duplicateScanWorkGroup) error) error {
	var cursor uint64
	for {
		next, fields, err := cache.HashScan(ctx, duplicateScanWorkHashKey(token, "groups"), cursor, duplicateScanBatchSize)
		if err != nil {
			return gerror.Wrap(err, "读取重复资料扫描分组失败")
		}
		for signature, value := range fields {
			var group duplicateScanWorkGroup
			present, decodeErr := decodeDuplicateScanWorkValue(value.Bytes(), &group)
			if decodeErr != nil {
				err = decodeErr
				return gerror.Wrap(err, "解析重复资料扫描分组失败")
			}
			if !present {
				continue
			}
			if err = visit(signature, &group); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func (w *duplicateScanWorkCache) group(signature string) (*duplicateScanWorkGroup, error) {
	if w.groupLoaded[signature] {
		return w.groups[signature], nil
	}
	values, err := cache.HashGetMany(w.ctx, duplicateScanWorkHashKey(w.token, "groups"), []string{signature})
	if err != nil {
		return nil, gerror.Wrap(err, "读取重复资料扫描分组失败")
	}
	w.groupLoaded[signature] = true
	if len(values) > 0 && !values[0].IsNil() {
		var group duplicateScanWorkGroup
		present, decodeErr := decodeDuplicateScanWorkValue(values[0].Bytes(), &group)
		if decodeErr != nil {
			err = decodeErr
			return nil, gerror.Wrap(err, "解析重复资料扫描分组失败")
		}
		if present {
			w.groups[signature] = &group
		}
	}
	return w.groups[signature], nil
}

func (w *duplicateScanWorkCache) setGroup(signature string, group *duplicateScanWorkGroup) {
	w.groupLoaded[signature] = true
	w.groups[signature] = group
	w.dirtyGroups[signature] = group
}

func (w *duplicateScanWorkCache) profileProcessed(profileId int64) (bool, error) {
	if processed, ok := w.processed[profileId]; ok {
		return processed, nil
	}
	field := fmt.Sprint(profileId)
	values, err := cache.HashGetMany(w.ctx, duplicateScanWorkHashKey(w.token, "processed"), []string{field})
	if err != nil {
		return false, gerror.Wrap(err, "读取重复资料扫描断点失败")
	}
	processed := len(values) > 0 && !values[0].IsNil()
	w.processed[profileId] = processed
	return processed, nil
}

func (w *duplicateScanWorkCache) markProfileProcessed(profileId int64) {
	w.processed[profileId] = true
	w.dirtyProcessed[fmt.Sprint(profileId)] = 1
}

func (w *duplicateScanWorkCache) appendPHash(profileId, createdAt int64, rows []duplicateImageRow) error {
	values, complete := duplicateImagePHashes(rows)
	if profileId <= 0 || !complete {
		return nil
	}
	candidateSignatures := make(map[string]struct{})
	for _, key := range duplicatePHashBucketKeys(values, true) {
		for _, signature := range w.buckets[key] {
			candidateSignatures[signature] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(candidateSignatures))
	for signature := range candidateSignatures {
		ordered = append(ordered, signature)
	}
	sort.Strings(ordered)
	groupSignature := ""
	for _, signature := range ordered {
		group, err := w.group(signature)
		if err != nil {
			return err
		}
		if group == nil || !profilePHashGroupAccepts(values, group.MemberIds, group.PHashSets, collectProfilePHashDuplicateThreshold) {
			continue
		}
		groupSignature = signature
		break
	}
	if groupSignature == "" {
		groupSignature = fmt.Sprintf("phash:%d", profileId)
	}
	group, err := w.group(groupSignature)
	if err != nil {
		return err
	}
	if group == nil {
		group = &duplicateScanWorkGroup{KeepProfileId: profileId, KeepCreatedAt: createdAt, PHashSets: make(map[int64][]string)}
	}
	group.addMember(profileId, createdAt)
	group.PHashSets[profileId] = values
	w.setGroup(groupSignature, group)
	for _, key := range duplicatePHashBucketKeys(values, false) {
		if !containsString(w.buckets[key], groupSignature) {
			w.buckets[key] = append(w.buckets[key], groupSignature)
		}
		w.dirtyBuckets[key] = w.buckets[key]
	}
	return nil
}

func (g *duplicateScanWorkGroup) addMember(profileId, createdAt int64) {
	if containsInt64(g.MemberIds, profileId) {
		return
	}
	if g.KeepProfileId == 0 || createdAt > g.KeepCreatedAt || (createdAt == g.KeepCreatedAt && profileId > g.KeepProfileId) {
		g.MemberIds = append([]int64{profileId}, g.MemberIds...)
		g.KeepProfileId, g.KeepCreatedAt = profileId, createdAt
		return
	}
	g.MemberIds = append(g.MemberIds, profileId)
}

func (w *duplicateScanWorkCache) save() error {
	groups, err := encodeDuplicateScanWorkValues(w.dirtyGroups)
	if err != nil {
		return gerror.Wrap(err, "编码重复资料扫描分组失败")
	}
	buckets, err := encodeDuplicateScanWorkValues(w.dirtyBuckets)
	if err != nil {
		return gerror.Wrap(err, "编码重复资料扫描索引失败")
	}
	if err = cache.HashSetMany(w.ctx, duplicateScanWorkHashKey(w.token, "groups"), groups, duplicateScanSessionTTL); err != nil {
		return gerror.Wrap(err, "保存重复资料扫描分组失败")
	}
	if err = cache.HashSetMany(w.ctx, duplicateScanWorkHashKey(w.token, "buckets"), buckets, duplicateScanSessionTTL); err != nil {
		return gerror.Wrap(err, "保存重复资料扫描索引失败")
	}
	if err := cache.HashSetMany(w.ctx, duplicateScanWorkHashKey(w.token, "processed"), w.dirtyProcessed, duplicateScanSessionTTL); err != nil {
		return gerror.Wrap(err, "保存重复资料扫描断点失败")
	}
	return nil
}

func encodeDuplicateScanWorkValues(fields map[string]any) (map[string]any, error) {
	encoded := make(map[string]any, len(fields))
	for field, value := range fields {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		encoded[field] = string(data)
	}
	return encoded, nil
}

func decodeDuplicateScanWorkValue(data []byte, target any) (bool, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return false, err
	}
	return true, nil
}

// Fuzzy similarity is not transitive. Requiring the incoming profile to match
// every existing member prevents an A~B~C chain where A and C are not duplicates.
func profilePHashGroupAccepts(values []string, memberIds []int64, sets map[int64][]string, threshold int) bool {
	for _, memberId := range memberIds {
		if !profilePHashSetsMatch(values, sets[memberId], threshold) {
			return false
		}
	}
	return true
}

func duplicatePHashBucketKeys(values []string, neighborhood bool) []string {
	const (
		blockCount = 8
		blockBits  = 8
	)
	// Always keep a deterministic whole-set key. LSH neighborhoods are only a
	// recall optimization and can miss hashes whose bits differ across blocks.
	// Exact whole-set matches must never depend on the LSH layout.
	keys := []string{"exact:" + strings.Join(sortedPHashValues(values), "|")}
	keys = append(keys, make([]string, 0, len(values)*blockCount)...)
	seen := make(map[string]struct{}, cap(keys))
	for _, value := range values {
		hash, ok := parseUploadPHash(value)
		if !ok {
			continue
		}
		for pos := 0; pos < blockCount; pos++ {
			block := uint8(hash.GetHash() >> uint((blockCount-pos-1)*blockBits))
			blocks := []uint8{block}
			if neighborhood {
				blocks = duplicatePHashByteNeighborhood(block)
			}
			for _, candidate := range blocks {
				key := fmt.Sprintf("%d:%d", pos+1, candidate)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func sortedPHashValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if _, ok := parseUploadPHash(value); ok {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

// Splitting a 64-bit hash into eight bytes guarantees that hashes within
// distance 20 share at least one byte within distance 2. Exact matching still
// happens in profilePHashSetsMatch; these keys only reduce candidate lookup.
func duplicatePHashByteNeighborhood(value uint8) []uint8 {
	result := make([]uint8, 0, 37)
	result = append(result, value)
	for first := 0; first < 8; first++ {
		result = append(result, value^(1<<first))
	}
	for first := 0; first < 8; first++ {
		for second := first + 1; second < 8; second++ {
			result = append(result, value^(1<<first)^(1<<second))
		}
	}
	return result
}
