package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type adminNoteCursor struct {
	IndexId  int64  `json:"indexId"`
	SortBy   string `json:"sortBy"`
	SortAt   string `json:"sortAt"`
	SortNull bool   `json:"sortNull,omitempty"`
}

const (
	adminNoteSortCreatedAt   = "createdAt"
	adminNoteSortPublishedAt = "publishedAt"
	adminNoteSortUpdatedAt   = "updatedAt"
)

func (s *sSysPublish) adminNoteIndexList(ctx context.Context, in *sysin.NoteListInp, tenantId int64, tenantIds []int64, accountIds []int64) ([]*sysin.ProfileModel, bool, string, error) {
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	// Validate ownership before limiting candidates; details are loaded afterwards.
	mod := pdao.YoubanPublishNoteIndex.Ctx(ctx).As("i").
		InnerJoin(publishProfileStateTable+" ps", "ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL").
		WhereNull("i.deleted_at")
	mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
	dbType := strings.TrimSpace(g.DB().GetConfig().Type)
	keywordTerms := noteIndexUnionKeywordTerms(in.Keyword)
	useKeywordUnion := g.Cfg().MustGet(ctx, "youbanPublish.noteKeywordUnionQueryEnabled", false).Bool() &&
		(strings.EqualFold(dbType, "pgsql") || strings.EqualFold(dbType, "postgres")) && len(keywordTerms) > 0
	filters := in.ProfileListInp
	if useKeywordUnion {
		filters.Keyword = ""
	}
	// Materialize the source candidates once instead of probing dispatch/event
	// for every visible profile. Keep authorization and pagination on the outer query.
	useSourceCandidates := (strings.EqualFold(dbType, "pgsql") || strings.EqualFold(dbType, "postgres")) &&
		in.CollectSourceId > 0 && strings.TrimSpace(in.CollectSourceChatId) != ""
	if useSourceCandidates {
		chatID := canonicalCollectSourceChatID(in.CollectSourceChatId)
		mod = mod.Where("i.profile_id IN (WITH source_profiles AS MATERIALIZED ("+
			"SELECT DISTINCT d.profile_id FROM "+publishCollectDispatchTable+" d "+
			"JOIN "+publishCollectEventTable+" e ON e.id=d.event_id "+
			"WHERE d.source_id=? AND e.source_chat_id IN (?,?)) SELECT profile_id FROM source_profiles)",
			in.CollectSourceId, chatID, "-100"+chatID)
		filters.CollectSourceId = 0
		filters.CollectSourceChatId = ""
	}
	mod = applyNoteIndexFilters(mod, &filters)
	var err error
	sortBy, err := normalizeAdminNoteSortBy(in.SortBy)
	if err != nil {
		return nil, false, "", err
	}
	in.SortBy = sortBy
	if mod, err = applyNoteIndexCursor(mod, in.Cursor, sortBy); err != nil {
		return nil, false, "", err
	}
	_, perPage, _ := form.CalPage(1, in.PerPage)
	in.Page = 1
	in.PerPage = perPage
	type noteIndexCandidate struct {
		NoteIndexId int64       `orm:"note_index_id"`
		ProfileId   int64       `orm:"id"`
		SortAt      *gtime.Time `orm:"sort_at"`
	}
	var candidates []noteIndexCandidate
	sortExpression := adminNoteSortExpression(sortBy)
	candidateStartedAt := time.Now()
	// Keep the ordered stream unbounded so every authorization and filter
	// predicate is applied before the outer LIMIT.
	singleTenant := (len(tenantIds) == 1 && tenantIds[0] > 0) || (len(tenantIds) == 0 && tenantId > 0)
	useOrderedSubquery := g.Cfg().MustGet(ctx, "youbanPublish.noteAccountBranchQueryEnabled", true).Bool() &&
		(strings.EqualFold(dbType, "pgsql") || strings.EqualFold(dbType, "postgres")) &&
		singleTenant && len(accountIds) >= 2 && strings.TrimSpace(in.Keyword) == "" &&
		in.Status == 0 && in.PushChannelId == 0 && in.CollectSourceId == 0 &&
		strings.TrimSpace(in.CollectSourceChatId) == "" && strings.TrimSpace(in.Province) == "" &&
		strings.TrimSpace(in.City) == "" && strings.TrimSpace(in.Tag) == "" &&
		strings.TrimSpace(in.ReviewStatus) == "" && strings.TrimSpace(in.Visibility) == "" &&
		sortBy == adminNoteSortCreatedAt+"Desc" && strings.TrimSpace(in.SourceScope) == "collected"
	if useOrderedSubquery {
		inner := pdao.YoubanPublishNoteIndex.Ctx(ctx).As("i").
			WhereNull("i.deleted_at").
			Where("i."+noteIndexCollectedColumn, true)
		inner = applyNoteIndexScope(inner, tenantId, tenantIds, nil, nil)
		inner, err = applyNoteIndexCursor(inner, in.Cursor, sortBy)
		if err != nil {
			return nil, false, "", err
		}
		inner = inner.Order("i.created_at DESC").Order("i.id DESC").Offset(0)
		mod = pdao.YoubanPublishNoteIndex.DB().Model("? AS i", inner).Ctx(ctx).
			WhereNull("i.deleted_at").
			Where("EXISTS (SELECT 1 FROM " + publishProfileStateTable + " ps WHERE ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL)")
		mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
		mod = applyNoteIndexFilters(mod, &in.ProfileListInp)
		mod, err = applyNoteIndexCursor(mod, in.Cursor, sortBy)
		if err != nil {
			return nil, false, "", err
		}
	}
	if useKeywordUnion {
		err = noteIndexKeywordUnionQuery(ctx, mod, keywordTerms, sortExpression, adminNoteSortDirection(sortBy), perPage+1).Scan(&candidates)
	} else {
		err = mod.Fields("i.id AS note_index_id,i.profile_id AS id," + sortExpression + " AS sort_at").
			Order(sortExpression + " " + adminNoteSortDirection(sortBy)).
			Order("i.id " + adminNoteSortDirection(sortBy)).
			Limit(perPage + 1).Scan(&candidates)
	}
	if err != nil {
		g.Log().Warning(ctx, "资料索引候选查询失败", g.Map{
			"traceId":  gctx.CtxId(ctx),
			"tenantId": tenantId,
			"sortBy":   sortBy,
			"perPage":  perPage,
			"keyword":  strings.TrimSpace(in.Keyword),
			"error":    err.Error(),
		})
		return nil, false, "", gerror.Wrap(err, "获取资料索引列表失败")
	}
	logSlowAdminNoteListStage(ctx, "profile_query_candidates", candidateStartedAt, len(candidates), perPage)
	hasMore := len(candidates) > perPage
	if hasMore {
		candidates = candidates[:perPage]
	}
	list := make([]*sysin.ProfileModel, 0, len(candidates))
	var lastSortAt *gtime.Time
	var lastIndexId int64
	if len(candidates) > 0 {
		ids := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.NoteIndexId)
		}
		var details []*sysin.ProfileModel
		detailMod := noteIndexModel(ctx).
			LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
		detailMod = applyNoteIndexScope(detailMod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
		detailStartedAt := time.Now()
		if err := detailMod.WhereIn("i.id", ids).Fields(adminNoteIndexFields()).Scan(&details); err != nil {
			return nil, false, "", gerror.Wrap(err, "获取资料索引详情失败")
		}
		logSlowAdminNoteListStage(ctx, "profile_query_details", detailStartedAt, len(details), len(ids))
		byIndex := make(map[int64]*sysin.ProfileModel, len(details))
		for _, detail := range details {
			if detail != nil {
				byIndex[detail.NoteIndexId] = detail
			}
		}
		for _, candidate := range candidates {
			if detail := byIndex[candidate.NoteIndexId]; detail != nil {
				list = append(list, detail)
			}
		}
	}
	nextCursor := ""
	if hasMore {
		// Details can disappear between reads. Advance past every candidate
		// already scanned, including those with no remaining detail row.
		if len(candidates) > 0 {
			last := candidates[len(candidates)-1]
			lastIndexId, lastSortAt = last.NoteIndexId, last.SortAt
		}
		if lastSortAt == nil && !g.Cfg().MustGet(ctx, "youbanPublish.noteNullCursorOutputEnabled", false).Bool() {
			return nil, false, "", gerror.New("当前排序包含空时间，分页暂不可用，请稍后重试")
		}
		nextCursor = encodeAdminNotePageCursor(lastIndexId, lastSortAt, sortBy)
	}
	return list, hasMore, nextCursor, nil
}

func (s *sSysPublish) adminNoteIndexProfileIds(ctx context.Context, in *sysin.NoteListInp, tenantId int64, tenantIds []int64, accountIds []int64) ([]int64, error) {
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	mod := noteIndexModel(ctx).LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
	mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
	mod = applyNoteIndexFilters(mod, &in.ProfileListInp)
	var rows []struct {
		Id int64 `orm:"id"`
	}
	if err := mod.Fields("i.profile_id AS id").Group("i.profile_id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取批量操作资料ID失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return ids, nil
}

func applyNoteIndexScope(mod *gdb.Model, tenantId int64, tenantIds []int64, accountIds []int64, in *sysin.ProfileListInp) *gdb.Model {
	if len(tenantIds) > 0 {
		mod = mod.WhereIn("i.tenant_id", tenantIds)
	} else if tenantId > 0 {
		mod = mod.Where("i.tenant_id", tenantId)
	}
	if len(accountIds) > 0 {
		return mod.WhereIn("i.account_id", accountIds)
	}
	if in != nil && in.AccountId > 0 {
		return mod.Where("i.account_id", in.AccountId)
	}
	return mod
}

func applyNoteIndexFilters(mod *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	if in == nil {
		return mod
	}
	if province := strings.TrimSpace(in.Province); province != "" {
		mod = mod.Where("i.province", province)
	}
	if city := strings.TrimSpace(in.City); city != "" {
		mod = mod.Where("i.city", city)
	}
	if reviewStatus := strings.TrimSpace(in.ReviewStatus); reviewStatus != "" {
		mod = mod.Where("i.review_status", reviewStatus)
	}
	if visibility := strings.TrimSpace(in.Visibility); visibility != "" {
		mod = mod.Where("i.visibility", visibility)
	}
	if in.Status == 2 && in.IncludePendingReview {
		mod = mod.WhereIn("i.status", []int{2, sysin.ProfileStatusReviewPending})
	} else if in.Status > 0 {
		mod = mod.Where("i.status", in.Status)
	}
	mod = applyProfileCollectionSourceFilter(mod, in, "i.profile_id")
	switch strings.TrimSpace(in.SourceScope) {
	case "collected":
		// Filter in the ordered read model, before joining profile/state rows.
		// Filtering p.source_type after LIMIT-driven index scans can otherwise
		// probe hundreds of thousands of unrelated profiles for an empty page.
		mod = mod.Where("i."+noteIndexCollectedColumn, true)
	case "manual":
		mod = mod.Where("i."+noteIndexCollectedColumn, false)
	}
	if tag := strings.TrimSpace(in.Tag); tag != "" {
		mod = applyNoteIndexTagFilter(mod, splitProfileTagValues(tag))
	}
	mod = applyProfilePushChannelFilter(mod, in, "i.tenant_id", "i.profile_id")
	return applyProfileKeywordSearch(mod, in.Keyword, noteIndexSearchFields())
}

func applyNoteIndexTagFilter(mod *gdb.Model, tags []string) *gdb.Model {
	conditions := make([]string, 0, len(tags)*4)
	args := make([]interface{}, 0, len(tags)*4)
	for _, item := range tags {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		conditions = append(conditions, "(i.tag = ? OR i.tag LIKE ? OR i.tag LIKE ? OR i.tag LIKE ?)")
		args = append(args, tag, tag+",%", "%,"+tag, "%,"+tag+",%")
	}
	if len(conditions) == 0 {
		return mod
	}
	return mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

func normalizeAdminNoteSortBy(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return adminNoteSortCreatedAt + "Desc", nil
	}
	for _, field := range []string{adminNoteSortCreatedAt, adminNoteSortPublishedAt, adminNoteSortUpdatedAt} {
		if value == field+"Asc" || value == field+"Desc" {
			return value, nil
		}
		// Keep compatibility with the original descending-only values.
		if value == field {
			return value + "Desc", nil
		}
	}
	return "", gerror.New("笔记列表排序字段不合法")
}

func adminNoteSortExpression(sortBy string) string {
	if strings.HasPrefix(sortBy, adminNoteSortPublishedAt) {
		return "COALESCE(i.published_at, '1970-01-01'::timestamp)"
	}
	if strings.HasPrefix(sortBy, adminNoteSortUpdatedAt) {
		return "i.updated_at"
	}
	return "i.created_at"
}

func adminNoteSortDirection(sortBy string) string {
	if strings.HasSuffix(sortBy, "Asc") {
		return "ASC"
	}
	return "DESC"
}

func applyNoteIndexCursor(mod *gdb.Model, raw string, sortBy string) (*gdb.Model, error) {
	cursor, err := decodeAdminNoteCursor(raw)
	if err != nil {
		return mod, err
	}
	if cursor == nil {
		return mod, nil
	}
	if cursor.SortBy != sortBy {
		return mod, gerror.New("笔记列表排序方式已变化，请重新加载")
	}
	condition, args, err := adminNoteCursorCondition(cursor, sortBy)
	if err != nil {
		return mod, err
	}
	return mod.Where(condition, args...), nil
}

func adminNoteCursorCondition(cursor *adminNoteCursor, sortBy string) (string, []interface{}, error) {
	expression := adminNoteSortExpression(sortBy)
	ascending := adminNoteSortDirection(sortBy) == "ASC"
	if cursor.SortNull {
		if strings.HasPrefix(sortBy, adminNoteSortPublishedAt) {
			return "", nil, gerror.New("笔记列表游标不合法")
		}
		if ascending {
			return "(" + expression + " IS NULL AND i.id > ?)", []interface{}{cursor.IndexId}, nil
		}
		return "((" + expression + " IS NULL AND i.id < ?) OR " + expression + " IS NOT NULL)", []interface{}{cursor.IndexId}, nil
	}
	sortAt, err := time.Parse(time.RFC3339Nano, cursor.SortAt)
	if err != nil {
		return "", nil, gerror.New("笔记列表游标不合法")
	}
	operator := "<"
	if ascending {
		operator = ">"
	}
	condition := "(" + expression + " " + operator + " ? OR (" + expression + " = ? AND i.id " + operator + " ?)"
	if ascending && !strings.HasPrefix(sortBy, adminNoteSortPublishedAt) {
		condition += " OR " + expression + " IS NULL"
	}
	return condition + ")", []interface{}{sortAt, sortAt, cursor.IndexId}, nil
}

func encodeAdminNoteCursor(item *sysin.ProfileModel, sortBy string) string {
	if item == nil || item.Id <= 0 {
		return ""
	}
	if item.NoteIndexId <= 0 {
		return ""
	}
	sortAt := item.CreatedAt
	if strings.HasPrefix(sortBy, adminNoteSortPublishedAt) {
		sortAt = item.PublishedAt
		if sortAt == nil {
			sortAt = gtime.NewFromTime(time.Unix(0, 0))
		}
	}
	if strings.HasPrefix(sortBy, adminNoteSortUpdatedAt) {
		sortAt = item.UpdatedAt
	}
	if sortAt == nil {
		return ""
	}
	return encodeAdminNoteCandidateCursor(item.NoteIndexId, sortAt, sortBy)
}

// The cursor uses the candidate scan values, which can differ from a later detail read.
func encodeAdminNoteCandidateCursor(indexId int64, sortAt *gtime.Time, sortBy string) string {
	if indexId <= 0 || sortAt == nil {
		return ""
	}
	payload, err := json.Marshal(adminNoteCursor{IndexId: indexId, SortBy: sortBy, SortAt: sortAt.Time.Format(time.RFC3339Nano)})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func encodeAdminNotePageCursor(indexId int64, sortAt *gtime.Time, sortBy string) string {
	if sortAt != nil {
		return encodeAdminNoteCandidateCursor(indexId, sortAt, sortBy)
	}
	if indexId <= 0 || strings.HasPrefix(sortBy, adminNoteSortPublishedAt) {
		return ""
	}
	payload, err := json.Marshal(adminNoteCursor{IndexId: indexId, SortBy: sortBy, SortNull: true})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeAdminNoteCursor(raw string) (*adminNoteCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, gerror.New("笔记列表游标不合法")
	}
	var cursor adminNoteCursor
	if err = json.Unmarshal(payload, &cursor); err != nil || cursor.IndexId <= 0 || cursor.SortBy == "" || (cursor.SortAt == "") == !cursor.SortNull {
		return nil, gerror.New("笔记列表游标不合法")
	}
	return &cursor, nil
}

func adminNoteIndexFields() string {
	return "i.id AS note_index_id,i.profile_id AS id,i.uuid,i.tenant_id,i.account_id,p.cover_media_id,p.source_type,p.profile_no,i.title,i.province,i.city,i.tag,i.visibility,i.review_status,i.status,i.published_at,i.created_at,i.updated_at,CASE WHEN COALESCE(ps.publish_task_status,'') IN ('pending','publishing') AND (LOWER(COALESCE(ps.publish_operation_no,'')) LIKE 'cycle_batch:%' OR LOWER(COALESCE(ps.publish_operation_no,'')) LIKE 'profile_cycle:%') THEN 'cycling' ELSE COALESCE(ps.publish_task_status,i.task_status) END AS task_status,a.nickname AS account_name,a.nickname,a.username"
}
