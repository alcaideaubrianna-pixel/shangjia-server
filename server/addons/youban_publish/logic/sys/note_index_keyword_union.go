package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// noteIndexUnionKeywordTerms excludes the two special search paths, which
// have different semantics from ordinary text search.
func noteIndexUnionKeywordTerms(keyword string) []string {
	keyword = normalizeProfileSearchKeyword(keyword)
	if keyword == "" {
		return nil
	}
	if _, _, ok := parseProfilePublishMark(keyword); ok {
		return nil
	}
	if profileSearchNoRegexp.MatchString(strings.ToUpper(keyword)) {
		return nil
	}
	return splitProfileSearchTerms(keyword)
}

func noteIndexKeywordUnionConditions(terms []string) (string, []interface{}, string, []interface{}) {
	const projection = "SELECT 1 FROM hg_youban_publish_note_search ns WHERE ns.tenant_id=i.tenant_id AND ns.account_id=i.account_id AND ns.profile_id=i.profile_id AND ns.deleted_at IS NULL"
	conditions := make([]string, 0, len(terms))
	args := make([]interface{}, 0, len(terms))
	for _, term := range terms {
		conditions = append(conditions, "EXISTS ("+projection+" AND ns.search_text ILIKE ?)")
		args = append(args, "%"+term+"%")
	}
	legacy, legacyArgs := profileTextSearchCondition(strings.Join(terms, " "), noteIndexSearchFields())
	return strings.Join(conditions, " AND "), args, "NOT EXISTS (" + projection + ") AND " + legacy, legacyArgs
}

// The caller supplies an unbounded model with all non-keyword filters and the
// cursor already applied. Each branch retains the original state join and all
// authorization predicates; ORDER and LIMIT apply only after UNION ALL.
func noteIndexKeywordUnionQuery(ctx context.Context, base *gdb.Model, terms []string, sortExpression, direction string, limit int) *gdb.Model {
	return noteIndexKeywordUnionQueryDB(ctx, pdao.YoubanPublishNoteIndex.DB(), base, terms, sortExpression, direction, limit)
}

func noteIndexKeywordUnionQueryDB(ctx context.Context, db gdb.DB, base *gdb.Model, terms []string, sortExpression, direction string, limit int) *gdb.Model {
	projectionCondition, projectionArgs, legacyCondition, legacyArgs := noteIndexKeywordUnionConditions(terms)
	fields := "i.id AS note_index_id,i.profile_id AS id," + sortExpression + " AS sort_at"
	projection := base.Clone().Where(projectionCondition, projectionArgs...).Fields(fields)
	legacy := base.Clone().Where(legacyCondition, legacyArgs...).Fields(fields)
	return db.Ctx(ctx).UnionAll(projection, legacy).Ctx(ctx).
		Order("sort_at " + direction).Order("note_index_id " + direction).Limit(limit)
}
