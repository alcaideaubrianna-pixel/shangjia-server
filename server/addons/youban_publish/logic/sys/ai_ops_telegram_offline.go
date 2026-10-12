package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type aiOpsOfflineTarget struct {
	item       *sysin.AIOpsTelegramOfflineItem
	profileId  int64
	jobIds     []int64
	hadMessage bool
	hadActive  bool
}

func (s *sSysPublish) AIOpsTelegramMessageOffline(ctx context.Context, in *sysin.AIOpsTelegramOfflineInp) (*sysin.AIOpsTelegramOfflineResult, error) {
	if in == nil {
		return nil, gerror.New("下架参数不能为空")
	}
	targetCount := len(in.URLs) + len(in.Messages) + len(in.ProfileIds) + len(in.ProfileNos)
	if targetCount == 0 || targetCount > 100 {
		return nil, gerror.New("单次只能处理1到100个消息或资料目标")
	}

	targets := make([]*aiOpsOfflineTarget, 0, targetCount)
	for _, rawURL := range in.URLs {
		ref, err := parseAIOpsTelegramMessageURL(rawURL)
		if err != nil {
			targets = append(targets, failedAIOpsOfflineTarget(strings.TrimSpace(rawURL), err.Error()))
			continue
		}
		targets = append(targets, s.resolveAIOpsOfflineMessage(ctx, ref, strings.TrimSpace(rawURL)))
	}
	for _, message := range in.Messages {
		target := fmt.Sprintf("%s/%d", strings.TrimSpace(message.ChatId), message.MessageId)
		if strings.TrimSpace(message.ChatId) == "" || message.MessageId <= 0 {
			targets = append(targets, failedAIOpsOfflineTarget(target, "Chat ID和消息ID不能为空"))
			continue
		}
		targets = append(targets, s.resolveAIOpsOfflineMessage(ctx, aiOpsTelegramMessageRef{ChatId: normalizeTelegramChannelChatID(message.ChatId), MessageId: message.MessageId}, target))
	}
	for _, profileId := range uniqueIds(in.ProfileIds) {
		targets = append(targets, s.resolveAIOpsOfflineProfile(ctx, profileId, "profile_id:"+fmt.Sprint(profileId)))
	}
	for _, profileNo := range uniqueStrings(in.ProfileNos) {
		targets = append(targets, s.resolveAIOpsOfflineProfileNo(ctx, profileNo))
	}

	profileIds := make([]int64, 0, len(targets))
	for _, target := range targets {
		if target.profileId > 0 {
			profileIds = append(profileIds, target.profileId)
		}
	}
	// A message URL identifies its owning profile. Once resolved, clean every
	// active publication of that profile so no copy remains in another channel.
	for _, target := range targets {
		if target.profileId <= 0 || target.item.Status != "found" {
			continue
		}
		resolved := s.resolveAIOpsOfflineProfileJobs(ctx, target.item)
		if resolved.item.Status == "failed" {
			target.item = resolved.item
			continue
		}
		target.jobIds = resolved.jobIds
		target.item.JobIds = resolved.item.JobIds
	}
	offlineProfiles, offlineErr := s.aiOpsOfflineResolvedProfiles(ctx, uniqueIds(profileIds))
	if offlineErr != nil {
		return nil, offlineErr
	}

	queuedJobs := make(map[int64]struct{})
	result := &sysin.AIOpsTelegramOfflineResult{Total: len(targets), Items: make([]*sysin.AIOpsTelegramOfflineItem, 0, len(targets))}
	for _, target := range targets {
		item := target.item
		if item.Status == "failed" || item.Status == "not_found" || item.Status == "already_deleted" {
			appendAIOpsOfflineResult(result, item)
			continue
		}
		item.Offline = offlineProfiles[target.profileId]
		for _, jobId := range uniqueIds(target.jobIds) {
			if _, exists := queuedJobs[jobId]; exists {
				continue
			}
			if err := s.enqueueTelegramCleanupJob(ctx, jobId, 0); err != nil {
				item.Status, item.Message = "failed", gerror.Wrap(err, "加入TG消息清理队列失败").Error()
				break
			}
			queuedJobs[jobId] = struct{}{}
			item.CleanupQueued = true
			if job, err := s.telegramJobById(ctx, jobId); err == nil {
				s.appendTelegramJobLog(ctx, job, "ai_ops_offline", "queued", "AI运维精确下架，TG消息已加入统一清理队列")
			}
		}
		if item.Status != "failed" {
			if item.CleanupQueued || len(target.jobIds) > 0 {
				item.Status, item.Message = "cleanup_queued", "资料已下架，TG消息已加入统一清理队列"
			} else if target.hadMessage && !target.hadActive {
				item.Status, item.Message = "already_deleted", "TG消息已经删除"
			} else {
				item.Status, item.Message = "offline", "资料已下架，未找到需要清理的TG消息"
			}
		}
		appendAIOpsOfflineResult(result, item)
	}
	g.Log().Info(ctx, "AI运维精确下架Telegram消息", g.Map{"result": result, "queuedJobIds": mapKeysInt64(queuedJobs)})
	return result, nil
}

func (s *sSysPublish) resolveAIOpsOfflineMessage(ctx context.Context, ref aiOpsTelegramMessageRef, target string) *aiOpsOfflineTarget {
	item := &sysin.AIOpsTelegramOfflineItem{Target: target, Status: "not_found", Message: "未找到系统发送记录"}
	mod := g.DB().Model(publishTgMessageTable+" m").Ctx(ctx).
		Fields("m.job_id,m.profile_id,m.status,p.profile_no").
		LeftJoin("hg_content_profile p", "p.id=m.profile_id").
		Where("m.tg_message_id", ref.MessageId)
	if ref.Username != "" {
		mod = mod.LeftJoin(publishTgJobTable+" j", "j.id=m.job_id").
			LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
			Where("LOWER(TRIM(LEADING '@' FROM c.channel_username)) = ?", strings.ToLower(ref.Username))
	} else {
		mod = mod.Where("m.target_chat_id", ref.ChatId)
	}
	rows, err := mod.OrderDesc("m.id").All()
	if err != nil {
		return failedAIOpsOfflineTarget(target, gerror.Wrap(err, "查询TG消息失败").Error())
	}
	resolved := &aiOpsOfflineTarget{item: item, hadMessage: len(rows) > 0}
	for _, row := range rows {
		if resolved.profileId == 0 {
			resolved.profileId = row["profile_id"].Int64()
			item.ProfileId, item.ProfileNo = resolved.profileId, row["profile_no"].String()
		}
		if row["status"].String() == "sent" {
			resolved.hadActive = true
			resolved.jobIds = append(resolved.jobIds, row["job_id"].Int64())
		}
	}
	item.JobIds = uniqueIds(resolved.jobIds)
	if !resolved.hadMessage {
		return resolved
	}
	if !resolved.hadActive {
		item.Status, item.Message = "already_deleted", "TG消息已经删除"
		return resolved
	}
	item.Status, item.Message = "found", "已定位TG消息"
	return resolved
}

func (s *sSysPublish) resolveAIOpsOfflineProfile(ctx context.Context, profileId int64, target string) *aiOpsOfflineTarget {
	item := &sysin.AIOpsTelegramOfflineItem{Target: target, ProfileId: profileId, Status: "not_found", Message: "资料不存在"}
	row, err := g.DB().Model("hg_content_profile").Ctx(ctx).Fields("id,profile_no").Where("id", profileId).WhereNull("deleted_at").One()
	if err != nil {
		return failedAIOpsOfflineTarget(target, gerror.Wrap(err, "查询资料失败").Error())
	}
	if row.IsEmpty() {
		return &aiOpsOfflineTarget{item: item}
	}
	item.ProfileNo = row["profile_no"].String()
	return s.resolveAIOpsOfflineProfileJobs(ctx, item)
}

func (s *sSysPublish) resolveAIOpsOfflineProfileNo(ctx context.Context, profileNo string) *aiOpsOfflineTarget {
	target := "profile_no:" + strings.TrimSpace(profileNo)
	rows, err := g.DB().Model("hg_content_profile").Ctx(ctx).Fields("id,profile_no").Where("profile_no", strings.TrimSpace(profileNo)).WhereNull("deleted_at").Limit(2).All()
	if err != nil {
		return failedAIOpsOfflineTarget(target, gerror.Wrap(err, "查询资料编号失败").Error())
	}
	if len(rows) == 0 {
		return &aiOpsOfflineTarget{item: &sysin.AIOpsTelegramOfflineItem{Target: target, ProfileNo: profileNo, Status: "not_found", Message: "资料编号不存在"}}
	}
	if len(rows) > 1 {
		return failedAIOpsOfflineTarget(target, "资料编号对应多条资料，请改用资料ID")
	}
	item := &sysin.AIOpsTelegramOfflineItem{Target: target, ProfileId: rows[0]["id"].Int64(), ProfileNo: rows[0]["profile_no"].String()}
	return s.resolveAIOpsOfflineProfileJobs(ctx, item)
}

func (s *sSysPublish) resolveAIOpsOfflineProfileJobs(ctx context.Context, item *sysin.AIOpsTelegramOfflineItem) *aiOpsOfflineTarget {
	var rows []struct {
		JobId int64 `json:"job_id"`
	}
	err := g.DB().Model(publishTgMessageTable).Ctx(ctx).Fields("job_id").Where("profile_id", item.ProfileId).Where("status", "sent").Group("job_id").Scan(&rows)
	if err != nil {
		return failedAIOpsOfflineTarget(item.Target, gerror.Wrap(err, "查询资料TG消息失败").Error())
	}
	jobIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		jobIds = append(jobIds, row.JobId)
	}
	item.JobIds, item.Status, item.Message = uniqueIds(jobIds), "found", "已定位资料"
	return &aiOpsOfflineTarget{item: item, profileId: item.ProfileId, jobIds: item.JobIds, hadMessage: len(rows) > 0, hadActive: len(rows) > 0}
}

func (s *sSysPublish) aiOpsOfflineResolvedProfiles(ctx context.Context, profileIds []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(profileIds))
	if len(profileIds) == 0 {
		return result, nil
	}
	var owners []struct {
		ProfileId int64 `json:"profile_id"`
		TenantId  int64 `json:"tenant_id"`
	}
	if err := g.DB().Model(publishProfileStateTable).Ctx(ctx).Fields("profile_id,tenant_id").WhereIn("profile_id", profileIds).WhereNull("deleted_at").Scan(&owners); err != nil {
		return nil, gerror.Wrap(err, "读取资料租户失败")
	}
	groups := make(map[int64][]int64)
	for _, owner := range owners {
		if owner.ProfileId > 0 && owner.TenantId > 0 {
			groups[owner.TenantId] = append(groups[owner.TenantId], owner.ProfileId)
		}
	}
	for tenantId, ids := range groups {
		if err := s.setProfilesOffline(ctx, uniqueIds(ids), tenantId); err != nil {
			return nil, gerror.Wrap(err, "更新资料下架状态失败")
		}
		for _, id := range ids {
			result[id] = true
		}
	}
	return result, nil
}

func failedAIOpsOfflineTarget(target, message string) *aiOpsOfflineTarget {
	return &aiOpsOfflineTarget{item: &sysin.AIOpsTelegramOfflineItem{Target: target, Status: "failed", Message: message}}
}

func appendAIOpsOfflineResult(result *sysin.AIOpsTelegramOfflineResult, item *sysin.AIOpsTelegramOfflineItem) {
	result.Items = append(result.Items, item)
	switch item.Status {
	case "cleanup_queued":
		result.CleanupQueued++
	case "already_deleted":
		result.AlreadyDeleted++
	case "not_found":
		result.NotFound++
	case "failed":
		result.Failed++
	}
}

func mapKeysInt64(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}
