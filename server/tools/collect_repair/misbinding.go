package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishsys "hotgo/addons/youban_publish/logic/sys"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type misbindingCandidate struct {
	VerifyEventID    int64   `json:"verifyEventId"`
	OldDisplayID     int64   `json:"oldDisplayId"`
	NewDisplayID     int64   `json:"newDisplayId"`
	ProfileID        int64   `json:"profileId"`
	ProfileNo        string  `json:"profileNo"`
	OldProfileID     int64   `json:"oldProfileId"`
	TenantID         int64   `json:"tenantId"`
	AccountID        int64   `json:"accountId"`
	SourceID         int64   `json:"sourceId"`
	SourceChatID     string  `json:"sourceChatId"`
	VerifyMessage    int64   `json:"verifyMessageId"`
	NewMessage       int64   `json:"newDisplayMessageId"`
	ReadyVideos      int     `json:"readyVideos"`
	VerifyMedia      int     `json:"verifyMedia"`
	TargetVerifies   int     `json:"targetVerifies"`
	OldIndependent   int     `json:"oldIndependentVerifies"`
	OldUsesCandidate int     `json:"oldProfileUsesCandidate"`
	RepairLogged     bool    `json:"repairLogged"`
	RepublishJobs    int     `json:"republishJobs"`
	ChannelIDs       []int64 `json:"channelIds,omitempty"`
	Decision         string  `json:"decision"`
	Reason           string  `json:"reason,omitempty"`
	Applied          bool    `json:"applied"`
	OfflineQueued    bool    `json:"offlineQueued"`
	RepublishQueued  bool    `json:"republishQueued"`
	Error            string  `json:"error,omitempty"`
}

type misbindingReport struct {
	GeneratedAt time.Time             `json:"generatedAt"`
	DryRun      bool                  `json:"dryRun"`
	SourceID    int64                 `json:"sourceId"`
	Since       string                `json:"since"`
	Until       string                `json:"until"`
	Candidates  []misbindingCandidate `json:"candidates"`
}

// scanHistoricalVerificationMisbindings intentionally uses PostgreSQL LATERAL
// only in this production repair tool. Runtime pairing remains database-neutral.
func scanHistoricalVerificationMisbindings(ctx context.Context, sourceID int64, since, until time.Time, limit int) ([]misbindingCandidate, error) {
	if sourceID <= 0 || since.IsZero() || until.IsZero() || !until.After(since) {
		return nil, gerror.New("历史错绑审计必须指定 source-id、since 和 until")
	}
	if limit <= 0 || limit > 5000 {
		return nil, gerror.New("历史错绑审计 limit 必须在1到5000之间")
	}
	query := `
SELECT v.id verify_event_id,COALESCE(rl.old_display_id,v.material_parent_event_id) old_display_id,n.id new_display_id,
       td.profile_id,COALESCE(p.profile_no,'') profile_no,od.profile_id old_profile_id,
       v.tenant_id,v.account_id,v.source_id,v.source_chat_id,
       v.source_message_id verify_message_id,n.source_message_id new_message_id,
       (SELECT COUNT(*) FROM hg_youban_publish_collect_event_media vm WHERE vm.event_id=v.id AND vm.media_type='video' AND vm.cache_status='ready' AND (COALESCE(vm.file_url,'')<>'' OR COALESCE(vm.storage_path,'')<>'')) ready_videos,
       (SELECT COUNT(*) FROM hg_youban_publish_collect_event_media vm WHERE vm.event_id=v.id AND vm.media_type='video') verify_media,
       (SELECT COUNT(*) FROM hg_youban_publish_media pm WHERE pm.profile_id=td.profile_id AND pm.purpose='verify' AND pm.media_type='video' AND pm.deleted_at IS NULL) target_verifies,
       (SELECT COUNT(*) FROM hg_youban_publish_collect_event ov WHERE ov.material_parent_event_id=v.material_parent_event_id AND ov.material_role='verify' AND ov.id<>v.id) old_independent
       ,(SELECT COUNT(*) FROM hg_youban_publish_media opm
          JOIN hg_youban_publish_collect_event_media vem ON vem.event_id=v.id AND vem.media_type='video'
          WHERE opm.profile_id=od.profile_id AND opm.purpose='verify' AND opm.media_type='video' AND opm.deleted_at IS NULL
            AND ((COALESCE(opm.storage_path,'')<>'' AND opm.storage_path=vem.storage_path) OR (COALESCE(opm.file_url,'')<>'' AND opm.file_url=vem.file_url))) old_uses_candidate
	       ,(rl.old_display_id IS NOT NULL) repair_logged
	       ,(SELECT COUNT(*) FROM hg_youban_publish_tg_job rj WHERE rj.profile_id=td.profile_id AND rj.operation_no=CONCAT('verify-misbinding-repair:',v.id,':profile:',td.profile_id)) republish_jobs
FROM hg_youban_publish_collect_event v
JOIN LATERAL (
  SELECT d.id,d.source_message_id
  FROM hg_youban_publish_collect_event d
  WHERE d.tenant_id=v.tenant_id AND d.account_id=v.account_id
    AND d.tg_account_id=v.tg_account_id AND d.source_id=v.source_id
    AND d.source_chat_id=v.source_chat_id AND d.material_role='display'
    AND d.source_message_id<v.source_message_id
    AND COALESCE(d.received_at,d.created_at) BETWEEN COALESCE(v.received_at,v.created_at)-INTERVAL '5 minutes' AND COALESCE(v.received_at,v.created_at)
  ORDER BY d.source_message_id DESC,d.id DESC LIMIT 1
) n ON true
	LEFT JOIN LATERAL (SELECT (l.meta_text::jsonb->>'oldParent')::bigint old_display_id FROM hg_youban_publish_collect_event_log l WHERE l.event_id=v.id AND l.stage='repair' AND l.status='misbinding_repaired' ORDER BY l.id DESC LIMIT 1) rl ON true
JOIN LATERAL (SELECT d.profile_id FROM hg_youban_publish_collect_dispatch d WHERE d.event_id=n.id AND d.profile_id>0 ORDER BY d.id DESC LIMIT 1) td ON true
LEFT JOIN LATERAL (SELECT d.profile_id FROM hg_youban_publish_collect_dispatch d WHERE d.event_id=v.material_parent_event_id AND d.profile_id>0 ORDER BY d.id DESC LIMIT 1) od ON true
LEFT JOIN hg_content_profile p ON p.id=td.profile_id
	WHERE v.source_id=? AND v.material_role='verify' AND v.material_parent_event_id>0
	  AND (v.material_parent_event_id<>n.id OR rl.old_display_id IS NOT NULL) AND v.created_at>=? AND v.created_at<?
ORDER BY v.id LIMIT ?`
	var rows []misbindingCandidate
	if err := g.DB().Raw(query, sourceID, since, until, limit+1).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "扫描历史验证视频错绑失败")
	}
	if len(rows) > limit {
		return nil, gerror.Newf("候选超过安全上限%d，请缩小时间范围", limit)
	}
	counts := make(map[int64]int)
	profileCounts := make(map[int64]int)
	for _, row := range rows {
		counts[row.NewDisplayID]++
		profileCounts[row.ProfileID]++
	}
	channelsByProfile, err := historicalProfileChannelIDs(ctx, rows)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		row := &rows[i]
		row.Decision, row.Reason = classifyMisbindingCandidate(*row, counts[row.NewDisplayID], profileCounts[row.ProfileID])
		row.ChannelIDs = channelsByProfile[row.ProfileID]
		if row.Decision == "safe" && len(row.ChannelIDs) == 0 {
			row.Decision, row.Reason = "manual", "资料没有历史成功发布频道"
		}
	}
	return rows, nil
}

func classifyMisbindingCandidate(row misbindingCandidate, targetCandidateCount, profileCandidateCount int) (string, string) {
	switch {
	case row.RepairLogged && row.RepublishJobs > 0:
		return "skip", "修复重新上架任务已存在"
	case row.RepairLogged && row.TargetVerifies > 0 && row.ReadyVideos == row.VerifyMedia && row.VerifyMedia > 0:
		return "safe", "已完成改绑，继续下架和重新上架"
	case row.RepairLogged:
		return "manual", "已改绑资料的验证媒体状态异常"
	case targetCandidateCount != 1:
		return "manual", "目标展示事件存在多个验证候选"
	case profileCandidateCount != 1:
		return "manual", "目标资料存在多个验证候选"
	case row.ProfileID <= 0 || row.OldProfileID <= 0:
		return "manual", "新旧展示事件缺少资料映射"
	case row.ProfileID == row.OldProfileID:
		return "skip", "新旧事件已映射同一资料"
	case row.VerifyMedia == 0 || row.ReadyVideos != row.VerifyMedia:
		return "manual", "验证媒体未全部缓存就绪"
	case row.TargetVerifies > 0:
		return "manual", "目标资料已存在验证视频"
	case row.OldIndependent == 0:
		return "manual", "旧资料没有独立验证视频，禁止自动拆绑"
	case row.OldUsesCandidate > 0:
		return "manual", "旧资料正在使用候选验证视频，需要人工拆除"
	default:
		return "safe", ""
	}
}

func historicalProfileChannelIDs(ctx context.Context, candidates []misbindingCandidate) (map[int64][]int64, error) {
	profileIDs := make([]int64, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.ProfileID <= 0 {
			continue
		}
		if _, ok := seen[candidate.ProfileID]; ok {
			continue
		}
		seen[candidate.ProfileID] = struct{}{}
		profileIDs = append(profileIDs, candidate.ProfileID)
	}
	result := make(map[int64][]int64, len(profileIDs))
	if len(profileIDs) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model("hg_youban_publish_tg_job").Ctx(ctx).
		Fields("profile_id,channel_id").WhereIn("profile_id", profileIDs).
		Where("status", "sent").Group("profile_id,channel_id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "批量读取资料原发布频道失败")
	}
	for _, row := range rows {
		profileID, channelID := row["profile_id"].Int64(), row["channel_id"].Int64()
		if profileID > 0 && channelID > 0 {
			result[profileID] = append(result[profileID], channelID)
		}
	}
	for profileID := range result {
		sort.Slice(result[profileID], func(i, j int) bool { return result[profileID][i] < result[profileID][j] })
	}
	return result, nil
}

func applyHistoricalVerificationRebind(ctx context.Context, row *misbindingCandidate) error {
	if row == nil || row.Decision != "safe" {
		return gerror.New("仅允许处理审计判定为 safe 的候选")
	}
	now := gtime.Now()
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		verify, err := tx.Model("hg_youban_publish_collect_event").Ctx(ctx).Where("id", row.VerifyEventID).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定验证采集事件失败")
		}
		parent := verify["material_parent_event_id"].Int64()
		if parent == row.NewDisplayID {
			row.Applied = true
			return nil
		}
		if parent != row.OldDisplayID {
			return gerror.Newf("验证事件父级已变化 verify:%d expected:%d actual:%d", row.VerifyEventID, row.OldDisplayID, parent)
		}
		if _, err = tx.Model("hg_youban_publish_collect_event").Ctx(ctx).Where("id", row.VerifyEventID).Data(g.Map{"material_parent_event_id": row.NewDisplayID, "material_group_status": "paired", "error_message": "", "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "修正验证事件父级失败")
		}
		insert := `INSERT INTO hg_youban_publish_media
 (tenant_id,merchant_id,account_id,profile_id,media_type,purpose,name,file_url,storage_path,poster_url,tg_file_id,tg_cache_asset_hash,tg_cache_status,mime_type,md5,perceptual_hash,size,sort_index,status,created_by,updated_by,created_at,updated_at)
 SELECT ?,?,?,?,'video','verify',CONCAT('verify-repair-',em.id),em.file_url,em.storage_path,em.poster_url,'',COALESCE(NULLIF(em.storage_path,''),em.file_url),'invalid',em.source_mime_type,em.file_md5,em.file_phash,em.source_size,em.sort_index,1,?,?,?,?
 FROM hg_youban_publish_collect_event_media em
 WHERE em.event_id=? AND em.media_type='video' AND em.cache_status='ready'
   AND NOT EXISTS (SELECT 1 FROM hg_youban_publish_media pm WHERE pm.profile_id=? AND pm.purpose='verify' AND pm.media_type='video' AND pm.deleted_at IS NULL AND (NULLIF(pm.storage_path,'')=NULLIF(em.storage_path,'') OR NULLIF(pm.file_url,'')=NULLIF(em.file_url,'')))`
		if _, err = tx.Exec(insert, row.TenantID, row.TenantID, row.AccountID, row.ProfileID, row.AccountID, row.AccountID, now, now, row.VerifyEventID, row.ProfileID); err != nil {
			return gerror.Wrap(err, "补充目标资料验证媒体失败")
		}
		videoCount, countErr := tx.Model("hg_youban_publish_media").Ctx(ctx).Where("profile_id", row.ProfileID).Where("media_type", "video").WhereNull("deleted_at").Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "统计目标资料视频失败")
		}
		if _, err = tx.Model("hg_content_profile").Ctx(ctx).Where("id", row.ProfileID).Data(g.Map{"has_verification_video": 1, "video_count": videoCount, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "更新目标资料验证状态失败")
		}
		contextJSON := fmt.Sprintf(`{"repair":"historical_verification_misbinding","oldParent":%d,"newParent":%d,"profileId":%d}`, row.OldDisplayID, row.NewDisplayID, row.ProfileID)
		_, err = tx.Model("hg_youban_publish_collect_event_log").Ctx(ctx).Data(g.Map{"tenant_id": row.TenantID, "account_id": row.AccountID, "event_id": row.VerifyEventID, "stage": "repair", "status": "misbinding_repaired", "message": "历史验证视频错绑已修正", "meta_text": contextJSON, "created_at": now}).Insert()
		if err != nil {
			return gerror.Wrap(err, "写入历史错绑修复审计日志失败")
		}
		row.Applied = true
		return nil
	})
}

func runMisbindingRepair(ctx context.Context, opts options) error {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	since, err := time.ParseInLocation("2006-01-02 15:04:05", opts.misbindingSince, loc)
	if err != nil {
		return gerror.New("misbinding-since 格式必须为 YYYY-MM-DD HH:MM:SS")
	}
	until, err := time.ParseInLocation("2006-01-02 15:04:05", opts.misbindingUntil, loc)
	if err != nil {
		return gerror.New("misbinding-until 格式必须为 YYYY-MM-DD HH:MM:SS")
	}
	rows, err := scanHistoricalVerificationMisbindings(ctx, opts.sourceID, since, until, opts.limit)
	if err != nil {
		return err
	}
	if opts.eventIDs != "" {
		ids, parseErr := parseEventIDs(opts.eventIDs)
		if parseErr != nil {
			return parseErr
		}
		selected := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			selected[id] = struct{}{}
		}
		filtered := make([]misbindingCandidate, 0, len(ids))
		for _, row := range rows {
			_, verifySelected := selected[row.VerifyEventID]
			_, displaySelected := selected[row.NewDisplayID]
			if verifySelected || displaySelected {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) != len(ids) {
			return gerror.New("event-ids 必须唯一命中历史错绑候选的验证事件或目标展示事件")
		}
		rows = filtered
	}
	report := misbindingReport{GeneratedAt: time.Now(), DryRun: !opts.misbindingApply, SourceID: opts.sourceID, Since: opts.misbindingSince, Until: opts.misbindingUntil, Candidates: rows}
	defer func() {
		file, createErr := os.OpenFile(opts.report, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if createErr == nil {
			defer file.Close()
			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			_ = encoder.Encode(report)
		}
	}()
	if !opts.misbindingApply {
		fmt.Printf("misbinding audit complete: candidates=%d report=%s; no data changed\n", len(rows), opts.report)
		return nil
	}
	service := publishsys.NewSysPublish()
	for i := range report.Candidates {
		row := &report.Candidates[i]
		if row.Decision != "safe" {
			continue
		}
		if err = applyHistoricalVerificationRebind(ctx, row); err != nil {
			row.Error = err.Error()
			continue
		}
		result, offlineErr := service.AIOpsTelegramMessageOffline(ctx, &sysin.AIOpsTelegramOfflineInp{ProfileIds: []int64{row.ProfileID}})
		if offlineErr != nil {
			row.Error = offlineErr.Error()
			continue
		}
		row.OfflineQueued = result.CleanupQueued > 0 || result.AlreadyDeleted > 0
	}
	if !opts.misbindingRepublish {
		return nil
	}
	for i := range report.Candidates {
		row := &report.Candidates[i]
		if !row.Applied || row.Error != "" {
			continue
		}
		deadline := time.Now().Add(opts.cleanupWait)
		for {
			active, countErr := g.DB().Model("hg_youban_publish_tg_message").Ctx(ctx).Where("profile_id", row.ProfileID).Where("status", "sent").Count()
			if countErr != nil {
				row.Error = countErr.Error()
				break
			}
			if active == 0 {
				break
			}
			if time.Now().After(deadline) {
				row.Error = "等待历史TG消息下架超时，未重新上架"
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		if row.Error != "" {
			continue
		}
		op := fmt.Sprintf("verify-misbinding-repair:%d:profile:%d", row.VerifyEventID, row.ProfileID)
		if err = service.AIOpsRepublishProfileChannels(ctx, row.ProfileID, row.ChannelIDs, op); err != nil {
			row.Error = err.Error()
			continue
		}
		row.RepublishQueued = true
	}
	errors := 0
	for _, row := range report.Candidates {
		if strings.TrimSpace(row.Error) != "" {
			errors++
		}
	}
	if errors > 0 {
		return gerror.Newf("历史错绑修复完成但有%d条失败，详情见%s", errors, opts.report)
	}
	return nil
}
