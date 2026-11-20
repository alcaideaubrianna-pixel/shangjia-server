package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var collectGeneratedTitleNumberPattern = regexp.MustCompile(`(?im)^\s*编号\s*[:：=]\s*C\d+\s*$\n?`)

const collectEventStorageAccountField = "_collect_storage_account_id"

// collectEventStorageAccountID is the persisted event owner, not the resolved
// business owner. Zero is meaningful for legacy Bot ingress and must survive.
func collectEventStorageAccountID(event gdb.Record) int64 {
	if value, ok := event[collectEventStorageAccountField]; ok && value != nil && !value.IsNil() {
		return value.Int64()
	}
	return event["account_id"].Int64()
}

// collectEventAccountScope is for exact source/event legacy lookups only.
// Profile ownership must always use event.account_id, never this relaxed scope.
func collectEventAccountScope(event gdb.Record) []int64 {
	owner, storage := event["account_id"].Int64(), collectEventStorageAccountID(event)
	if owner == storage || storage < 0 {
		return []int64{owner}
	}
	return []int64{owner, storage}
}

func collectEventForBranch(event, source gdb.Record) (gdb.Record, error) {
	owner := source["account_id"].Int64()
	storage := collectEventStorageAccountID(event)
	if event.IsEmpty() || owner <= 0 || storage < 0 || source["id"].Int64() != event["source_id"].Int64() ||
		source["tenant_id"].Int64() != event["tenant_id"].Int64() ||
		(storage > 0 && storage != owner) || (storage == 0 && event["source_type"].String() != sysin.CollectSourceTypeBot) {
		return nil, gerror.New("采集事件与来源业务账户归属不一致")
	}
	branch := make(gdb.Record, len(event)+1)
	for key, value := range event {
		branch[key] = value
	}
	branch[collectEventStorageAccountField] = g.NewVar(storage)
	branch["account_id"] = g.NewVar(owner)
	return branch, nil
}

// Exact accepted-branch lookup: legacy account=0 and resolved owner are
// aliases only inside the same tenant/source/event/rule, never globally.
func findCollectBranchDispatch(ctx context.Context, model *gdb.Model, event, rule gdb.Record) (gdb.Record, error) {
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	rows, err := model.Ctx(ctx).Where("tenant_id", event["tenant_id"].Int64()).
		WhereIn("account_id", collectEventAccountScope(event)).Where("source_id", event["source_id"].Int64()).
		Where("event_id", event["id"].Int64()).Where("rule_id", rule["id"].Int64()).
		OrderDesc("id").Limit(3).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集分发幂等记录失败")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	if len(rows) > 2 {
		return nil, gerror.New("采集分支存在多个历史分发记录，拒绝自动归并")
	}
	var selected gdb.Record
	for _, row := range rows {
		job, queryErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Fields("id").Where("tenant_id", event["tenant_id"].Int64()).
			Where("operation_no", fmt.Sprintf("collect:%d", row["id"].Int64())).Limit(1).One()
		if queryErr != nil {
			return nil, gerror.Wrap(queryErr, "确认历史采集分支发布操作失败")
		}
		if !job.IsEmpty() {
			if !selected.IsEmpty() {
				return nil, gerror.New("历史采集分支存在多个发布操作，拒绝重复交付")
			}
			selected = row
		}
	}
	if selected.IsEmpty() {
		return nil, gerror.New("历史采集账户别名存在多个未交付分支，拒绝猜测归属")
	}
	return selected, nil
}

func validateCollectBranchProfileOwner(ctx context.Context, dispatch, event gdb.Record) error {
	tenantID, owner := event["tenant_id"].Int64(), event["account_id"].Int64()
	profileID := dispatch["profile_id"].Int64()
	accountScope := collectEventAccountScope(event)
	dispatchAccountAllowed := false
	for _, accountID := range accountScope {
		dispatchAccountAllowed = dispatchAccountAllowed || dispatch["account_id"].Int64() == accountID
	}
	if tenantID <= 0 || owner <= 0 || !dispatchAccountAllowed || dispatch["tenant_id"].Int64() != tenantID || dispatch["source_id"].Int64() != event["source_id"].Int64() {
		return gerror.New("历史采集分支归属不一致，拒绝迁移")
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	if profileID > 0 {
		states, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Fields("tenant_id,account_id").Where("profile_id", profileID).WhereNull("deleted_at").Limit(2).All()
		if err != nil {
			return gerror.Wrap(err, "核对历史采集资料账户归属失败")
		}
		if len(states) != 1 || states[0]["tenant_id"].Int64() != tenantID || states[0]["account_id"].Int64() != owner {
			return gerror.New("历史采集资料真实账户归属冲突或已删除，拒绝迁移")
		}
	}
	// Only a proven business profile may accept legacy account-zero evidence.
	// Preserve the original operation, including jobs written under dispatch's
	// storage account; do not mistake them for a missing business-account job.
	conflict, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id").Where("tenant_id", tenantID).
		Where("operation_no", fmt.Sprintf("collect:%d", dispatch["id"].Int64())).
		Where("(account_id NOT IN (?) OR profile_id<>?)", accountScope, profileID).Limit(1).One()
	if err != nil {
		return gerror.Wrap(err, "核对历史采集发布操作归属失败")
	}
	if !conflict.IsEmpty() {
		return gerror.New("历史采集发布操作账户或资料归属冲突，拒绝迁移或重发")
	}
	return nil
}

func collectPublishClientRequestId(event gdb.Record, rule gdb.Record) string {
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	sourceID := event["source_id"].Int64()
	ruleID := rule["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 {
		return ""
	}
	// A Bot/account identifies the transport, not the collection branch.
	// Every rule/source owns its own profile, even on the same target channel.
	prefix := fmt.Sprintf("collect:v4:%d:%d:source:%d:rule:%d:", tenantID, accountID, sourceID, ruleID)
	chatID := normalizeTelegramChannelChatID(event["source_chat_id"].String())
	if chatID != "" {
		if groupedID := strings.TrimSpace(event["source_grouped_id"].String()); groupedID != "" {
			return prefix + fmt.Sprintf("%s:group:%s", chatID, groupedID)
		}
		if messageID := event["source_message_id"].Int64(); messageID > 0 {
			return prefix + fmt.Sprintf("%s:message:%d", chatID, messageID)
		}
	}
	if uniqueKey := strings.TrimSpace(event["source_unique_key"].String()); uniqueKey != "" {
		return prefix + "key:" + collectHash(uniqueKey)
	}
	return ""
}

func (s *sSysPublish) markCollectEvent(ctx context.Context, id int64, status string, message string) error {
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	data := g.Map{
		eventCols.Status:       status,
		eventCols.ErrorMessage: message,
		eventCols.UpdatedAt:    gtime.Now(),
	}
	if status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusIgnored || status == sysin.CollectEventStatusFailed {
		data[eventCols.ProcessedAt] = gtime.Now()
	}
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where(eventCols.Id, id).Data(data).Update()
	if err != nil {
		return err
	}
	if collectEventTerminalStatus(status) || status == sysin.CollectEventStatusDispatched {
		s.syncCollectSourceStatsForEvent(ctx, id)
	}
	return nil
}

// syncCollectSourceStats rebuilds counters from authoritative rows so queue
// retries and duplicate deliveries cannot inflate them.
func (s *sSysPublish) syncCollectSourceStats(ctx context.Context, sourceID int64) error {
	if sourceID <= 0 {
		return nil
	}
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	dispatchDao := pdao.YoubanPublishCollectDispatch
	dispatchCols := dispatchDao.Columns()
	sourceCols := pdao.YoubanPublishCollectSource.Columns()

	stats, err := g.DB().GetOne(ctx, fmt.Sprintf(`SELECT
		COUNT(*) AS event_total,
		COALESCE(SUM(CASE WHEN %s=? AND COALESCE(material_role,'')<>? THEN 1 ELSE 0 END),0) AS failed_total,
		COALESCE(SUM(CASE WHEN %s=? THEN 1 ELSE 0 END),0) AS blocked_total,
		MAX(%s) AS last_event_at
	FROM %s
	WHERE %s=?`, eventCols.Status, eventCols.Status, eventCols.ReceivedAt, eventDao.Table(), eventCols.SourceId),
		sysin.CollectEventStatusFailed, collectMaterialRoleVerify, sysin.CollectEventStatusIgnored, sourceID)
	if err != nil {
		return gerror.Wrap(err, "统计采集源事件失败")
	}
	success, err := g.DB().GetValue(ctx, fmt.Sprintf(`SELECT COUNT(DISTINCT %s)
	FROM %s
	WHERE %s=? AND %s=?`, dispatchCols.EventId, dispatchDao.Table(), dispatchCols.SourceId, dispatchCols.Status),
		sourceID, sysin.CollectDispatchStatusSent)
	if err != nil {
		return gerror.Wrap(err, "统计采集源成功数失败")
	}
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(sourceCols.Id, sourceID).
		Data(g.Map{
			sourceCols.EventTotal:   stats["event_total"].Int64(),
			sourceCols.SuccessTotal: success.Int64(),
			sourceCols.FailedTotal:  stats["failed_total"].Int64(),
			"blocked_total":         stats["blocked_total"].Int64(),
			sourceCols.LastEventAt:  stats["last_event_at"],
			sourceCols.UpdatedAt:    gtime.Now(),
		}).Update()
	return gerror.Wrap(err, "更新采集源统计失败")
}

// syncCollectSourceStatsBatch aggregates all requested sources in two scans,
// instead of running one COUNT/SUM/MAX query per source.
func (s *sSysPublish) syncCollectSourceStatsBatch(ctx context.Context, sourceIDs []int64) error {
	if len(sourceIDs) == 0 {
		return nil
	}
	type stat struct {
		total, failed, success int64
		last                   interface{}
	}
	stats := make(map[int64]*stat, len(sourceIDs))
	eventDao := pdao.YoubanPublishCollectEvent
	eventCols := eventDao.Columns()
	rows, err := g.DB().GetAll(ctx, fmt.Sprintf(`SELECT %s AS source_id,COUNT(*) AS event_total,
 COALESCE(SUM(CASE WHEN %s=? AND COALESCE(material_role,'')<>? THEN 1 ELSE 0 END),0) AS failed_total,
 MAX(%s) AS last_event_at FROM %s WHERE %s IN (?) GROUP BY %s`, eventCols.SourceId, eventCols.Status, eventCols.ReceivedAt, eventDao.Table(), eventCols.SourceId, eventCols.SourceId), sysin.CollectEventStatusFailed, collectMaterialRoleVerify, sourceIDs)
	if err != nil {
		return gerror.Wrap(err, "批量统计采集源事件失败")
	}
	for _, row := range rows {
		stats[row["source_id"].Int64()] = &stat{total: row["event_total"].Int64(), failed: row["failed_total"].Int64(), last: row["last_event_at"]}
	}
	dispatchDao := pdao.YoubanPublishCollectDispatch
	dispatchCols := dispatchDao.Columns()
	rows, err = g.DB().GetAll(ctx, fmt.Sprintf(`SELECT %s AS source_id,COUNT(DISTINCT %s) AS success_total FROM %s WHERE %s IN (?) AND %s=? GROUP BY %s`, dispatchCols.SourceId, dispatchCols.EventId, dispatchDao.Table(), dispatchCols.SourceId, dispatchCols.Status, dispatchCols.SourceId), sourceIDs, sysin.CollectDispatchStatusSent)
	if err != nil {
		return gerror.Wrap(err, "批量统计采集源成功数失败")
	}
	for _, row := range rows {
		if v := stats[row["source_id"].Int64()]; v != nil {
			v.success = row["success_total"].Int64()
		}
	}
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	for _, id := range sourceIDs {
		v := stats[id]
		if v == nil {
			v = &stat{}
		}
		_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).Where(sourceCols.Id, id).Data(g.Map{sourceCols.EventTotal: v.total, sourceCols.SuccessTotal: v.success, sourceCols.FailedTotal: v.failed, sourceCols.LastEventAt: v.last, sourceCols.UpdatedAt: gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "批量更新采集源统计失败")
		}
	}
	return nil
}

func (s *sSysPublish) syncCollectSourceStatsForEvent(ctx context.Context, eventID int64) {
	if eventID <= 0 {
		return
	}
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	sourceID, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where(eventCols.Id, eventID).
		Fields(eventCols.SourceId).
		Value()
	if err != nil {
		g.Log().Warningf(ctx, "读取采集事件统计归属失败 eventId:%d err:%+v", eventID, err)
		return
	}
	if err = s.scheduleCollectSourceStats(ctx, sourceID.Int64()); err != nil {
		g.Log().Warningf(ctx, "同步采集源统计失败 eventId:%d sourceId:%d err:%+v", eventID, sourceID.Int64(), err)
	}
}

func collectTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = collectGeneratedTitleNumberPattern.ReplaceAllString(text, "")
	if title, _, _ := materialImportTitle(text); title != "" {
		return collectProfileTitle(title)
	}
	runes := []rune(text)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return text
}

func collectHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
