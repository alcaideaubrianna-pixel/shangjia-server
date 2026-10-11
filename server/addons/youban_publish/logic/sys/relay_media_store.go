package sys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	hglock "hotgo/internal/library/hgrds/lock"
)

type relayMediaRef struct {
	Id             int64  `json:"id"`
	RelayChannelId int64  `json:"relayChannelId"`
	RelayChatId    string `json:"relayChatId"`
	MessageId      int64  `json:"messageId"`
	FileId         string `json:"fileId"`
}

func relayLeaseAvailable(status string, leaseUntil, now time.Time) bool {
	return status == "uploading" && !leaseUntil.IsZero() && leaseUntil.After(now)
}

// relayAvailableForTenant prevents ordinary publishing from entering the relay
// queue when the tenant has no enabled and healthy relay channel.
func relayAvailableForTenant(ctx context.Context, tenantId int64) (bool, error) {
	if tenantId <= 0 {
		return false, nil
	}
	count, err := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("status", relayChannelStatusEnabled).
		Where("health_status", relayChannelHealthReady).
		Where("(cooldown_until IS NULL OR cooldown_until < ?)", time.Now()).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查 relay 频道可用性失败")
	}
	return count > 0, nil
}

func relayLeaseToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(b)
}

func relayReadyMedia(ctx context.Context, tenantId, mediaId int64, purpose, version string) (*relayMediaRef, error) {
	var ref relayMediaRef
	err := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx).Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Where("status", "ready").Scan(&ref)
	if err != nil {
		return nil, gerror.Wrap(err, "读取 relay 媒体索引失败")
	}
	if ref.Id == 0 {
		return nil, nil
	}
	return &ref, nil
}

func claimRelayMedia(ctx context.Context, tenantId, mediaId int64, purpose, version string, lease time.Duration) (string, error) {
	lock := hglock.NewConfig(lease, 100*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:relay:claim:%d:%d:%s:%s", tenantId, mediaId, purpose, version))
	if err := lock.Lock(ctx); err != nil {
		return "", gerror.Wrap(err, "获取 relay 媒体租约锁失败")
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	if ready, err := relayReadyMedia(ctx, tenantId, mediaId, purpose, version); err != nil {
		return "", err
	} else if ready != nil {
		return "", nil
	}
	token := relayLeaseToken()
	now := time.Now()
	until := now.Add(lease)
	model := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx)
	var state struct {
		Status     string    `orm:"status"`
		LeaseUntil time.Time `orm:"lease_until"`
	}
	if err := model.Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Scan(&state); err != nil {
		return "", gerror.Wrap(err, "读取 relay 媒体租约失败")
	}
	if relayLeaseAvailable(state.Status, state.LeaseUntil, now) {
		return "", nil
	}
	data := g.Map{"status": "uploading", "lease_token": token, "lease_until": until, "updated_at": now}
	if state.Status == "" {
		data["tenant_id"] = tenantId
		data["media_id"] = mediaId
		data["purpose"] = purpose
		data["media_version"] = version
		if _, err := model.Data(data).Insert(); err != nil {
			return "", gerror.Wrap(err, "创建 relay 媒体租约失败")
		}
	} else {
		result, err := model.Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Data(data).Update()
		if err != nil {
			return "", gerror.Wrap(err, "更新 relay 媒体租约失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return "", nil
		}
	}
	value, _ := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx).Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Value("lease_token")
	current := ""
	if value != nil {
		current = value.String()
	}
	if strings.TrimSpace(current) != token {
		return "", nil
	}
	return token, nil
}

func completeRelayMedia(ctx context.Context, tenantId, mediaId int64, purpose, version, token string, ref relayMediaRef) error {
	_, err := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx).Data(g.Map{"relay_channel_id": ref.RelayChannelId, "relay_chat_id": ref.RelayChatId, "message_id": ref.MessageId, "file_id": ref.FileId, "status": "ready", "lease_token": "", "lease_until": nil, "last_error": "", "updated_at": time.Now()}).Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Where("lease_token", token).Update()
	return err
}

func failRelayMedia(ctx context.Context, tenantId, mediaId int64, purpose, version, token string, cause error) error {
	_, err := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx).Data(g.Map{"status": "failed", "lease_token": "", "lease_until": nil, "last_error": cause.Error(), "updated_at": time.Now()}).Where("tenant_id", tenantId).Where("media_id", mediaId).Where("purpose", purpose).Where("media_version", version).Where("lease_token", token).Update()
	return err
}

func chooseRelayChannel(ctx context.Context, tenantId int64) (*relayChannelRecord, error) {
	var channel relayChannelRecord
	err := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).Where("tenant_id", tenantId).Where("status", relayChannelStatusEnabled).Where("(health_status = ? OR (health_status = ? AND cooldown_until < ?))", relayChannelHealthReady, relayChannelHealthCooldown, time.Now()).Where("(cooldown_until IS NULL OR cooldown_until < ?)", time.Now()).Order("in_flight_count ASC, last_used_at ASC, id ASC").Limit(1).Scan(&channel)
	if err != nil {
		return nil, gerror.Wrap(err, "选择 relay 频道失败")
	}
	if channel.Id == 0 {
		return nil, nil
	}
	if channel.HealthStatus == relayChannelHealthCooldown {
		_, _ = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(channel.Id).Data(g.Map{"health_status": relayChannelHealthReady, "cooldown_until": nil, "updated_at": time.Now()}).Update()
	}
	return &channel, nil
}

func markRelayChannelUse(ctx context.Context, id int64, delta int) {
	if delta > 0 {
		_, _ = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(id).Data(g.Map{"in_flight_count": gdb.Raw(fmt.Sprintf("in_flight_count + %d", delta)), "last_used_at": time.Now()}).Update()
		return
	}
	_, _ = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(id).Where("in_flight_count > 0").Data(g.Map{"in_flight_count": gdb.Raw(fmt.Sprintf("in_flight_count + %d", delta)), "last_used_at": time.Now()}).Update()
}

func markRelayChannelFailure(ctx context.Context, id int64, cause error, cooldown time.Duration) {
	if id <= 0 || cause == nil {
		return
	}
	status := relayChannelHealthError
	if cooldown > 0 {
		status = relayChannelHealthCooldown
	}
	_, _ = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(id).Data(g.Map{"health_status": status, "cooldown_until": time.Now().Add(cooldown), "last_error": cause.Error(), "updated_at": time.Now()}).Update()
}
