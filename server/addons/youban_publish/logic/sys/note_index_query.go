package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type adminNoteCursor struct {
	IndexId   int64  `json:"indexId"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *sSysPublish) adminNoteIndexList(ctx context.Context, in *sysin.NoteListInp, tenantId int64, tenantIds []int64, accountIds []int64) ([]*sysin.ProfileModel, bool, string, error) {
	if in == nil {
		in = &sysin.NoteListInp{}
	}
	mod := noteIndexModel(ctx).LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
	mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, &in.ProfileListInp)
	mod = applyNoteIndexFilters(mod, &in.ProfileListInp)
	var err error
	if mod, err = applyNoteIndexCursor(mod, in.Cursor); err != nil {
		return nil, false, "", err
	}
	_, perPage, _ := form.CalPage(1, in.PerPage)
	in.Page = 1
	in.PerPage = perPage
	var list []*sysin.ProfileModel
	if err := mod.Clone().Fields(adminNoteIndexFields()).
		OrderDesc("i.updated_at").OrderDesc("i.id").
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
		nextCursor = encodeAdminNoteCursor(list[len(list)-1])
	}
	return list, hasMore, nextCursor, nil
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
		mod = mod.Where("EXISTS (SELECT 1 FROM "+publishCollectDispatchTable+" d WHERE d.profile_id=i.profile_id AND d.source_id=?)", in.CollectSourceId)
	}
	if tag := strings.TrimSpace(in.Tag); tag != "" {
		mod = applyNoteIndexTagFilter(mod, splitProfileTagValues(tag))
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		if profileNo, ok := normalizeProfileNoSearchKeyword(keyword); ok {
			return mod.Where("i.profile_no", profileNo)
		}
		terms := splitProfileSearchTerms(keyword)
		if len(terms) > 0 {
			condition, args := segmentedLikeConditionNullSafe([]string{"i.profile_no", "i.title", "i.plain_text"}, terms)
			mod = mod.Where(condition, args...)
		}
	}
	return mod
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

func applyNoteIndexCursor(mod *gdb.Model, raw string) (*gdb.Model, error) {
	cursor, err := decodeAdminNoteCursor(raw)
	if err != nil {
		return mod, err
	}
	if cursor == nil {
		return mod, nil
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, cursor.UpdatedAt)
	if err != nil {
		return mod, gerror.New("笔记列表游标不合法")
	}
	return mod.Where("(i.updated_at < ? OR (i.updated_at = ? AND i.id < ?))", updatedAt, updatedAt, cursor.IndexId), nil
}

func encodeAdminNoteCursor(item *sysin.ProfileModel) string {
	if item == nil || item.Id <= 0 || item.UpdatedAt == nil {
		return ""
	}
	if item.NoteIndexId <= 0 {
		return ""
	}
	payload, err := json.Marshal(adminNoteCursor{IndexId: item.NoteIndexId, UpdatedAt: item.UpdatedAt.Time.Format(time.RFC3339Nano)})
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
	if err = json.Unmarshal(payload, &cursor); err != nil || cursor.IndexId <= 0 || cursor.UpdatedAt == "" {
		return nil, gerror.New("笔记列表游标不合法")
	}
	return &cursor, nil
}

func adminNoteIndexFields() string {
	return "i.id AS note_index_id,i.profile_id AS id,i.uuid,i.tenant_id,i.account_id,p.source_type,i.profile_no,i.title,i.summary,i.plain_text,i.province,i.city,i.tag,i.visibility,i.review_status,i.status,i.published_at,i.created_at,i.updated_at,i.task_status,a.nickname AS account_name,a.nickname,a.username"
}
