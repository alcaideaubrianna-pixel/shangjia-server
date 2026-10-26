package sys

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/hgrds/lock"
)

const (
	proxyMediaRepairStage     = "proxy_media_repair"
	proxyMediaRepairScanLimit = 20000
)

var proxyMediaRepairProfilePattern = regexp.MustCompile(`profileId:([0-9]+)`)

type proxyMediaRepairCandidate struct {
	EventID   int64
	ProfileID int64
	TenantID  int64
	AccountID int64
}

var proxyMediaRepairHTTPClient = &http.Client{Timeout: 10 * time.Second}

func parseProxyMediaRepairProfileID(message string) int64 {
	parts := proxyMediaRepairProfilePattern.FindStringSubmatch(message)
	if len(parts) != 2 {
		return 0
	}
	id, _ := strconv.ParseInt(parts[1], 10, 64)
	return id
}

// ProcessProxyCollectMediaRepairs replaces profiles whose old media produced a
// confirmed HTTP 404 and whose fresh proxy-collection event was deduplicated.
// The old profile remains intact until the replacement has been committed.
func (s *sSysPublish) ProcessProxyCollectMediaRepairs(ctx context.Context, limit int) (processed, repaired int, err error) {
	if limit <= 0 || limit > 20 {
		return 0, 0, gerror.New("代理采集媒体修复批次必须为1到20条")
	}
	mutex := lock.NewConfig(55*time.Minute, 100*time.Millisecond).Mutex("youban_publish:proxy-media-repair")
	if err = mutex.TryLock(ctx); err != nil {
		if gerror.Is(err, lock.ErrLockFailed) {
			return 0, 0, nil
		}
		return 0, 0, gerror.Wrap(err, "获取代理采集媒体修复锁失败")
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()

	candidates, err := s.proxyCollectMediaRepairCandidates(ctx, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, candidate := range candidates {
		processed++
		if repairErr := s.repairProxyCollectMediaCandidate(ctx, candidate); repairErr != nil {
			s.appendCollectEventLog(ctx, candidate.EventID, proxyMediaRepairStage, "retry", "替换缺失媒体资料失败，下次继续", repairErr.Error())
			g.Log().Warningf(ctx, "代理采集缺失媒体资料替换失败 eventId:%d profileId:%d err:%+v", candidate.EventID, candidate.ProfileID, repairErr)
			continue
		}
		repaired++
	}
	return processed, repaired, nil
}

func (s *sSysPublish) proxyCollectMediaRepairCandidates(ctx context.Context, limit int) ([]proxyMediaRepairCandidate, error) {
	query := fmt.Sprintf(`WITH recent_events AS MATERIALIZED (
		SELECT e.id,e.tenant_id,e.account_id
		FROM %s e
		JOIN %s s ON s.id=e.source_id AND s.deleted_at IS NULL
		WHERE s.source_type='account' AND e.material_role=? AND e.received_at>=?
		ORDER BY e.id DESC LIMIT %d
	), dedupe AS MATERIALIZED (
		SELECT e.id AS event_id,e.tenant_id,e.account_id,
		       substring(l.message FROM 'profileId:([0-9]+)')::bigint AS profile_id
		FROM recent_events e
		JOIN %s l ON l.event_id=e.id AND l.stage='dedupe' AND l.status='skipped'
		WHERE l.message ~ 'profileId:[0-9]+'
		  AND NOT EXISTS (
			SELECT 1 FROM %s done
			WHERE done.event_id=e.id AND done.stage=? AND done.status='completed'
		  )
	), broken AS MATERIALIZED (
		SELECT DISTINCT j.profile_id
		FROM %s j
		JOIN (SELECT DISTINCT profile_id FROM dedupe) d ON d.profile_id=j.profile_id
		WHERE j.error_message LIKE '%%HTTP 404%%'
	), eligible AS (
		SELECT DISTINCT ON (d.profile_id)
		       d.event_id,d.profile_id,d.tenant_id,d.account_id
		FROM dedupe d
		JOIN broken b ON b.profile_id=d.profile_id
		JOIN %s ps ON ps.profile_id=d.profile_id AND ps.tenant_id=d.tenant_id
		          AND ps.account_id=d.account_id AND ps.deleted_at IS NULL
		JOIN hg_content_profile p ON p.id=d.profile_id AND p.deleted_at IS NULL
		ORDER BY d.profile_id,d.event_id DESC
	)
	SELECT event_id,profile_id,tenant_id,account_id
	FROM eligible
	ORDER BY event_id DESC
	LIMIT %d`,
		pdao.YoubanPublishCollectEvent.Table(), pdao.YoubanPublishCollectSource.Table(),
		proxyMediaRepairScanLimit, pdao.YoubanPublishCollectEventLog.Table(),
		pdao.YoubanPublishCollectEventLog.Table(), publishTgJobTable,
		publishProfileStateTable, limit)
	result := make([]proxyMediaRepairCandidate, 0, limit)
	rows, err := g.DB().GetAll(ctx, query, collectMaterialRoleDisplay,
		gtime.Now().Add(-48*time.Hour), proxyMediaRepairStage)
	if err != nil {
		return nil, gerror.Wrap(err, "扫描代理采集缺失媒体修复候选失败")
	}
	if err = rows.Structs(&result); err != nil {
		return nil, gerror.Wrap(err, "解析代理采集缺失媒体修复候选失败")
	}
	g.Log().Info(ctx, "代理采集缺失媒体修复候选扫描完成", g.Map{
		"windowHours": 48,
		"scanLimit":   proxyMediaRepairScanLimit,
		"batchLimit":  limit,
		"candidates":  len(result),
	})
	return result, nil
}

func (s *sSysPublish) proxyCollectMediaRepairEligible(ctx context.Context, candidate proxyMediaRepairCandidate) (bool, error) {
	exists, err := g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		InnerJoin("hg_content_profile p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		Where("ps.profile_id", candidate.ProfileID).Where("ps.tenant_id", candidate.TenantID).
		Where("ps.account_id", candidate.AccountID).WhereNull("ps.deleted_at").Count()
	if err != nil || exists == 0 {
		return false, gerror.Wrap(err, "校验待替换资料归属失败")
	}
	failed404, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", candidate.ProfileID).WhereLike("error_message", "%HTTP 404%").Count()
	if err != nil {
		return false, gerror.Wrap(err, "校验资料媒体404记录失败")
	}
	return failed404 > 0, nil
}

func (s *sSysPublish) repairProxyCollectMediaCandidate(ctx context.Context, candidate proxyMediaRepairCandidate) error {
	s.appendCollectEventLog(ctx, candidate.EventID, proxyMediaRepairStage, "processing", "开始替换缺失媒体资料", fmt.Sprintf("oldProfileId=%d", candidate.ProfileID))
	reset, err := s.resetMissingCollectEventMedia(ctx, candidate.EventID)
	if err != nil {
		return err
	}
	if reset > 0 {
		return gerror.Newf("已重置%d个失效媒体，等待媒体Worker从Telegram重新下载", reset)
	}
	existingReplacementID, err := s.proxyCollectMediaReplacementProfileID(ctx, candidate)
	if err != nil {
		return err
	}
	if existingReplacementID > 0 {
		if err = s.detachProfileFingerprints(ctx, []int64{candidate.ProfileID}); err != nil {
			return err
		}
		return s.finalizeProxyCollectMediaReplacement(ctx, candidate, existingReplacementID)
	}
	if err := s.detachProfileFingerprints(ctx, []int64{candidate.ProfileID}); err != nil {
		return err
	}
	restoreOldFingerprint := true
	defer func() {
		if restoreOldFingerprint {
			if refreshErr := s.refreshProfileFingerprintProjection(context.Background(), candidate.ProfileID); refreshErr != nil {
				g.Log().Warningf(context.Background(), "恢复旧资料指纹失败 profileId:%d err:%+v", candidate.ProfileID, refreshErr)
			}
		}
	}()

	processed, failed, err := RequeueCollectEventsBestEffort(ctx, []int64{candidate.EventID})
	if err != nil {
		return err
	}
	if processed != 1 || len(failed) > 0 {
		event, _ := pdao.YoubanPublishCollectEvent.Ctx(ctx).Fields("error_message").WherePri(candidate.EventID).One()
		return gerror.Newf("重新处理采集事件失败：%s", strings.TrimSpace(event["error_message"].String()))
	}
	dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Fields("profile_id").
		Where("event_id", candidate.EventID).WhereGT("profile_id", 0).OrderDesc("id").One()
	if err != nil {
		return gerror.Wrap(err, "读取替换资料分发结果失败")
	}
	newProfileID := dispatch["profile_id"].Int64()
	if newProfileID <= 0 || newProfileID == candidate.ProfileID {
		return gerror.New("采集事件未生成新的替换资料")
	}
	// A committed replacement must be reused on later hourly runs while its
	// channel jobs are pending. Restoring the old fingerprint here would make
	// the same event deduplicate against the broken profile again.
	restoreOldFingerprint = false
	return s.finalizeProxyCollectMediaReplacement(ctx, candidate, newProfileID)
}

func (s *sSysPublish) proxyCollectMediaReplacementProfileID(ctx context.Context, candidate proxyMediaRepairCandidate) (int64, error) {
	dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Fields("profile_id").
		Where("event_id", candidate.EventID).WhereGT("profile_id", 0).OrderDesc("id").One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取缺失媒体替换资料失败")
	}
	profileID := dispatch["profile_id"].Int64()
	if profileID == candidate.ProfileID {
		return 0, nil
	}
	return profileID, nil
}

func (s *sSysPublish) finalizeProxyCollectMediaReplacement(ctx context.Context, candidate proxyMediaRepairCandidate, newProfileID int64) error {
	complete, err := collectProfileMediaComplete(ctx, newProfileID)
	if err != nil {
		return gerror.Wrap(err, "检查替换资料媒体失败")
	}
	if !complete {
		return gerror.New("替换资料媒体仍不完整")
	}
	jobStatus, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("COUNT(*) AS total,SUM(CASE WHEN status='sent' THEN 1 ELSE 0 END) AS sent").
		Where("collect_event_id", candidate.EventID).Where("profile_id", newProfileID).One()
	if err != nil {
		return gerror.Wrap(err, "检查替换资料推送任务失败")
	}
	if jobStatus["total"].Int() == 0 {
		return gerror.New("替换资料未生成推送任务")
	}
	if jobStatus["sent"].Int() != jobStatus["total"].Int() {
		return gerror.Newf("替换资料尚未完成全部频道推送：sent=%d total=%d", jobStatus["sent"].Int(), jobStatus["total"].Int())
	}
	if err = s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: []int64{candidate.ProfileID}}, candidate.TenantID, candidate.AccountID); err != nil {
		return gerror.Wrap(err, "删除旧缺失媒体资料失败")
	}
	s.appendCollectEventLog(ctx, candidate.EventID, proxyMediaRepairStage, "completed", "缺失媒体资料已替换", fmt.Sprintf("oldProfileId=%d newProfileId=%d", candidate.ProfileID, newProfileID))
	return nil
}

func (s *sSysPublish) resetMissingCollectEventMedia(ctx context.Context, eventID int64) (int, error) {
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(mediaCols.Id, mediaCols.FileUrl, mediaCols.StoragePath).
		Where(mediaCols.EventId, eventID).Where(mediaCols.CacheStatus, collectMediaCacheReady).All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待验证采集媒体失败")
	}
	missingIDs := make([]int64, 0)
	for _, row := range rows {
		mediaURL := normalizeMediaFileURL(row[mediaCols.FileUrl].String(), row[mediaCols.StoragePath].String())
		missing, checkErr := proxyMediaURLMissing(ctx, mediaURL)
		if checkErr != nil {
			return 0, checkErr
		}
		if missing {
			missingIDs = append(missingIDs, row[mediaCols.Id].Int64())
		}
	}
	if len(missingIDs) == 0 {
		return 0, nil
	}
	result, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.Id, missingIDs).Where(mediaCols.CacheStatus, collectMediaCacheReady).
		Data(g.Map{
			mediaCols.CacheStatus: collectMediaCachePending,
			mediaCols.FileUrl:     "", mediaCols.StoragePath: "", mediaCols.FileMd5: "", mediaCols.FilePhash: "",
			mediaCols.ErrorMessage:  "持久化媒体返回404，等待从Telegram源消息重新下载",
			collectMediaNextRetryAt: nil, mediaCols.UpdatedAt: gtime.Now(),
		}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "重置失效采集媒体失败")
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

func proxyMediaURLMissing(ctx context.Context, rawURL string) (bool, error) {
	if strings.TrimSpace(rawURL) == "" {
		return true, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return false, gerror.Wrap(err, "创建媒体对象探测请求失败")
	}
	response, err := proxyMediaRepairHTTPClient.Do(req)
	if err != nil {
		return false, gerror.Wrap(err, "探测媒体对象失败")
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone, nil
}
