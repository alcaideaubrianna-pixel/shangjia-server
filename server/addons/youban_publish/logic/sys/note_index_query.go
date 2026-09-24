package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type adminNoteCursor struct {
	IndexId int64  `json:"indexId"`
	SortBy  string `json:"sortBy"`
	SortAt  string `json:"sortAt"`
}

const (
	adminNoteSortCreatedAt   = "createdAt"
	adminNoteSortPublishedAt = "publishedAt"
)

func (s *sSysPublish) adminNoteIndexList(ctx context.Context, in *sysin.NoteListInp, tenantId int64, tenantIds []int64, accountIds []int64) ([]*sysin.ProfileModel, bool, string, error) {
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	mod := noteIndexModel(ctx).LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
	mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
	mod = applyNoteIndexFilters(mod, &in.ProfileListInp)
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
	var list []*sysin.ProfileModel
	if err := mod.Clone().Fields(adminNoteIndexFields()).
		OrderDesc(adminNoteSortExpression(sortBy)).OrderDesc("i.id").
		Limit(perPage + 1).Scan(&list); err != nil {
		return nil, false, "", gerror.Wrap(err, "获取资料索引列表失败")
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	hasMore := len(list) > perPage
	if hasMore {
		list = list[:perPage]
	}
	nextCursor := ""
	if hasMore && len(list) > 0 {
		nextCursor = encodeAdminNoteCursor(list[len(list)-1], sortBy)
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
	if in.Status > 0 {
		mod = mod.Where("i.status", in.Status)
	}
	if in.CollectSourceId > 0 {
		condition := "EXISTS (SELECT 1 FROM " + publishCollectDispatchTable + " d JOIN " + publishCollectEventTable + " e ON e.id=d.event_id WHERE d.profile_id=i.profile_id AND d.source_id=?"
		args := []interface{}{in.CollectSourceId}
		if chatId := strings.TrimSpace(in.CollectSourceChatId); chatId != "" {
			condition += " AND e.source_chat_id=?"
			args = append(args, chatId)
		}
		mod = mod.Where(condition+")", args...)
	}
	switch strings.TrimSpace(in.SourceScope) {
	case "collected":
		// note_index already joins content_profile as p. The profile source type is
		// the canonical, indexed source of truth for collected materials; avoid a
		// per-row collect_dispatch existence scan on the admin list path.
		mod = mod.Where("p.source_type", collectProfileSourceType)
	case "manual":
		mod = mod.Where("p.source_type != ? OR p.source_type IS NULL OR p.source_type = ''", collectProfileSourceType)
	}
	if tag := strings.TrimSpace(in.Tag); tag != "" {
		mod = applyNoteIndexTagFilter(mod, splitProfileTagValues(tag))
	}
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
	switch strings.TrimSpace(value) {
	case "", adminNoteSortCreatedAt:
		return adminNoteSortCreatedAt, nil
	case adminNoteSortPublishedAt:
		return adminNoteSortPublishedAt, nil
	default:
		return "", gerror.New("笔记列表排序字段不合法")
	}
}

func adminNoteSortExpression(sortBy string) string {
	if sortBy == adminNoteSortPublishedAt {
		return "COALESCE(i.published_at, '1970-01-01'::timestamp)"
	}
	return "i.created_at"
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
	sortAt, err := time.Parse(time.RFC3339Nano, cursor.SortAt)
	if err != nil {
		return mod, gerror.New("笔记列表游标不合法")
	}
	expression := adminNoteSortExpression(sortBy)
	return mod.Where("("+expression+" < ? OR ("+expression+" = ? AND i.id < ?))", sortAt, sortAt, cursor.IndexId), nil
}

func encodeAdminNoteCursor(item *sysin.ProfileModel, sortBy string) string {
	if item == nil || item.Id <= 0 {
		return ""
	}
	if item.NoteIndexId <= 0 {
		return ""
	}
	sortAt := item.CreatedAt
	if sortBy == adminNoteSortPublishedAt {
		sortAt = item.PublishedAt
		if sortAt == nil {
			sortAt = gtime.NewFromTime(time.Unix(0, 0))
		}
	}
	if sortAt == nil {
		return ""
	}
	payload, err := json.Marshal(adminNoteCursor{IndexId: item.NoteIndexId, SortBy: sortBy, SortAt: sortAt.Time.Format(time.RFC3339Nano)})
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
	if err = json.Unmarshal(payload, &cursor); err != nil || cursor.IndexId <= 0 || cursor.SortBy == "" || cursor.SortAt == "" {
		return nil, gerror.New("笔记列表游标不合法")
	}
	return &cursor, nil
}

func adminNoteIndexFields() string {
	return "i.id AS note_index_id,i.profile_id AS id,i.uuid,i.tenant_id,i.account_id,p.source_type,p.profile_no,i.title,i.summary,i.plain_text,i.province,i.city,i.tag,i.visibility,i.review_status,i.status,i.published_at,i.created_at,i.updated_at,i.task_status,a.nickname AS account_name,a.nickname,a.username"
}
