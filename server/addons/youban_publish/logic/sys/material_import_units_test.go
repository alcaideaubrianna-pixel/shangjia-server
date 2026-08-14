package sys

import (
	"database/sql"
	"errors"
	"testing"
)

func TestMaterialImportIndexLookupSucceeded(t *testing.T) {
	if !materialImportIndexLookupSucceeded(nil) {
		t.Fatal("nil lookup error should succeed")
	}
	if !materialImportIndexLookupSucceeded(sql.ErrNoRows) {
		t.Fatal("missing index job should create a new job")
	}
	if materialImportIndexLookupSucceeded(errors.New("database unavailable")) {
		t.Fatal("database errors must not be ignored")
	}
}

func TestCollectMaterialUnitsMergeCrossPageLeadingUnits(t *testing.T) {
	newerPage := []*collectMaterialUnit{
		{
			GroupedId: "14277572021261444",
			MessageId: 96,
			Messages:  []int{96},
			Media:     []collectMediaItem{{Type: "photo"}},
		},
		{
			MessageId: 97,
			Messages:  []int{97},
			Media:     []collectMediaItem{{Type: "video"}},
		},
	}

	processable, pending := splitCollectMaterialUnits(newerPage)
	if len(processable) != 0 {
		t.Fatalf("expected no processable units from newer page, got %d", len(processable))
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending units from newer page, got %d", len(pending))
	}

	olderPage := []*collectMaterialUnit{
		{
			GroupedId: "14277572021261444",
			RawText:   "昵称：八方来财324",
			MessageId: 93,
			Messages:  []int{93, 94, 95},
			Media: []collectMediaItem{
				{Type: "photo"},
				{Type: "photo"},
				{Type: "photo"},
			},
		},
	}
	processable, carry := splitCollectMaterialUnits(olderPage)
	if len(carry) != 0 {
		t.Fatalf("expected no carry units from older page, got %d", len(carry))
	}

	merged := mergeCollectMaterialUnits(append(processable, pending...))
	if len(merged) != 2 {
		t.Fatalf("expected display unit and verify unit, got %d", len(merged))
	}
	if got := len(merged[0].Messages); got != 4 {
		t.Fatalf("expected display messages 93-96, got %d", got)
	}
	if merged[0].Messages[0] != 93 || merged[0].Messages[3] != 96 {
		t.Fatalf("unexpected display message ids: %#v", merged[0].Messages)
	}
	if merged[1].MessageId != 97 || len(merged[1].Media) != 1 || merged[1].Media[0].Type != "video" {
		t.Fatalf("unexpected verify unit: %#v", merged[1])
	}
}

func TestMaterialImportTitleFallbackLeadingText(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		title     string
		profileNo string
		nickname  string
	}{
		{
			name:      "leading title line",
			text:      "朴朴芙蓉B3054\n省份: 郑州\n城市：开封\n年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "title prefix",
			text:      "标题：朴朴芙蓉B3054\n省份: 郑州\n城市：开封",
			title:     "朴朴芙蓉B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "number and nickname",
			text:      "编号：B3054\n昵称：朴朴芙蓉\n省份: 郑州",
			title:     "B3054",
			profileNo: "B3054",
			nickname:  "朴朴芙蓉",
		},
		{
			name:      "nickname only",
			text:      "昵称：朴朴芙蓉\n省份: 郑州\n城市：开封",
			title:     "朴朴芙蓉",
			profileNo: "",
			nickname:  "朴朴芙蓉",
		},
		{
			name:      "nickname with located province and city",
			text:      "昵称:\u00a0B182\n所在省份:江苏\n所在城市:南京\n年龄:21\n罩杯:C",
			title:     "B182",
			profileNo: "",
			nickname:  "B182",
		},
		{
			name:      "nickname separated by whitespace",
			text:      "昵称 A26\n省份: 山西\n城市: 山西",
			title:     "A26",
			profileNo: "",
			nickname:  "A26",
		},
		{
			name:      "field label contains whitespace",
			text:      "昵称 A26\n省份: 山西\n城 市: 山西",
			title:     "A26",
			profileNo: "",
			nickname:  "A26",
		},
		{
			name:      "leading number before inline nickname",
			text:      "JJ14 昵称: 小安\n省份：浙江",
			title:     "小安",
			profileNo: "",
			nickname:  "小安",
		},
		{
			name:      "mixed same line",
			text:      "朴朴芙蓉B3054 省份: 郑州 城市：开封 年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
		{
			name:      "title contains field word",
			text:      "开封城市女孩B3054\n年龄：18",
			title:     "B3054",
			profileNo: "",
			nickname:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, profileNo, nickname := materialImportTitle(tc.text)
			if title != tc.title {
				t.Fatalf("unexpected title: %q", title)
			}
			if profileNo != tc.profileNo {
				t.Fatalf("unexpected profileNo: %q", profileNo)
			}
			if nickname != tc.nickname {
				t.Fatalf("unexpected nickname: %q", nickname)
			}
		})
	}
}

func TestMaterialImportIgnoredNotice(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "duplicate notice", text: "❌ 重复投稿\n168 小时内已提交过高度相似资料\n编号：2607212921"},
		{name: "success notice", text: "✅ 提交成功！ 📋 编号：qwby0723613 投稿已自动审核通过并成功分发到 7 个频道。"},
		{name: "failed notice", text: "收录失败！此信息已存在，无需重复收录 编号：65772177"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !profileMessageIgnoredNotice(test.text) {
				t.Fatalf("expected notice to be ignored: %q", test.text)
			}
		})
	}
	if profileMessageIgnoredNotice("昵称：A26\n省份：山西\n城市：山西") {
		t.Fatal("valid material was incorrectly classified as a notice")
	}
}

func TestMaterialImportProfileText(t *testing.T) {
	valid := []string{
		"昵称：B182\n年龄：21",
		"昵称：B182\n身高：165",
		"昵称：B182\n体重：45kg",
		"昵称：B182\n所在城市：南京",
		"昵称：B182\n城 市：南京",
	}
	for _, text := range valid {
		if !profileMessageHasProfileText(text) {
			t.Fatalf("expected profile text: %q", text)
		}
	}
	invalid := []string{"哇发我", "P31225", "随手拍了一张照片", "自拍视频"}
	for _, text := range invalid {
		if profileMessageHasProfileText(text) {
			t.Fatalf("expected ordinary media text: %q", text)
		}
	}
}

func TestMaterialImportMatchedTagNames(t *testing.T) {
	got := materialImportMatchedTagNames("昵称：小安\n职业：老师/演员\n城市：北京", []string{"演员", "老师", "学生"})
	if got != "演员,老师" {
		t.Fatalf("matched tags = %q, want %q", got, "演员,老师")
	}
}

func TestJoinMaterialImportRegionIDs(t *testing.T) {
	got := joinMaterialImportRegionIDs(map[int64]struct{}{110000: {}, 310000: {}, 440000: {}})
	if got != "110000,310000,440000" {
		t.Fatalf("region ids = %q, want %q", got, "110000,310000,440000")
	}
}

func TestMaterialImportRegionCodesFromIndex(t *testing.T) {
	province := &legacyCMSRegionOption{Id: 610000, Level: 1, Title: "陕西省"}
	city := &legacyCMSRegionOption{Id: 610100, Pid: province.Id, Level: 2, Title: "西安市"}
	district := &legacyCMSRegionOption{Id: 610122, Pid: city.Id, Level: 3, Title: "蓝田县"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"陕西": province},
		citiesByName:    map[string][]*legacyCMSRegionOption{"西安": {city}},
		districtsByName: map[string][]*legacyCMSRegionOption{"蓝田": {district}},
		optionsById:     map[int64]*legacyCMSRegionOption{province.Id: province, city.Id: city, district.Id: district},
	}
	tests := []struct {
		name         string
		text         string
		wantProvince string
		wantCity     string
	}{
		{name: "city without suffix", text: "所在城市：西安", wantProvince: "610000", wantCity: "610100"},
		{name: "city with suffix", text: "所在城市：西安市", wantProvince: "610000", wantCity: "610100"},
		{name: "county without suffix", text: "地区：蓝田", wantProvince: "610000", wantCity: "610100"},
		{name: "county with suffix", text: "地区：蓝田县", wantProvince: "610000", wantCity: "610100"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provinceCode, cityCode := materialImportRegionCodesFromIndex(test.text, index)
			if provinceCode != test.wantProvince || cityCode != test.wantCity {
				t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, test.wantProvince, test.wantCity)
			}
		})
	}
}

func TestMaterialImportRegionCodesPreferLocationFields(t *testing.T) {
	henan := &legacyCMSRegionOption{Id: 410000, Level: 1, Title: "河南省"}
	zhengzhou := &legacyCMSRegionOption{Id: 410100, Pid: henan.Id, Level: 2, Title: "郑州市"}
	yichang := &legacyCMSRegionOption{Id: 420500, Pid: 420000, Level: 2, Title: "宜昌市"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"河南": henan},
		citiesByName: map[string][]*legacyCMSRegionOption{
			"宜昌": {yichang},
			"郑州": {zhengzhou},
		},
		optionsById: map[int64]*legacyCMSRegionOption{henan.Id: henan, zhengzhou.Id: zhengzhou, yichang.Id: yichang},
	}
	text := "省份:河南\n城市:郑州\n介绍人说可以去宜昌\n能否飞往其他城市:可以"
	provinceCode, cityCode := materialImportRegionCodesFromIndex(text, index)
	if provinceCode != "410000" || cityCode != "410100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "410000", "410100")
	}
}

func TestMaterialImportRegionNameMatchesRejectsSingleCharacterAlias(t *testing.T) {
	if materialImportRegionNameMatches("河南安阳", "安") {
		t.Fatal("single-character region alias must not match full text")
	}
	if !materialImportRegionNameMatches("河南安阳", "安阳") {
		t.Fatal("two-character region alias should match full text")
	}
}

func TestMaterialImportRegionCodesSupportsMunicipality(t *testing.T) {
	chongqing := &legacyCMSRegionOption{Id: 500000, Level: 1, Title: "重庆市"}
	city := &legacyCMSRegionOption{Id: 500100, Pid: chongqing.Id, Level: 2, Title: "市辖区"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"重庆": chongqing},
		citiesByName:    map[string][]*legacyCMSRegionOption{"辖区": {city}},
		childrenByPid:   map[int64][]*legacyCMSRegionOption{chongqing.Id: {city}},
		optionsById:     map[int64]*legacyCMSRegionOption{chongqing.Id: chongqing, city.Id: city},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省份:重庆\n所在城市:重庆市", index)
	if provinceCode != "500000" || cityCode != "500100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "500000", "500100")
	}
}

func TestMaterialImportRegionCodesSupportsMunicipalityDistrict(t *testing.T) {
	shanghai := &legacyCMSRegionOption{Id: 310000, Level: 1, Title: "上海市"}
	city := &legacyCMSRegionOption{Id: 310100, Pid: shanghai.Id, Level: 2, Title: "市辖区"}
	minhang := &legacyCMSRegionOption{Id: 310112, Pid: city.Id, Level: 3, Title: "闵行区"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"上海": shanghai},
		citiesByName:    map[string][]*legacyCMSRegionOption{"辖区": {city}},
		districtsByName: map[string][]*legacyCMSRegionOption{"闵行区": {minhang}},
		childrenByPid:   map[int64][]*legacyCMSRegionOption{shanghai.Id: {city}, city.Id: {minhang}},
		optionsById:     map[int64]*legacyCMSRegionOption{shanghai.Id: shanghai, city.Id: city, minhang.Id: minhang},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省份:上海\n所在城市:闵行区", index)
	if provinceCode != "310000" || cityCode != "310100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "310000", "310100")
	}
}

func TestMaterialImportRegionCodesSupportsMunicipalityFromCityField(t *testing.T) {
	beijing := &legacyCMSRegionOption{Id: 110000, Level: 1, Title: "北京市"}
	city := &legacyCMSRegionOption{Id: 110100, Pid: beijing.Id, Level: 2, Title: "市辖区"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"北京": beijing},
		childrenByPid:   map[int64][]*legacyCMSRegionOption{beijing.Id: {city}},
		optionsById:     map[int64]*legacyCMSRegionOption{beijing.Id: beijing, city.Id: city},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在城市：北京", index)
	if provinceCode != "110000" || cityCode != "110100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "110000", "110100")
	}
}

func TestMaterialImportRegionCodesPrefersLongestDistrictName(t *testing.T) {
	chongqing := &legacyCMSRegionOption{Id: 500000, Level: 1, Title: "重庆市"}
	city := &legacyCMSRegionOption{Id: 500100, Pid: chongqing.Id, Level: 2, Title: "市辖区"}
	yuzhong := &legacyCMSRegionOption{Id: 500103, Pid: city.Id, Level: 3, Title: "渝中区"}
	otherCity := &legacyCMSRegionOption{Id: 370100, Pid: 370000, Level: 2, Title: "济南市"}
	shortAlias := &legacyCMSRegionOption{Id: 370103, Pid: otherCity.Id, Level: 3, Title: "市中区"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"重庆": chongqing},
		districtsByName: map[string][]*legacyCMSRegionOption{"渝中区": {yuzhong}, "中区": {shortAlias}},
		childrenByPid:   map[int64][]*legacyCMSRegionOption{chongqing.Id: {city}},
		optionsById: map[int64]*legacyCMSRegionOption{
			chongqing.Id:  chongqing,
			city.Id:       city,
			yuzhong.Id:    yuzhong,
			otherCity.Id:  otherCity,
			shortAlias.Id: shortAlias,
		},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省：渝中区\n所在城市：重庆市", index)
	if provinceCode != "500000" || cityCode != "500100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "500000", "500100")
	}
}

func TestMaterialImportRegionCodesSeparatesProvinceAndCityFields(t *testing.T) {
	heilongjiang := &legacyCMSRegionOption{Id: 230000, Level: 1, Title: "黑龙江省"}
	harbin := &legacyCMSRegionOption{Id: 230100, Pid: heilongjiang.Id, Level: 2, Title: "哈尔滨市"}
	qiqihar := &legacyCMSRegionOption{Id: 230200, Pid: heilongjiang.Id, Level: 2, Title: "齐齐哈尔市"}
	longjiang := &legacyCMSRegionOption{Id: 230221, Pid: qiqihar.Id, Level: 3, Title: "龙江县"}
	qinhuangdao := &legacyCMSRegionOption{Id: 130300, Pid: 130000, Level: 2, Title: "秦皇岛市"}
	taiyuan := &legacyCMSRegionOption{Id: 140100, Pid: 140000, Level: 2, Title: "太原市"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"黑龙江": heilongjiang},
		citiesByName: map[string][]*legacyCMSRegionOption{
			"哈尔滨": {harbin},
			"秦皇岛": {qinhuangdao},
			"太原":  {taiyuan},
		},
		districtsByName: map[string][]*legacyCMSRegionOption{"龙江": {longjiang}},
		optionsById: map[int64]*legacyCMSRegionOption{
			heilongjiang.Id: heilongjiang,
			harbin.Id:       harbin,
			qiqihar.Id:      qiqihar,
			longjiang.Id:    longjiang,
			qinhuangdao.Id:  qinhuangdao,
			taiyuan.Id:      taiyuan,
		},
	}
	text := "所在省份：黑龙江\n所在城市：哈尔滨\n能否飞往其他城市：太原\n介绍人：秦皇岛"
	provinceCode, cityCode := materialImportRegionCodesFromIndex(text, index)
	if provinceCode != "230000" || cityCode != "230100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "230000", "230100")
	}
}

func TestMaterialImportRegionCodesSupportsLegacyLocationLines(t *testing.T) {
	shandong := &legacyCMSRegionOption{Id: 370000, Level: 1, Title: "山东省"}
	binzhou := &legacyCMSRegionOption{Id: 371600, Pid: shandong.Id, Level: 2, Title: "滨州市"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"山东": shandong},
		citiesByName:    map[string][]*legacyCMSRegionOption{"滨州": {binzhou}},
		optionsById:     map[int64]*legacyCMSRegionOption{shandong.Id: shandong, binzhou.Id: binzhou},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省；山东\n所在市滨州\n介绍人：秦皇岛", index)
	if provinceCode != "370000" || cityCode != "371600" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "370000", "371600")
	}
}

func TestMaterialImportRegionCodesSupportsMultipleCities(t *testing.T) {
	sichuan := &legacyCMSRegionOption{Id: 510000, Level: 1, Title: "四川省"}
	chengdu := &legacyCMSRegionOption{Id: 510100, Pid: sichuan.Id, Level: 2, Title: "成都市"}
	deyang := &legacyCMSRegionOption{Id: 510600, Pid: sichuan.Id, Level: 2, Title: "德阳市"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"四川": sichuan},
		citiesByName: map[string][]*legacyCMSRegionOption{
			"成都": {chengdu},
			"德阳": {deyang},
		},
		optionsById: map[int64]*legacyCMSRegionOption{sichuan.Id: sichuan, chengdu.Id: chengdu, deyang.Id: deyang},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省份：四川\n所在城市：德阳/成都", index)
	if provinceCode != "510000" || cityCode != "510100,510600" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "510000", "510100,510600")
	}
}

func TestMaterialImportRegionCodesSupportsRegionAliases(t *testing.T) {
	guangdong := &legacyCMSRegionOption{Id: 440000, Level: 1, Title: "广东省"}
	guangzhou := &legacyCMSRegionOption{Id: 440100, Pid: guangdong.Id, Level: 2, Title: "广州市"}
	baiyun := &legacyCMSRegionOption{Id: 440111, Pid: guangzhou.Id, Level: 3, Title: "白云区"}
	yunnan := &legacyCMSRegionOption{Id: 530000, Level: 1, Title: "云南省"}
	dehong := &legacyCMSRegionOption{Id: 533100, Pid: yunnan.Id, Level: 2, Title: "德宏傣族景颇族自治州"}
	mangshi := &legacyCMSRegionOption{Id: 533103, Pid: dehong.Id, Level: 3, Title: "芒市"}
	guizhou := &legacyCMSRegionOption{Id: 520000, Level: 1, Title: "贵州省"}
	qiannan := &legacyCMSRegionOption{Id: 522700, Pid: guizhou.Id, Level: 2, Title: "黔南布依族苗族自治州"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"广东": guangdong, "云南": yunnan, "贵州": guizhou},
		citiesByName: map[string][]*legacyCMSRegionOption{
			"广州":         {guangzhou},
			"德宏傣族景颇族自治州": {dehong},
			"黔南布依族苗族自治州": {qiannan},
		},
		districtsByName: map[string][]*legacyCMSRegionOption{"白云区": {baiyun}, "芒": {mangshi}},
		optionsById: map[int64]*legacyCMSRegionOption{
			guangdong.Id: guangdong,
			guangzhou.Id: guangzhou,
			baiyun.Id:    baiyun,
			yunnan.Id:    yunnan,
			dehong.Id:    dehong,
			mangshi.Id:   mangshi,
			guizhou.Id:   guizhou,
			qiannan.Id:   qiannan,
		},
	}
	tests := []struct {
		name         string
		text         string
		wantProvince string
		wantCity     string
	}{
		{name: "district without suffix", text: "所在省份：广东\n所在城市：白云", wantProvince: "440000", wantCity: "440100"},
		{name: "single character city name", text: "所在省份：云南\n所在城市：芒市", wantProvince: "530000", wantCity: "533100"},
		{name: "autonomous prefecture short name", text: "所在省份：贵州\n所在城市：黔南", wantProvince: "520000", wantCity: "522700"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provinceCode, cityCode := materialImportRegionCodesFromIndex(test.text, index)
			if provinceCode != test.wantProvince || cityCode != test.wantCity {
				t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, test.wantProvince, test.wantCity)
			}
		})
	}
}

func TestMaterialImportRegionCodesPrefersCityOverDistrictSuffixAlias(t *testing.T) {
	shaanxi := &legacyCMSRegionOption{Id: 610000, Level: 1, Title: "陕西省"}
	xian := &legacyCMSRegionOption{Id: 610100, Pid: shaanxi.Id, Level: 2, Title: "西安市"}
	jilin := &legacyCMSRegionOption{Id: 220000, Level: 1, Title: "吉林省"}
	liaoyuan := &legacyCMSRegionOption{Id: 220400, Pid: jilin.Id, Level: 2, Title: "辽源市"}
	xianDistrict := &legacyCMSRegionOption{Id: 220403, Pid: liaoyuan.Id, Level: 3, Title: "西安区"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"陕西": shaanxi, "吉林": jilin},
		citiesByName:    map[string][]*legacyCMSRegionOption{"西安": {xian}, "辽源": {liaoyuan}},
		districtsByName: map[string][]*legacyCMSRegionOption{"西安区": {xianDistrict}},
		optionsById: map[int64]*legacyCMSRegionOption{
			shaanxi.Id:      shaanxi,
			xian.Id:         xian,
			jilin.Id:        jilin,
			liaoyuan.Id:     liaoyuan,
			xianDistrict.Id: xianDistrict,
		},
	}
	provinceCode, cityCode := materialImportRegionCodesFromIndex("所在省：西安\n所在城市：西安", index)
	if provinceCode != "610000" || cityCode != "610100" {
		t.Fatalf("codes = (%q, %q), want (%q, %q)", provinceCode, cityCode, "610000", "610100")
	}
}

func TestNormalizeLegacyRegionNameSupportsCountySuffix(t *testing.T) {
	if got := normalizeLegacyRegionName("西安市"); got != "西安" {
		t.Fatalf("normalize city = %q, want %q", got, "西安")
	}
	if got := normalizeLegacyRegionName("蓝田县"); got != "蓝田" {
		t.Fatalf("normalize county = %q, want %q", got, "蓝田")
	}
}

func TestParseMaterialImportChannelReference(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		channel  string
		username string
	}{
		{name: "public channel url", raw: "https://t.me/TestChannel/16313?single", username: "testchannel"},
		{name: "private channel url", raw: "https://t.me/c/123456/16313", channel: "-100123456"},
		{name: "username", raw: "@TestChannel", username: "testchannel"},
		{name: "numeric id", raw: "-100123456", channel: "-100123456"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel, username, err := parseMaterialImportChannelReference(test.raw)
			if err != nil {
				t.Fatalf("parseMaterialImportChannelReference(%q) error: %v", test.raw, err)
			}
			if channel != test.channel || username != test.username {
				t.Fatalf("parseMaterialImportChannelReference(%q) = (%q, %q), want (%q, %q)", test.raw, channel, username, test.channel, test.username)
			}
		})
	}
}
