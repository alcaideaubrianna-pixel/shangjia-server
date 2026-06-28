package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/location"
	"hotgo/internal/library/token"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	contentSourceFeiNiu         = "feiniu"
	contentImportCronName       = "content_import_feiniu"
	contentImportCronTitle      = "FeiNiu 内容自动同步"
	contentImportCronPattern    = "0 */1 * * * *"
	contentReviewConfigGroup    = "content_import_review"
	feiNiuCosURLPrefix          = "https://sh-hk-qiuniu-1368574312.cos.ap-hongkong.myqcloud.com/"
	feiNiuProxyCosPrefix        = "/prod-api/telegram/content-note/cos/"
	contentFilterOptionsKey     = "content:profile:filter_options"
	contentRegionsKey           = "content:profile:regions"
	contentHomeCardsKeyBase     = "content:home:profile_cards"
	contentProfileStatsTable    = "hg_content_profile_stats"
	contentImagePHashSigKey     = "content:profile:image_phash_sig:"
	contentProfileHomeRecommend = "home_recommend"
	contentProfileHomeSort      = "home_sort"
)

var errFeiNiuMediaPending = errors.New("feiniu media pending")

var introFeeRegexp = regexp.MustCompile(`(?im)(?:^|[\s，,。；;、|｜/／（(【\[])(?:介绍费|介紹費|中介费|中介費|服务费|服務費)\s*[:：]?\s*(?:[¥￥]?\s*)?[0-9０-９][0-9０-９,，.\s]*(?:元|块|w|W|万)?[^\n\r，,。；;、|｜）)】\]]*`)
var regionNoiseRegexp = regexp.MustCompile(`(?i)(省份|城市|地区|区域|位置|坐标|地址|可|t8|T8|年龄|身高|体重|罩杯|编号|:|：|\(|\)|（|）|\[|\]|【|】)`)

var provinceAliases = map[string]string{
	"北京": "北京", "北京市": "北京",
	"天津": "天津", "天津市": "天津",
	"河北": "河北", "河北省": "河北",
	"山西": "山西", "山西省": "山西",
	"内蒙": "内蒙古", "内蒙古": "内蒙古", "内蒙古自治区": "内蒙古",
	"辽宁": "辽宁", "辽宁省": "辽宁",
	"吉林": "吉林", "吉林省": "吉林",
	"黑龙江": "黑龙江", "黑龙江省": "黑龙江",
	"上海": "上海", "上海市": "上海",
	"江苏": "江苏", "江苏省": "江苏",
	"浙江": "浙江", "浙江省": "浙江",
	"安徽": "安徽", "安徽省": "安徽",
	"福建": "福建", "福建省": "福建",
	"江西": "江西", "江西省": "江西",
	"山东": "山东", "山东省": "山东",
	"河南": "河南", "河南省": "河南",
	"湖北": "湖北", "湖北省": "湖北",
	"湖南": "湖南", "湖南省": "湖南",
	"广东": "广东", "广东省": "广东",
	"广西": "广西", "广西壮族": "广西", "广西壮族自治区": "广西",
	"海南": "海南", "海南省": "海南",
	"重庆": "重庆", "重庆市": "重庆",
	"四川": "四川", "四川省": "四川",
	"贵州": "贵州", "贵州省": "贵州",
	"云南": "云南", "云南省": "云南",
	"西藏": "西藏", "西藏自治区": "西藏",
	"陕西": "陕西", "陕西省": "陕西",
	"甘肃": "甘肃", "甘肃省": "甘肃",
	"青海": "青海", "青海省": "青海",
	"宁夏": "宁夏", "宁夏回族自治区": "宁夏",
	"新疆": "新疆", "新疆维吾尔自治区": "新疆",
	"香港": "香港", "香港特别行政区": "香港", "中国香港": "香港",
	"澳门": "澳门", "澳门特别行政区": "澳门",
	"台湾": "台湾", "台湾省": "台湾",
}

var cityProvinceMap = map[string]string{
	"北京": "北京", "天津": "天津", "上海": "上海", "重庆": "重庆",
	"广州": "广东", "深圳": "广东", "佛山": "广东", "东莞": "广东", "珠海": "广东", "惠州": "广东", "中山": "广东", "汕头": "广东", "揭阳": "广东", "湛江": "广东", "江门": "广东", "肇庆": "广东", "茂名": "广东", "清远": "广东", "河源": "广东", "梅州": "广东", "汕尾": "广东", "潮州": "广东", "韶关": "广东", "阳江": "广东", "云浮": "广东",
	"杭州": "浙江", "宁波": "浙江", "温州": "浙江", "绍兴": "浙江", "金华": "浙江", "义乌": "浙江", "嘉兴": "浙江",
	"南京": "江苏", "苏州": "江苏", "无锡": "江苏", "常州": "江苏",
	"成都": "四川", "武汉": "湖北", "长沙": "湖南", "郑州": "河南", "西安": "陕西", "昆明": "云南",
	"厦门": "福建", "福州": "福建", "青岛": "山东", "济南": "山东", "合肥": "安徽", "南昌": "江西",
	"太原": "山西", "沈阳": "辽宁", "大连": "辽宁", "长春": "吉林", "哈尔滨": "黑龙江",
	"南宁": "广西", "海口": "海南", "贵阳": "贵州", "兰州": "甘肃", "银川": "宁夏", "西宁": "青海",
	"乌鲁木齐": "新疆", "拉萨": "西藏", "香港": "香港", "澳门": "澳门", "台北": "台湾",
	"吉隆坡": "马来西亚", "新加坡": "新加坡", "东京": "日本", "大阪": "日本", "曼谷": "泰国",
	"首尔": "韩国", "纽约": "美国", "洛杉矶": "美国", "旧金山": "美国", "伦敦": "英国",
}

var overseasRegionSeeds = map[string][]string{
	"马来西亚": {"吉隆坡", "槟城"},
	"新加坡":  {"新加坡"},
	"日本":   {"东京", "大阪"},
	"泰国":   {"曼谷"},
	"美国":   {"纽约", "旧金山", "洛杉矶"},
	"英国":   {"伦敦", "利兹"},
	"澳大利亚": {"堪培拉", "墨尔本"},
	"韩国":   {"首尔"},
	"奥地利":  {"维也纳"},
	"菲律宾":  {"马尼拉"},
	"柬埔寨":  {"柬埔寨"},
	"阿联酋":  {"迪拜"},
}

type sSysContent struct{}

type contentRegionAggRow struct {
	Province string `json:"province"`
	City     string `json:"city"`
	Count    int    `json:"count"`
}

type contentOptionAggRow struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type homeProfileFeed string

const (
	homeProfileFeedNearby homeProfileFeed = "nearby"
	homeProfileFeedLatest homeProfileFeed = "latest"
	homeProfileFeedHot    homeProfileFeed = "hot"
)

func NewSysContent() *sSysContent {
	return &sSysContent{}
}

func init() {
	service.RegisterSysContent(NewSysContent())
}

func aliasField(alias string, column string) string {
	return alias + "." + column
}

func aliasFields(alias string, columns ...string) string {
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, aliasField(alias, column))
	}
	return strings.Join(fields, ",")
}

func isPgsql() bool {
	return strings.EqualFold(g.DB().GetConfig().Type, consts.DBPgsql)
}

type contentFilterRange struct {
	Min int
	Max int
}

func splitFilterValues(value string) []string {
	parts := strings.Split(value, ",")
	list := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		list = append(list, part)
	}
	return list
}

func parseFilterRanges(value string) []contentFilterRange {
	values := splitFilterValues(value)
	ranges := make([]contentFilterRange, 0, len(values))
	for _, item := range values {
		pair := strings.SplitN(item, "-", 2)
		min := g.NewVar(pair[0]).Int()
		max := 0
		if len(pair) > 1 {
			max = g.NewVar(pair[1]).Int()
		}
		if min <= 0 && max <= 0 {
			continue
		}
		ranges = append(ranges, contentFilterRange{Min: min, Max: max})
	}
	return ranges
}

func applyRangeFilters(mod *gdb.Model, field string, ranges []contentFilterRange) *gdb.Model {
	conditions := make([]string, 0, len(ranges))
	args := make([]interface{}, 0, len(ranges)*2)
	for _, item := range ranges {
		if item.Min > 0 && item.Max > 0 {
			conditions = append(conditions, "("+field+" >= ? AND "+field+" <= ?)")
			args = append(args, item.Min, item.Max)
			continue
		}
		if item.Min > 0 {
			conditions = append(conditions, "("+field+" >= ?)")
			args = append(args, item.Min)
			continue
		}
		if item.Max > 0 {
			conditions = append(conditions, "("+field+" <= ?)")
			args = append(args, item.Max)
		}
	}
	if len(conditions) == 0 {
		return mod
	}
	return mod.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

func last24HoursCondition(field string) string {
	if isPgsql() {
		return field + " >= NOW() - INTERVAL '24 HOURS'"
	}
	return field + " >= DATE_SUB(NOW(), INTERVAL 24 HOUR)"
}

func coalesceZero(field string) string {
	if isPgsql() {
		return "COALESCE(" + field + ",0)"
	}
	return "IFNULL(" + field + ",0)"
}

func normalizeHomeProfileFeed(feed string, sort string) homeProfileFeed {
	switch strings.ToLower(strings.TrimSpace(feed)) {
	case string(homeProfileFeedNearby):
		return homeProfileFeedNearby
	case string(homeProfileFeedLatest), "fresh", "newest":
		return homeProfileFeedLatest
	case string(homeProfileFeedHot), "active":
		return homeProfileFeedHot
	}
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "hot":
		return homeProfileFeedHot
	case "latest", "fresh", "newest":
		return homeProfileFeedLatest
	default:
		return homeProfileFeedLatest
	}
}

func (s *sSysContent) HomeProfileCards(ctx context.Context, in *sysin.HomeProfileCardsInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	feed := normalizeHomeProfileFeed(in.Feed, in.Sort)
	if in.Page <= 1 && s.requestMemberId(ctx) <= 0 && in.Keyword == "" && in.Province == "" && in.City == "" {
		cacheKey := homeProfileCardsCacheKey(in)
		cacheVar, cacheErr := cache.Instance().Get(ctx, cacheKey)
		if cacheErr == nil && !cacheVar.IsNil() {
			if cacheErr = cacheVar.Scan(&list); cacheErr == nil && len(list) > 0 {
				return list, len(list), nil
			}
		}
		defer func() {
			if err == nil && len(list) > 0 {
				_ = cache.Instance().Set(ctx, cacheKey, list, 5*time.Minute)
			}
		}()
	}
	if feed == homeProfileFeedNearby && in.Page <= 1 && in.Keyword == "" && in.Province == "" && in.City == "" {
		list, totalCount, err = s.homeRecommendedProfiles(ctx, in)
		if err != nil {
			if !isMissingHomeRecommendColumnError(err) {
				return
			}
			err = nil
		}
		if len(list) > 0 {
			return
		}
	}
	listInp := in.ContentProfileListInp
	listInp.Feed = string(feed)
	listInp.Sort = listInp.Feed
	return s.ListProfiles(ctx, &listInp)
}

func homeProfileCardsCacheKey(in *sysin.HomeProfileCardsInp) string {
	pageSize := in.PerPage
	if pageSize <= 0 {
		pageSize = 3
	}
	if pageSize > 12 {
		pageSize = 12
	}
	feed := normalizeHomeProfileFeed(in.Feed, in.Sort)
	return fmt.Sprintf("%s:%d:%s", contentHomeCardsKeyBase, pageSize, feed)
}

func (s *sSysContent) homeRecommendedProfiles(ctx context.Context, in *sysin.HomeProfileCardsInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	pageSize := in.PerPage
	if pageSize <= 0 {
		pageSize = 3
	}
	if pageSize > 12 {
		pageSize = 12
	}

	profileColumns := dao.ContentProfile.Columns()
	rows := make([]contentProfileRow, 0, pageSize)
	err = s.publicProfileWhere(dao.ContentProfile.Ctx(ctx).As("p")).
		Fields(
			aliasFields("p",
				profileColumns.Id,
				profileColumns.ProfileNo,
				profileColumns.Title,
				profileColumns.Summary,
				profileColumns.Province,
				profileColumns.City,
				profileColumns.Age,
				profileColumns.Height,
				profileColumns.Weight,
				profileColumns.CupSize,
				profileColumns.HasVerificationVideo,
				profileColumns.MemberOnlyVideo,
				profileColumns.ImageCount,
				profileColumns.VideoCount,
				profileColumns.PublishedAt,
			)+", "+aliasField("p", contentProfileHomeRecommend)+", "+aliasField("p", contentProfileHomeSort),
		).
		Where(aliasField("p", contentProfileHomeRecommend), 1).
		OrderDesc(aliasField("p", contentProfileHomeSort)).
		OrderDesc(aliasField("p", profileColumns.SourceCreatedAt)).
		OrderDesc(aliasField("p", profileColumns.Id)).
		Limit(pageSize).
		Scan(&rows)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取首页推荐资料失败")
	}
	list, err = s.buildProfileListFromRows(ctx, rows)
	return list, len(list), err
}

func isMissingHomeRecommendColumnError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "does not exist") &&
		(strings.Contains(errMsg, contentProfileHomeRecommend) || strings.Contains(errMsg, contentProfileHomeSort))
}

func (s *sSysContent) ImageSearch(ctx context.Context, in *sysin.ContentProfileImageSearchInp, file *ghttp.UploadFile) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.ContentProfileImageSearchInp{}
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 12
	}
	if in.PerPage > 50 {
		in.PerPage = 50
	}
	threshold := in.Threshold
	if threshold <= 0 {
		threshold = 12
	}
	if threshold > 32 {
		threshold = 32
	}
	queryHash, err := imagePHashFromUpload(file)
	if err != nil {
		return nil, 0, err
	}
	profileIds, totalCount, err := s.findSimilarProfileIdsByPHash(ctx, queryHash, threshold, in.Page, in.PerPage)
	if err != nil {
		return
	}
	if len(profileIds) == 0 {
		return []*sysin.ContentProfileListModel{}, totalCount, nil
	}
	list, _, err = s.listProfilesByIds(ctx, profileIds)
	return
}

// ListProfiles 获取前台资料列表。
func (s *sSysContent) ListProfiles(ctx context.Context, in *sysin.ContentProfileListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	feed := normalizeHomeProfileFeed(in.Feed, in.Sort)
	queryInp := *in
	autoProvince := false
	if strings.EqualFold(strings.TrimSpace(in.Feed), string(homeProfileFeedNearby)) &&
		strings.TrimSpace(queryInp.Province) == "" &&
		strings.TrimSpace(queryInp.City) == "" &&
		strings.TrimSpace(queryInp.Keyword) == "" {
		if province := s.requestProvince(ctx); province != "" {
			queryInp.Province = province
			autoProvince = true
		}
	}

	list, totalCount, err = s.listProfilesByFilter(ctx, &queryInp, feed)
	if err != nil {
		return
	}
	if autoProvince && len(list) == 0 && queryInp.Page <= 1 && queryInp.City == "" && queryInp.Keyword == "" {
		fallbackInp := *in
		fallbackInp.Province = ""
		return s.listProfilesByFilter(ctx, &fallbackInp, feed)
	}
	return
}

func (s *sSysContent) listProfilesByFilter(ctx context.Context, in *sysin.ContentProfileListInp, feed homeProfileFeed) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	profileColumns := dao.ContentProfile.Columns()
	mod := dao.ContentProfile.Ctx(ctx).As("p")
	mod = s.publicProfileWhere(mod)
	if len(in.ExcludeActions) == 0 {
		in.ExcludeActions = []string{"block"}
	}
	if memberId := s.requestMemberId(ctx); memberId > 0 {
		mod = s.excludeMemberProfileActions(ctx, mod, memberId, in.ExcludeActions)
	}

	if in.Keyword != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.Where(
			"("+strings.Join([]string{
				aliasField("p", profileColumns.ProfileNo) + " LIKE ?",
				aliasField("p", profileColumns.Title) + " LIKE ?",
				aliasField("p", profileColumns.Summary) + " LIKE ?",
				aliasField("p", profileColumns.Province) + " LIKE ?",
				aliasField("p", profileColumns.City) + " LIKE ?",
				aliasField("p", profileColumns.CupSize) + " LIKE ?",
			}, " OR ")+")",
			keyword, keyword, keyword, keyword, keyword, keyword,
		)
	}
	if in.Province != "" {
		mod = mod.WhereIn(aliasField("p", profileColumns.Province), provinceFilterValues(in.Province))
	}
	if in.City != "" {
		mod = mod.WhereIn(aliasField("p", profileColumns.City), cityFilterValues(in.Province, in.City))
	}
	if in.AgeRanges != "" {
		mod = applyRangeFilters(mod, aliasField("p", profileColumns.Age), parseFilterRanges(in.AgeRanges))
	} else if in.AgeMin > 0 {
		mod = mod.WhereGTE(aliasField("p", profileColumns.Age), in.AgeMin)
		if in.AgeMax > 0 {
			mod = mod.WhereLTE(aliasField("p", profileColumns.Age), in.AgeMax)
		}
	}
	if in.HeightRanges != "" {
		mod = applyRangeFilters(mod, aliasField("p", profileColumns.Height), parseFilterRanges(in.HeightRanges))
	} else if in.HeightMin > 0 {
		mod = mod.WhereGTE(aliasField("p", profileColumns.Height), in.HeightMin)
		if in.HeightMax > 0 {
			mod = mod.WhereLTE(aliasField("p", profileColumns.Height), in.HeightMax)
		}
	}
	if in.WeightRanges != "" {
		mod = applyRangeFilters(mod, aliasField("p", profileColumns.Weight), parseFilterRanges(in.WeightRanges))
	} else if in.WeightMin > 0 {
		mod = mod.WhereGTE(aliasField("p", profileColumns.Weight), in.WeightMin)
		if in.WeightMax > 0 {
			mod = mod.WhereLTE(aliasField("p", profileColumns.Weight), in.WeightMax)
		}
	}
	if in.Cups != "" {
		cups := splitFilterValues(in.Cups)
		if len(cups) > 0 {
			mod = mod.WhereIn(aliasField("p", profileColumns.CupSize), cups)
		}
	} else if in.Cup != "" {
		mod = mod.Where(aliasField("p", profileColumns.CupSize), in.Cup)
	}
	if in.HasVideo == 1 {
		mod = mod.WhereGT(aliasField("p", profileColumns.VideoCount), 0)
	}
	if in.HasVerification == 1 {
		mod = mod.Where(aliasField("p", profileColumns.HasVerificationVideo), 1)
	}
	if in.CanFly == 1 {
		mod = mod.Where(aliasField("p", profileColumns.CanFlyToProvince), 1)
	}
	if in.CanGoAbroad == 1 {
		mod = mod.Where(aliasField("p", profileColumns.CanGoAbroad), 1)
	}
	if in.CanOvernight == 1 {
		mod = mod.Where(aliasField("p", profileColumns.CanOvernight), 1)
	}
	if in.CanCohabitate == 1 {
		mod = mod.Where(aliasField("p", profileColumns.CanCohabitate), 1)
	}
	if in.HasHealthCheck == 1 {
		mod = mod.Where(aliasField("p", profileColumns.HasHealthCheck), 1)
	}
	if in.IsFullMonth == 1 {
		mod = mod.Where(aliasField("p", profileColumns.IsFullMonth), 1)
	}
	if in.IsVirgin == 1 {
		mod = mod.Where(aliasField("p", profileColumns.IsVirgin), 1)
	}
	if in.AcceptSm == 1 {
		mod = mod.Where(aliasField("p", profileColumns.AcceptSm), 1)
	}
	if in.NoCondom == 1 {
		mod = mod.Where(aliasField("p", profileColumns.NoCondomAfterCheck), 1)
	}
	if in.AllowCreampie == 1 {
		mod = mod.Where(aliasField("p", profileColumns.AllowCreampie), 1)
	}
	if in.HasTattoo == 1 {
		mod = mod.Where(aliasField("p", profileColumns.HasTattoo), 1)
	}

	if in.WithTotal == 1 {
		totalCount, err = mod.Count()
		if err != nil {
			err = gerror.Wrap(err, "获取资料数据行失败")
			return
		}
		if totalCount == 0 {
			list = []*sysin.ContentProfileListModel{}
			return
		}
	}

	mod = mod.Fields(
		aliasFields("p",
			profileColumns.Id,
			profileColumns.ProfileNo,
			profileColumns.Title,
			profileColumns.Summary,
			profileColumns.Province,
			profileColumns.City,
			profileColumns.Age,
			profileColumns.Height,
			profileColumns.Weight,
			profileColumns.CupSize,
			profileColumns.HasVerificationVideo,
			profileColumns.MemberOnlyVideo,
			profileColumns.ImageCount,
			profileColumns.VideoCount,
			profileColumns.PublishedAt,
		),
	)
	mod = mod.Page(in.Page, in.PerPage)
	switch feed {
	case homeProfileFeedHot:
		mod = mod.
			LeftJoin(contentProfileStatsTable+" ps", "ps.profile_id="+aliasField("p", profileColumns.Id)).
			OrderDesc(coalesceZero("ps.hot_score")).
			OrderDesc(coalesceZero("ps.view_24h")).
			OrderDesc(aliasField("p", profileColumns.SourceCreatedAt)).
			OrderDesc(aliasField("p", profileColumns.Id))
	case homeProfileFeedLatest:
		mod = mod.OrderDesc(aliasField("p", profileColumns.SourceCreatedAt)).OrderDesc(aliasField("p", profileColumns.SourceNoteId)).OrderDesc(aliasField("p", profileColumns.Id))
	default:
		mod = mod.
			LeftJoin(contentProfileStatsTable+" ps", "ps.profile_id="+aliasField("p", profileColumns.Id)).
			OrderDesc(coalesceZero("ps.hot_score")).
			OrderDesc(coalesceZero("ps.view_24h")).
			OrderDesc(aliasField("p", profileColumns.SourceCreatedAt)).
			OrderDesc(aliasField("p", profileColumns.Id))
	}

	var rows []contentProfileRow
	if err = mod.Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取资料列表失败，请稍后重试")
		return
	}

	list, err = s.buildProfileListFromRows(ctx, rows)
	return
}

func (s *sSysContent) buildProfileListFromRows(ctx context.Context, rows []contentProfileRow) (list []*sysin.ContentProfileListModel, err error) {
	list = make([]*sysin.ContentProfileListModel, 0, len(rows))
	profileIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		profileIds = append(profileIds, row.Id)
	}
	coverMap, err := s.getProfileCoverMap(ctx, profileIds)
	if err != nil {
		return
	}
	mediaMap, err := s.getProfileMediaMap(ctx, profileIds, s.isRequestVip(ctx))
	if err != nil {
		return
	}
	for _, row := range rows {
		item := row.toListModel()
		item.CoverUrl = contentAssetURL(coverMap[row.Id])
		item.Avatar = item.CoverUrl
		item.Media = mediaMap[row.Id]
		item.Photos = mediaPhotos(item.Media)
		list = append(list, item)
	}
	return
}

func (s *sSysContent) listProfilesByIds(ctx context.Context, profileIds []int64) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	if len(profileIds) == 0 {
		return []*sysin.ContentProfileListModel{}, 0, nil
	}
	profileColumns := dao.ContentProfile.Columns()
	rows := make([]contentProfileRow, 0, len(profileIds))
	err = s.publicProfileWhere(dao.ContentProfile.Ctx(ctx).As("p")).
		Fields(
			aliasFields("p",
				profileColumns.Id,
				profileColumns.ProfileNo,
				profileColumns.Title,
				profileColumns.Summary,
				profileColumns.Province,
				profileColumns.City,
				profileColumns.Age,
				profileColumns.Height,
				profileColumns.Weight,
				profileColumns.CupSize,
				profileColumns.HasVerificationVideo,
				profileColumns.MemberOnlyVideo,
				profileColumns.ImageCount,
				profileColumns.VideoCount,
				profileColumns.PublishedAt,
			),
		).
		WhereIn(aliasField("p", profileColumns.Id), profileIds).
		Scan(&rows)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取相似资料失败，请稍后重试")
	}
	rowMap := make(map[int64]contentProfileRow, len(rows))
	for _, row := range rows {
		rowMap[row.Id] = row
	}
	coverMap, err := s.getProfileCoverMap(ctx, profileIds)
	if err != nil {
		return
	}
	mediaMap, err := s.getProfileMediaMap(ctx, profileIds, s.isRequestVip(ctx))
	if err != nil {
		return
	}
	list = make([]*sysin.ContentProfileListModel, 0, len(profileIds))
	for _, id := range profileIds {
		row, ok := rowMap[id]
		if !ok {
			continue
		}
		item := row.toListModel()
		item.CoverUrl = contentAssetURL(coverMap[row.Id])
		item.Avatar = item.CoverUrl
		item.Media = mediaMap[row.Id]
		item.Photos = mediaPhotos(item.Media)
		list = append(list, item)
	}
	return list, len(list), nil
}

// FilterOptions 获取前台资料筛选选项。
func (s *sSysContent) FilterOptions(ctx context.Context) (res *sysin.ContentProfileFilterOptionsModel, err error) {
	cacheVar, err := cache.Instance().Get(ctx, contentFilterOptionsKey)
	if err == nil && !cacheVar.IsNil() {
		if err = cacheVar.Scan(&res); err == nil && res != nil {
			return
		}
	}

	res = &sysin.ContentProfileFilterOptionsModel{
		Regions:    []*sysin.ContentProfileRegionOption{},
		Cups:       []*sysin.ContentProfileFilterOption{},
		Attributes: []*sysin.ContentProfileAttributeOption{},
	}
	res.Regions, err = s.buildProfileRegionOptions(ctx)
	if err != nil {
		return
	}
	res.Cups, err = s.buildProfileCupOptions(ctx)
	if err != nil {
		return
	}
	res.Attributes, err = s.buildProfileAttributeOptions(ctx)
	if err != nil {
		return
	}
	_ = cache.Instance().Set(ctx, contentFilterOptionsKey, res, 10*time.Minute)
	return
}

func (s *sSysContent) Regions(ctx context.Context) (res *sysin.ContentProfileRegionsModel, err error) {
	cacheVar, err := cache.Instance().Get(ctx, contentRegionsKey)
	if err == nil && !cacheVar.IsNil() {
		if err = cacheVar.Scan(&res); err == nil && res != nil {
			return
		}
	}

	regions, err := s.buildStandardRegionOptions(ctx)
	if err != nil {
		return
	}
	res = &sysin.ContentProfileRegionsModel{Regions: regions}
	_ = cache.Instance().Set(ctx, contentRegionsKey, res, 24*time.Hour)
	return
}

func (s *sSysContent) buildProfileRegionOptions(ctx context.Context) (list []*sysin.ContentProfileRegionOption, err error) {
	standardRegions, err := s.buildStandardRegionOptions(ctx)
	if err != nil {
		return
	}
	total, provinceCountMap, cityCountMap, err := s.buildProfileRegionCountMaps(ctx)
	if err != nil {
		return
	}
	return applyRegionCounts(standardRegions, total, provinceCountMap, cityCountMap, true), nil
}

func applyRegionCounts(regions []*sysin.ContentProfileRegionOption, total int, provinceCountMap map[string]int, cityCountMap map[string]map[string]int, onlyWithCount bool) (list []*sysin.ContentProfileRegionOption) {
	list = []*sysin.ContentProfileRegionOption{{
		Label: "全国",
		Value: "",
		Count: total,
	}}
	for _, region := range regions {
		if region.Value == "" {
			continue
		}
		item := cloneRegionOption(region)
		item.Count = provinceCountMap[item.Province]
		children := make([]*sysin.ContentProfileRegionOption, 0, len(item.Children))
		for _, child := range item.Children {
			child.Count = cityCountMap[item.Province][child.City]
			if child.Count > 0 {
				children = append(children, child)
			}
		}
		item.Children = children
		if onlyWithCount && item.Count == 0 && len(item.Children) == 0 {
			continue
		}
		list = append(list, item)
	}
	return
}

func (s *sSysContent) buildProfileRegionCountMaps(ctx context.Context) (total int, provinceCountMap map[string]int, cityCountMap map[string]map[string]int, err error) {
	profileColumns := dao.ContentProfile.Columns()
	var rows []*contentRegionAggRow
	if err = s.publicProfileWhere(dao.ContentProfile.Ctx(ctx).As("p")).
		Fields(aliasField("p", profileColumns.Province)+" AS province", aliasField("p", profileColumns.City)+" AS city", "COUNT(1) AS count").
		WhereNot(aliasField("p", profileColumns.Province), "").
		Group(aliasField("p", profileColumns.Province), aliasField("p", profileColumns.City)).
		OrderDesc("count").
		Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取地区筛选数据失败")
		return
	}

	provinceCountMap = make(map[string]int)
	cityCountMap = make(map[string]map[string]int)
	for _, row := range rows {
		province, city := normalizeProfileRegionForOption(row.Province, row.City)
		if province == "" {
			continue
		}
		total += row.Count
		provinceCountMap[province] += row.Count
		if city != "" && city != province {
			if _, ok := cityCountMap[province]; !ok {
				cityCountMap[province] = make(map[string]int)
			}
			cityCountMap[province][city] += row.Count
		}
	}
	return
}

func (s *sSysContent) buildStandardRegionOptions(ctx context.Context) (list []*sysin.ContentProfileRegionOption, err error) {
	cols := dao.SysProvinces.Columns()
	var rows []*entity.SysProvinces
	if err = dao.SysProvinces.Ctx(ctx).
		Fields(cols.Id, cols.Title, cols.Pid, cols.Level, cols.Sort).
		Where(cols.Status, consts.StatusEnabled).
		WhereIn(cols.Level, []int{1, 2}).
		OrderAsc(cols.Level).
		OrderAsc(cols.Sort).
		OrderAsc(cols.Id).
		Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取地区目录失败")
		return
	}

	provinceMap := make(map[int64]*sysin.ContentProfileRegionOption)
	order := make([]int64, 0)
	for _, row := range rows {
		title := normalizeProvinceAlias(cleanRegionToken(row.Title))
		if title == "" {
			title = cleanRegionToken(row.Title)
		}
		if title == "" {
			continue
		}
		if row.Level == 1 {
			if _, ok := provinceMap[row.Id]; ok {
				continue
			}
			provinceMap[row.Id] = &sysin.ContentProfileRegionOption{
				Label:    title,
				Value:    title,
				Province: title,
				Children: []*sysin.ContentProfileRegionOption{},
			}
			order = append(order, row.Id)
			continue
		}
		parent := provinceMap[row.Pid]
		if parent == nil {
			continue
		}
		city := normalizeCityForProvince(parent.Province, title)
		if city == "" || city == parent.Province {
			continue
		}
		parent.Children = append(parent.Children, &sysin.ContentProfileRegionOption{
			Label:    city,
			Value:    parent.Province + "/" + city,
			Province: parent.Province,
			City:     city,
		})
	}

	list = []*sysin.ContentProfileRegionOption{{Label: "全国", Value: ""}}
	exists := make(map[string]bool)
	for _, id := range order {
		item := provinceMap[id]
		if item == nil || exists[item.Province] {
			continue
		}
		exists[item.Province] = true
		item.Children = dedupeRegionChildren(item.Children)
		list = append(list, item)
	}
	for province, cities := range overseasRegionSeeds {
		if exists[province] {
			continue
		}
		item := &sysin.ContentProfileRegionOption{
			Label:    province,
			Value:    province,
			Province: province,
			Children: []*sysin.ContentProfileRegionOption{},
		}
		for _, city := range cities {
			if city == province {
				continue
			}
			item.Children = append(item.Children, &sysin.ContentProfileRegionOption{
				Label:    city,
				Value:    province + "/" + city,
				Province: province,
				City:     city,
			})
		}
		list = append(list, item)
	}
	return
}

func normalizeProfileRegionForOption(provinceValue string, cityValue string) (province string, city string) {
	province = cleanRegionToken(provinceValue)
	city = cleanRegionToken(cityValue)
	if normalized := normalizeProvinceAlias(province); normalized != "" {
		province = normalized
	} else if mapped := provinceByCity(province); mapped != "" {
		city = firstKnownCity(province, city)
		province = mapped
	}
	if province == "" {
		if mapped := provinceByCity(city); mapped != "" {
			province = mapped
		}
	}
	if province == "" && city != "" {
		province = city
		city = ""
	}
	if province != "" && city != "" {
		city = normalizeCityForProvince(province, city)
	}
	if city == province {
		city = ""
	}
	return
}

func cloneRegionOption(region *sysin.ContentProfileRegionOption) *sysin.ContentProfileRegionOption {
	item := &sysin.ContentProfileRegionOption{
		Label:    region.Label,
		Value:    region.Value,
		Province: region.Province,
		City:     region.City,
		Count:    region.Count,
		Children: make([]*sysin.ContentProfileRegionOption, 0, len(region.Children)),
	}
	for _, child := range region.Children {
		item.Children = append(item.Children, &sysin.ContentProfileRegionOption{
			Label:    child.Label,
			Value:    child.Value,
			Province: child.Province,
			City:     child.City,
			Count:    child.Count,
		})
	}
	return item
}

func dedupeRegionChildren(children []*sysin.ContentProfileRegionOption) []*sysin.ContentProfileRegionOption {
	seen := make(map[string]bool)
	list := make([]*sysin.ContentProfileRegionOption, 0, len(children))
	for _, child := range children {
		if child == nil || child.City == "" || seen[child.City] {
			continue
		}
		seen[child.City] = true
		list = append(list, child)
	}
	return list
}

func cleanRegionToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "，", ",")
	value = strings.ReplaceAll(value, "、", ",")
	value = strings.ReplaceAll(value, "／", "/")
	value = strings.TrimSpace(regionNoiseRegexp.ReplaceAllString(value, ""))
	for _, sep := range []string{"\n", "\t"} {
		if strings.Contains(value, sep) {
			value = strings.TrimSpace(strings.Split(value, sep)[0])
		}
	}
	return strings.TrimSpace(value)
}

func normalizeProvinceAlias(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if province, ok := provinceAliases[value]; ok {
		return province
	}
	for alias, province := range provinceAliases {
		if strings.HasPrefix(value, alias) {
			return province
		}
	}
	return ""
}

func provinceByCity(value string) string {
	value = cleanRegionToken(value)
	if value == "" {
		return ""
	}
	if province, ok := cityProvinceMap[value]; ok {
		return province
	}
	for city, province := range cityProvinceMap {
		if strings.HasPrefix(value, city) || strings.Contains(value, city) {
			return province
		}
	}
	return ""
}

func firstKnownCity(values ...string) string {
	for _, value := range values {
		value = cleanRegionToken(value)
		if value == "" {
			continue
		}
		for city := range cityProvinceMap {
			if value == city || strings.HasPrefix(value, city) || strings.Contains(value, city) {
				return city
			}
		}
	}
	return ""
}

func normalizeCityForProvince(province string, city string) string {
	city = strings.TrimSpace(city)
	if city == "" {
		return ""
	}
	if strings.HasPrefix(city, province) {
		city = strings.TrimSpace(strings.TrimPrefix(city, province))
		city = strings.Trim(city, "/,，、 -")
	}
	if mapped := provinceByCity(city); mapped == province {
		return firstKnownCity(city)
	}
	return city
}

func (s *sSysContent) buildProfileCupOptions(ctx context.Context) (list []*sysin.ContentProfileFilterOption, err error) {
	profileColumns := dao.ContentProfile.Columns()
	var rows []*contentOptionAggRow
	if err = s.publicProfileWhere(dao.ContentProfile.Ctx(ctx).As("p")).
		Fields(aliasField("p", profileColumns.CupSize)+" AS value", "COUNT(1) AS count").
		WhereNot(aliasField("p", profileColumns.CupSize), "").
		Group(aliasField("p", profileColumns.CupSize)).
		OrderAsc(aliasField("p", profileColumns.CupSize)).
		Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取标签筛选数据失败")
		return
	}
	list = []*sysin.ContentProfileFilterOption{{Label: "不限标签", Value: ""}}
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		if value == "" {
			continue
		}
		list = append(list, &sysin.ContentProfileFilterOption{
			Label: value,
			Value: value,
			Count: row.Count,
		})
	}
	return
}

func (s *sSysContent) buildProfileAttributeOptions(ctx context.Context) (list []*sysin.ContentProfileAttributeOption, err error) {
	specs := []struct {
		Key   string
		Label string
	}{
		{Key: "hasVideo", Label: "有视频"},
		{Key: "hasVerification", Label: "验证视频"},
		{Key: "canFly", Label: "可飞"},
		{Key: "canGoAbroad", Label: "出国"},
		{Key: "canOvernight", Label: "过夜"},
		{Key: "canCohabitate", Label: "同居"},
		{Key: "hasHealthCheck", Label: "体检"},
		{Key: "isFullMonth", Label: "满月"},
		{Key: "isVirgin", Label: "处"},
		{Key: "acceptSm", Label: "SM"},
		{Key: "noCondom", Label: "无套"},
		{Key: "allowCreampie", Label: "内射"},
		{Key: "hasTattoo", Label: "纹身"},
	}
	list = make([]*sysin.ContentProfileAttributeOption, 0, len(specs))
	for _, spec := range specs {
		list = append(list, &sysin.ContentProfileAttributeOption{
			Key: spec.Key,
			ContentProfileFilterOption: sysin.ContentProfileFilterOption{
				Label: spec.Label,
				Value: spec.Key,
				Count: 0,
			},
		})
	}
	return
}

func (s *sSysContent) requestProvince(ctx context.Context) string {
	// 优先使用网关/CDN 注入的地理信息，取不到时用客户端 IP 做本地库解析。
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return ""
	}
	for _, key := range []string{"X-Province", "X-Region", "X-Real-Province"} {
		if value := strings.TrimSpace(r.Header.Get(key)); value != "" {
			return normalizeProvinceName(value)
		}
	}
	clientIP := strings.TrimSpace(location.GetClientIp(r))
	if clientIP == "" {
		return ""
	}
	loc, err := location.GetLocation(ctx, clientIP)
	if err != nil || loc == nil {
		return ""
	}
	if loc.Province != "" {
		return normalizeProvinceName(loc.Province)
	}
	return normalizeProvinceName(loc.Region)
}

func normalizeProvinceName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "省")
	value = strings.TrimSuffix(value, "市")
	value = strings.TrimSuffix(value, "壮族自治区")
	value = strings.TrimSuffix(value, "回族自治区")
	value = strings.TrimSuffix(value, "维吾尔自治区")
	value = strings.TrimSuffix(value, "自治区")
	value = strings.TrimSuffix(value, "特别行政区")
	return value
}

func normalizeCityName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "市")
	value = strings.TrimSuffix(value, "地区")
	value = strings.TrimSuffix(value, "自治州")
	value = strings.TrimSuffix(value, "盟")
	return value
}

func provinceFilterValues(value string) []string {
	raw := strings.TrimSpace(value)
	normalized := normalizeProvinceName(raw)
	values := uniqueNonEmptyStrings(raw, normalized)
	switch normalized {
	case "广西":
		values = append(values, "广西壮族自治区")
	case "宁夏":
		values = append(values, "宁夏回族自治区")
	case "新疆":
		values = append(values, "新疆维吾尔自治区")
	case "内蒙古", "西藏":
		values = append(values, normalized+"自治区")
	case "香港", "澳门":
		values = append(values, normalized+"特别行政区", "中国"+normalized)
	default:
		if normalized != "" && normalized != raw {
			values = append(values, normalized+"省", normalized+"市")
		}
	}
	return uniqueNonEmptyStrings(values...)
}

func cityFilterValues(province string, city string) []string {
	raw := strings.TrimSpace(city)
	normalized := normalizeCityName(raw)
	provinceName := normalizeProvinceName(province)
	values := uniqueNonEmptyStrings(raw, normalized)
	if normalized != "" {
		values = append(values, normalized+"市", normalized+"地区", normalized+"自治州", normalized+"盟")
		if provinceName != "" {
			values = append(values,
				provinceName+normalized,
				provinceName+"省"+normalized,
				provinceName+normalized+"市",
				provinceName+"省"+normalized+"市",
			)
		}
	}
	return uniqueNonEmptyStrings(values...)
}

func uniqueNonEmptyStrings(values ...string) []string {
	seen := make(map[string]bool)
	list := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		list = append(list, value)
	}
	return list
}

func (s *sSysContent) requestMemberId(ctx context.Context) int64 {
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return 0
	}
	user, err := token.ParseLoginUser(r)
	if err != nil || user == nil || user.Id <= 0 {
		return 0
	}
	return user.Id
}

func (s *sSysContent) excludeMemberProfileActions(ctx context.Context, mod *gdb.Model, memberId int64, actionTypes []string) *gdb.Model {
	if memberId <= 0 || len(actionTypes) == 0 {
		return mod
	}
	actionColumns := dao.MemberProfileAction.Columns()
	return mod.WhereNotIn(
		aliasField("p", dao.ContentProfile.Columns().Id),
		dao.MemberProfileAction.Ctx(ctx).
			Fields(actionColumns.ProfileId).
			Where(actionColumns.MemberId, memberId).
			WhereIn(actionColumns.ActionType, actionTypes).
			WhereNull(actionColumns.DeletedAt),
	)
}

// ViewProfile 获取前台资料详情。
func (s *sSysContent) ViewProfile(ctx context.Context, in *sysin.ContentProfileViewInp) (res *sysin.ContentProfileViewModel, err error) {
	profileColumns := dao.ContentProfile.Columns()
	var row *contentProfileRow
	if err = s.publicProfileWhere(dao.ContentProfile.Ctx(ctx).As("p")).
		Fields(aliasField("p", "*")).
		Where(aliasField("p", profileColumns.Id), in.Id).
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "获取资料详情失败，请稍后重试")
		return
	}
	if row == nil {
		err = gerror.New("资料不存在或暂未公开")
		return
	}

	isVip := s.isRequestVip(ctx)
	res = &sysin.ContentProfileViewModel{
		ContentProfileListModel: *row.toListModel(),
		Intro:                   row.Summary,
		Attributes:              buildContentProfileAttributes(row, isVip),
		ImageCount:              row.ImageCount,
		VideoCount:              row.VideoCount,
		MemberOnly:              row.Visibility == consts.ContentVisibilityMemberOnly,
	}
	if isVip {
		res.PlainText = filterSensitivePlainText(row.PlainText)
	}
	res.Media, err = s.listProfileMedia(ctx, row.Id, isVip)
	if err != nil {
		return
	}
	res.Photos = make([]string, 0, len(res.Media))
	for _, item := range res.Media {
		if item.Type == consts.ContentMediaTypeImage && item.DisplayUrl != "" {
			res.Photos = append(res.Photos, item.DisplayUrl)
		}
	}
	if len(res.Photos) > 0 {
		res.CoverUrl = res.Photos[0]
		res.Avatar = res.Photos[0]
	}
	return
}

// ImportFeiNiu 从 FeiNiu_bot 增量导入资料。
func (s *sSysContent) ImportFeiNiu(ctx context.Context, in *sysin.ContentImportFeiNiuInp) (res *sysin.ContentImportFeiNiuModel, err error) {
	res = new(sysin.ContentImportFeiNiuModel)
	sourceGroup := g.Cfg().MustGet(ctx, "contentImport.feiniu.dbGroup", "feiniu").String()
	batchSize := in.BatchSize
	if batchSize <= 0 {
		batchSize = g.Cfg().MustGet(ctx, "contentImport.feiniu.batchSize", 200).Int()
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 200
	}
	triggerType := in.TriggerType
	if triggerType == "" {
		triggerType = "manual"
	}
	startedAt := gtime.Now()
	runId, err := s.createImportRun(ctx, contentSourceFeiNiu, triggerType, batchSize, startedAt)
	if err != nil {
		return
	}
	defer func() {
		status := "success"
		errorMessage := ""
		if err != nil {
			status = "failed"
			errorMessage = err.Error()
		}
		_ = s.finishImportRun(ctx, runId, status, errorMessage, startedAt, res)
	}()

	lastNoteId, err := s.getCheckpoint(ctx, contentSourceFeiNiu)
	if err != nil {
		return
	}

	sourceDB := g.DB(sourceGroup)
	reviewConfig, err := s.ImportReviewConfig(ctx, &sysin.ContentImportReviewConfigInp{SourceName: contentSourceFeiNiu})
	if err != nil {
		return
	}
	rows, err := sourceDB.GetAll(ctx, `
SELECT
  note_id,note_uuid,note_code,title,summary,plain_text,source_key,province,city,age,height,weight,cup_size,
  html_text,source_type,category_code,days_with_escort,expected_living_cost,can_fly_to_province,can_go_abroad,
  can_overnight,can_cohabitate,has_health_check,is_full_month,is_virgin,accept_sm,no_condom_after_check,
  allow_creampie,has_tattoo,has_verification_video,is_favorite,edited_at,group_params,tag_params,cover_asset_id,
  image_count,video_count,text_block_count,duplicate_note_id,storage_policy,ingest_status,remark,status,
  create_by,create_time,update_by,update_time
FROM tg_content_note
WHERE note_id > ? AND status = '0'
ORDER BY note_id ASC
LIMIT ?`, lastNoteId, batchSize)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 资料失败，请检查 contentImport.feiniu.dbGroup 配置")
		_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
		return
	}
	backfillMode := false
	if len(rows) == 0 {
		backfillWindow := g.Cfg().MustGet(ctx, "contentImport.feiniu.backfillWindow", 5000).Int64()
		if backfillWindow > 0 && lastNoteId > 0 {
			queryAfterNoteId := int64(0)
			if lastNoteId > backfillWindow {
				queryAfterNoteId = lastNoteId - backfillWindow
			}
			rows, err = sourceDB.GetAll(ctx, `
SELECT
  note_id,note_uuid,note_code,title,summary,plain_text,source_key,province,city,age,height,weight,cup_size,
  html_text,source_type,category_code,days_with_escort,expected_living_cost,can_fly_to_province,can_go_abroad,
  can_overnight,can_cohabitate,has_health_check,is_full_month,is_virgin,accept_sm,no_condom_after_check,
  allow_creampie,has_tattoo,has_verification_video,is_favorite,edited_at,group_params,tag_params,cover_asset_id,
  image_count,video_count,text_block_count,duplicate_note_id,storage_policy,ingest_status,remark,status,
  create_by,create_time,update_by,update_time
FROM tg_content_note
WHERE note_id > ? AND note_id <= ? AND status = '0'
ORDER BY update_time ASC,note_id ASC
LIMIT ?`, queryAfterNoteId, lastNoteId, batchSize)
			if err != nil {
				err = gerror.Wrap(err, "回扫 FeiNiu 资料失败，请检查 contentImport.feiniu.dbGroup 配置")
				_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
				return
			}
			backfillMode = len(rows) > 0
		}
	}

	for _, source := range rows {
		res.Scanned++
		sourceNoteId := source["note_id"].Int64()
		res.LastSourceNote = sourceNoteId
		imported, profileId, importErr := s.importFeiNiuProfile(ctx, sourceDB, source, reviewConfig)
		if importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		if imported {
			res.Imported++
		} else {
			res.Duplicate++
		}
		mediaCount, importErr := s.importFeiNiuMedia(ctx, sourceDB, profileId, sourceNoteId, source["image_count"].Int()+source["video_count"].Int())
		if importErr != nil {
			if errors.Is(importErr, errFeiNiuMediaPending) {
				_ = s.markFeiNiuProfileMediaPending(ctx, profileId)
				_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, importErr)
				continue
			}
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		if importErr = s.syncFeiNiuCoverMedia(ctx, profileId, source["cover_asset_id"].Int64()); importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		if importErr = s.freezeDuplicateProfileByImagePHash(ctx, profileId); importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		if importErr = s.markFeiNiuProfileMediaSynced(ctx, profileId); importErr != nil {
			err = importErr
			_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
			return
		}
		res.MediaImported += mediaCount
	}

	if !backfillMode && res.LastSourceNote > lastNoteId {
		if err = s.saveCheckpoint(ctx, contentSourceFeiNiu, res.LastSourceNote); err != nil {
			return
		}
	}
	repaired, err := s.repairFeiNiuMissingMedia(ctx, sourceDB, batchSize)
	if err != nil {
		_ = s.saveCheckpointError(ctx, contentSourceFeiNiu, err)
		return
	}
	if backfillMode || res.LastSourceNote <= lastNoteId {
		_ = s.clearCheckpointError(ctx, contentSourceFeiNiu)
	}
	res.MediaImported += repaired
	return
}

func (s *sSysContent) DedupeProfilesByImagePHash(ctx context.Context, in *sysin.ContentDedupePHashInp) (res *sysin.ContentDedupePHashModel, err error) {
	if in == nil {
		in = &sysin.ContentDedupePHashInp{}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	res = &sysin.ContentDedupePHashModel{}
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	query := dao.ContentProfile.Ctx(ctx).As("p").
		Fields(aliasField("p", profileColumns.Id)).
		LeftJoin(dao.ContentMedia.Table()+" m", aliasField("m", mediaColumns.ProfileId)+"="+aliasField("p", profileColumns.Id)).
		Where(aliasField("p", profileColumns.Status), consts.StatusEnabled).
		Where(aliasField("p", profileColumns.ReviewStatus), consts.ContentReviewApproved).
		WhereIn(aliasField("p", profileColumns.ImportStatus), []string{"imported", "duplicate"}).
		WhereIn(aliasField("p", profileColumns.Visibility), []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		WhereNull(aliasField("p", profileColumns.DeletedAt)).
		Where(aliasField("m", mediaColumns.MediaType), consts.ContentMediaTypeImage).
		Where(aliasField("m", mediaColumns.Status), consts.StatusEnabled).
		WhereNot(aliasField("m", mediaColumns.PerceptualHash), "")
	if in.StartId > 0 {
		query = query.Where(aliasField("p", profileColumns.Id)+">?", in.StartId)
	}
	rows, err := query.
		Group(aliasField("p", profileColumns.Id)).
		Having("COUNT(m.id)>0 AND COUNT(m.id)=COUNT(NULLIF(m.perceptual_hash,''))").
		OrderAsc(aliasField("p", profileColumns.Id)).
		Limit(limit).
		All()
	if err != nil {
		err = gerror.Wrap(err, "读取图片感知哈希待去重资料失败")
		return
	}
	for _, row := range rows {
		profileId := row[profileColumns.Id].Int64()
		if profileId <= 0 {
			continue
		}
		res.LastId = profileId
		res.Scanned++
		duplicateOfId, signature, findErr := s.findDuplicateProfileByImagePHash(ctx, profileId)
		if findErr != nil {
			err = findErr
			return
		}
		if duplicateOfId <= 0 {
			continue
		}
		if freezeErr := s.freezeDuplicateProfile(ctx, profileId, duplicateOfId, signature); freezeErr != nil {
			err = freezeErr
			return
		}
		res.Frozen++
	}
	return
}

// ImportOverview 获取内容导入概览。
func (s *sSysContent) ImportOverview(ctx context.Context, in *sysin.ContentImportOverviewInp) (res *sysin.ContentImportOverviewModel, err error) {
	sourceName := in.SourceName
	if sourceName == "" {
		sourceName = contentSourceFeiNiu
	}
	res = &sysin.ContentImportOverviewModel{SourceName: sourceName}
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	checkpointColumns := dao.ContentImportCheckpoint.Columns()
	runColumns := dao.ContentImportRun.Columns()

	res.ProfileTotal, err = dao.ContentProfile.Ctx(ctx).Where(profileColumns.SourceType, sourceName).Count()
	if err != nil {
		err = gerror.Wrap(err, "统计资料总数失败")
		return
	}
	res.PublicTotal, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.SourceType, sourceName).
		Where(profileColumns.ReviewStatus, consts.ContentReviewApproved).
		WhereIn(profileColumns.Visibility, []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计公开资料数失败")
		return
	}
	res.PendingTotal, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.SourceType, sourceName).
		Where(profileColumns.ReviewStatus, consts.ContentReviewPending).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计待审核资料数失败")
		return
	}
	res.DuplicateTotal, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.SourceType, sourceName).
		WhereGT(profileColumns.DuplicateOfId, 0).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计重复资料数失败")
		return
	}
	res.MediaTotal, err = dao.ContentMedia.Ctx(ctx).As("m").
		LeftJoin(dao.ContentProfile.Table()+" p", aliasField("p", profileColumns.Id)+"="+aliasField("m", mediaColumns.ProfileId)).
		Where(aliasField("p", profileColumns.SourceType), sourceName).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计媒体总数失败")
		return
	}
	res.DuplicateMedia, err = dao.ContentMedia.Ctx(ctx).As("m").
		LeftJoin(dao.ContentProfile.Table()+" p", aliasField("p", profileColumns.Id)+"="+aliasField("m", mediaColumns.ProfileId)).
		Where(aliasField("p", profileColumns.SourceType), sourceName).
		WhereGT(aliasField("m", mediaColumns.DuplicateOfMediaId), 0).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "统计重复媒体数失败")
		return
	}

	checkpoint, err := dao.ContentImportCheckpoint.Ctx(ctx).
		Fields(checkpointColumns.LastSourceNoteId, checkpointColumns.LastSuccessAt, checkpointColumns.LastError).
		Where(checkpointColumns.SourceName, sourceName).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取导入游标失败")
		return
	}
	if checkpoint != nil {
		res.LastSourceNoteId = checkpoint[checkpointColumns.LastSourceNoteId].Int64()
		res.LastSuccessAt = checkpoint[checkpointColumns.LastSuccessAt].GTime()
		res.LastError = checkpoint[checkpointColumns.LastError].String()
	}

	lastRun, err := dao.ContentImportRun.Ctx(ctx).
		Fields(runColumns.Status, runColumns.CostMs).
		Where(runColumns.SourceName, sourceName).
		OrderDesc(runColumns.Id).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取最近导入运行记录失败")
		return
	}
	if lastRun != nil {
		res.LastRunStatus = lastRun[runColumns.Status].String()
		res.LastRunCostMs = lastRun[runColumns.CostMs].Int()
	}
	if err = s.fillImportAutoSyncOverview(ctx, res); err != nil {
		return
	}
	return
}

// SetImportAutoSync 设置内容自动同步状态。
func (s *sSysContent) SetImportAutoSync(ctx context.Context, in *sysin.ContentImportAutoSyncInp) (res *sysin.ContentImportAutoSyncModel, err error) {
	sourceName := in.SourceName
	if sourceName == "" {
		sourceName = contentSourceFeiNiu
	}
	cronData, err := s.ensureContentImportCron(ctx)
	if err != nil {
		return
	}

	status := consts.StatusDisable
	if in.Enabled {
		status = consts.StatusEnabled
	}
	if err = service.SysCron().Status(ctx, &sysin.CronStatusInp{SysCron: entity.SysCron{Id: cronData.Id, Status: status}}); err != nil {
		err = gerror.Wrap(err, "更新内容自动同步状态失败")
		return
	}
	if in.Enabled {
		if err = service.SysCron().OnlineExec(ctx, &sysin.OnlineExecInp{SysCron: entity.SysCron{Id: cronData.Id, Name: cronData.Name, Params: cronData.Params}}); err != nil {
			err = gerror.Wrap(err, "启动内容自动同步失败")
			return
		}
	}

	cronData.Status = status
	return s.buildImportAutoSyncModel(sourceName, cronData), nil
}

func (s *sSysContent) ImportReviewConfig(ctx context.Context, in *sysin.ContentImportReviewConfigInp) (res *sysin.ContentImportReviewConfigModel, err error) {
	sourceName := in.SourceName
	if sourceName == "" {
		sourceName = contentSourceFeiNiu
	}
	res = defaultContentImportReviewConfig(sourceName)
	configColumns := dao.SysConfig.Columns()
	rows, err := dao.SysConfig.Ctx(ctx).
		Fields(configColumns.Key, configColumns.Value).
		Where(configColumns.Group, contentReviewConfigGroup+"_"+sourceName).
		All()
	if err != nil {
		err = gerror.Wrap(err, "读取内容审核配置失败")
		return
	}
	for _, row := range rows {
		switch row[configColumns.Key].String() {
		case "reviewBatchSize":
			res.ReviewBatchSize = row[configColumns.Value].Int()
		case "reviewIntervalMinutes":
			res.ReviewIntervalMinutes = row[configColumns.Value].Int()
		case "autoApproveImported":
			res.AutoApproveImported = row[configColumns.Value].Int()
		case "freezeDuplicate":
			res.FreezeDuplicate = row[configColumns.Value].Int()
		case "defaultReviewStatus":
			res.DefaultReviewStatus = row[configColumns.Value].String()
		case "reviewRemark":
			res.ReviewRemark = row[configColumns.Value].String()
		}
	}
	return
}

func (s *sSysContent) SetImportReviewConfig(ctx context.Context, in *sysin.ContentImportReviewConfigEditInp) (res *sysin.ContentImportReviewConfigModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	group := contentReviewConfigGroup + "_" + in.SourceName
	items := []struct {
		key   string
		name  string
		typ   string
		value interface{}
	}{
		{"reviewBatchSize", "审核数量", "int", in.ReviewBatchSize},
		{"reviewIntervalMinutes", "审核间隔分钟", "int", in.ReviewIntervalMinutes},
		{"autoApproveImported", "导入后自动通过", "int", in.AutoApproveImported},
		{"freezeDuplicate", "重复资料自动冻结", "int", in.FreezeDuplicate},
		{"defaultReviewStatus", "默认审核状态", "string", in.DefaultReviewStatus},
		{"reviewRemark", "审核备注", "string", in.ReviewRemark},
	}
	for index, item := range items {
		if err = s.saveContentImportReviewConfigItem(ctx, group, item.key, item.name, item.typ, g.NewVar(item.value).String(), index+1); err != nil {
			return
		}
	}
	return s.ImportReviewConfig(ctx, &sysin.ContentImportReviewConfigInp{SourceName: in.SourceName})
}

func defaultContentImportReviewConfig(sourceName string) *sysin.ContentImportReviewConfigModel {
	return &sysin.ContentImportReviewConfigModel{
		SourceName:            sourceName,
		ReviewBatchSize:       200,
		ReviewIntervalMinutes: 30,
		AutoApproveImported:   1,
		FreezeDuplicate:       0,
		DefaultReviewStatus:   consts.ContentReviewApproved,
	}
}

func (s *sSysContent) saveContentImportReviewConfigItem(ctx context.Context, group string, key string, name string, typ string, value string, sort int) (err error) {
	configColumns := dao.SysConfig.Columns()
	now := gtime.Now()
	one, err := dao.SysConfig.Ctx(ctx).
		Fields(configColumns.Id).
		Where(configColumns.Group, group).
		Where(configColumns.Key, key).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取内容审核配置项失败")
		return
	}
	data := g.Map{
		configColumns.Group:        group,
		configColumns.Name:         name,
		configColumns.Type:         typ,
		configColumns.Key:          key,
		configColumns.Value:        value,
		configColumns.DefaultValue: value,
		configColumns.Sort:         sort,
		configColumns.Tip:          "内容导入审核配置",
		configColumns.IsDefault:    1,
		configColumns.Status:       1,
		configColumns.UpdatedAt:    now,
	}
	if one != nil {
		_, err = dao.SysConfig.Ctx(ctx).Where(configColumns.Id, one[configColumns.Id].Int64()).Data(data).Update()
	} else {
		data[configColumns.CreatedAt] = now
		_, err = dao.SysConfig.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存内容审核配置失败")
	}
	return
}

// ImportRunList 获取内容导入运行记录。
func (s *sSysContent) ImportRunList(ctx context.Context, in *sysin.ContentImportRunListInp) (list []*sysin.ContentImportRunListModel, totalCount int, err error) {
	runColumns := dao.ContentImportRun.Columns()
	mod := dao.ContentImportRun.Ctx(ctx)
	if in.SourceName != "" {
		mod = mod.Where(runColumns.SourceName, in.SourceName)
	}
	if in.Status != "" {
		mod = mod.Where(runColumns.Status, in.Status)
	}
	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "统计导入运行记录失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.ContentImportRunListModel{}
		return
	}
	err = mod.Fields(
		runColumns.Id,
		runColumns.SourceName,
		runColumns.TriggerType,
		runColumns.BatchSize,
		runColumns.Scanned,
		runColumns.Imported,
		runColumns.Duplicate,
		runColumns.MediaImported,
		runColumns.LastSourceNoteId,
		runColumns.Status,
		runColumns.ErrorMessage,
		runColumns.StartedAt,
		runColumns.FinishedAt,
		runColumns.CostMs,
	).
		Page(in.Page, in.PerPage).
		OrderDesc(runColumns.Id).
		Scan(&list)
	if err != nil {
		err = gerror.Wrap(err, "获取导入运行记录失败")
	}
	return
}

func (s *sSysContent) publicProfileWhere(mod *gdb.Model) *gdb.Model {
	profileColumns := dao.ContentProfile.Columns()
	return mod.
		Where(aliasField("p", profileColumns.Status), 1).
		WhereIn(aliasField("p", profileColumns.ImportStatus), []string{"imported", "duplicate"}).
		Where(aliasField("p", profileColumns.ReviewStatus), consts.ContentReviewApproved).
		WhereIn(aliasField("p", profileColumns.Visibility), []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly})
}

func (s *sSysContent) ClearHomeProfileCardsCache(ctx context.Context) {
	keys := []interface{}{
		contentHomeCardsKeyBase + ":3:" + string(homeProfileFeedNearby),
		contentHomeCardsKeyBase + ":3:" + string(homeProfileFeedLatest),
		contentHomeCardsKeyBase + ":3:" + string(homeProfileFeedHot),
		contentHomeCardsKeyBase + ":9:" + string(homeProfileFeedNearby),
		contentHomeCardsKeyBase + ":9:" + string(homeProfileFeedLatest),
		contentHomeCardsKeyBase + ":9:" + string(homeProfileFeedHot),
		contentHomeCardsKeyBase + ":12:" + string(homeProfileFeedNearby),
		contentHomeCardsKeyBase + ":12:" + string(homeProfileFeedLatest),
		contentHomeCardsKeyBase + ":12:" + string(homeProfileFeedHot),
	}
	_, _ = cache.Instance().Remove(ctx, keys...)
}

func (s *sSysContent) fillImportAutoSyncOverview(ctx context.Context, res *sysin.ContentImportOverviewModel) (err error) {
	cronData, err := s.ensureContentImportCron(ctx)
	if err != nil {
		return
	}
	model := s.buildImportAutoSyncModel(res.SourceName, cronData)
	res.AutoSyncCronId = model.AutoSyncCronId
	res.AutoSyncEnabled = model.AutoSyncEnabled
	res.AutoSyncStatus = model.AutoSyncStatus
	res.AutoSyncPattern = model.AutoSyncPattern
	if res.LastRunStatus == "running" && res.AutoSyncEnabled {
		res.AutoSyncStatus = "running"
	}
	return
}

func (s *sSysContent) ensureContentImportCron(ctx context.Context) (data *entity.SysCron, err error) {
	data, err = s.getContentImportCron(ctx)
	if err != nil {
		return
	}
	cronColumns := dao.SysCron.Columns()
	now := gtime.Now()
	if data != nil {
		if data.Pattern != contentImportCronPattern || !strings.Contains(data.Remark, "每分钟") {
			remark := "每分钟从 FeiNiu_bot 增量同步最多 200 条资料"
			if _, err = dao.SysCron.Ctx(ctx).
				Where(cronColumns.Id, data.Id).
				Data(g.Map{cronColumns.Pattern: contentImportCronPattern, cronColumns.Remark: remark, cronColumns.UpdatedAt: now}).
				Update(); err != nil {
				return nil, gerror.Wrap(err, "更新内容自动同步任务配置失败")
			}
			data.Pattern = contentImportCronPattern
			data.Remark = remark
			data.UpdatedAt = now
		}
		return
	}
	id, err := dao.SysCron.Ctx(ctx).Data(g.Map{
		cronColumns.GroupId:   1,
		cronColumns.Title:     contentImportCronTitle,
		cronColumns.Name:      contentImportCronName,
		cronColumns.Params:    "",
		cronColumns.Pattern:   contentImportCronPattern,
		cronColumns.Policy:    consts.CronPolicySingle,
		cronColumns.Count:     0,
		cronColumns.Sort:      20,
		cronColumns.Remark:    "每分钟从 FeiNiu_bot 增量同步最多 200 条资料",
		cronColumns.Status:    consts.StatusDisable,
		cronColumns.CreatedAt: now,
		cronColumns.UpdatedAt: now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "初始化内容自动同步任务失败")
	}
	return &entity.SysCron{
		Id:        id,
		GroupId:   1,
		Title:     contentImportCronTitle,
		Name:      contentImportCronName,
		Params:    "",
		Pattern:   contentImportCronPattern,
		Policy:    consts.CronPolicySingle,
		Count:     0,
		Sort:      20,
		Remark:    "每分钟从 FeiNiu_bot 增量同步最多 200 条资料",
		Status:    consts.StatusDisable,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *sSysContent) getContentImportCron(ctx context.Context) (data *entity.SysCron, err error) {
	cronColumns := dao.SysCron.Columns()
	if err = dao.SysCron.Ctx(ctx).
		Where(cronColumns.Name, contentImportCronName).
		OrderDesc(cronColumns.Id).
		Scan(&data); err != nil {
		err = gerror.Wrap(err, "读取内容自动同步任务失败")
	}
	return
}

func (s *sSysContent) buildImportAutoSyncModel(sourceName string, cronData *entity.SysCron) *sysin.ContentImportAutoSyncModel {
	if sourceName == "" {
		sourceName = contentSourceFeiNiu
	}
	res := &sysin.ContentImportAutoSyncModel{
		SourceName:      sourceName,
		AutoSyncStatus:  "not_configured",
		AutoSyncPattern: contentImportCronPattern,
	}
	if cronData == nil {
		return res
	}
	res.AutoSyncCronId = cronData.Id
	res.AutoSyncEnabled = cronData.Status == consts.StatusEnabled
	res.AutoSyncPattern = cronData.Pattern
	if res.AutoSyncEnabled {
		res.AutoSyncStatus = "enabled"
	} else {
		res.AutoSyncStatus = "paused"
	}
	return res
}

func (s *sSysContent) isRequestVip(ctx context.Context) bool {
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return false
	}
	user, err := token.ParseLoginUser(r)
	if err != nil || user == nil || user.Id <= 0 {
		return false
	}
	if strings.EqualFold(user.RoleKey, consts.SuperRoleKey) {
		return true
	}
	ok, err := service.AdminMember().IsVip(ctx, user.Id)
	return err == nil && ok
}

func (s *sSysContent) getProfileCoverUrl(ctx context.Context, profileId int64) (url string, err error) {
	mediaColumns := dao.ContentMedia.Columns()
	one, err := dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.DisplayStoragePath).
		Where(mediaColumns.ProfileId, profileId).
		Where(mediaColumns.MediaType, consts.ContentMediaTypeImage).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.SortIndex).
		One()
	if err != nil || one == nil {
		return "", err
	}
	return contentAssetURL(one[mediaColumns.DisplayStoragePath].String()), nil
}

func (s *sSysContent) getProfileCoverMap(ctx context.Context, profileIds []int64) (res map[int64]string, err error) {
	res = make(map[int64]string, len(profileIds))
	if len(profileIds) == 0 {
		return
	}
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.ProfileId, mediaColumns.DisplayStoragePath).
		WhereIn(mediaColumns.ProfileId, profileIds).
		Where(mediaColumns.MediaType, consts.ContentMediaTypeImage).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.ProfileId).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		All()
	if err != nil {
		err = gerror.Wrap(err, "获取资料封面失败，请稍后重试")
		return
	}
	for _, row := range rows {
		profileId := row[mediaColumns.ProfileId].Int64()
		if _, exists := res[profileId]; exists {
			continue
		}
		res[profileId] = contentAssetURL(row[mediaColumns.DisplayStoragePath].String())
	}
	return
}

func (s *sSysContent) getProfileMediaMap(ctx context.Context, profileIds []int64, isMember bool) (res map[int64][]*sysin.ContentMediaModel, err error) {
	res = make(map[int64][]*sysin.ContentMediaModel, len(profileIds))
	if len(profileIds) == 0 {
		return
	}
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentMedia.Ctx(ctx).
		Fields(
			mediaColumns.Id,
			mediaColumns.ProfileId,
			mediaColumns.MediaType,
			mediaColumns.DisplayStoragePath,
			mediaColumns.PreviewStoragePath,
			mediaColumns.Width,
			mediaColumns.Height,
			mediaColumns.Duration,
			mediaColumns.ProcessStatus,
		).
		WhereIn(mediaColumns.ProfileId, profileIds).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.ProfileId).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		All()
	if err != nil {
		err = gerror.Wrap(err, "获取资料媒体失败，请稍后重试")
		return
	}
	imageSeen := make(map[int64]int, len(profileIds))
	for _, row := range rows {
		profileId := row[mediaColumns.ProfileId].Int64()
		mediaType := row[mediaColumns.MediaType].String()
		imageIndex := imageSeen[profileId]
		if mediaType == consts.ContentMediaTypeImage {
			imageSeen[profileId] = imageIndex + 1
		}
		res[profileId] = append(res[profileId], buildContentMediaModel(row, isMember, imageIndex))
	}
	return
}

func (s *sSysContent) listProfileMedia(ctx context.Context, profileId int64, isMember bool) (list []*sysin.ContentMediaModel, err error) {
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentMedia.Ctx(ctx).
		Fields(
			mediaColumns.Id,
			mediaColumns.MediaType,
			mediaColumns.DisplayStoragePath,
			mediaColumns.PreviewStoragePath,
			mediaColumns.Width,
			mediaColumns.Height,
			mediaColumns.Duration,
			mediaColumns.ProcessStatus,
		).
		Where(mediaColumns.ProfileId, profileId).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		All()
	if err != nil {
		err = gerror.Wrap(err, "获取资料媒体失败，请稍后重试")
		return
	}
	list = make([]*sysin.ContentMediaModel, 0, len(rows))
	imageIndex := 0
	for _, row := range rows {
		if row[mediaColumns.MediaType].String() == consts.ContentMediaTypeImage {
			list = append(list, buildContentMediaModel(row, isMember, imageIndex))
			imageIndex++
			continue
		}
		list = append(list, buildContentMediaModel(row, isMember, imageIndex))
	}
	return
}

func buildContentMediaModel(row gdb.Record, isMember bool, imageIndex int) *sysin.ContentMediaModel {
	mediaColumns := dao.ContentMedia.Columns()
	mediaType := row[mediaColumns.MediaType].String()
	locked := !isMember && (mediaType == consts.ContentMediaTypeVideo || (mediaType == consts.ContentMediaTypeImage && imageIndex > 0))
	displayUrl := contentAssetURL(row[mediaColumns.DisplayStoragePath].String())
	previewUrl := contentAssetURL(row[mediaColumns.PreviewStoragePath].String())
	if locked {
		displayUrl = ""
		previewUrl = ""
	}
	return &sysin.ContentMediaModel{
		Id:          row[mediaColumns.Id].Int64(),
		Type:        mediaType,
		DisplayUrl:  displayUrl,
		PreviewUrl:  previewUrl,
		Width:       row[mediaColumns.Width].Int(),
		Height:      row[mediaColumns.Height].Int(),
		Duration:    row[mediaColumns.Duration].Int(),
		Locked:      locked,
		Placeholder: locked,
		ProcessDone: row[mediaColumns.ProcessStatus].String() == "processed",
	}
}

func mediaPhotos(media []*sysin.ContentMediaModel) []string {
	photos := make([]string, 0, len(media))
	for _, item := range media {
		if item.Type == consts.ContentMediaTypeImage && item.DisplayUrl != "" {
			photos = append(photos, item.DisplayUrl)
		}
	}
	return photos
}

func (s *sSysContent) getCheckpoint(ctx context.Context, sourceName string) (lastNoteId int64, err error) {
	checkpointColumns := dao.ContentImportCheckpoint.Columns()
	one, err := dao.ContentImportCheckpoint.Ctx(ctx).
		Fields(checkpointColumns.LastSourceNoteId).
		Where(checkpointColumns.SourceName, sourceName).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取内容导入游标失败")
		return
	}
	if one == nil {
		return 0, nil
	}
	return one[checkpointColumns.LastSourceNoteId].Int64(), nil
}

func (s *sSysContent) saveCheckpoint(ctx context.Context, sourceName string, lastNoteId int64) (err error) {
	now := gtime.Now()
	checkpointColumns := dao.ContentImportCheckpoint.Columns()
	one, err := dao.ContentImportCheckpoint.Ctx(ctx).Fields(checkpointColumns.Id).Where(checkpointColumns.SourceName, sourceName).One()
	if err != nil {
		return gerror.Wrap(err, "读取内容导入游标失败")
	}
	data := g.Map{
		checkpointColumns.LastSourceNoteId: lastNoteId,
		checkpointColumns.LastSuccessAt:    now,
		checkpointColumns.LastError:        "",
		checkpointColumns.UpdatedAt:        now,
	}
	if one == nil {
		data[checkpointColumns.SourceName] = sourceName
		data[checkpointColumns.CreatedAt] = now
		_, err = dao.ContentImportCheckpoint.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.ContentImportCheckpoint.Ctx(ctx).Where(checkpointColumns.Id, one[checkpointColumns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存内容导入游标失败")
	}
	return
}

func (s *sSysContent) saveCheckpointError(ctx context.Context, sourceName string, sourceErr error) (err error) {
	checkpointColumns := dao.ContentImportCheckpoint.Columns()
	_, err = dao.ContentImportCheckpoint.Ctx(ctx).
		Where(checkpointColumns.SourceName, sourceName).
		Data(g.Map{checkpointColumns.LastError: sourceErr.Error(), checkpointColumns.UpdatedAt: gtime.Now()}).
		Update()
	return
}

func (s *sSysContent) clearCheckpointError(ctx context.Context, sourceName string) (err error) {
	checkpointColumns := dao.ContentImportCheckpoint.Columns()
	_, err = dao.ContentImportCheckpoint.Ctx(ctx).
		Where(checkpointColumns.SourceName, sourceName).
		Data(g.Map{checkpointColumns.LastError: "", checkpointColumns.UpdatedAt: gtime.Now()}).
		Update()
	return
}

func (s *sSysContent) createImportRun(ctx context.Context, sourceName string, triggerType string, batchSize int, startedAt *gtime.Time) (id int64, err error) {
	runColumns := dao.ContentImportRun.Columns()
	id, err = dao.ContentImportRun.Ctx(ctx).Data(g.Map{
		runColumns.SourceName:  sourceName,
		runColumns.TriggerType: triggerType,
		runColumns.BatchSize:   batchSize,
		runColumns.Status:      "running",
		runColumns.StartedAt:   startedAt,
		runColumns.CreatedAt:   startedAt,
		runColumns.UpdatedAt:   startedAt,
	}).InsertAndGetId()
	if err != nil {
		err = gerror.Wrap(err, "创建内容导入运行记录失败")
	}
	return
}

func (s *sSysContent) finishImportRun(ctx context.Context, runId int64, status string, errorMessage string, startedAt *gtime.Time, res *sysin.ContentImportFeiNiuModel) (err error) {
	if runId <= 0 {
		return nil
	}
	finishedAt := gtime.Now()
	costMs := int(finishedAt.Sub(startedAt).Milliseconds())
	runColumns := dao.ContentImportRun.Columns()
	data := g.Map{
		runColumns.Status:           status,
		runColumns.ErrorMessage:     errorMessage,
		runColumns.FinishedAt:       finishedAt,
		runColumns.CostMs:           costMs,
		runColumns.UpdatedAt:        finishedAt,
		runColumns.Scanned:          res.Scanned,
		runColumns.Imported:         res.Imported,
		runColumns.Duplicate:        res.Duplicate,
		runColumns.MediaImported:    res.MediaImported,
		runColumns.LastSourceNoteId: res.LastSourceNote,
	}
	_, err = dao.ContentImportRun.Ctx(ctx).Where(runColumns.Id, runId).Data(data).Update()
	if err != nil {
		err = gerror.Wrap(err, "更新内容导入运行记录失败")
	}
	return
}

func (s *sSysContent) importFeiNiuProfile(ctx context.Context, sourceDB gdb.DB, source gdb.Record, reviewConfig *sysin.ContentImportReviewConfigModel) (imported bool, profileId int64, err error) {
	profileColumns := dao.ContentProfile.Columns()
	profileNo := source["note_code"].String()
	if profileNo == "" {
		profileNo = "FN" + source["note_id"].String()
	}
	one, err := dao.ContentProfile.Ctx(ctx).
		Fields(profileColumns.Id).
		Where(profileColumns.SourceType, contentSourceFeiNiu).
		Where(profileColumns.SourceNoteId, source["note_id"].Int64()).
		One()
	if err != nil {
		err = gerror.Wrap(err, "检查资料是否存在失败")
		return
	}

	sourceInfo, err := s.getFeiNiuSourceInfo(ctx, sourceDB, source["note_id"].Int64())
	if err != nil {
		return
	}
	channelId, err := s.upsertFeiNiuChannel(ctx, sourceInfo)
	if err != nil {
		return
	}
	duplicateOfId, err := s.findDuplicateProfileId(ctx, source["note_id"].Int64(), source["duplicate_note_id"].Int64(), sourceInfo)
	if err != nil {
		return
	}
	reviewStatus := mapFeiNiuReviewStatus(source["ingest_status"].String(), reviewConfig)
	status := 1
	if duplicateOfId > 0 && reviewConfig != nil && reviewConfig.FreezeDuplicate == 1 {
		status = 2
	}
	province, city := normalizeProfileRegionForOption(source["province"].String(), source["city"].String())

	now := gtime.Now()
	data := g.Map{
		profileColumns.ProfileNo:            profileNo,
		profileColumns.SourceType:           contentSourceFeiNiu,
		profileColumns.SourceNoteId:         source["note_id"].Int64(),
		profileColumns.SourceNoteUuid:       source["note_uuid"].String(),
		profileColumns.SourceKey:            source["source_key"].String(),
		profileColumns.SourceTextHash:       recordString(sourceInfo, "source_text_hash"),
		profileColumns.ChannelId:            channelId,
		profileColumns.DuplicateOfId:        duplicateOfId,
		profileColumns.Title:                source["title"].String(),
		profileColumns.Summary:              source["summary"].String(),
		profileColumns.PlainText:            source["plain_text"].String(),
		profileColumns.HtmlText:             source["html_text"].String(),
		profileColumns.SourceCategoryCode:   source["category_code"].String(),
		profileColumns.DaysWithEscort:       source["days_with_escort"].Int(),
		profileColumns.ExpectedLivingCost:   source["expected_living_cost"].Int(),
		profileColumns.CanFlyToProvince:     flagToInt(source["can_fly_to_province"].String()),
		profileColumns.CanGoAbroad:          flagToInt(source["can_go_abroad"].String()),
		profileColumns.CanOvernight:         flagToInt(source["can_overnight"].String()),
		profileColumns.CanCohabitate:        flagToInt(source["can_cohabitate"].String()),
		profileColumns.HasHealthCheck:       flagToInt(source["has_health_check"].String()),
		profileColumns.IsFullMonth:          flagToInt(source["is_full_month"].String()),
		profileColumns.IsVirgin:             flagToInt(source["is_virgin"].String()),
		profileColumns.AcceptSm:             flagToInt(source["accept_sm"].String()),
		profileColumns.NoCondomAfterCheck:   flagToInt(source["no_condom_after_check"].String()),
		profileColumns.AllowCreampie:        flagToInt(source["allow_creampie"].String()),
		profileColumns.HasTattoo:            flagToInt(source["has_tattoo"].String()),
		profileColumns.IsFavorite:           flagToInt(source["is_favorite"].String()),
		profileColumns.SourceEditedAt:       source["edited_at"].GTime(),
		profileColumns.GroupParams:          source["group_params"].String(),
		profileColumns.TagParams:            source["tag_params"].String(),
		profileColumns.TextBlockCount:       source["text_block_count"].Int(),
		profileColumns.StoragePolicy:        source["storage_policy"].String(),
		profileColumns.SourceRemark:         source["remark"].String(),
		profileColumns.SourceCreateBy:       source["create_by"].String(),
		profileColumns.SourceUpdateBy:       source["update_by"].String(),
		profileColumns.SourceCreatedAt:      source["create_time"].GTime(),
		profileColumns.SourceUpdatedAt:      source["update_time"].GTime(),
		profileColumns.Province:             province,
		profileColumns.City:                 city,
		profileColumns.Age:                  source["age"].Int(),
		profileColumns.Height:               source["height"].Int(),
		profileColumns.Weight:               source["weight"].Int(),
		profileColumns.CupSize:              source["cup_size"].String(),
		profileColumns.HasVerificationVideo: flagToInt(source["has_verification_video"].String()),
		profileColumns.ImageCount:           source["image_count"].Int(),
		profileColumns.VideoCount:           source["video_count"].Int(),
		profileColumns.Visibility:           consts.ContentVisibilityPublic,
		profileColumns.ReviewStatus:         reviewStatus,
		profileColumns.ImportStatus:         "imported",
		profileColumns.Status:               status,
		profileColumns.UpdatedAt:            now,
	}
	if duplicateOfId > 0 {
		data[profileColumns.ImportStatus] = "duplicate"
	}
	if one == nil {
		data[profileColumns.CreatedAt] = now
		id, insertErr := dao.ContentProfile.Ctx(ctx).Data(data).InsertAndGetId()
		if insertErr != nil {
			err = gerror.Wrap(insertErr, "导入资料失败")
			return
		}
		if err = s.upsertFeiNiuSourceMap(ctx, id, sourceInfo); err != nil {
			return
		}
		return true, id, nil
	}
	profileId = one[profileColumns.Id].Int64()
	delete(data, profileColumns.ProfileNo)
	if _, err = dao.ContentProfile.Ctx(ctx).Where(profileColumns.Id, profileId).Data(data).Update(); err != nil {
		err = gerror.Wrap(err, "更新资料失败")
		return
	}
	if err = s.upsertFeiNiuSourceMap(ctx, profileId, sourceInfo); err != nil {
		return
	}
	return false, profileId, nil
}

func (s *sSysContent) importFeiNiuMedia(ctx context.Context, sourceDB gdb.DB, profileId int64, sourceNoteId int64, expectedMediaCount int) (count int, err error) {
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := sourceDB.GetAll(ctx, `
SELECT
  b.asset_id,b.sort_index,a.asset_type,a.binary_md5,a.perceptual_hash,a.width,a.height,a.duration,
  a.origin_uri,a.preview_uri,a.local_preview_path,c.cos_path,c.status AS cos_status
FROM tg_content_block b
JOIN tg_content_asset a ON a.asset_id = b.asset_id
LEFT JOIN tg_content_asset_cos c ON c.asset_id = a.asset_id
WHERE b.note_id = ? AND b.asset_id IS NOT NULL
ORDER BY b.sort_index ASC,b.block_id ASC`, sourceNoteId)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 媒体失败")
		return
	}
	if expectedMediaCount > 0 && len(rows) < expectedMediaCount {
		err = fmt.Errorf("%w: FeiNiu 媒体未就绪 note:%d expected:%d actual:%d", errFeiNiuMediaPending, sourceNoteId, expectedMediaCount, len(rows))
		return
	}

	for _, row := range rows {
		assetId := row["asset_id"].Int64()
		if assetId <= 0 {
			continue
		}
		exists, checkErr := dao.ContentMedia.Ctx(ctx).
			Fields(mediaColumns.Id).
			Where(mediaColumns.ProfileId, profileId).
			Where(mediaColumns.SourceAssetId, assetId).
			One()
		if checkErr != nil {
			err = gerror.Wrap(checkErr, "检查媒体是否存在失败")
			return
		}
		mediaType := row["asset_type"].String()
		if mediaType == "" {
			mediaType = consts.ContentMediaTypeImage
		}
		cosPath := normalizeFeiNiuCosPath(row["cos_path"].String())
		cosURL := feiNiuCosURL(cosPath)
		originPath := firstNonEmpty(cosURL, normalizeFeiNiuMediaURL(row["origin_uri"].String()), cosPath)
		previewPath := firstNonEmpty(normalizeFeiNiuMediaURL(row["preview_uri"].String()), normalizeFeiNiuMediaURL(row["local_preview_path"].String()))
		displayPath := firstNonEmpty(previewPath, originPath)
		if mediaType == consts.ContentMediaTypeVideo {
			displayPath = firstNonEmpty(cosURL, originPath)
			previewPath = firstNonEmpty(previewPath, feiNiuPosterURL(cosPath))
		}
		if !isFeiNiuMediaReady(mediaType, cosPath, displayPath) {
			err = fmt.Errorf("%w: FeiNiu 媒体 COS 未就绪 note:%d asset:%d", errFeiNiuMediaPending, sourceNoteId, assetId)
			return
		}
		duplicateMedia, checkDuplicateErr := s.getDuplicateMediaByMD5(ctx, mediaType, row["binary_md5"].String())
		if checkDuplicateErr != nil {
			err = checkDuplicateErr
			return
		}
		duplicateMediaId := int64(0)
		if duplicateMedia != nil {
			duplicateMediaId = duplicateMedia[mediaColumns.Id].Int64()
			displayPath = duplicateMedia[mediaColumns.DisplayStoragePath].String()
			previewPath = duplicateMedia[mediaColumns.PreviewStoragePath].String()
		}
		now := gtime.Now()
		data := g.Map{
			mediaColumns.ProfileId:           profileId,
			mediaColumns.SourceAssetId:       assetId,
			mediaColumns.MediaType:           mediaType,
			mediaColumns.SortIndex:           row["sort_index"].Int(),
			mediaColumns.OriginalStoragePath: originalStoragePath(cosPath, duplicateMediaId),
			mediaColumns.DisplayStoragePath:  displayPath,
			mediaColumns.PreviewStoragePath:  previewPath,
			mediaColumns.DuplicateOfMediaId:  duplicateMediaId,
			mediaColumns.BinaryMd5:           row["binary_md5"].String(),
			mediaColumns.PerceptualHash:      row["perceptual_hash"].String(),
			mediaColumns.Width:               row["width"].Int(),
			mediaColumns.Height:              row["height"].Int(),
			mediaColumns.Duration:            row["duration"].Int(),
			mediaColumns.ProcessStatus:       "raw",
			mediaColumns.EncryptStatus:       "none",
			mediaColumns.Status:              1,
			mediaColumns.UpdatedAt:           now,
		}
		if exists == nil {
			data[mediaColumns.CreatedAt] = now
			_, err = dao.ContentMedia.Ctx(ctx).Data(data).Insert()
			if err != nil {
				err = gerror.Wrap(err, "导入媒体失败")
				return
			}
			count++
			continue
		}
		_, err = dao.ContentMedia.Ctx(ctx).Where(mediaColumns.Id, exists[mediaColumns.Id].Int64()).Data(data).Update()
		if err != nil {
			err = gerror.Wrap(err, "更新媒体失败")
			return
		}
	}
	return
}

func (s *sSysContent) markFeiNiuProfileMediaPending(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	profileColumns := dao.ContentProfile.Columns()
	_, err := dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Data(g.Map{profileColumns.ImportStatus: "media_pending", profileColumns.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "标记资料媒体待同步失败")
	}
	return nil
}

func (s *sSysContent) markFeiNiuProfileMediaSynced(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	profileColumns := dao.ContentProfile.Columns()
	one, err := dao.ContentProfile.Ctx(ctx).
		Fields(profileColumns.DuplicateOfId).
		Where(profileColumns.Id, profileId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取资料重复状态失败")
	}
	if one == nil {
		return nil
	}
	importStatus := "imported"
	if one[profileColumns.DuplicateOfId].Int64() > 0 {
		importStatus = "duplicate"
	}
	_, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Data(g.Map{profileColumns.ImportStatus: importStatus, profileColumns.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "标记资料媒体已同步失败")
	}
	return nil
}

func (s *sSysContent) repairFeiNiuMissingMedia(ctx context.Context, sourceDB gdb.DB, limit int) (count int, err error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentProfile.Ctx(ctx).As("p").
		Fields(aliasField("p", profileColumns.Id), aliasField("p", profileColumns.SourceNoteId), aliasField("p", profileColumns.ImageCount), aliasField("p", profileColumns.VideoCount)).
		LeftJoin(dao.ContentMedia.Table()+" m", aliasField("m", mediaColumns.ProfileId)+"="+aliasField("p", profileColumns.Id)).
		Where(aliasField("p", profileColumns.SourceType), contentSourceFeiNiu).
		Where(aliasField("p", profileColumns.Status), 1).
		WhereGT(aliasField("p", profileColumns.ImageCount)+"+"+aliasField("p", profileColumns.VideoCount), 0).
		Group(aliasField("p", profileColumns.Id)).
		Having("COUNT(m.id)=0 OR SUM(CASE WHEN m.display_storage_path IS NULL OR m.display_storage_path='' OR m.display_storage_path LIKE '/prod-api/telegram/content-note/cos/%' OR m.display_storage_path LIKE 'telegram/content/%' OR m.preview_storage_path LIKE '/prod-api/telegram/content-note/cos/%' OR m.preview_storage_path LIKE 'telegram/content/%' THEN 1 ELSE 0 END)>0").
		OrderDesc(aliasField("p", profileColumns.SourceUpdatedAt)).
		OrderDesc(aliasField("p", profileColumns.Id)).
		Limit(limit).
		All()
	if err != nil {
		err = gerror.Wrap(err, "读取缺失媒体资料失败")
		return
	}
	for _, row := range rows {
		imported, importErr := s.importFeiNiuMedia(ctx, sourceDB, row[profileColumns.Id].Int64(), row[profileColumns.SourceNoteId].Int64(), row[profileColumns.ImageCount].Int()+row[profileColumns.VideoCount].Int())
		if importErr != nil {
			if errors.Is(importErr, errFeiNiuMediaPending) {
				_ = s.markFeiNiuProfileMediaPending(ctx, row[profileColumns.Id].Int64())
				continue
			}
			err = importErr
			return
		}
		if importErr = s.syncFeiNiuCoverMedia(ctx, row[profileColumns.Id].Int64(), 0); importErr != nil {
			err = importErr
			return
		}
		if importErr = s.freezeDuplicateProfileByImagePHash(ctx, row[profileColumns.Id].Int64()); importErr != nil {
			err = importErr
			return
		}
		if importErr = s.markFeiNiuProfileMediaSynced(ctx, row[profileColumns.Id].Int64()); importErr != nil {
			err = importErr
			return
		}
		count += imported
	}
	return
}

func (s *sSysContent) syncFeiNiuCoverMedia(ctx context.Context, profileId int64, coverAssetId int64) (err error) {
	if profileId <= 0 {
		return nil
	}
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	mod := dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.Id).
		Where(mediaColumns.ProfileId, profileId).
		Where(mediaColumns.MediaType, consts.ContentMediaTypeImage).
		Where(mediaColumns.Status, 1)
	if coverAssetId > 0 {
		mod = mod.Where(mediaColumns.SourceAssetId, coverAssetId)
	}
	one, err := mod.OrderAsc(mediaColumns.SortIndex).OrderAsc(mediaColumns.Id).One()
	if err != nil {
		return gerror.Wrap(err, "读取封面媒体失败")
	}
	if one == nil && coverAssetId > 0 {
		one, err = dao.ContentMedia.Ctx(ctx).
			Fields(mediaColumns.Id).
			Where(mediaColumns.ProfileId, profileId).
			Where(mediaColumns.MediaType, consts.ContentMediaTypeImage).
			Where(mediaColumns.Status, 1).
			OrderAsc(mediaColumns.SortIndex).
			OrderAsc(mediaColumns.Id).
			One()
		if err != nil {
			return gerror.Wrap(err, "读取默认封面媒体失败")
		}
	}
	if one == nil {
		return nil
	}
	_, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Data(g.Map{profileColumns.CoverMediaId: one[mediaColumns.Id].Int64(), profileColumns.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新封面媒体失败")
	}
	return nil
}

func mapFeiNiuReviewStatus(status string, config *sysin.ContentImportReviewConfigModel) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case consts.ContentReviewApproved, "approve", "passed", "pass", "success", "published":
		return consts.ContentReviewApproved
	case consts.ContentReviewRejected, "reject", "blocked", "failed":
		return consts.ContentReviewRejected
	case consts.ContentReviewPending, "reviewing":
		return consts.ContentReviewPending
	}
	if config != nil {
		if config.AutoApproveImported == 1 {
			return consts.ContentReviewApproved
		}
		if config.DefaultReviewStatus != "" {
			return config.DefaultReviewStatus
		}
	}
	return consts.ContentReviewApproved
}

func (s *sSysContent) getFeiNiuSourceInfo(ctx context.Context, sourceDB gdb.DB, sourceNoteId int64) (row gdb.Record, err error) {
	row, err = sourceDB.GetOne(ctx, `
SELECT
  s.source_id,s.note_id,s.source_type,s.source_key,s.channel_id,s.tg_chat_id,s.tg_message_id,s.tg_grouped_id,
  s.raw_text,s.source_text_hash,s.raw_message_json,
  c.title AS channel_title,c.username AS channel_username,c.invite_link AS channel_invite_link
FROM tg_content_source s
LEFT JOIN tg_channel c ON c.channel_id = s.channel_id
WHERE s.note_id = ?
ORDER BY
  CASE WHEN s.source_type = 'telegram_group' THEN 0 ELSE 1 END,
  s.source_id ASC
LIMIT 1`, sourceNoteId)
	if err != nil {
		err = gerror.Wrap(err, "读取 FeiNiu 来源映射失败")
		return
	}
	if row == nil {
		row = gdb.Record{}
	}
	return
}

func (s *sSysContent) upsertFeiNiuChannel(ctx context.Context, sourceInfo gdb.Record) (channelId int64, err error) {
	channelColumns := dao.ContentChannel.Columns()
	sourceChannelId := recordInt64(sourceInfo, "channel_id")
	if sourceChannelId <= 0 {
		return 0, nil
	}

	now := gtime.Now()
	one, err := dao.ContentChannel.Ctx(ctx).
		Fields(channelColumns.Id).
		Where(channelColumns.SourceType, contentSourceFeiNiu).
		Where(channelColumns.SourceChannelId, sourceChannelId).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取本地来源频道失败")
		return
	}

	data := g.Map{
		channelColumns.SourceChannelId: sourceChannelId,
		channelColumns.TgChatId:        recordString(sourceInfo, "tg_chat_id"),
		channelColumns.Title:           recordString(sourceInfo, "channel_title"),
		channelColumns.Username:        recordString(sourceInfo, "channel_username"),
		channelColumns.InviteLink:      recordString(sourceInfo, "channel_invite_link"),
		channelColumns.SourceType:      contentSourceFeiNiu,
		channelColumns.UpdatedAt:       now,
	}
	if one == nil {
		data[channelColumns.CreatedAt] = now
		data[channelColumns.PublicStatus] = "hidden"
		data[channelColumns.AuthStatus] = "none"
		data[channelColumns.Status] = 1
		channelId, err = dao.ContentChannel.Ctx(ctx).Data(data).InsertAndGetId()
	} else {
		channelId = one[channelColumns.Id].Int64()
		_, err = dao.ContentChannel.Ctx(ctx).Where(channelColumns.Id, channelId).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存本地来源频道失败")
	}
	return
}

func (s *sSysContent) upsertFeiNiuSourceMap(ctx context.Context, profileId int64, sourceInfo gdb.Record) (err error) {
	sourceMapColumns := dao.ContentSourceMap.Columns()
	sourceKey := recordString(sourceInfo, "source_key")
	if sourceKey == "" {
		return nil
	}

	now := gtime.Now()
	one, err := dao.ContentSourceMap.Ctx(ctx).Fields(sourceMapColumns.Id).Where(sourceMapColumns.SourceKey, sourceKey).One()
	if err != nil {
		return gerror.Wrap(err, "读取内容来源映射失败")
	}
	data := g.Map{
		sourceMapColumns.ProfileId:       profileId,
		sourceMapColumns.SourceType:      recordString(sourceInfo, "source_type"),
		sourceMapColumns.SourceKey:       sourceKey,
		sourceMapColumns.SourceChannelId: recordInt64(sourceInfo, "channel_id"),
		sourceMapColumns.SourceMessageId: recordInt64(sourceInfo, "tg_message_id"),
		sourceMapColumns.SourceGroupedId: recordInt64(sourceInfo, "tg_grouped_id"),
		sourceMapColumns.SourceTextHash:  recordString(sourceInfo, "source_text_hash"),
		sourceMapColumns.RawText:         recordString(sourceInfo, "raw_text"),
		sourceMapColumns.RawMessageJson:  recordString(sourceInfo, "raw_message_json"),
	}
	if data[sourceMapColumns.SourceType] == "" {
		data[sourceMapColumns.SourceType] = contentSourceFeiNiu
	}
	if one == nil {
		data[sourceMapColumns.CreatedAt] = now
		_, err = dao.ContentSourceMap.Ctx(ctx).Data(data).Insert()
	} else {
		_, err = dao.ContentSourceMap.Ctx(ctx).Where(sourceMapColumns.Id, one[sourceMapColumns.Id].Int64()).Data(data).Update()
	}
	if err != nil {
		err = gerror.Wrap(err, "保存内容来源映射失败")
	}
	return
}

func (s *sSysContent) freezeDuplicateProfileByImagePHash(ctx context.Context, profileId int64) (err error) {
	duplicateOfId, signature, err := s.findDuplicateProfileByImagePHash(ctx, profileId)
	if err != nil || duplicateOfId <= 0 {
		return err
	}
	return s.freezeDuplicateProfile(ctx, profileId, duplicateOfId, signature)
}

func (s *sSysContent) freezeDuplicateProfile(ctx context.Context, profileId int64, duplicateOfId int64, signature string) (err error) {
	profileColumns := dao.ContentProfile.Columns()
	_, err = dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Data(g.Map{
			profileColumns.DuplicateOfId: duplicateOfId,
			profileColumns.ImportStatus:  "duplicate",
			profileColumns.Status:        consts.StatusDisable,
			profileColumns.AdminRemark:   "图片感知哈希完全重复，自动停用",
			profileColumns.UpdatedAt:     gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "停用图片重复资料失败")
	}
	if signature != "" {
		_ = cache.Instance().Set(ctx, imagePHashSignatureCacheKey(signature), duplicateOfId, 24*time.Hour)
	}
	return nil
}

func (s *sSysContent) findDuplicateProfileByImagePHash(ctx context.Context, profileId int64) (duplicateOfId int64, signature string, err error) {
	signature, imageCount, err := s.profileImagePHashSignature(ctx, profileId)
	if err != nil || signature == "" || imageCount == 0 {
		return
	}
	cacheKey := imagePHashSignatureCacheKey(signature)
	cacheVar, cacheErr := cache.Instance().Get(ctx, cacheKey)
	if cacheErr == nil && !cacheVar.IsNil() {
		cachedId := cacheVar.Int64()
		if cachedId > 0 && cachedId != profileId {
			ok, verifyErr := s.isActiveProfileImagePHashSignature(ctx, cachedId, signature, imageCount)
			if verifyErr != nil {
				err = verifyErr
				return
			}
			if ok {
				return cachedId, signature, nil
			}
		}
	}

	mediaColumns := dao.ContentMedia.Columns()
	profileColumns := dao.ContentProfile.Columns()
	rows, queryErr := dao.ContentMedia.Ctx(ctx).As("m").
		Fields(aliasField("m", mediaColumns.ProfileId), aliasField("m", mediaColumns.PerceptualHash)).
		LeftJoin(dao.ContentProfile.Table()+" p", aliasField("p", profileColumns.Id)+"="+aliasField("m", mediaColumns.ProfileId)).
		Where(aliasField("m", mediaColumns.MediaType), consts.ContentMediaTypeImage).
		Where(aliasField("m", mediaColumns.Status), consts.StatusEnabled).
		WhereNot(aliasField("m", mediaColumns.PerceptualHash), "").
		Where(aliasField("p", profileColumns.Status), consts.StatusEnabled).
		Where(aliasField("p", profileColumns.ReviewStatus), consts.ContentReviewApproved).
		WhereIn(aliasField("p", profileColumns.ImportStatus), []string{"imported", "duplicate"}).
		WhereIn(aliasField("p", profileColumns.Visibility), []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		WhereNull(aliasField("p", profileColumns.DeletedAt)).
		Where(aliasField("m", mediaColumns.ProfileId)+"<>?", profileId).
		Where(aliasField("m", mediaColumns.ProfileId)+"<?", profileId).
		WhereIn(aliasField("m", mediaColumns.PerceptualHash), strings.Split(signature, "|")).
		OrderAsc(aliasField("m", mediaColumns.ProfileId)).
		All()
	if queryErr != nil {
		err = gerror.Wrap(queryErr, "查询图片感知哈希重复资料失败")
		return
	}

	hashesByProfile := make(map[int64][]string)
	for _, row := range rows {
		id := row[mediaColumns.ProfileId].Int64()
		hash := strings.TrimSpace(row[mediaColumns.PerceptualHash].String())
		if id <= 0 || hash == "" {
			continue
		}
		hashesByProfile[id] = append(hashesByProfile[id], hash)
	}
	candidateIds := make([]int64, 0, len(hashesByProfile))
	for candidateId := range hashesByProfile {
		candidateIds = append(candidateIds, candidateId)
	}
	sort.Slice(candidateIds, func(i, j int) bool {
		return candidateIds[i] < candidateIds[j]
	})
	for _, candidateId := range candidateIds {
		hashes := hashesByProfile[candidateId]
		sort.Strings(hashes)
		if strings.Join(hashes, "|") != signature {
			continue
		}
		ok, verifyErr := s.isActiveProfileImagePHashSignature(ctx, candidateId, signature, imageCount)
		if verifyErr != nil {
			err = verifyErr
			return
		}
		if ok {
			duplicateOfId = candidateId
			_ = cache.Instance().Set(ctx, cacheKey, duplicateOfId, 24*time.Hour)
			return
		}
	}
	_ = cache.Instance().Set(ctx, cacheKey, profileId, 24*time.Hour)
	return
}

func (s *sSysContent) isActiveProfileImagePHashSignature(ctx context.Context, profileId int64, signature string, imageCount int) (ok bool, err error) {
	profileColumns := dao.ContentProfile.Columns()
	count, err := dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Where(profileColumns.Status, consts.StatusEnabled).
		Where(profileColumns.ReviewStatus, consts.ContentReviewApproved).
		WhereIn(profileColumns.ImportStatus, []string{"imported", "duplicate"}).
		WhereIn(profileColumns.Visibility, []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		WhereNull(profileColumns.DeletedAt).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "校验图片重复资料状态失败")
		return
	}
	if count == 0 {
		return false, nil
	}
	currentSignature, currentImageCount, err := s.profileImagePHashSignature(ctx, profileId)
	if err != nil {
		return
	}
	ok = currentImageCount == imageCount && currentSignature == signature
	return
}

func (s *sSysContent) profileImagePHashSignature(ctx context.Context, profileId int64) (signature string, imageCount int, err error) {
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.PerceptualHash).
		Where(mediaColumns.ProfileId, profileId).
		Where(mediaColumns.MediaType, consts.ContentMediaTypeImage).
		Where(mediaColumns.Status, consts.StatusEnabled).
		OrderAsc(mediaColumns.SortIndex).
		OrderAsc(mediaColumns.Id).
		Array()
	if err != nil {
		err = gerror.Wrap(err, "读取资料图片感知哈希失败")
		return
	}
	imageCount = len(rows)
	if imageCount == 0 {
		return
	}
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		hash := strings.TrimSpace(row.String())
		if hash == "" {
			return "", imageCount, nil
		}
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	signature = strings.Join(hashes, "|")
	return
}

type profilePHashDistance struct {
	ProfileId int64
	Distance  int
}

func (s *sSysContent) findSimilarProfileIdsByPHash(ctx context.Context, queryHash *goimagehash.ImageHash, threshold int, page int, perPage int) (profileIds []int64, totalCount int, err error) {
	profileColumns := dao.ContentProfile.Columns()
	mediaColumns := dao.ContentMedia.Columns()
	rows, err := dao.ContentMedia.Ctx(ctx).As("m").
		Fields(aliasField("m", mediaColumns.ProfileId), aliasField("m", mediaColumns.PerceptualHash)).
		LeftJoin(dao.ContentProfile.Table()+" p", aliasField("p", profileColumns.Id)+"="+aliasField("m", mediaColumns.ProfileId)).
		Where(aliasField("m", mediaColumns.MediaType), consts.ContentMediaTypeImage).
		Where(aliasField("m", mediaColumns.Status), consts.StatusEnabled).
		WhereNot(aliasField("m", mediaColumns.PerceptualHash), "").
		Where(aliasField("p", profileColumns.Status), consts.StatusEnabled).
		Where(aliasField("p", profileColumns.ReviewStatus), consts.ContentReviewApproved).
		WhereIn(aliasField("p", profileColumns.ImportStatus), []string{"imported", "duplicate"}).
		WhereIn(aliasField("p", profileColumns.Visibility), []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly}).
		WhereNull(aliasField("p", profileColumns.DeletedAt)).
		All()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "查询图片相似资料失败")
	}
	distanceByProfile := make(map[int64]int)
	for _, row := range rows {
		profileId := row[mediaColumns.ProfileId].Int64()
		if profileId <= 0 {
			continue
		}
		hash, ok := parseStoredPHash(row[mediaColumns.PerceptualHash].String())
		if !ok {
			continue
		}
		distance, distanceErr := queryHash.Distance(hash)
		if distanceErr != nil || distance > threshold {
			continue
		}
		current, exists := distanceByProfile[profileId]
		if !exists || distance < current {
			distanceByProfile[profileId] = distance
		}
	}
	items := make([]profilePHashDistance, 0, len(distanceByProfile))
	for profileId, distance := range distanceByProfile {
		items = append(items, profilePHashDistance{ProfileId: profileId, Distance: distance})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].ProfileId > items[j].ProfileId
		}
		return items[i].Distance < items[j].Distance
	})
	totalCount = len(items)
	if totalCount == 0 {
		return []int64{}, 0, nil
	}
	start := (page - 1) * perPage
	if start < 0 {
		start = 0
	}
	if start >= totalCount {
		return []int64{}, totalCount, nil
	}
	end := int(math.Min(float64(start+perPage), float64(totalCount)))
	profileIds = make([]int64, 0, end-start)
	for _, item := range items[start:end] {
		profileIds = append(profileIds, item.ProfileId)
	}
	return
}

func imagePHashFromUpload(file *ghttp.UploadFile) (*goimagehash.ImageHash, error) {
	if file == nil {
		return nil, gerror.New("请先上传要搜索的图片")
	}
	reader, err := file.Open()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上传图片失败")
	}
	defer reader.Close()
	img, _, err := image.Decode(reader)
	if err != nil {
		return nil, gerror.New("图片格式不支持，请上传 JPG、PNG 或 GIF")
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return nil, gerror.Wrap(err, "计算图片感知哈希失败")
	}
	return hash, nil
}

func parseStoredPHash(value string) (*goimagehash.ImageHash, bool) {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return nil, false
	}
	if strings.Contains(normalized, ":") {
		parts := strings.Split(normalized, ":")
		normalized = parts[len(parts)-1]
	}
	normalized = strings.TrimPrefix(normalized, "0x")
	base := 16
	if len(normalized) != 16 {
		if _, err := strconv.ParseUint(normalized, 10, 64); err == nil {
			base = 10
		} else {
			return nil, false
		}
	}
	hashValue, err := strconv.ParseUint(normalized, base, 64)
	if err != nil {
		return nil, false
	}
	return goimagehash.NewImageHash(hashValue, goimagehash.PHash), true
}

func imagePHashSignatureCacheKey(signature string) string {
	sum := sha1.Sum([]byte(signature))
	return contentImagePHashSigKey + hex.EncodeToString(sum[:])
}

func (s *sSysContent) findDuplicateProfileId(ctx context.Context, sourceNoteId int64, duplicateNoteId int64, sourceInfo gdb.Record) (id int64, err error) {
	profileColumns := dao.ContentProfile.Columns()
	sourceMapColumns := dao.ContentSourceMap.Columns()
	if duplicateNoteId > 0 {
		one, queryErr := dao.ContentProfile.Ctx(ctx).
			Fields(profileColumns.Id).
			Where(profileColumns.SourceType, contentSourceFeiNiu).
			Where(profileColumns.SourceNoteId, duplicateNoteId).
			One()
		if queryErr != nil {
			err = gerror.Wrap(queryErr, "检查重复资料失败")
			return
		}
		if one != nil {
			return one[profileColumns.Id].Int64(), nil
		}
	}

	sourceTextHash := recordString(sourceInfo, "source_text_hash")
	sourceChannelId := recordInt64(sourceInfo, "channel_id")
	if sourceTextHash == "" || sourceChannelId <= 0 {
		return 0, nil
	}
	one, err := dao.ContentProfile.Ctx(ctx).As("p").
		Fields(aliasField("p", profileColumns.Id)).
		LeftJoin(dao.ContentSourceMap.Table()+" s", aliasField("s", sourceMapColumns.ProfileId)+"="+aliasField("p", profileColumns.Id)).
		Where(aliasField("p", profileColumns.SourceType), contentSourceFeiNiu).
		Where(aliasField("p", profileColumns.SourceNoteId)+"<>?", sourceNoteId).
		Where(aliasField("s", sourceMapColumns.SourceChannelId), sourceChannelId).
		Where(aliasField("s", sourceMapColumns.SourceTextHash), sourceTextHash).
		OrderAsc(aliasField("p", profileColumns.Id)).
		One()
	if err != nil {
		err = gerror.Wrap(err, "检查文本重复资料失败")
		return
	}
	if one == nil {
		return 0, nil
	}
	return one[profileColumns.Id].Int64(), nil
}

func (s *sSysContent) getDuplicateMediaByMD5(ctx context.Context, mediaType string, md5 string) (row gdb.Record, err error) {
	if mediaType != consts.ContentMediaTypeImage || md5 == "" {
		return nil, nil
	}
	mediaColumns := dao.ContentMedia.Columns()
	row, err = dao.ContentMedia.Ctx(ctx).
		Fields(mediaColumns.Id, mediaColumns.DisplayStoragePath, mediaColumns.PreviewStoragePath).
		Where(mediaColumns.MediaType, mediaType).
		Where(mediaColumns.BinaryMd5, md5).
		Where(mediaColumns.Status, 1).
		OrderAsc(mediaColumns.Id).
		One()
	if err != nil {
		err = gerror.Wrap(err, "检查重复媒体失败")
	}
	return
}

func originalStoragePath(cosPath string, duplicateMediaId int64) string {
	if duplicateMediaId > 0 {
		return ""
	}
	return cosPath
}

func normalizeFeiNiuCosPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, feiNiuCosURLPrefix)
	value = strings.TrimPrefix(value, feiNiuProxyCosPrefix)
	return strings.TrimLeft(value, "/")
}

func normalizeFeiNiuMediaURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, feiNiuProxyCosPrefix) {
		return feiNiuCosURL(normalizeFeiNiuCosPath(value))
	}
	if strings.HasPrefix(value, "telegram/content/") {
		return feiNiuCosURL(value)
	}
	return value
}

func feiNiuCosURL(path string) string {
	path = normalizeFeiNiuCosPath(path)
	if path == "" {
		return ""
	}
	return feiNiuCosURLPrefix + path
}

func contentAssetURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	cdnBase := strings.TrimRight(g.Cfg().MustGet(context.Background(), "content.cdnBaseUrl", "").String(), "/")
	if cdnBase == "" {
		return value
	}
	if strings.HasPrefix(value, feiNiuCosURLPrefix) {
		return cdnBase + "/" + normalizeFeiNiuCosPath(value)
	}
	if strings.HasPrefix(value, feiNiuProxyCosPrefix) || strings.HasPrefix(value, "telegram/content/") {
		return cdnBase + "/" + normalizeFeiNiuCosPath(value)
	}
	if cosPath := normalizeContentCosURLPath(value); cosPath != "" {
		return cdnBase + "/" + cosPath
	}
	return value
}

func normalizeContentCosURLPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return ""
	}
	host := strings.ToLower(parsed.Host)
	if !strings.Contains(host, ".cos.") || !strings.HasSuffix(host, ".myqcloud.com") {
		return ""
	}
	path := strings.TrimLeft(parsed.EscapedPath(), "/")
	path, err = url.PathUnescape(path)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(path, "telegram/content/") {
		return ""
	}
	return normalizeFeiNiuCosPath(path)
}

func feiNiuPosterURL(cosPath string) string {
	if cosPath == "" {
		return ""
	}
	return feiNiuCosURL(cosPath + ".poster.jpg")
}

func isFeiNiuMediaReady(mediaType string, cosPath string, displayPath string) bool {
	if strings.TrimSpace(displayPath) == "" {
		return false
	}
	if mediaType == consts.ContentMediaTypeVideo {
		return strings.TrimSpace(cosPath) != ""
	}
	return strings.TrimSpace(cosPath) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func boolText(value int) string {
	if value == 1 {
		return "是"
	}
	return "否"
}

func appendAttribute(list []*sysin.ContentProfileAttributeItem, label string, value interface{}, suffix string) []*sysin.ContentProfileAttributeItem {
	text := strings.TrimSpace(g.NewVar(value).String())
	if text == "" || text == "0" || text == "<nil>" {
		return list
	}
	return append(list, &sysin.ContentProfileAttributeItem{
		Label: label,
		Value: text + suffix,
	})
}

func buildContentProfileAttributes(row *contentProfileRow, isVip bool) []*sysin.ContentProfileAttributeItem {
	if row == nil {
		return []*sysin.ContentProfileAttributeItem{}
	}
	list := make([]*sysin.ContentProfileAttributeItem, 0, 18)
	list = appendAttribute(list, "年龄", row.Age, "岁")
	list = appendAttribute(list, "身高", row.Height, "cm")
	list = appendAttribute(list, "体重", row.Weight, "kg")
	list = appendAttribute(list, "罩杯", row.CupSize, "")
	list = appendAttribute(list, "城市", strings.TrimSpace(row.Province+" "+row.City), "")
	list = appendAttribute(list, "期望生活费", row.ExpectedLivingCost, "")
	if !isVip {
		return list
	}
	list = appendAttribute(list, "陪伴天数", row.DaysWithEscort, "天")
	list = append(list,
		&sysin.ContentProfileAttributeItem{Label: "可飞外省", Value: boolText(row.CanFlyToProvince)},
		&sysin.ContentProfileAttributeItem{Label: "可出国", Value: boolText(row.CanGoAbroad)},
		&sysin.ContentProfileAttributeItem{Label: "可过夜", Value: boolText(row.CanOvernight)},
		&sysin.ContentProfileAttributeItem{Label: "可同居", Value: boolText(row.CanCohabitate)},
		&sysin.ContentProfileAttributeItem{Label: "有体检", Value: boolText(row.HasHealthCheck)},
		&sysin.ContentProfileAttributeItem{Label: "满月", Value: boolText(row.IsFullMonth)},
		&sysin.ContentProfileAttributeItem{Label: "是否处", Value: boolText(row.IsVirgin)},
		&sysin.ContentProfileAttributeItem{Label: "接受SM", Value: boolText(row.AcceptSm)},
		&sysin.ContentProfileAttributeItem{Label: "体检后无套", Value: boolText(row.NoCondomAfterCheck)},
		&sysin.ContentProfileAttributeItem{Label: "可内射", Value: boolText(row.AllowCreampie)},
		&sysin.ContentProfileAttributeItem{Label: "有纹身", Value: boolText(row.HasTattoo)},
	)
	list = appendAttribute(list, "备注", row.SourceRemark, "")
	return list
}

func filterSensitivePlainText(text string) string {
	text = introFeeRegexp.ReplaceAllString(text, "")
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func recordString(record gdb.Record, key string) string {
	if record == nil {
		return ""
	}
	value := record[key]
	if value == nil {
		return ""
	}
	return value.String()
}

func recordInt64(record gdb.Record, key string) int64 {
	if record == nil {
		return 0
	}
	value := record[key]
	if value == nil {
		return 0
	}
	return value.Int64()
}

func flagToInt(flag string) int {
	if flag == "Y" || flag == "y" || flag == "1" {
		return 1
	}
	return 0
}

type contentProfileRow struct {
	Id                   int64       `json:"id"`
	ProfileNo            string      `json:"profileNo"`
	Title                string      `json:"title"`
	Summary              string      `json:"summary"`
	PlainText            string      `json:"plainText"`
	DaysWithEscort       int         `json:"daysWithEscort"`
	ExpectedLivingCost   int         `json:"expectedLivingCost"`
	CanFlyToProvince     int         `json:"canFlyToProvince"`
	CanGoAbroad          int         `json:"canGoAbroad"`
	CanOvernight         int         `json:"canOvernight"`
	CanCohabitate        int         `json:"canCohabitate"`
	HasHealthCheck       int         `json:"hasHealthCheck"`
	IsFullMonth          int         `json:"isFullMonth"`
	IsVirgin             int         `json:"isVirgin"`
	AcceptSm             int         `json:"acceptSm"`
	NoCondomAfterCheck   int         `json:"noCondomAfterCheck"`
	AllowCreampie        int         `json:"allowCreampie"`
	HasTattoo            int         `json:"hasTattoo"`
	GroupParams          string      `json:"groupParams"`
	TagParams            string      `json:"tagParams"`
	SourceRemark         string      `json:"sourceRemark"`
	Province             string      `json:"province"`
	City                 string      `json:"city"`
	Age                  int         `json:"age"`
	Height               int         `json:"height"`
	Weight               int         `json:"weight"`
	CupSize              string      `json:"cupSize"`
	HasVerificationVideo int         `json:"hasVerificationVideo"`
	MemberOnlyVideo      int         `json:"memberOnlyVideo"`
	ImageCount           int         `json:"imageCount"`
	VideoCount           int         `json:"videoCount"`
	HomeRecommend        int         `json:"homeRecommend"`
	HomeSort             int         `json:"homeSort"`
	Visibility           string      `json:"visibility"`
	PublishedAt          *gtime.Time `json:"publishedAt"`
	ActionAt             *gtime.Time `json:"actionAt"`
}

func (row contentProfileRow) toListModel() *sysin.ContentProfileListModel {
	name := row.Title
	if name == "" {
		name = row.ProfileNo
	}
	return &sysin.ContentProfileListModel{
		Id:            row.Id,
		ProfileNo:     row.ProfileNo,
		Name:          name,
		Title:         row.Title,
		Summary:       row.Summary,
		Province:      row.Province,
		City:          row.City,
		Age:           row.Age,
		Height:        row.Height,
		Weight:        row.Weight,
		Cup:           row.CupSize,
		HasVideo:      row.VideoCount > 0,
		VideoLocked:   row.VideoCount > 0 && row.MemberOnlyVideo == 1,
		Verified:      row.HasVerificationVideo == 1,
		ImageCount:    row.ImageCount,
		VideoCount:    row.VideoCount,
		HomeRecommend: row.HomeRecommend,
		HomeSort:      row.HomeSort,
		PublishedAt:   row.PublishedAt,
	}
}
