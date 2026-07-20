package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

var profileSearchProfileNoRegexp = botProfileNoRegexp

func (s *sSysPublish) searchProfilePage(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	keyword := strings.TrimSpace(in.Keyword)
	base = s.applyProfileNonKeywordFilters(ctx, base, in)
	if keyword == "" {
		return s.scanProfilePage(base, in, fields, countErrMessage, listErrMessage)
	}

	if profileNo, ok := normalizeProfileNoSearchKeyword(keyword); ok {
		mod := base.Clone().Where("p.profile_no", profileNo)
		return s.scanProfilePage(mod, in, fields, countErrMessage, listErrMessage)
	}

	terms := splitProfileSearchTerms(keyword)
	if len(terms) == 0 {
		return s.scanProfilePage(base, in, fields, countErrMessage, listErrMessage)
	}

	page, perPage, offset := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = perPage

	titleCondition, titleArgs := segmentedLikeCondition([]string{"p.title", "t.title"}, terms)
	titleExcludeCondition, titleExcludeArgs := segmentedLikeConditionNullSafe([]string{"p.title", "t.title"}, terms)
	titleMod := base.Clone().Where(titleCondition, titleArgs...)
	titleCount, err := titleMod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, countErrMessage)
	}

	bodyCondition, bodyArgs := segmentedLikeCondition([]string{"p.plain_text", "t.plain_text"}, terms)
	bodyMod := base.Clone().
		Where(bodyCondition, bodyArgs...).
		Where("NOT ("+titleExcludeCondition+")", titleExcludeArgs...)
	bodyCount, err := bodyMod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, countErrMessage)
	}

	totalCount := titleCount + bodyCount
	if totalCount == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}

	list := make([]*sysin.ProfileModel, 0, perPage)
	if offset < titleCount {
		limit := perPage
		if remain := titleCount - offset; remain < limit {
			limit = remain
		}
		titleList, err := scanProfileOffsetPage(titleMod, fields, offset, limit, listErrMessage)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, titleList...)
	}

	if len(list) < perPage {
		bodyOffset := 0
		if offset > titleCount {
			bodyOffset = offset - titleCount
		}
		bodyList, err := scanProfileOffsetPage(bodyMod, fields, bodyOffset, perPage-len(list), listErrMessage)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, bodyList...)
	}

	return list, totalCount, nil
}

func (s *sSysPublish) scanProfilePage(mod *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	totalCount, err := mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, countErrMessage)
	}
	if totalCount == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	page, perPage, _ := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = perPage
	list, err := scanProfileOffsetPage(mod, fields, (page-1)*perPage, perPage, listErrMessage)
	return list, totalCount, err
}

func scanProfileOffsetPage(mod *gdb.Model, fields string, offset int, limit int, listErrMessage string) ([]*sysin.ProfileModel, error) {
	if limit <= 0 {
		return []*sysin.ProfileModel{}, nil
	}
	var list []*sysin.ProfileModel
	if err := mod.Clone().
		Fields(fields).
		Limit(offset, limit).
		OrderDesc("p.updated_at").
		OrderDesc("p.id").
		Scan(&list); err != nil {
		return nil, gerror.Wrap(err, listErrMessage)
	}
	if list == nil {
		list = []*sysin.ProfileModel{}
	}
	return list, nil
}

func normalizeProfileNoSearchKeyword(keyword string) (string, bool) {
	profileNo := normalizeBotProfileNo(keyword)
	return profileNo, profileSearchProfileNoRegexp.MatchString(profileNo)
}

func splitProfileSearchTerms(keyword string) []string {
	parts := strings.Fields(strings.TrimSpace(keyword))
	if len(parts) == 0 {
		return nil
	}
	terms := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func segmentedLikeCondition(fields []string, terms []string) (string, []interface{}) {
	return buildSegmentedLikeCondition(fields, terms, false)
}

func segmentedLikeConditionNullSafe(fields []string, terms []string) (string, []interface{}) {
	return buildSegmentedLikeCondition(fields, terms, true)
}

func buildSegmentedLikeCondition(fields []string, terms []string, nullSafe bool) (string, []interface{}) {
	conditions := make([]string, 0, len(terms))
	args := make([]interface{}, 0, len(terms)*len(fields))
	for _, term := range terms {
		likes := make([]string, 0, len(fields))
		like := "%" + term + "%"
		for _, field := range fields {
			if nullSafe {
				likes = append(likes, "COALESCE("+field+", '') LIKE ?")
			} else {
				likes = append(likes, field+" LIKE ?")
			}
			args = append(args, like)
		}
		conditions = append(conditions, "("+strings.Join(likes, " OR ")+")")
	}
	return "(" + strings.Join(conditions, " AND ") + ")", args
}
