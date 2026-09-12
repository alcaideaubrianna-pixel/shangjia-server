package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const (
	profileFingerprintCachePrefix  = "youban_publish:profile_fingerprint:v1"
	profileFingerprintReadyKey     = "youban_publish:profile_fingerprint:v1:backfill_ready"
	profileFingerprintCacheTTL     = 24 * time.Hour
	profileFingerprintMissCacheTTL = 10 * time.Minute
	profileFingerprintReadyTTL     = 30 * time.Minute
	profileFingerprintCacheRetries = 3
	profileFingerprintCacheMiss    = int64(-1)
)

type profileFingerprint struct {
	ChannelID      int64  `json:"channelId"`
	Layer          string `json:"layer"`
	Signature      string `json:"signature"`
	ItemTotal      int    `json:"itemTotal"`
	SignatureCount int    `json:"signatureCount"`
}

type profileFingerprintDuplicateError struct {
	ProfileID int64
	ChannelID int64
	Layer     string
	CacheHit  bool
}

func (e *profileFingerprintDuplicateError) Error() string {
	return fmt.Sprintf("资料已存在 profileId:%d channelId:%d layer:%s", e.ProfileID, e.ChannelID, e.Layer)
}

func buildProfileFingerprints(channelIDs []int64, finalText string, media []collectMediaItem) []profileFingerprint {
	normalizedText := normalizeCollectText(normalizeCollectKeywordText(finalText))
	material := collectDedupeMaterialFromItems(collectHash(normalizedText), media)
	signatures := material.signatures(true)
	channelIDs = uniqueIds(channelIDs)
	sort.Slice(channelIDs, func(i, j int) bool { return channelIDs[i] < channelIDs[j] })
	result := make([]profileFingerprint, 0, len(channelIDs)*len(signatures))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		for _, signature := range signatures {
			result = append(result, profileFingerprint{
				ChannelID: channelID, Layer: signature.layer, Signature: signature.value,
				ItemTotal: signature.total, SignatureCount: signature.count,
			})
		}
	}
	return result
}

func profileFingerprintCacheKey(tenantID, accountID int64, item profileFingerprint) string {
	return fmt.Sprintf("%s:%d:%d:%d:%s:%s:%d:%d", profileFingerprintCachePrefix,
		tenantID, accountID, item.ChannelID, item.Layer, item.Signature, item.ItemTotal, item.SignatureCount)
}

func profileFingerprintProfileKey(profileID int64, item profileFingerprint) string {
	return fmt.Sprintf("%d:%d:%s:%s:%d:%d", profileID, item.ChannelID, item.Layer, item.Signature, item.ItemTotal, item.SignatureCount)
}

func profileFingerprintBackfillReady(ctx context.Context) (bool, error) {
	value, err := cache.Instance().Get(ctx, profileFingerprintReadyKey)
	if err != nil {
		return false, gerror.Wrap(err, "读取资料指纹回填状态失败")
	}
	return !value.IsNil() && value.Int() == 1, nil
}

func markProfileFingerprintBackfillReady(ctx context.Context) error {
	return cache.Instance().Set(ctx, profileFingerprintReadyKey, 1, profileFingerprintReadyTTL)
}

func (s *sSysPublish) findProfileFingerprintDuplicate(ctx context.Context, tenantID, accountID int64, items []profileFingerprint, excludeProfileID int64) (*profileFingerprintDuplicateError, error) {
	allCached := len(items) > 0
	for _, item := range items {
		value, err := cache.Instance().Get(ctx, profileFingerprintCacheKey(tenantID, accountID, item))
		if err != nil {
			g.Log().Warningf(ctx, "读取资料指纹缓存失败 tenantId:%d accountId:%d channelId:%d layer:%s err:%+v", tenantID, accountID, item.ChannelID, item.Layer, err)
			allCached = false
			break
		}
		if value.IsNil() {
			allCached = false
			continue
		}
		profileID := value.Int64()
		if profileID == profileFingerprintCacheMiss || profileID == excludeProfileID {
			continue
		}
		if profileID > 0 && profileID != excludeProfileID {
			exists, validateErr := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).
				Where("tenant_id", tenantID).Where("account_id", accountID).Where("profile_id", profileID).
				Where("channel_id", item.ChannelID).Where("layer", item.Layer).Where("signature", item.Signature).
				Where("item_total", item.ItemTotal).Where("signature_count", item.SignatureCount).
				Where("owner_marker", "owner").Count()
			if validateErr != nil {
				return nil, gerror.Wrap(validateErr, "校验资料指纹缓存失败")
			}
			if exists > 0 {
				return &profileFingerprintDuplicateError{ProfileID: profileID, ChannelID: item.ChannelID, Layer: item.Layer, CacheHit: true}, nil
			}
			allCached = false
			clearProfileFingerprintCache(ctx, tenantID, accountID, []profileFingerprint{item})
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	if allCached {
		return nil, nil
	}
	mod := g.DB().Model(publishProfileFingerprintTable+" f").Safe().Ctx(ctx).
		Fields("f.profile_id,f.channel_id,f.layer,f.signature,f.item_total,f.signature_count").
		Where("f.tenant_id", tenantID).Where("f.account_id", accountID).Where("f.owner_marker", "owner")
	conditions := make([]string, 0, len(items))
	args := make([]interface{}, 0, len(items)*5)
	for _, item := range items {
		conditions = append(conditions, "(f.channel_id=? AND f.layer=? AND f.signature=? AND f.item_total=? AND f.signature_count=?)")
		args = append(args, item.ChannelID, item.Layer, item.Signature, item.ItemTotal, item.SignatureCount)
	}
	mod = mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
	if excludeProfileID > 0 {
		mod = mod.WhereNot("f.profile_id", excludeProfileID)
	}
	row, err := mod.OrderDesc("f.profile_id").Limit(1).One()
	if err != nil {
		return nil, gerror.Wrap(err, "查询资料库指纹失败")
	}
	if row.IsEmpty() {
		failures := 0
		for _, item := range items {
			if err = cache.Instance().Set(ctx, profileFingerprintCacheKey(tenantID, accountID, item), profileFingerprintCacheMiss, profileFingerprintMissCacheTTL); err != nil {
				failures++
			}
		}
		if failures > 0 {
			g.Log().Warningf(ctx, "写入资料指纹未命中缓存部分失败 tenantId:%d accountId:%d failures:%d total:%d", tenantID, accountID, failures, len(items))
		}
		return nil, nil
	}
	hit := &profileFingerprintDuplicateError{ProfileID: row["profile_id"].Int64(), ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String()}
	hitItem := profileFingerprint{ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(), Signature: row["signature"].String(), ItemTotal: row["item_total"].Int(), SignatureCount: row["signature_count"].Int()}
	_ = cache.Instance().Set(ctx, profileFingerprintCacheKey(tenantID, accountID, hitItem), hit.ProfileID, profileFingerprintCacheTTL)
	return hit, nil
}

func attachProfileFingerprintsTx(ctx context.Context, tx gdb.TX, tenantID, accountID, profileID int64, items []profileFingerprint) error {
	if _, err := detachProfileFingerprintsTx(ctx, tx, []int64{profileID}); err != nil {
		return err
	}
	now := gtime.Now()
	for _, item := range items {
		_, err := tx.Model(publishProfileFingerprintTable).Ctx(ctx).Data(g.Map{
			"tenant_id": tenantID, "account_id": accountID, "profile_id": profileID, "channel_id": item.ChannelID,
			"layer": item.Layer, "signature": item.Signature, "item_total": item.ItemTotal,
			"signature_count": item.SignatureCount, "owner_marker": "owner", "created_at": now, "updated_at": now,
		}).Insert()
		if err != nil {
			if isDuplicateKeyError(err) {
				row, lookupErr := tx.Model(publishProfileFingerprintTable).Ctx(ctx).Fields("profile_id").
					Where("tenant_id", tenantID).Where("account_id", accountID).Where("channel_id", item.ChannelID).
					Where("layer", item.Layer).Where("signature", item.Signature).
					Where("item_total", item.ItemTotal).Where("signature_count", item.SignatureCount).One()
				if lookupErr != nil {
					return gerror.Wrap(lookupErr, "读取并发资料指纹占用失败")
				}
				return &profileFingerprintDuplicateError{ProfileID: row["profile_id"].Int64(), ChannelID: item.ChannelID, Layer: item.Layer}
			}
			return gerror.Wrap(err, "保存资料指纹失败")
		}
	}
	return nil
}

func warmProfileFingerprintCache(ctx context.Context, tenantID, accountID, profileID int64, items []profileFingerprint) {
	failures := 0
	for _, item := range items {
		if err := cache.Instance().Set(ctx, profileFingerprintCacheKey(tenantID, accountID, item), profileID, profileFingerprintCacheTTL); err != nil {
			failures++
		}
	}
	if failures > 0 {
		g.Log().Warningf(ctx, "写入资料指纹缓存部分失败 tenantId:%d accountId:%d profileId:%d failures:%d total:%d", tenantID, accountID, profileID, failures, len(items))
	}
}

func clearProfileFingerprintCache(ctx context.Context, tenantID, accountID int64, items []profileFingerprint) int {
	failures := 0
	for _, item := range items {
		key := profileFingerprintCacheKey(tenantID, accountID, item)
		var err error
		for attempt := 0; attempt < profileFingerprintCacheRetries; attempt++ {
			_, err = cache.Instance().Remove(ctx, key)
			if err == nil {
				break
			}
		}
		if err != nil {
			failures++
		}
	}
	if failures > 0 {
		g.Log().Warningf(ctx, "清理资料指纹缓存部分失败 tenantId:%d accountId:%d failures:%d total:%d", tenantID, accountID, failures, len(items))
	}
	return failures
}

func insertProfileFingerprintRowsBatched(ctx context.Context, rows []g.Map) error {
	const batchSize = 500
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		if _, err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).Data(rows[start:end]).InsertIgnore(); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) replaceProfileFingerprintProjectionTx(ctx context.Context, tx gdb.TX, tenantID, accountID, profileID int64, channelIDs []int64) ([]profileFingerprint, []profileFingerprint, error) {
	oldRows, err := tx.Model(publishProfileFingerprintTable).Ctx(ctx).Where("profile_id", profileID).WhereNot("layer", "_indexed").All()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取资料旧指纹失败")
	}
	oldItems := fingerprintRowsToItems(oldRows)
	profileColumns := dao.ContentProfile.Columns()
	profile, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Fields(profileColumns.PlainText).
		Where(profileColumns.Id, profileID).WhereNull(profileColumns.DeletedAt).One()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取资料指纹正文失败")
	}
	mediaRows, err := tx.Model(publishMediaTable).Ctx(ctx).Fields("media_type,md5,perceptual_hash").
		Where("profile_id", profileID).WhereNull("deleted_at").OrderAsc("sort_index").OrderAsc("id").All()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取资料指纹媒体失败")
	}
	media := make([]collectMediaItem, 0, len(mediaRows))
	for _, row := range mediaRows {
		media = append(media, collectMediaItem{Type: row["media_type"].String(), FileMd5: row["md5"].String(), FilePhash: row["perceptual_hash"].String()})
	}
	items := buildProfileFingerprints(channelIDs, profile["plain_text"].String(), media)
	if _, err = detachProfileFingerprintsTx(ctx, tx, []int64{profileID}); err != nil {
		return nil, nil, err
	}
	for _, item := range items {
		data := g.Map{
			"tenant_id": tenantID, "account_id": accountID, "profile_id": profileID, "channel_id": item.ChannelID,
			"layer": item.Layer, "signature": item.Signature, "item_total": item.ItemTotal,
			"signature_count": item.SignatureCount, "owner_marker": "owner", "created_at": gtime.Now(), "updated_at": gtime.Now(),
		}
		result, insertErr := tx.Model(publishProfileFingerprintTable).Ctx(ctx).Data(data).InsertIgnore()
		err = insertErr
		if err != nil {
			return nil, nil, gerror.Wrap(err, "更新资料指纹失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			data["owner_marker"] = nil
			if _, err = tx.Model(publishProfileFingerprintTable).Ctx(ctx).Data(data).InsertIgnore(); err != nil {
				return nil, nil, gerror.Wrap(err, "登记重复资料指纹关联失败")
			}
		}
	}
	ownedRows, err := tx.Model(publishProfileFingerprintTable).Ctx(ctx).
		Where("profile_id", profileID).Where("owner_marker", "owner").All()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取资料新指纹失败")
	}
	return oldItems, fingerprintRowsToItems(ownedRows), nil
}

func (s *sSysPublish) refreshProfileFingerprintProjection(ctx context.Context, profileID int64) error {
	if profileID <= 0 {
		return nil
	}
	state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id").Where("profile_id", profileID).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取资料指纹归属失败")
	}
	if state.IsEmpty() {
		return s.detachProfileFingerprints(ctx, []int64{profileID})
	}
	tenantID, accountID := state["tenant_id"].Int64(), state["account_id"].Int64()
	channelIDs, err := s.profileChannelIdsOrDefaults(ctx, tenantID, accountID, profileID)
	if err != nil {
		return err
	}
	var oldItems, newItems []profileFingerprint
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var replaceErr error
		oldItems, newItems, replaceErr = s.replaceProfileFingerprintProjectionTx(ctx, tx, tenantID, accountID, profileID, channelIDs)
		return replaceErr
	})
	if err != nil {
		return err
	}
	clearProfileFingerprintCache(ctx, tenantID, accountID, oldItems)
	warmProfileFingerprintCache(ctx, tenantID, accountID, profileID, newItems)
	g.Log().Infof(ctx, "资料指纹已刷新 profileId:%d channels:%d fingerprints:%d", profileID, len(channelIDs), len(newItems))
	return nil
}

func fingerprintRowsToItems(rows gdb.Result) []profileFingerprint {
	items := make([]profileFingerprint, 0, len(rows))
	for _, row := range rows {
		if row["layer"].String() == "_indexed" {
			continue
		}
		items = append(items, profileFingerprint{ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(), Signature: row["signature"].String(), ItemTotal: row["item_total"].Int(), SignatureCount: row["signature_count"].Int()})
	}
	return items
}

func (s *sSysPublish) detachProfileFingerprints(ctx context.Context, profileIDs []int64) error {
	profileIDs = uniqueIds(profileIDs)
	if len(profileIDs) == 0 {
		return nil
	}
	rows, err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).WhereIn("profile_id", profileIDs).All()
	if err != nil {
		return gerror.Wrap(err, "读取待删除资料指纹失败")
	}
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, txErr := detachProfileFingerprintsTx(ctx, tx, profileIDs)
		return txErr
	}); err != nil {
		return err
	}
	grouped := make(map[[2]int64][]profileFingerprint)
	for _, row := range rows {
		scope := [2]int64{row["tenant_id"].Int64(), row["account_id"].Int64()}
		grouped[scope] = append(grouped[scope], profileFingerprint{ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(), Signature: row["signature"].String(), ItemTotal: row["item_total"].Int(), SignatureCount: row["signature_count"].Int()})
	}
	for scope, items := range grouped {
		clearProfileFingerprintCache(ctx, scope[0], scope[1], items)
	}
	g.Log().Infof(ctx, "资料指纹已清理 profiles:%d fingerprints:%d", len(profileIDs), len(rows))
	return nil
}

func detachProfileFingerprintsTx(ctx context.Context, tx gdb.TX, profileIDs []int64) (gdb.Result, error) {
	profileIDs = uniqueIds(profileIDs)
	if len(profileIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.Model(publishProfileFingerprintTable).Ctx(ctx).WhereIn("profile_id", profileIDs).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待删除资料指纹失败")
	}
	if _, err = tx.Model(publishProfileFingerprintTable).Ctx(ctx).WhereIn("profile_id", profileIDs).Delete(); err != nil {
		return nil, gerror.Wrap(err, "删除资料指纹失败")
	}
	for _, row := range rows {
		if row["owner_marker"].String() != "owner" || row["layer"].String() == "_indexed" {
			continue
		}
		candidate, findErr := tx.Model(publishProfileFingerprintTable).Ctx(ctx).Fields("id").
			Where("tenant_id", row["tenant_id"]).Where("account_id", row["account_id"]).Where("channel_id", row["channel_id"]).
			Where("layer", row["layer"]).Where("signature", row["signature"]).Where("item_total", row["item_total"]).
			Where("signature_count", row["signature_count"]).WhereNull("owner_marker").OrderDesc("profile_id").Limit(1).One()
		if findErr != nil {
			return nil, gerror.Wrap(findErr, "读取资料指纹接替记录失败")
		}
		if !candidate.IsEmpty() {
			if _, findErr = tx.Model(publishProfileFingerprintTable).Ctx(ctx).Where("id", candidate["id"].Int64()).Data(g.Map{"owner_marker": "owner", "updated_at": gtime.Now()}).Update(); findErr != nil {
				return nil, gerror.Wrap(findErr, "提升资料指纹接替记录失败")
			}
		}
	}
	return rows, nil
}

// backfillProfileFingerprints incrementally projects existing
// profiles. Conflicting historical duplicates keep the newest profile that
// already owns the unique fingerprint; the later cleanup job can remove the
// redundant profiles without changing the hot-path index.
func (s *sSysPublish) backfillProfileFingerprints(ctx context.Context, limit int) (int, error) {
	startedAt := time.Now()
	if limit <= 0 {
		limit = 200
	}
	rows, err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("p.id,p.plain_text,ps.tenant_id,ps.account_id").WhereNull("p.deleted_at").
		Where("NOT EXISTS (SELECT 1 FROM " + publishProfileFingerprintTable + " f WHERE f.profile_id=p.id AND f.layer='_indexed')").
		OrderDesc("p.id").Limit(limit).All()
	if err != nil || len(rows) == 0 {
		return 0, gerror.Wrap(err, "读取待回填资料指纹失败")
	}
	profileIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		profileIDs = append(profileIDs, row["id"].Int64())
	}
	mediaRows, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("profile_id,media_type,md5,perceptual_hash").WhereIn("profile_id", profileIDs).
		WhereNull("deleted_at").OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待回填资料媒体指纹失败")
	}
	mediaByProfile := make(map[int64][]collectMediaItem, len(rows))
	for _, row := range mediaRows {
		profileID := row["profile_id"].Int64()
		mediaByProfile[profileID] = append(mediaByProfile[profileID], collectMediaItem{
			Type: row["media_type"].String(), FileMd5: row["md5"].String(), FilePhash: row["perceptual_hash"].String(),
		})
	}
	channelsByProfile := make(map[int64][]int64, len(rows))
	channelRows, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("profile_id,channel_id").WhereIn("profile_id", profileIDs).All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待回填已发布频道失败")
	}
	configuredRows, err := g.DB().Model(publishProfileChannelTable).Safe().Ctx(ctx).
		Fields("profile_id,channel_id").WhereIn("profile_id", profileIDs).WhereNull("deleted_at").All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待回填配置频道失败")
	}
	dispatchRows, err := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		InnerJoin(collectDispatchChannelTable+" dc", "dc.dispatch_id=d.id").
		Fields("d.profile_id,dc.channel_id").WhereIn("d.profile_id", profileIDs).All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取待回填采集频道失败")
	}
	for _, result := range []gdb.Result{channelRows, configuredRows, dispatchRows} {
		for _, row := range result {
			profileID := row["profile_id"].Int64()
			channelsByProfile[profileID] = append(channelsByProfile[profileID], row["channel_id"].Int64())
		}
	}
	defaultChannelsByTenant := make(map[int64][]int64)
	now := gtime.Now()
	ownerData := make([]g.Map, 0, len(rows)*3)
	associationData := make([]g.Map, 0, len(rows)*3)
	associationKeys := make([]string, 0, len(rows)*3)
	markerData := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		profileID := row["id"].Int64()
		channelIDs := channelsByProfile[profileID]
		if len(channelIDs) == 0 {
			tenantID := row["tenant_id"].Int64()
			var ok bool
			channelIDs, ok = defaultChannelsByTenant[tenantID]
			if !ok {
				channelIDs, err = s.defaultSelectedPublishChannelIds(ctx, tenantID)
				if err != nil {
					return 0, err
				}
				defaultChannelsByTenant[tenantID] = channelIDs
			}
		}
		items := buildProfileFingerprints(channelIDs, row["plain_text"].String(), mediaByProfile[profileID])
		for _, item := range items {
			data := g.Map{
				"tenant_id": row["tenant_id"].Int64(), "account_id": row["account_id"].Int64(), "profile_id": profileID,
				"channel_id": item.ChannelID, "layer": item.Layer, "signature": item.Signature,
				"item_total": item.ItemTotal, "signature_count": item.SignatureCount,
				"owner_marker": "owner", "created_at": now, "updated_at": now,
			}
			ownerData = append(ownerData, data)
			association := g.Map{}
			for key, value := range data {
				association[key] = value
			}
			association["owner_marker"] = nil
			associationData = append(associationData, association)
			associationKeys = append(associationKeys, profileFingerprintProfileKey(profileID, item))
		}
		markerData = append(markerData, g.Map{
			"tenant_id": row["tenant_id"].Int64(), "account_id": row["account_id"].Int64(), "profile_id": profileID,
			"channel_id": 0, "layer": "_indexed", "signature": collectHash(fmt.Sprintf("profile:%d", profileID)),
			"item_total": 0, "signature_count": 0, "created_at": now, "updated_at": now,
		})
	}
	if len(ownerData) > 0 {
		if err = insertProfileFingerprintRowsBatched(ctx, ownerData); err != nil {
			return 0, gerror.Wrap(err, "批量回填资料指纹 owner 失败")
		}
	}
	owned, err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).
		WhereIn("profile_id", profileIDs).WhereNot("layer", "_indexed").Where("owner_marker", "owner").All()
	if err != nil {
		return 0, gerror.Wrap(err, "读取批量回填资料 owner 失败")
	}
	ownedKeys := make(map[string]struct{}, len(owned))
	for _, item := range owned {
		profileID := item["profile_id"].Int64()
		fingerprint := profileFingerprint{ChannelID: item["channel_id"].Int64(), Layer: item["layer"].String(), Signature: item["signature"].String(), ItemTotal: item["item_total"].Int(), SignatureCount: item["signature_count"].Int()}
		ownedKeys[profileFingerprintProfileKey(profileID, fingerprint)] = struct{}{}
	}
	duplicateAssociations := make([]g.Map, 0, len(associationData))
	for index, data := range associationData {
		if _, isOwner := ownedKeys[associationKeys[index]]; !isOwner {
			duplicateAssociations = append(duplicateAssociations, data)
		}
	}
	if err = insertProfileFingerprintRowsBatched(ctx, duplicateAssociations); err != nil {
		return 0, gerror.Wrap(err, "批量回填重复资料指纹关联失败")
	}
	if err = insertProfileFingerprintRowsBatched(ctx, markerData); err != nil {
		return 0, gerror.Wrap(err, "批量标记资料指纹回填完成失败")
	}
	processed := len(rows)
	if processed > 0 {
		g.Log().Infof(ctx, "资料指纹增量回填完成 scanned:%d projected:%d owners:%d duplicates:%d maxProfileId:%d minProfileId:%d elapsed:%s",
			len(rows), processed, len(ownerData)-len(duplicateAssociations), len(duplicateAssociations), profileIDs[0], profileIDs[len(profileIDs)-1], time.Since(startedAt).Round(time.Millisecond))
	}
	return processed, nil
}

func (s *sSysPublish) runProfileFingerprintBackfill(ctx context.Context) {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	totalProcessed := 0
	for {
		mutex := lock.NewConfig(2*time.Minute, 100*time.Millisecond).Mutex("youban_publish:profile-fingerprint:backfill")
		if err := mutex.Lock(ctx); err != nil {
			g.Log().Warningf(ctx, "获取资料指纹回填锁失败 err:%+v", err)
		} else {
			count, err := s.backfillProfileFingerprints(ctx, 1000)
			_ = mutex.Unlock(context.Background())
			if err != nil {
				g.Log().Warningf(ctx, "资料指纹回填失败 err:%+v", err)
			} else if count == 0 {
				if err = markProfileFingerprintBackfillReady(ctx); err != nil {
					g.Log().Warningf(ctx, "标记资料指纹回填完成失败 err:%+v", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(10 * time.Second):
					}
					continue
				}
				g.Log().Infof(ctx, "资料指纹回填已就绪 totalProcessed:%d", totalProcessed)
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Minute):
				}
				continue
			} else {
				totalProcessed += count
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}
