package sys

import (
	"context"
	"strings"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	collectRequeueLegacyChannelReason = "账户频道采集未启用或审批已拒绝"
	collectRequeueMaxPairedVerifies   = 32
	collectRequeueMaxMediaRows        = 512
	// Material columns are absent from the legacy generated DAO columns.
	collectRequeueRoleColumn   = "material_role"
	collectRequeueParentColumn = "material_parent_event_id"
)

// Generated timestamp fields are *gtime.Time rather than any. Embed the DO
// and override only the explicitly cleared timestamp; never edit generated
// types or rely on a nil DO field, which OmitNilData would skip. Keep the
// generated Go field name without an alternate orm tag: GF must flatten one
// key, not both a nil CamelCase timestamp and a snake_case NULL override.
type collectRequeueEventUpdate struct {
	pdo.YoubanPublishCollectEvent
	ProcessedAt any
}

type collectRequeueMediaUpdate struct {
	pdo.YoubanPublishCollectEventMedia
	NextRetryAt any
}

// No fuzzy error matching: manual bans, removed bots, keyword filters, unmatched
// verification and unrelated terminal outcomes are never reopened by replay.
func collectRequeueLegacyAuthorizationIgnored(event gdb.Record) bool {
	return event["status"].String() == sysin.CollectEventStatusIgnored &&
		event["error_message"].String() == collectRequeueLegacyChannelReason
}

// Only the exact inherited form is accepted, and only while this same parent
// is itself being recovered from the legacy authorization incident.
func collectRequeueInheritedAuthorizationReason(display gdb.Record, reason string) bool {
	return collectRequeueLegacyAuthorizationIgnored(display) &&
		(reason == collectRequeueLegacyChannelReason || reason == "父展示资料已忽略："+collectRequeueLegacyChannelReason)
}

func collectRequeueRestoreIgnored(display, event gdb.Record) bool {
	if event["id"].Int64() == display["id"].Int64() {
		return collectRequeueLegacyAuthorizationIgnored(display)
	}
	return event["status"].String() == sysin.CollectEventStatusIgnored &&
		event["material_role"].String() == collectMaterialRoleVerify &&
		event["material_parent_event_id"].Int64() == display["id"].Int64() &&
		collectRequeueInheritedAuthorizationReason(display, event["error_message"].String())
}

func validateCollectRequeueStatus(event gdb.Record) error {
	switch event["status"].String() {
	case sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched,
		sysin.CollectEventStatusFailed, "matched":
		return nil
	case sysin.CollectEventStatusIgnored:
		if collectRequeueLegacyAuthorizationIgnored(event) {
			return nil
		}
	}
	return gerror.New("事件当前状态或忽略原因不允许重放")
}

func validateCollectRequeueDisplay(payload collectProcessQueuePayload, display, source gdb.Record) error {
	if display.IsEmpty() || payload.EventId <= 0 || display["id"].Int64() != payload.EventId ||
		payload.TenantId <= 0 || display["tenant_id"].Int64() != payload.TenantId ||
		payload.SourceId <= 0 || display["source_id"].Int64() != payload.SourceId ||
		payload.AccountId < 0 || source.IsEmpty() {
		return gerror.New("重放事件与租户、来源范围不一致")
	}
	branch, err := collectEventForBranch(display, source)
	if err != nil {
		return err
	}
	if payload.AccountId != collectEventStorageAccountID(display) && payload.AccountId != branch["account_id"].Int64() {
		return gerror.New("重放事件与账户范围不一致")
	}
	chat := canonicalCollectSourceChatID(display["source_chat_id"].String())
	if chat == "" || (payload.SourceChatId != "" && chat != canonicalCollectSourceChatID(payload.SourceChatId)) {
		return gerror.New("重放事件实际频道与请求不一致")
	}
	if display["material_role"].String() != collectMaterialRoleDisplay ||
		display["material_parent_event_id"].Int64() != 0 || isCollectRepairSuspended(display["status"].String()) {
		return gerror.New("重放事件不是独立展示资料或仍处于挂起状态")
	}
	if err = validateCollectRequeueStatus(display); err != nil {
		return err
	}
	if display["source_type"].String() != source["source_type"].String() {
		return gerror.New("展示资料与采集源类型不一致")
	}
	for _, field := range []string{"bot_id", "tg_account_id"} {
		if actual, configured := display[field].Int64(), source[field].Int64(); actual > 0 && actual != configured {
			return gerror.New("展示资料与采集源运行身份不一致")
		}
	}
	return nil
}

func validateCollectRequeuePair(display, verify gdb.Record) error {
	if verify.IsEmpty() || verify["id"].Int64() <= 0 ||
		verify["material_role"].String() != collectMaterialRoleVerify ||
		verify["material_parent_event_id"].Int64() != display["id"].Int64() ||
		isCollectRepairSuspended(verify["status"].String()) {
		return gerror.New("验证资料配对或当前状态不合法")
	}
	if verify["status"].String() != sysin.CollectEventStatusIgnored {
		if err := validateCollectRequeueStatus(verify); err != nil {
			return err
		}
	}
	for _, field := range []string{"tenant_id", "account_id", "source_id", "source_type", "bot_id", "tg_account_id"} {
		if display[field].String() != verify[field].String() {
			return gerror.Newf("展示与验证资料归属不一致：%s", field)
		}
	}
	chat := canonicalCollectSourceChatID(display["source_chat_id"].String())
	if chat == "" || chat != canonicalCollectSourceChatID(verify["source_chat_id"].String()) {
		return gerror.New("展示与验证资料实际频道不一致")
	}
	if verify["status"].String() == sysin.CollectEventStatusIgnored &&
		!collectRequeueRestoreIgnored(display, verify) {
		return gerror.New("验证资料不是同一次旧频道授权误拦截，禁止重开")
	}
	return nil
}

func validateCollectRequeueMedia(display, media gdb.Record) error {
	for _, field := range []string{"tenant_id", "account_id", "source_id"} {
		if media[field].Int64() != display[field].Int64() {
			return gerror.New("误拦截媒体归属与配对事件不一致")
		}
	}
	chat := canonicalCollectSourceChatID(display["source_chat_id"].String())
	if chat == "" || canonicalCollectSourceChatID(media["source_chat_id"].String()) != chat {
		return gerror.New("误拦截媒体频道与配对事件不一致")
	}
	if media["cache_status"].String() == collectMediaCacheCanceled {
		reason := media["error_message"].String()
		if reason != collectRequeueLegacyChannelReason &&
			(media["event_id"].Int64() == display["id"].Int64() ||
				!collectRequeueInheritedAuthorizationReason(display, reason)) {
			return gerror.New("媒体因其他原因取消，禁止通过频道授权修复重开")
		}
	}
	return nil
}

// prepareCollectEventRequeue performs only bounded authorization/identity reads
// before restoring an explicitly accepted pair. No source/relationship/account,
// dispatch, profile, media or queue identity is rewritten. Only this incident's
// ignored event state and canceled media state may be reopened.
func (s *sSysPublish) prepareCollectEventRequeue(ctx context.Context, payload collectProcessQueuePayload) (gdb.Record, gdb.Result, bool, error) {
	if payload.TenantId <= 0 || payload.SourceId <= 0 || payload.EventId <= 0 || payload.AccountId < 0 {
		return nil, nil, false, gerror.New("重放必须提供明确的事件、租户和来源")
	}
	if !s.collectGlobalEnabled(ctx) {
		return nil, nil, false, gerror.New("采集总开关已关闭，拒绝重放")
	}
	if err := s.ensureTenantVipFeature(ctx, payload.TenantId, sysin.TenantVipFeatureCollectSource); err != nil {
		return nil, nil, false, gerror.Wrap(err, "当前租户不能执行采集重放")
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	var display gdb.Record
	var verifies gdb.Result
	var restoredEvents gdb.Result
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		defer tx.Ctx(ctx)
		if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
			if _, err := tx.Exec("SET LOCAL lock_timeout = '1s'"); err != nil {
				return err
			}
			if _, err := tx.Exec("SET LOCAL statement_timeout = '3s'"); err != nil {
				return err
			}
		}
		cols := pdao.YoubanPublishCollectEvent.Columns()
		var err error
		display, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where(cols.Id, payload.EventId).Where(cols.TenantId, payload.TenantId).
			Where(cols.SourceId, payload.SourceId).LockUpdate().One()
		if err != nil {
			return gerror.Wrap(err, "锁定重放展示资料失败")
		}
		sc := pdao.YoubanPublishCollectSource.Columns()
		source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).Where(sc.Id, payload.SourceId).
			Where(sc.TenantId, payload.TenantId).WhereNull(sc.DeletedAt).One()
		if err != nil {
			return gerror.Wrap(err, "读取重放采集源失败")
		}
		if err = validateCollectRequeueDisplay(payload, display, source); err != nil {
			return err
		}
		if source[sc.CollectEnabled].Int() != 1 || source[sc.Status].Int() != 1 ||
			source[sc.ChannelBlacklisted].Int() != 0 {
			return gerror.New("采集源已禁用或被屏蔽，拒绝重放")
		}
		ownerID := source[sc.AccountId].Int64()
		access, err := resolveCollectEventChannelAccess(ctx, display, ownerID)
		if err != nil {
			return err
		}
		if !access.Allowed {
			return gerror.New("实际频道当前禁止采集，拒绝重放")
		}
		if botID := source[sc.BotId].Int64(); botID > 0 {
			botRelation, botErr := resolveCollectEventBotChannel(ctx, payload.TenantId, botID, access)
			if botErr != nil {
				return botErr
			}
			if botRelation.Status == sysin.CollectBotChannelRemoved {
				return gerror.New("机器人已移出实际频道，拒绝重放")
			}
		} else if source[sc.SourceType].String() == sysin.CollectSourceTypeBot {
			return gerror.New("BOT采集源缺少机器人归属")
		}
		if payload.RuleId > 0 {
			bc := pdao.YoubanPublishCollectSourceRule.Columns()
			binding, ruleErr := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).
				Where(bc.TenantId, payload.TenantId).Where(bc.SourceId, payload.SourceId).
				Where(bc.RuleId, payload.RuleId).Where("("+bc.Status+" IS NULL OR "+bc.Status+"<>?)", 2).
				Limit(1).One()
			if ruleErr != nil {
				return ruleErr
			}
			rc := pdao.YoubanPublishCollectRule.Columns()
			rule, ruleErr := pdao.YoubanPublishCollectRule.Ctx(ctx).
				Where(rc.Id, payload.RuleId).Where(rc.TenantId, payload.TenantId).
				Where(rc.AccountId, ownerID).WhereNull(rc.DeletedAt).
				Where("("+rc.Status+" IS NULL OR "+rc.Status+"<>?)", 2).One()
			if ruleErr != nil {
				return ruleErr
			}
			if binding.IsEmpty() || rule.IsEmpty() {
				return gerror.New("请求规则不属于该采集源的当前有效规则")
			}
		}
		// Match the material index's source/chat/role/parent prefix. LIMIT
		// alone cannot bound a parent-only lookup on the historical event table.
		// Cross-source corrupt links belong to separate targeted diagnostics.
		verifies, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where(cols.TenantId, payload.TenantId).Where(cols.SourceId, payload.SourceId).
			WhereIn(cols.SourceChatId, tgChannelCacheLookupIds(display[cols.SourceChatId].String())).
			Where(collectRequeueParentColumn, payload.EventId).Where(collectRequeueRoleColumn, collectMaterialRoleVerify).
			OrderAsc(cols.SourceMessageId).OrderAsc(cols.Id).
			Limit(collectRequeueMaxPairedVerifies + 1).LockUpdate().All()
		if err != nil {
			return gerror.Wrap(err, "锁定重放配对验证资料失败")
		}
		if len(verifies) > collectRequeueMaxPairedVerifies {
			return gerror.New("配对验证组超过重放安全上限")
		}
		for _, verify := range verifies {
			if err = validateCollectRequeuePair(display, verify); err != nil {
				return err
			}
		}
		// Cancellation was propagated to media by ignoreCollectEvent. Restore
		// only canceled rows from this exact authorized parent/child incident.
		restoreIDs := make([]int64, 0, len(verifies)+1)
		for _, event := range append(gdb.Result{display}, verifies...) {
			if collectRequeueRestoreIgnored(display, event) {
				restoredEvents = append(restoredEvents, event)
				restoreIDs = append(restoreIDs, event[cols.Id].Int64())
			}
		}
		if len(restoreIDs) > 0 {
			mc := pdao.YoubanPublishCollectEventMedia.Columns()
			media, mediaErr := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
				Fields(mc.Id, mc.EventId, mc.TenantId, mc.AccountId, mc.SourceId, mc.SourceChatId, mc.CacheStatus, mc.ErrorMessage).
				WhereIn(mc.EventId, restoreIDs).OrderAsc(mc.Id).
				Limit(collectRequeueMaxMediaRows + 1).LockUpdate().All()
			if mediaErr != nil {
				return gerror.Wrap(mediaErr, "读取误拦截媒体状态失败")
			}
			if len(media) > collectRequeueMaxMediaRows {
				return gerror.New("重放媒体行超过安全上限")
			}
			for _, row := range media {
				if err := validateCollectRequeueMedia(display, row); err != nil {
					return err
				}
				if row[mc.CacheStatus].String() != collectMediaCacheCanceled {
					continue
				}
				result, updateErr := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
					Where(mc.Id, row[mc.Id].Int64()).Where(mc.EventId, row[mc.EventId].Int64()).
					Where(mc.CacheStatus, collectMediaCacheCanceled).Where(mc.ErrorMessage, row[mc.ErrorMessage].String()).
					Data(collectRequeueMediaUpdate{
						YoubanPublishCollectEventMedia: pdo.YoubanPublishCollectEventMedia{
							CacheStatus: collectMediaCachePending, ErrorMessage: "", DownloadErrorType: "",
						},
						NextRetryAt: gdb.Raw("NULL"),
					}).OmitNilData().Update()
				if updateErr != nil {
					return gerror.Wrap(updateErr, "恢复误拦截媒体失败")
				}
				affected, affectedErr := result.RowsAffected()
				if affectedErr != nil {
					return gerror.Wrap(affectedErr, "确认误拦截媒体恢复结果失败")
				}
				if affected != 1 {
					return gerror.New("媒体状态已变化，本组恢复已回滚")
				}
			}
		}
		// Validate the complete snapshot before changing either parent or child.
		// Existing processed/dispatched records are intentionally untouched.
		for _, event := range restoredEvents {
			result, updateErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
				Where(cols.Id, event[cols.Id].Int64()).Where(cols.TenantId, payload.TenantId).
				Where(cols.AccountId, event[cols.AccountId].Int64()).Where(cols.SourceId, payload.SourceId).
				Where(cols.Status, sysin.CollectEventStatusIgnored).
				Where(cols.ErrorMessage, event[cols.ErrorMessage].String()).
				Where(collectRequeueRoleColumn, event[collectRequeueRoleColumn].String()).
				Where(collectRequeueParentColumn, event[collectRequeueParentColumn].Int64()).
				Data(collectRequeueEventUpdate{
					YoubanPublishCollectEvent: pdo.YoubanPublishCollectEvent{
						Status: sysin.CollectEventStatusGroupCollect, ErrorMessage: "",
					},
					ProcessedAt: gdb.Raw("NULL"),
				}).OmitNilData().Update()
			if updateErr != nil {
				return gerror.Wrap(updateErr, "恢复旧频道授权误拦截事件失败")
			}
			affected, affectedErr := result.RowsAffected()
			if affectedErr != nil {
				return gerror.Wrap(affectedErr, "确认重放恢复状态失败")
			}
			if affected != 1 {
				return gerror.New("重放事件状态已变化，本组恢复已回滚")
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	// Reflect only the state updates actually committed above. Do not mutate
	// snapshots until all ignored parent/child checks have succeeded.
	for _, event := range restoredEvents {
		event["status"] = g.NewVar(sysin.CollectEventStatusGroupCollect)
		event["error_message"] = g.NewVar("")
		event["processed_at"] = g.NewVar(nil)
	}
	return display, verifies, len(restoredEvents) > 0, nil
}
