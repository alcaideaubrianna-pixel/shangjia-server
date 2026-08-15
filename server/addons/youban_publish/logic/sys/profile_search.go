package sys

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/model/input/form"
)

var (
	profileSearchLabelRegexp = regexp.MustCompile(`(?i)^\s*(?:资料)?编号\s*[:：=]?\s*`)
	profileSearchNoRegexp    = regexp.MustCompile(`^[A-Z][A-Z0-9]{4,}$`)
	profileSearchMarkRegexp  = regexp.MustCompile(`^(.+?)([0-9]{3})$`)
)

type profileSearchFields struct {
	ProfileNo    string
	Title        string
	Summary      string
	PlainText    string
	AccountAlias string
	SettingAlias string
	StateAlias   string
}

func (s *sSysPublish) searchProfilePage(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string) ([]*sysin.ProfileModel, int, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	mod := s.profileSearchModel(ctx, base, in)
	return s.scanProfilePage(mod, in, fields, countErrMessage, listErrMessage)
}

func (s *sSysPublish) searchDistinctProfilePage(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string, countCacheKeys ...string) ([]*sysin.ProfileModel, int, error) {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	mod := s.profileSearchModel(ctx, base, in)
	return s.scanDistinctProfilePage(ctx, mod, in, fields, countErrMessage, listErrMessage, countCacheKeys...)
}

func (s *sSysPublish) profileSearchModel(ctx context.Context, base *gdb.Model, in *sysin.ProfileListInp) *gdb.Model {
	if in == nil {
		in = &sysin.ProfileListInp{}
	}
	base = s.applyProfileNonKeywordFilters(ctx, base, in)
	return applyProfileKeywordSearch(base, in.Keyword, profileTableSearchFields())
}

func profileSearchKeywordFields() []string {
	return []string{"p.profile_no", "p.title", "p.summary", "p.plain_text"}
}

func profileTableSearchFields() profileSearchFields {
	return profileSearchFields{
		ProfileNo: "p.profile_no", Title: "p.title", Summary: "p.summary", PlainText: "p.plain_text",
		AccountAlias: "a", SettingAlias: "account_setting", StateAlias: "ps",
	}
}

func noteIndexSearchFields() profileSearchFields {
	return profileSearchFields{
		ProfileNo: "p.profile_no", Title: "i.title", Summary: "i.summary", PlainText: "i.plain_text",
		AccountAlias: "a", SettingAlias: "account_setting", StateAlias: "ps",
	}
}

func applyProfileKeywordSearch(mod *gdb.Model, keyword string, fields profileSearchFields) *gdb.Model {
	keyword = normalizeProfileSearchKeyword(keyword)
	if keyword == "" {
		return mod
	}
	upper := strings.ToUpper(keyword)
	if profileSearchNoRegexp.MatchString(upper) {
		condition, args := profileTextSearchCondition(keyword, fields)
		return mod.Where("(UPPER("+fields.ProfileNo+") = ? OR "+condition+")", append([]interface{}{upper}, args...)...)
	}
	if sequence, prefix, ok := parseProfilePublishMark(keyword); ok {
		mod = mod.LeftJoin(publishAccountSettingTable+" account_setting", "account_setting.tenant_id="+fields.StateAlias+".tenant_id AND account_setting.account_id="+fields.StateAlias+".account_id AND account_setting.deleted_at IS NULL")
		sequenceExpr := profileAccountSequenceExpr(fields.StateAlias)
		condition, args := profileTextSearchCondition(keyword, fields)
		if prefix == "" {
			return mod.Where("("+sequenceExpr+" = ? OR "+condition+")", append([]interface{}{sequence}, args...)...)
		}
		return mod.Where("(UPPER("+profilePublishMarkExpr(fields, sequenceExpr)+") = ? OR "+condition+")", append([]interface{}{strings.ToUpper(prefix + sequence)}, args...)...)
	}
	condition, args := profileTextSearchCondition(keyword, fields)
	return mod.Where(condition, args...)
}

func profileTextSearchCondition(keyword string, fields profileSearchFields) (string, []interface{}) {
	terms := splitProfileSearchTerms(keyword)
	if len(terms) == 0 {
		return "1=1", nil
	}
	return segmentedLikeConditionNullSafe([]string{fields.ProfileNo, fields.Title, fields.Summary, fields.PlainText}, terms)
}

func normalizeProfileSearchKeyword(keyword string) string {
	return strings.TrimSpace(profileSearchLabelRegexp.ReplaceAllString(strings.TrimSpace(keyword), ""))
}

func parseProfilePublishMark(keyword string) (sequence string, prefix string, ok bool) {
	keyword = strings.TrimSpace(keyword)
	if profileSearchNoRegexp.MatchString(strings.ToUpper(keyword)) {
		return "", "", false
	}
	if len(keyword) == 3 {
		for _, r := range keyword {
			if r < '0' || r > '9' {
				return "", "", false
			}
		}
		return keyword, "", true
	}
	matches := profileSearchMarkRegexp.FindStringSubmatch(keyword)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[2], strings.TrimSpace(matches[1]), true
}

func profileAccountSequenceExpr(stateAlias string) string {
	return "LPAD(CONCAT('',(SELECT COUNT(1) FROM " + publishProfileStateTable + " ps_seq WHERE ps_seq.tenant_id=" + stateAlias + ".tenant_id AND ps_seq.account_id=" + stateAlias + ".account_id AND ps_seq.id<=" + stateAlias + ".id AND ps_seq.deleted_at IS NULL)),3,'0')"
}

func profilePublishMarkExpr(fields profileSearchFields, sequenceExpr string) string {
	prefix := "CASE WHEN " + fields.SettingAlias + ".mark_mode='custom' AND COALESCE(" + fields.SettingAlias + ".custom_mark_text,'')<>'' THEN " + fields.SettingAlias + ".custom_mark_text ELSE COALESCE(" + fields.AccountAlias + ".nickname,'') END"
	return "CASE WHEN COALESCE(" + fields.SettingAlias + ".enable_title_mark,0)=1 AND COALESCE(" + fields.SettingAlias + ".number_source,'sequence')<>'random' THEN CONCAT(" + prefix + "," + sequenceExpr + ") ELSE '' END"
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

func (s *sSysPublish) scanDistinctProfilePage(ctx context.Context, mod *gdb.Model, in *sysin.ProfileListInp, fields string, countErrMessage string, listErrMessage string, countCacheKeys ...string) ([]*sysin.ProfileModel, int, error) {
	var row struct {
		Total int `orm:"total"`
	}
	countStartedAt := time.Now()
	countCacheKey := ""
	if len(countCacheKeys) > 0 {
		countCacheKey = countCacheKeys[0]
	}
	countCached := false
	if countCacheKey != "" {
		if cached, err := cache.Instance().Get(ctx, countCacheKey); err == nil && !cached.IsNil() {
			if scanErr := cached.Scan(&row.Total); scanErr == nil {
				countCached = true
			}
		}
	}
	if !countCached {
		if err := mod.Clone().Fields("COUNT(DISTINCT p.id) AS total").Scan(&row); err != nil {
			return nil, 0, gerror.Wrap(err, countErrMessage)
		}
		if countCacheKey != "" {
			_ = cache.Instance().Set(ctx, countCacheKey, row.Total, adminNoteCountCacheTTL)
		}
	}
	logSlowAdminNoteListStage(ctx, "count", countStartedAt, row.Total, 0)
	if row.Total == 0 {
		return []*sysin.ProfileModel{}, 0, nil
	}
	page, perPage, _ := form.CalPage(in.Page, in.PerPage)
	in.Page = page
	in.PerPage = perPage
	pageStartedAt := time.Now()
	list, err := scanProfileOffsetPage(mod, fields, (page-1)*perPage, perPage, listErrMessage)
	logSlowAdminNoteListStage(ctx, "page", pageStartedAt, len(list), perPage)
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
