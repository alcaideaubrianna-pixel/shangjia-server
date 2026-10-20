package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// The snapshot is committed before any Telegram call. Recovery, cleanup and
// retries must use exactly the same IDs and order even after profile edits.
func (s *sSysPublish) selectTelegramDisplayMediaForTenant(ctx context.Context, job telegramJobRecord, media []*telegramMediaItem) (selected []*telegramMediaItem, err error) {
	stage := beginTelegramPublishStage(ctx, "media.selection", g.Map{"jobId": job.Id})
	defer stage.Finish(&err)
	// The common retry path needs only one read, with no lock or VIP lookup.
	row, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Fields("id,display_media_selection").Where("id", job.Id).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取展示媒体选择失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("TG发送任务不存在")
	}
	if payload := row["display_media_selection"].String(); payload != "" {
		return restoreTelegramDisplaySelection(payload, media)
	}
	// Never wait for Redis or membership queries while holding the job lock.
	status, err := s.tenantVipStatus(ctx, job.TenantId)
	if err != nil {
		return nil, err
	}
	randomEnabled := status.IsVip && containsString(status.Features, sysin.TenantVipFeatureRandomMedia)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, err := tx.Model(publishTgJobTable).Ctx(ctx).Fields("id,display_media_selection,send_phase").Where("id", job.Id).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定展示媒体选择失败")
		}
		if row.IsEmpty() {
			return gerror.New("TG发送任务不存在")
		}
		if payload := row["display_media_selection"].String(); payload != "" {
			selected, err = restoreTelegramDisplaySelection(payload, media)
			return err
		}
		attempts, err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("job_id", job.Id).Where("phase", "display").Count()
		if err != nil {
			return gerror.Wrap(err, "检查历史展示发送记录失败")
		}
		phase := row["send_phase"].String()
		if attempts > 0 || phase == telegramSendPhaseDisplaySending || phase == telegramSendPhaseDisplayConfirmed || phase == telegramSendPhaseVerifySending || phase == telegramSendPhaseVerifyConfirmed || phase == telegramSendPhaseCompletedNoVerify {
			// Already-started jobs retain the exact pre-upgrade algorithm.
			selected = selectTelegramDisplayMedia(job, media, telegramMediaGroupMaxItems, randomEnabled)
		} else {
			state, stateErr := tx.Model(publishProfileStateTable).Ctx(ctx).Fields("random_media_config").Where("profile_id", job.ProfileId).Where("tenant_id", job.TenantId).One()
			if stateErr != nil {
				return gerror.Wrap(stateErr, "读取随机选图配置失败")
			}
			if state.IsEmpty() {
				return gerror.New("资料发布配置不存在")
			}
			policy, policyErr := decodeProfileRandomMedia(state["random_media_config"].String())
			if policyErr != nil {
				return policyErr
			}
			if randomEnabled {
				selected = selectTelegramDisplayMediaWithPolicy(job, media, policy)
			} else {
				selected = selectFreeTelegramDisplayMedia(media, telegramMediaGroupMaxItems)
			}
		}
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(selected))
		for _, item := range selected {
			ids = append(ids, item.Id)
		}
		payload, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		_, err = tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).Data(g.Map{"display_media_selection": string(payload)}).Update()
		return gerror.Wrap(err, "保存展示媒体选择失败")
	})
	return
}

func restoreTelegramDisplaySelection(payload string, media []*telegramMediaItem) ([]*telegramMediaItem, error) {
	var ids []int64
	if err := json.Unmarshal([]byte(payload), &ids); err != nil {
		return nil, gerror.Wrap(err, "解析展示媒体快照失败")
	}
	byID := make(map[int64]*telegramMediaItem, len(media))
	for _, item := range media {
		if item != nil {
			byID[item.Id] = item
		}
	}
	selected := make([]*telegramMediaItem, 0, len(ids))
	for _, id := range ids {
		item := byID[id]
		if item == nil {
			return nil, gerror.Newf("本次发送的展示媒体%d已缺失，不能重新随机选择", id)
		}
		selected = append(selected, item)
	}
	return selected, nil
}

func selectTelegramDisplayMediaWithPolicy(job telegramJobRecord, media []*telegramMediaItem, policy *sysin.ProfileRandomMediaPolicy) []*telegramMediaItem {
	var images, requiredImages, videos, requiredVideos, other []*telegramMediaItem
	for _, item := range media {
		if item == nil || strings.EqualFold(strings.TrimSpace(item.Purpose), "verify") {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.MediaType)) {
		case "image":
			if item.MustSend {
				requiredImages = append(requiredImages, item)
			} else {
				images = append(images, item)
			}
		case "video":
			if item.MustSend {
				requiredVideos = append(requiredVideos, item)
			} else {
				videos = append(videos, item)
			}
		default:
			other = append(other, item)
		}
	}
	enabled, count := len(images)+len(requiredImages) > 10, 10
	if policy != nil && policy.Enabled != nil {
		enabled = *policy.Enabled
		count = policy.Count
		if count <= 0 {
			count = 5
		}
	}
	if enabled {
		sortTelegramRandomMedia(job, images)
		slots := count - len(requiredImages)
		if slots < 0 {
			slots = 0
		}
		if len(images) > slots {
			images = images[:slots]
		}
	}
	if len(requiredVideos) == 0 && len(videos) > 0 {
		sortTelegramRandomMedia(job, videos)
		other = append(other, videos[0])
	}
	// Ordinary media first, required media last. Required ordering remains the
	// profile order; optional image ordering changes only for a new job.
	selected := append(images, other...)
	selected = append(selected, requiredVideos...)
	return append(selected, requiredImages...)
}

func sortTelegramRandomMedia(job telegramJobRecord, media []*telegramMediaItem) {
	sort.SliceStable(media, func(i, j int) bool {
		a := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", job.Id, job.ProfileId, job.ChannelId, media[i].Id)))
		b := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", job.Id, job.ProfileId, job.ChannelId, media[j].Id)))
		if a == b {
			return media[i].Id < media[j].Id
		}
		return bytes.Compare(a[:], b[:]) < 0
	})
}
