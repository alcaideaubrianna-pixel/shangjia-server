package sys

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	idao "hotgo/internal/dao"
)

// Explicit rollout gate: no schema probing or DDL in the request/write path.
func profileOriginEnabled(ctx context.Context) (bool, error) {
	v, err := g.Cfg().Get(ctx, "youbanPublish.profileOriginEnabled", false)
	if err != nil {
		return false, gerror.Wrap(err, "读取资料来源配置失败")
	}
	return v.Bool(), nil
}

func profileOriginCompleteness(o *sysin.ProfileOriginModel) string {
	known := o.CollectMethod == sysin.CollectSourceTypeAccount || o.CollectMethod == sysin.CollectSourceTypeBot || o.CollectMethod == sysin.CollectSourceTypeFollow
	collectorKnown := (o.CollectMethod == sysin.CollectSourceTypeAccount && o.TgAccountId > 0) || (o.CollectMethod == sysin.CollectSourceTypeBot && o.BotId > 0) || (o.CollectMethod == sysin.CollectSourceTypeFollow && o.FollowAccountId > 0)
	if o.ChatId != "" && known && o.MessageId > 0 && collectorKnown {
		return "complete"
	}
	if o.ChatId != "" || o.MessageId > 0 || known {
		return "partial"
	}
	return "unknown"
}

func collectProfileOrigin(event gdb.Record, profileId int64) *sysin.ProfileOriginModel {
	method := strings.TrimSpace(event["source_type"].String())
	switch method {
	case sysin.CollectSourceTypeAccount, sysin.CollectSourceTypeBot, sysin.CollectSourceTypeFollow:
	default:
		method = "unknown"
	}
	o := &sysin.ProfileOriginModel{
		TenantId: event["tenant_id"].Int64(), AccountId: event["account_id"].Int64(), ProfileId: profileId,
		OriginKind: "collect", CollectMethod: method, SourceId: event["source_id"].Int64(), EventId: event["id"].Int64(),
		BotId: event["bot_id"].Int64(), TgAccountId: event["tg_account_id"].Int64(), FollowAccountId: event["follow_account_id"].Int64(),
		ChatIdRaw: strings.TrimSpace(event["source_chat_id"].String()), MessageId: event["source_message_id"].Int64(), GroupedId: strings.TrimSpace(event["source_grouped_id"].String()),
		ChannelTitle: event["source_name_snapshot"].String(), ChannelUsername: strings.TrimPrefix(strings.TrimSpace(event["source_username_snapshot"].String()), "@"), EvidenceKind: "event",
	}
	if !event["event_id"].IsNil() {
		o.EventId = event["event_id"].Int64()
	}
	o.ChatId = normalizeTelegramChannelChatID(o.ChatIdRaw)
	// Bot API already supplies signed channel IDs; positive IDs may be private chats.
	if method == sysin.CollectSourceTypeBot && !strings.HasPrefix(o.ChatIdRaw, "-") {
		o.ChatId = o.ChatIdRaw
	}
	if !event["received_at"].IsNil() {
		o.ReceivedAt = event["received_at"].GTime()
	}
	identity := event["source_unique_key"].String()
	if identity == "" {
		identity = fmt.Sprintf("%s:%d:%d:%d:%s:%d:%s", method, o.SourceId, o.BotId, o.TgAccountId, o.ChatId, o.MessageId, o.GroupedId)
	}
	// Does not use rule/dispatch IDs: workflow retries must not create new origins.
	o.OriginKey = fmt.Sprintf("event:%x", sha256.Sum256([]byte(identity)))
	if o.EventId > 0 {
		o.OriginKey = fmt.Sprintf("event:%d", o.EventId)
	}
	if method == sysin.CollectSourceTypeFollow && o.FollowAccountId == 0 && strings.HasPrefix(o.ChatIdRaw, "account:") {
		o.FollowAccountId, _ = strconv.ParseInt(strings.TrimPrefix(o.ChatIdRaw, "account:"), 10, 64)
	}
	o.Completeness = profileOriginCompleteness(o)
	return o
}

func insertProfileOriginTx(ctx context.Context, tx gdb.TX, o *sysin.ProfileOriginModel) (int64, error) {
	if o.TenantId <= 0 || o.AccountId <= 0 || o.ProfileId <= 0 || o.OriginKey == "" {
		return 0, gerror.New("资料来源归属不完整")
	}
	c := pdao.YoubanPublishProfileOrigin.Columns()
	result, err := tx.Model(pdao.YoubanPublishProfileOrigin.Table()).Ctx(ctx).
		OnConflict(c.TenantId, c.AccountId, c.ProfileId, c.OriginKey).
		Data(pdo.YoubanPublishProfileOrigin{
			TenantId: o.TenantId, AccountId: o.AccountId, ProfileId: o.ProfileId, OriginKey: o.OriginKey, OriginKind: o.OriginKind, CollectMethod: o.CollectMethod,
			SourceId: o.SourceId, EventId: o.EventId, BotId: o.BotId, TgAccountId: o.TgAccountId, FollowAccountId: o.FollowAccountId,
			ChatIdRaw: o.ChatIdRaw, ChatId: o.ChatId, MessageId: o.MessageId, GroupedId: o.GroupedId, ChannelTitle: o.ChannelTitle, ChannelUsername: o.ChannelUsername,
			ReceivedAt: o.ReceivedAt, EvidenceKind: o.EvidenceKind, Completeness: o.Completeness,
		}).InsertIgnore()
	if err != nil {
		return 0, gerror.Wrap(err, "保存资料永久来源失败")
	}
	return result.RowsAffected()
}

func saveCollectProfileOriginTx(ctx context.Context, tx gdb.TX, profileId int64, event gdb.Record) error {
	enabled, err := profileOriginEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	o := collectProfileOrigin(event, profileId)
	if o.Completeness != "complete" || o.EventId <= 0 || o.SourceId <= 0 {
		return gerror.Newf("采集来源缺少频道、消息或采集方式，无法入库 eventId:%d", o.EventId)
	}
	parents, lockErr := lockProfileOriginParentsTx(ctx, tx, []int64{profileId}, true)
	if lockErr != nil {
		return lockErr
	}
	if !parents[profileId] {
		return gerror.New("资料已删除，无法保存采集来源")
	}
	// Snapshot title enrichment stays inside the existing DB transaction, no Telegram I/O.
	if o.ChannelTitle == "" || o.ChannelUsername == "" || o.CollectMethod == sysin.CollectSourceTypeFollow {
		c := pdao.YoubanPublishCollectSource.Columns()
		source, queryErr := tx.Model(pdao.YoubanPublishCollectSource.Table()).Ctx(ctx).Unscoped().
			Fields(c.Title, c.SourceUsername, c.FollowAccountId).Where(c.Id, o.SourceId).Where(c.TenantId, o.TenantId).Where(c.AccountId, o.AccountId).One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "读取来源快照失败")
		}
		if o.ChannelTitle == "" {
			o.ChannelTitle = source[c.Title].String()
		}
		if o.ChannelUsername == "" {
			o.ChannelUsername = source[c.SourceUsername].String()
		}
		if o.FollowAccountId == 0 {
			o.FollowAccountId = source[c.FollowAccountId].Int64()
		}
	}
	_, err = insertProfileOriginTx(ctx, tx, o)
	return err
}

// loadLegacyProfileOrigins reads observations, never guesses a collector from a
// profile key or from today's source configuration. Soft-deleted sources may
// enrich a name, but method/collector identity comes from the event/snapshot.
func loadLegacyProfileOrigins(ctx context.Context, ids []int64) (map[int64][]*sysin.ProfileOriginModel, error) {
	result := make(map[int64][]*sysin.ProfileOriginModel)
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model(publishCollectDispatchTable+" d").Ctx(ctx).Unscoped().
		LeftJoin(publishCollectEventTable+" e", "e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id").
		LeftJoin(publishCollectSourceTable+" s", "s.id=d.source_id AND s.tenant_id=d.tenant_id AND s.account_id=d.account_id").
		WhereIn("d.profile_id", ids).
		Fields("d.id,d.tenant_id,d.account_id,d.profile_id,d.event_id,d.source_id,COALESCE(NULLIF(d.source_type_snapshot,''),e.source_type,'unknown') AS source_type,COALESCE(NULLIF(d.source_chat_id_snapshot,''),e.source_chat_id,'') AS source_chat_id,COALESCE(NULLIF(d.source_message_id_snapshot,0),e.source_message_id,0) AS source_message_id,COALESCE(NULLIF(d.source_name_snapshot,''),s.title,'') AS source_name_snapshot,COALESCE(NULLIF(d.source_username_snapshot,''),s.source_username,'') AS source_username_snapshot,e.bot_id,e.tg_account_id,e.source_grouped_id,e.source_unique_key,e.received_at,d.created_at").
		OrderAsc("d.id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史资料来源失败")
	}
	seen := map[string]bool{}
	for _, row := range rows {
		o := collectProfileOrigin(row, row["profile_id"].Int64())
		o.EventId = row["event_id"].Int64()
		o.EvidenceKind = "legacy_dispatch"

		if !row["created_at"].IsNil() {
			o.CreatedAt = row["created_at"].GTime()
		}
		key := fmt.Sprintf("%d:%d:%d:%s", o.TenantId, o.AccountId, o.ProfileId, o.OriginKey)
		if seen[key] {
			continue
		}
		seen[key] = true
		result[o.ProfileId] = append(result[o.ProfileId], o)
	}
	if err := loadProfileOriginFallbacks(ctx, ids, result); err != nil {
		return nil, err
	}
	return result, nil
}

func loadProfileOrigins(ctx context.Context, ids []int64) (map[int64][]*sysin.ProfileOriginModel, error) {
	result, err := loadLegacyProfileOrigins(ctx, ids)
	if err != nil {
		return nil, err
	}
	enabled, err := profileOriginEnabled(ctx)
	if err != nil || !enabled || len(ids) == 0 {
		return result, err
	}
	c := pdao.YoubanPublishProfileOrigin.Columns()
	var rows []*sysin.ProfileOriginModel
	if err = pdao.YoubanPublishProfileOrigin.Ctx(ctx).WhereIn(c.ProfileId, ids).OrderAsc(c.Id).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取资料永久来源失败")
	}
	durable := make(map[int64][]*sysin.ProfileOriginModel)
	for _, o := range rows {
		durable[o.ProfileId] = append(durable[o.ProfileId], o)
	}
	for id, origins := range durable {
		// Keep legacy observations from old writers during rolling deployments.
		seen := map[string]bool{}
		for _, o := range origins {
			seen[o.OriginKey] = true
		}
		for _, o := range result[id] {
			if o.EvidenceKind == "legacy_key" || (o.EvidenceKind == "legacy_job" && o.EventId <= 0) {
				continue
			}
			if !seen[o.OriginKey] {
				origins = append(origins, o)
				seen[o.OriginKey] = true
			}
		}
		sort.SliceStable(origins, func(i, j int) bool {
			a, b := origins[i].ReceivedAt, origins[j].ReceivedAt
			if a == nil {
				a = origins[i].CreatedAt
			}
			if b == nil {
				b = origins[j].CreatedAt
			}
			if a != nil && b != nil && !a.Equal(b) {
				return a.Before(b)
			}
			if (a == nil) != (b == nil) {
				return a != nil
			}
			if origins[i].Id != origins[j].Id {
				return origins[i].Id < origins[j].Id
			}
			return origins[i].OriginKey < origins[j].OriginKey
		})
		result[id] = origins
	}
	return result, nil
}

func applyProfileOriginFields(item *sysin.ProfileModel, origins []*sysin.ProfileOriginModel) {
	item.Origins = nil
	for _, o := range origins {
		if o.TenantId == item.TenantId && o.AccountId == item.AccountId {
			item.Origins = append(item.Origins, o)
		}
	}
	if len(item.Origins) == 0 {
		if item.IsCollected {
			item.CollectMethod = "unknown"
			item.OriginCompleteness = "unknown"
		}
		return
	}
	first := item.Origins[0]
	hasCoordinates := item.CollectSourceChatId != "" || item.CollectSourceMessageId > 0 || item.CollectSourceId > 0
	if hasCoordinates {
		first = nil
		for _, candidate := range item.Origins {
			if item.CollectSourceId > 0 && candidate.SourceId != item.CollectSourceId {
				continue
			}
			if item.CollectSourceMessageId > 0 && candidate.MessageId != item.CollectSourceMessageId {
				continue
			}
			if item.CollectSourceChatId != "" && !profileOriginChatMatches(item.CollectSourceChatId, candidate) {
				continue
			}
			first = candidate
			break
		}
	}
	item.IsCollected = true
	if first == nil {
		item.CollectMethod = "unknown"
		item.OriginCompleteness = "unknown"
		return
	}
	item.CollectMethod = first.CollectMethod
	item.OriginCompleteness = first.Completeness
	// Fill only a matching observation. Never combine different source messages.

	if item.CollectSourceChatId == "" {
		item.CollectSourceChatId = first.ChatId
	}
	if item.CollectSourceName == "" {
		item.CollectSourceName = first.ChannelTitle
	}
	if item.CollectSourceUsername == "" {
		item.CollectSourceUsername = first.ChannelUsername
	}
	if item.CollectSourceMessageId == 0 {
		item.CollectSourceMessageId = first.MessageId
	}
	if item.CollectSourceId == 0 {
		item.CollectSourceId = first.SourceId
	}
	if item.CollectSourceUrl == "" {
		item.CollectSourceUrl = collectedTelegramMessageURL(item.CollectSourceChatId, item.CollectSourceUsername, item.CollectSourceMessageId)
	}
}

// Operational rows may disappear; jobs and canonical keys can still prove
// coordinates. They cannot prove the collector once the event is gone.
func loadProfileOriginFallbacks(ctx context.Context, ids []int64, result map[int64][]*sysin.ProfileOriginModel) error {
	missing := make([]int64, 0, len(ids))
	for _, id := range ids {
		needsFallback := len(result[id]) == 0
		for _, o := range result[id] {
			if o.ChatId == "" || o.MessageId <= 0 {
				needsFallback = true
			}
		}
		if needsFallback {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	jobs, err := g.DB().Model(publishTgJobTable+" j").Ctx(ctx).
		LeftJoin(publishCollectEventTable+" e", "e.id=j.collect_event_id AND e.tenant_id=j.tenant_id AND e.account_id=j.account_id").
		WhereIn("j.profile_id", missing).WhereGT("j.collect_source_message_id", 0).
		Fields("j.id,j.tenant_id,j.account_id,j.profile_id,j.collect_source_id AS source_id,j.collect_event_id AS event_id,j.collect_source_chat_id AS source_chat_id,j.collect_source_message_id AS source_message_id,e.source_type,e.bot_id,e.tg_account_id,e.source_unique_key,e.source_grouped_id,e.received_at,j.created_at").OrderAsc("j.id").All()
	if err != nil {
		return gerror.Wrap(err, "读取历史任务来源坐标失败")
	}
	for _, row := range jobs {
		id := row["profile_id"].Int64()
		if len(result[id]) > 0 {
			for _, existing := range result[id] {
				if existing.EventId <= 0 || existing.EventId != row["event_id"].Int64() {
					continue
				}
				candidate := collectProfileOrigin(row, id)
				if existing.ChatId != "" && existing.ChatId != candidate.ChatId {
					continue
				}
				if existing.MessageId > 0 && existing.MessageId != candidate.MessageId {
					continue
				}
				if existing.ChatId == "" {
					existing.ChatId = candidate.ChatId
					existing.ChatIdRaw = candidate.ChatIdRaw
				}
				if existing.MessageId <= 0 {
					existing.MessageId = candidate.MessageId
				}
				existing.Completeness = profileOriginCompleteness(existing)
			}
			continue
		}
		o := collectProfileOrigin(row, id)
		o.EventId = row["event_id"].Int64()
		o.EvidenceKind = "legacy_job"
		if o.EventId <= 0 {
			o.OriginKey = fmt.Sprintf("legacy:job:%d", row["id"].Int64())
		}
		if !row["created_at"].IsNil() {
			o.CreatedAt = row["created_at"].GTime()
		}
		result[id] = []*sysin.ProfileOriginModel{o}
	}
	profiles, err := g.DB().Model(idao.ContentProfile.Table()+" p").Ctx(ctx).
		LeftJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		WhereIn("p.id", missing).Where("p.source_type", collectProfileSourceType).
		Fields("p.id,p.source_key,ps.tenant_id,ps.account_id").All()
	if err != nil {
		return gerror.Wrap(err, "读取历史资料来源身份失败")
	}
	for _, row := range profiles {
		id := row["id"].Int64()
		if len(result[id]) > 0 {
			continue
		}
		o := profileOriginFromKey(row["source_key"].String(), row["tenant_id"].Int64(), row["account_id"].Int64(), id)
		if o != nil {
			result[id] = []*sysin.ProfileOriginModel{o}
		}
	}
	return nil
}

func profileOriginFromKey(key string, tenantId, accountId, profileId int64) *sysin.ProfileOriginModel {
	parts := strings.Split(key, ":")
	if len(parts) != 7 || parts[0] != "collect" || parts[1] != "v2" {
		return nil
	}
	tenant, _ := strconv.ParseInt(parts[2], 10, 64)
	account, _ := strconv.ParseInt(parts[3], 10, 64)
	if tenantId <= 0 || accountId <= 0 || tenant != tenantId || account != accountId {
		return nil
	}
	o := &sysin.ProfileOriginModel{TenantId: tenantId, AccountId: accountId, ProfileId: profileId, OriginKey: fmt.Sprintf("legacy:key:%x", sha256.Sum256([]byte(key))), OriginKind: "collect", CollectMethod: "unknown", ChatIdRaw: parts[4], ChatId: normalizeTelegramChannelChatID(parts[4]), EvidenceKind: "legacy_key"}
	switch parts[5] {
	case "message":
		o.MessageId, _ = strconv.ParseInt(parts[6], 10, 64)
	case "group":
		o.GroupedId = parts[6]
	default:
		return nil
	}
	o.Completeness = profileOriginCompleteness(o)
	return o
}

// BackfillProfileOrigins is bounded, explicitly scoped and defaults to dry-run
// at the CLI boundary. It never changes a profile or queues publication work.
func (s *sSysPublish) BackfillProfileOrigins(ctx context.Context, tenantId, accountId, lastId int64, limit int, apply bool) (*sysin.ProfileOriginBackfillResult, error) {
	if tenantId <= 0 || accountId <= 0 || lastId < 0 || limit <= 0 || limit > 500 {
		return nil, gerror.New("来源回填必须指定租户、账号及1至500条批次")
	}
	rows, err := g.DB().Model(idao.ContentProfile.Table()+" p").Ctx(ctx).
		Where("p.source_type", collectProfileSourceType).WhereGT("p.id", lastId).WhereNull("p.deleted_at").
		Where("EXISTS (SELECT 1 FROM "+publishProfileStateTable+" ps WHERE ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", tenantId, accountId).
		Fields("p.id").OrderAsc("p.id").Limit(limit).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取来源回填批次失败")
	}
	res := &sysin.ProfileOriginBackfillResult{Scanned: len(rows), NextId: lastId, Applied: apply}
	if len(rows) == 0 {
		return res, nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].Int64())
	}
	res.NextId = ids[len(ids)-1]
	origins, err := loadLegacyProfileOrigins(ctx, ids)
	if err != nil {
		return nil, err
	}
	candidates := make([]*sysin.ProfileOriginModel, 0)
	for _, id := range ids {
		found := false
		for _, o := range origins[id] {
			if o.TenantId != tenantId || o.AccountId != accountId {
				continue
			}
			found = true
			res.Origins++
			switch o.Completeness {
			case "complete":
				res.Complete++
			case "partial":
				res.Partial++
			default:
				res.Unknown++
			}
			candidates = append(candidates, o)
		}
		if !found {
			res.Unknown++
		}
	}
	if !apply {
		return res, nil
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		parents, lockErr := lockProfileOriginParentsTx(ctx, tx, ids, true)
		if lockErr != nil {
			return lockErr
		}
		var inserted int64
		for _, o := range candidates {
			if !parents[o.ProfileId] {
				res.Skipped++
				continue
			}
			n, insertErr := insertProfileOriginTx(ctx, tx, o)
			if insertErr != nil {
				return insertErr
			}
			inserted += n
		}
		res.Inserted = inserted
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Enabled only after migration; retain the legacy branch for rolling rollback.
func applyProfileOriginSourceFilter(mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	c := pdao.YoubanPublishProfileOrigin.Columns()
	durable := "EXISTS (SELECT 1 FROM " + pdao.YoubanPublishProfileOrigin.Table() + " o WHERE o." + c.ProfileId + "=p.id AND o." + c.TenantId + "=ps.tenant_id AND o." + c.AccountId + "=ps.account_id AND o." + c.SourceId + "=?"
	legacy := "EXISTS (SELECT 1 FROM " + publishCollectDispatchTable + " d LEFT JOIN " + publishCollectEventTable + " e ON e.id=d.event_id AND e.tenant_id=d.tenant_id AND e.account_id=d.account_id WHERE d.profile_id=p.id AND d.tenant_id=ps.tenant_id AND d.account_id=ps.account_id AND d.source_id=?"
	durableArgs := []interface{}{in.CollectSourceId}
	legacyArgs := []interface{}{in.CollectSourceId}
	if chat := strings.TrimSpace(in.CollectSourceChatId); chat != "" {
		normalized := normalizeTelegramChannelChatID(chat)
		raw := strings.TrimPrefix(normalized, "-100")
		durable += " AND o." + c.ChatId + " IN (?,?)"
		durableArgs = append(durableArgs, normalized, raw)
		legacy += " AND COALESCE(NULLIF(d.source_chat_id_snapshot,''),e.source_chat_id) IN (?,?)"
		legacyArgs = append(legacyArgs, normalized, raw)
	}
	return mod.Where("("+durable+") OR "+legacy+"))", append(durableArgs, legacyArgs...)...)
}

func deleteProfileOriginsTx(ctx context.Context, tx gdb.TX, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	enabled, err := profileOriginEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	if _, err = lockProfileOriginParentsTx(ctx, tx, ids, false); err != nil {
		return err
	}
	c := pdao.YoubanPublishProfileOrigin.Columns()
	_, err = tx.Model(pdao.YoubanPublishProfileOrigin.Table()).Ctx(ctx).WhereIn(c.ProfileId, ids).Delete()
	return gerror.Wrap(err, "清理已删除资料永久来源失败")
}

func (s *sSysPublish) resolveAIOpsOriginMessage(ctx context.Context, ref aiOpsTelegramMessageRef, result *sysin.AIOpsTelegramMessageContextModel) error {
	enabled, err := profileOriginEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	if ref.MessageId <= 0 || (ref.ChatId == "" && ref.Username == "") {
		return nil
	}
	c := pdao.YoubanPublishProfileOrigin.Columns()
	mod := pdao.YoubanPublishProfileOrigin.Ctx(ctx).As("o").Where(c.MessageId, ref.MessageId).
		Where("EXISTS (SELECT 1 FROM " + idao.ContentProfile.Table() + " p WHERE p.id=o.profile_id AND p.deleted_at IS NULL)").
		Where("EXISTS (SELECT 1 FROM " + publishProfileStateTable + " ps WHERE ps.profile_id=o.profile_id AND ps.tenant_id=o.tenant_id AND ps.account_id=o.account_id AND ps.deleted_at IS NULL)")
	if ref.Username != "" {
		mod = mod.Where("LOWER("+c.ChannelUsername+")=?", strings.ToLower(ref.Username))
	} else {
		normalized := normalizeTelegramChannelChatID(ref.ChatId)
		mod = mod.WhereIn(c.ChatId, []string{ref.ChatId, normalized})
	}
	rows, err := mod.Fields(c.ProfileId, c.TenantId, c.AccountId).Distinct().Limit(2).All()
	if err != nil {
		return gerror.Wrap(err, "查询永久来源消息失败")
	}
	if len(rows) > 1 {
		return gerror.New("来源消息对应多份资料，请使用目标资料的发布消息定位")
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	result.Found = true
	result.MatchType = "profile_origin"
	result.ProfileId = row[c.ProfileId].Int64()
	result.TenantId = row[c.TenantId].Int64()
	result.AccountId = row[c.AccountId].Int64()
	return nil
}

// Writers, backfills and explicit deletion use parent -> origin lock order.
// This prevents backfills from resurrecting origins after a concurrent purge.
func lockProfileOriginParentsTx(ctx context.Context, tx gdb.TX, ids []int64, activeOnly bool) (map[int64]bool, error) {
	c := idao.ContentProfile.Columns()
	mod := tx.Model(idao.ContentProfile.Table()).Ctx(ctx).Unscoped().Fields(c.Id).WhereIn(c.Id, ids).OrderAsc(c.Id).LockUpdate()
	if activeOnly {
		mod = mod.WhereNull(c.DeletedAt)
	}
	rows, err := mod.All()
	if err != nil {
		return nil, gerror.Wrap(err, "锁定资料来源归属失败")
	}
	result := map[int64]bool{}
	for _, row := range rows {
		result[row[c.Id].Int64()] = true
	}
	return result, nil
}

func profileOriginChatMatches(chat string, o *sysin.ProfileOriginModel) bool {
	if o.CollectMethod == sysin.CollectSourceTypeBot && !strings.HasPrefix(o.ChatId, "-") {
		return strings.TrimSpace(chat) == o.ChatId
	}
	return normalizeTelegramChannelChatID(chat) == o.ChatId
}
