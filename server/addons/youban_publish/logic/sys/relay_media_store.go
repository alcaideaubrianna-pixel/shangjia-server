package sys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type relayMediaRef struct {
	Id             int64  `json:"id"`
	RelayChannelId int64  `json:"relayChannelId"`
	RelayChatId    string `json:"relayChatId"`
	MessageId      int64  `json:"messageId"`
	FileId         string `json:"fileId"`
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
	if ready, err := relayReadyMedia(ctx, tenantId, mediaId, purpose, version); err != nil {
		return "", err
	} else if ready != nil {
		return "", nil
	}
	token := relayLeaseToken()
	now := time.Now()
	until := now.Add(lease)
	_, err := g.DB().Model("hg_youban_publish_relay_media").Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "media_id": mediaId, "purpose": purpose, "media_version": version,
		"status": "uploading", "lease_token": token, "lease_until": until, "updated_at": now,
	}).OnConflict("tenant_id,media_id,purpose,media_version").OnDuplicate(g.Map{"status": "uploading", "lease_token": token, "lease_until": until, "updated_at": now}).Save()
	if err != nil {
		return "", gerror.Wrap(err, "申请 relay 媒体租约失败")
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
	err := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).Where("tenant_id", tenantId).Where("status", relayChannelStatusEnabled).Where("health_status", relayChannelHealthReady).Where("(cooldown_until IS NULL OR cooldown_until < ?)", time.Now()).Order("in_flight_count ASC, last_used_at ASC, id ASC").Limit(1).Scan(&channel)
	if err != nil {
		return nil, gerror.Wrap(err, "选择 relay 频道失败")
	}
	if channel.Id == 0 {
		return nil, nil
	}
	return &channel, nil
}

func markRelayChannelUse(ctx context.Context, id int64, delta int) {
	var current int
	value, _ := g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(id).Value("in_flight_count")
	if value != nil {
		current = value.Int()
	}
	current += delta
	if current < 0 {
		current = 0
	}
	_, _ = g.DB().Model("hg_youban_publish_relay_channel").Ctx(ctx).WherePri(id).Data(g.Map{"in_flight_count": current, "last_used_at": time.Now()}).Update()
}
