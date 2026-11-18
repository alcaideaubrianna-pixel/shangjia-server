package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	collectIdentityTable        = "hg_youban_publish_collect_identity"
	collectDuplicateGroupStatus = "duplicate_mapped"
)

// recordCollectDuplicateIdentity persists the fingerprint decision, not a
// guessed relationship from error text. The row is immutable per rule branch.
func (s *sSysPublish) recordCollectDuplicateIdentity(ctx context.Context, event, rule gdb.Record, hit *profileFingerprintDuplicateError) error {
	defer restoreCollectMaterialTransactionContext(ctx)()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if event.IsEmpty() || rule.IsEmpty() || hit == nil || hit.ProfileID <= 0 {
		return gerror.New("重复资料身份映射参数不完整")
	}
	tenantID, accountID, sourceID, ruleID, eventID := event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64(), rule["id"].Int64(), event["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 || eventID <= 0 || collectRuleSourceID(event, rule) != sourceID {
		return gerror.New("重复资料身份映射归属不完整")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := pdao.YoubanPublishCollectEvent.Columns()
		locked, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(c.Id, eventID).Where(c.TenantId, tenantID).Where(c.AccountId, collectEventStorageAccountID(event)).Where(c.SourceId, sourceID).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定采集身份事件失败")
		}
		if locked.IsEmpty() {
			return gerror.New("采集身份事件不存在或归属变化")
		}
		if canonicalCollectSourceChatID(locked[c.SourceChatId].String()) != canonicalCollectSourceChatID(event[c.SourceChatId].String()) {
			return gerror.New("采集身份实际频道已变化")
		}
		owner, err := tx.Model(publishProfileStateTable).Ctx(ctx).Where("profile_id", hit.ProfileID).Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at").One()
		if err != nil {
			return gerror.Wrap(err, "核对资料身份归属失败")
		}
		if owner.IsEmpty() {
			return gerror.New("资料身份归属无效")
		}
		scope := func() *gdb.Model {
			return tx.Model(collectIdentityTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).Where("source_id", sourceID).Where("rule_id", ruleID).Where("event_id", eventID)
		}
		old, err := scope().One()
		if err != nil {
			return gerror.Wrap(err, "读取采集身份失败")
		}
		if !old.IsEmpty() {
			if old["profile_id"].Int64() != hit.ProfileID || canonicalCollectSourceChatID(old["source_chat_id"].String()) != canonicalCollectSourceChatID(event["source_chat_id"].String()) {
				return gerror.New("采集身份已绑定其他资料，拒绝覆盖")
			}
			return nil
		}
		d := pdao.YoubanPublishCollectDispatch.Columns()
		rows, err := tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).Fields(d.EventId).
			Where(d.TenantId, tenantID).WhereIn(d.AccountId, collectEventAccountScope(event)).Where(d.SourceId, sourceID).Where(d.RuleId, ruleID).Where(d.ProfileId, hit.ProfileID).
			WhereIn(d.Status, []string{sysin.CollectDispatchStatusSent, sysin.CollectDispatchStatusPending}).OrderAsc(d.Id).Limit(2).All()
		if err != nil {
			return gerror.Wrap(err, "读取正式资料分支失败")
		}
		canonicalID := int64(0)
		if len(rows) == 1 {
			original, readErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Fields(c.Id).
				Where(c.Id, rows[0][d.EventId].Int64()).Where(c.TenantId, tenantID).WhereIn(c.AccountId, collectEventAccountScope(event)).Where(c.SourceId, sourceID).
				WhereIn(c.SourceChatId, tgChannelCacheLookupIds(event["source_chat_id"].String())).One()
			if readErr != nil {
				return gerror.Wrap(readErr, "核对正式资料实际频道失败")
			}
			if !original.IsEmpty() {
				canonicalID = original[c.Id].Int64()
			}
		}
		_, err = tx.Model(collectIdentityTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantID, "account_id": accountID, "source_id": sourceID, "rule_id": ruleID, "event_id": eventID,
			"profile_id": hit.ProfileID, "canonical_event_id": canonicalID, "source_chat_id": canonicalCollectSourceChatID(event["source_chat_id"].String()), "match_layer": hit.Layer,
			"created_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "持久化采集身份失败")
		}
		_, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(c.Id, eventID).
			Data(g.Map{"material_group_status": collectDuplicateGroupStatus, "updated_at": gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "标记重复采集身份失败")
		}
		g.Log().Info(ctx, "采集重复资料身份已保存", g.Map{
			"event": "collect_duplicate_identity_recorded", "tenantId": tenantID, "accountId": accountID,
			"sourceId": sourceID, "ruleId": ruleID, "collectEventId": eventID,
			"profileId": hit.ProfileID, "canonicalEventId": canonicalID, "matchLayer": hit.Layer,
		})
		return nil
	})
}

// Mapped duplicates remain source evidence, never replacement parents. Only
// their rule-scoped profile association is reused for collection media merge.
func collectEventHasDuplicateIdentity(ctx context.Context, event gdb.Record) (bool, error) {
	return collectEventHasDuplicateIdentityModel(ctx, g.DB().Model(collectIdentityTable).Ctx(ctx), event)
}

func collectEventHasDuplicateIdentityModel(ctx context.Context, model *gdb.Model, event gdb.Record) (bool, error) {
	rows, err := model.Ctx(ctx).Fields("event_id").
		Where("tenant_id", event["tenant_id"].Int64()).Where("source_id", event["source_id"].Int64()).
		Where("event_id", event["id"].Int64()).Where("source_chat_id", canonicalCollectSourceChatID(event["source_chat_id"].String())).
		WhereGT("canonical_event_id", 0).Where("match_layer", "text_hash").Limit(1).All()
	return len(rows) > 0, gerror.Wrap(err, "读取采集事件身份映射失败")
}

func collectEventCanResumeDuplicate(ctx context.Context, event gdb.Record) (bool, error) {
	if event["status"].String() != sysin.CollectEventStatusIgnored || event["material_group_status"].String() != collectDuplicateGroupStatus {
		return false, nil
	}
	return collectEventHasDuplicateIdentity(ctx, event)
}

// mergeCollectDuplicateVerification changes collection material only. It does
// not enqueue a TG Job, alter source pairing, replace display, or republish.
func (s *sSysPublish) mergeCollectDuplicateVerification(ctx context.Context, event, rule gdb.Record, content *collectContentResult) error {
	if !collectContentHasVerification(content) || rule["review_enabled"].Int() == 1 {
		return nil
	}
	identity, err := g.DB().Model(collectIdentityTable).Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).Where("account_id", event["account_id"].Int64()).
		Where("source_id", event["source_id"].Int64()).Where("rule_id", rule["id"].Int64()).Where("event_id", event["id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取验证归并身份失败")
	}
	if identity.IsEmpty() || identity["canonical_event_id"].Int64() <= 0 || identity["match_layer"].String() != "text_hash" {
		return nil
	}
	if identity["source_chat_id"].String() != canonicalCollectSourceChatID(event["source_chat_id"].String()) {
		return gerror.New("验证归并身份频道冲突")
	}
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return err
	}
	profileID := identity["profile_id"].Int64()
	if err = s.mergeCollectIdentityPreparedVerification(ctx, event, profileID, prepared.Media); err != nil {
		return err
	}

	if err = s.syncProfileNoteIndex(ctx, profileID); err != nil {
		return err
	}
	g.Log().Info(ctx, "采集验证资料已按显式身份归并", g.Map{"event": "collect_verification_identity_merged", "collectEventId": event["id"].Int64(), "ruleId": rule["id"].Int64(), "profileId": profileID, "canonicalEventId": identity["canonical_event_id"].Int64()})
	return nil
}

// mergeCollectIdentityPreparedVerification is the collection commit boundary.
// Asset preparation stays outside the short owner/media transaction.
func (s *sSysPublish) mergeCollectIdentityPreparedVerification(ctx context.Context, event gdb.Record, profileID int64, media []collectPreparedMedia) error {

	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		owner, err := tx.Model(publishProfileStateTable).Ctx(ctx).Where("profile_id", profileID).
			Where("tenant_id", event["tenant_id"].Int64()).Where("account_id", event["account_id"].Int64()).WhereNull("deleted_at").LockUpdate().One()
		if err != nil {
			return err
		}
		if owner.IsEmpty() {
			return gerror.New("验证归并资料归属失效")
		}
		release, err := s.lockProfileMediaSyncTx(ctx, tx, profileID)
		if err != nil {
			return err
		}
		defer release()
		old, err := tx.Model(publishMediaTable).Ctx(ctx).Fields("id").Where("profile_id", profileID).Where("purpose", collectMaterialRoleVerify).WhereNull("deleted_at").Limit(1).One()
		if err != nil {
			return err
		}
		if !old.IsEmpty() {
			return nil
		}
		for _, media := range media {
			if media.Purpose != collectMaterialRoleVerify || media.MediaType != "video" {
				continue
			}
			_, err = tx.Model(publishMediaTable).Ctx(ctx).Data(g.Map{
				"tenant_id": event["tenant_id"].Int64(), "merchant_id": event["tenant_id"].Int64(), "account_id": event["account_id"].Int64(), "profile_id": profileID,
				"media_type": media.MediaType, "purpose": collectMaterialRoleVerify, "name": fmt.Sprintf("collect-verify-%d-%d", event["id"].Int64(), media.SortIndex),
				"tg_file_id": media.FileId, "file_url": media.FileURL, "storage_path": media.StoragePath, "size": media.Size, "md5": media.MD5,
				"tg_cache_asset_hash": mediaAssetHash(media.StoragePath, media.FileURL), "tg_cache_status": tgCacheStatusValid,
				"poster_url": media.PosterURL, "poster_storage_path": media.PosterStoragePath, "perceptual_hash": media.PerceptualHash, "sort_index": media.SortIndex, "status": 1,
				"created_by": event["account_id"].Int64(), "updated_by": event["account_id"].Int64(), "created_at": gtime.Now(), "updated_at": gtime.Now(),
			}).Insert()
			if err != nil {
				return gerror.Wrap(err, "写入采集验证归并媒体失败")
			}
		}
		return s.syncOwnedMediaToProfile(ctx, tx, owner, profileID)
	})
	if err != nil {
		return err
	}
	return nil
}
