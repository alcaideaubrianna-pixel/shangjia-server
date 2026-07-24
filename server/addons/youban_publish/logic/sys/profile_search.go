package sys

import (
	"context"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

var profileSearchProfileNoRegexp = regexp.MustCompile(`^[A-Z][A-Z0-9]{4,}$`)

func (s *sSysPublish) searchProfilePage(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	mod := s.profileSearchModel(ctx, base, in)
	return s.scanProfilePage(mod, in, fields, countErrMessage, listErrMessage)
}

func (s *sSysPublish) searchDistinctProfilePage(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	mod := s.profileSearchModel(ctx, base, in)
	return s.scanDistinctProfilePage(mod, in, fields, countErrMessage, listErrMessage)
}

func (s *sSysPublish) profileSearchModel(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	keyword := strings.TrimSpace(in.Keyword)
	base = s.applyProfileNonKeywordFilters(ctx, base, in)
	if keyword == "" {
		return base
	}

	if profileNo, ok := normalizeProfileNoSearchKeyword(keyword); ok {
		return base.Clone().Where("p.profile_no", profileNo)
	}

	terms := splitProfileSearchTerms(keyword)
	if len(terms) == 0 {
		return base
	}

	searchCondition, searchArgs := segmentedLikeCondition([]string{
		"p.title",
		"t.title",
		"p.plain_text",
		"t.plain_text",
	}, terms)
	return base.Clone().Where(searchCondition, searchArgs...)
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

func (s *sSysPublish) scanDistinctProfilePage(mod *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	var row struct {
		Total int `orm:"total"`
	}
	if err := mod.Clone().Fields("COUNT(DISTINCT p.id) AS total").Scan(&row); err != nil {
		return nil, 0, gerror.Wrap(err, countErrMessage)
	}
	if row.Total == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	page, perPage, _ := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = perPage
	list, err := scanProfileOffsetPage(mod, fields, (page-1)*perPage, perPage, listErrMessage)
	return list, row.Total, err
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
	if !profileSearchProfileNoRegexp.MatchString(profileNo) {
		return profileNo, false
	}
	for _, char := range profileNo {
		if char >= '0' && char <= '9' {
			return profileNo, true
		}
	}
	return profileNo, false
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
