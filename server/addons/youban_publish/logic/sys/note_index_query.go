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

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/model/input/form"
)

const adminNoteIndexCoverageTTL = 30 * time.Second

func (s *sSysPublish) adminNoteIndexList(ctx context.Context, in *sysin.ProfileListInp, tenantId int64, tenantIds []int64, accountIds []int64) ([]*sysin.ProfileModel, int, bool, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	if !s.adminNoteIndexReady(ctx, tenantId, tenantIds, accountIds) {
		return nil, 0, false, nil
	}
	mod := noteIndexModel(ctx).LeftJoin(publishAccountTable+" a", "a.id=i.account_id AND a.deleted_at IS NULL")
	mod = applyNoteIndexScope(mod, tenantId, tenantIds, accountIds, in)
	mod = applyNoteIndexFilters(mod, in)
	totalCount, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, true, gerror.Wrap(err, "统计资料索引失败")
	}
	if totalCount == 0 {
		return []*sysin.ProfileModel{}, 0, true, nil
	}
	page, perPage, _ := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = perPage
	var list []*sysin.ProfileModel
	if err = mod.Clone().Fields(adminNoteIndexFields()).
		OrderDesc("i.updated_at").OrderDesc("i.id").
		Limit((page-1)*perPage, perPage).Scan(&list); err != nil {
		return nil, 0, true, gerror.Wrap(err, "获取资料索引列表失败")
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	return list, totalCount, true, nil
}

func (s *sSysPublish) adminNoteIndexReady(ctx context.Context, tenantId int64, tenantIds []int64, accountIds []int64) bool {
	key := adminNoteIndexCoverageKey(tenantId, tenantIds, accountIds)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		return value.Int() == 1
	}
	sourceCount, err := adminNoteIndexSourceCount(ctx, tenantId, tenantIds, accountIds)
	if err != nil {
		return false
	}
	indexCount, err := adminNoteIndexCount(ctx, tenantId, tenantIds, accountIds)
	if err != nil {
		return false
	}
	ready := sourceCount == indexCount
	value := 0
	if ready {
		value = 1
	}
	_ = cache.Instance().Set(ctx, key, value, adminNoteIndexCoverageTTL)
	return ready
}

func adminNoteIndexSourceCount(ctx context.Context, tenantId int64, tenantIds []int64, accountIds []int64) (int, error) {
	mod := gdbModel(ctx, "hg_content_profile p").
		InnerJoin("hg_youban_publish_task t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		WhereNull("p.deleted_at")
	if len(tenantIds) > 0 {
		mod = mod.WhereIn("t.tenant_id", tenantIds)
	} else if tenantId > 0 {
		mod = mod.Where("t.tenant_id", tenantId)
	}
	if len(accountIds) > 0 {
		mod = mod.WhereIn("t.account_id", accountIds)
	}
	return mod.Count()
}

func adminNoteIndexCount(ctx context.Context, tenantId int64, tenantIds []int64, accountIds []int64) (int, error) {
	mod := noteIndexModel(ctx)
	if len(tenantIds) > 0 {
		mod = mod.WhereIn("i.tenant_id", tenantIds)
	} else if tenantId > 0 {
		mod = mod.Where("i.tenant_id", tenantId)
	}
	if len(accountIds) > 0 {
		mod = mod.WhereIn("i.account_id", accountIds)
	}
	return mod.Count()
}

func gdbModel(ctx context.Context, table string) *gdb.Model {
	return g.DB().Model(table).Safe().Ctx(ctx)
}

func adminNoteIndexCoverageKey(tenantId int64, tenantIds []int64, accountIds []int64) string {
	tenantIds = append([]int64(nil), tenantIds...)
	accountIds = append([]int64(nil), accountIds...)
	sort.Slice(tenantIds, func(i, j int) bool { return tenantIds[i] < tenantIds[j] })
	sort.Slice(accountIds, func(i, j int) bool { return accountIds[i] < accountIds[j] })
	return fmt.Sprintf("youban_publish:note_index:coverage:%s", mediaPHashHashKey(fmt.Sprintf("%d:%v:%v", tenantId, tenantIds, accountIds)))
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

func adminNoteIndexFields() string {
	return "i.profile_id AS id,i.uuid,i.task_id,i.tenant_id,i.account_id,i.profile_no,i.title,i.summary,i.plain_text,i.province,i.city,i.tag,i.visibility,i.review_status,i.status,i.published_at,i.created_at,i.updated_at,i.task_status,a.nickname AS account_name,a.nickname,a.username"
}
