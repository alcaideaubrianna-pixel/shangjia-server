package sys

import (
	"context"
	"strings"
	"testing"
)

func TestSafeProfileRegionValueClearsOversizedValue(t *testing.T) {
	if got := safeProfileRegionValue(" 北京 "); got != "北京" {
		t.Fatalf("trimmed region = %q, want %q", got, "北京")
	}
	if got := safeProfileRegionValue("北京"); got != "北京" {
		t.Fatalf("valid region = %q, want %q", got, "北京")
	}
	longValue := strings.Repeat("北", profileRegionMaxRunes+1)
	if got := safeProfileRegionValue(longValue); got != "" {
		t.Fatalf("oversized region should be cleared, got length %d", len([]rune(got)))
	}
}

func TestSelectProfileRegions(t *testing.T) {
	tests := []struct {
		name                                string
		prepared                            profileRegionPreparation
		create                              bool
		oldProvince, oldCity, oldText, text string
		province, city                      string
	}{
		{name: "深圳正文改杭州", prepared: profileRegionPreparation{rawProvince: "440000", rawCity: "440300", province: "440000", city: "440300", bodyProvince: "330000", bodyCity: "330100"}, oldProvince: "440000", oldCity: "440300", oldText: "城市：深圳", text: "城市：杭州", province: "330000", city: "330100"},
		{name: "手动南京优先", prepared: profileRegionPreparation{rawProvince: "江苏", rawCity: "南京", province: "320000", city: "320100", bodyProvince: "330000", bodyCity: "330100"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "杭州", province: "320000", city: "320100"},
		{name: "海外无匹配保留", prepared: profileRegionPreparation{rawProvince: "440000", rawCity: "440300", province: "440000", city: "440300"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "城市：海外", province: "440000", city: "440300"},
		{name: "正文仅新省份不拼旧市", prepared: profileRegionPreparation{bodyProvince: "330000"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "省份：浙江", province: "330000"},
		{name: "手动仅新省份不拼旧市", prepared: profileRegionPreparation{rawProvince: "330000", province: "330000"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "深圳", province: "330000"},
		{name: "手动新省回显旧市不拼接", prepared: profileRegionPreparation{rawProvince: "330000", rawCity: "440300", province: "330000", city: "440300"}, oldProvince: "440000", oldCity: "440300", province: "330000"},
		{name: "只改市采用规范化整组", prepared: profileRegionPreparation{rawCity: "杭州", province: "330000", city: "330100"}, oldProvince: "440000", oldCity: "440300", province: "330000", city: "330100"},
		{name: "省份相同缺市无匹配保留", prepared: profileRegionPreparation{rawProvince: "440000", province: "440000"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "海外", province: "440000", city: "440300"},
		{name: "旧地区全空正文未改也补齐", prepared: profileRegionPreparation{bodyProvince: "330000", bodyCity: "330100"}, oldText: "杭州", text: "杭州", province: "330000", city: "330100"},
		{name: "旧市空正文未改整组补齐", prepared: profileRegionPreparation{rawProvince: "440000", province: "440000", bodyProvince: "330000", bodyCity: "330100"}, oldProvince: "440000", oldText: "杭州", text: "杭州", province: "330000", city: "330100"},
		{name: "旧省空正文未改整组补齐", prepared: profileRegionPreparation{rawCity: "440300", city: "440300", bodyProvince: "330000", bodyCity: "330100"}, oldCity: "440300", oldText: "杭州", text: "杭州", province: "330000", city: "330100"},
		{name: "旧地区空手动选择仍优先", prepared: profileRegionPreparation{rawProvince: "320000", rawCity: "320100", province: "320000", city: "320100", bodyProvince: "330000", bodyCity: "330100"}, oldText: "杭州", text: "杭州", province: "320000", city: "320100"},
		{name: "正文未改不覆盖", prepared: profileRegionPreparation{bodyProvince: "330000", bodyCity: "330100"}, oldProvince: "440000", oldCity: "440300", oldText: "杭州", text: "杭州", province: "440000", city: "440300"},
		{name: "输入规范化异常保留", prepared: profileRegionPreparation{rawProvince: "浙江", province: "浙江", explicitFailed: true, bodyProvince: "330000", bodyCity: "330100"}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "杭州", province: "440000", city: "440300"},
		{name: "正文查询失败无候选保留", prepared: profileRegionPreparation{}, oldProvince: "440000", oldCity: "440300", oldText: "深圳", text: "杭州", province: "440000", city: "440300"},
		{name: "列表顺序等价非手改", prepared: profileRegionPreparation{rawProvince: "[\"440000\",\"330000\"]", rawCity: "440300,330100", province: "440000,330000", city: "440300,330100", bodyProvince: "320000", bodyCity: "320100"}, oldProvince: "330000,440000", oldCity: "330100,440300", oldText: "旧正文", text: "南京", province: "320000", city: "320100"},
		{name: "新建显式优先", create: true, prepared: profileRegionPreparation{province: "320000", city: "320100", bodyProvince: "330000", bodyCity: "330100"}, province: "320000", city: "320100"},
		{name: "新建部分显式不混正文", create: true, prepared: profileRegionPreparation{province: "320000", bodyProvince: "330000", bodyCity: "330100"}, province: "320000"},
		{name: "新建缺省用正文多地区", create: true, prepared: profileRegionPreparation{bodyProvince: "330000,440000", bodyCity: "330100,440300"}, province: "330000,440000", city: "330100,440300"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			province, city := selectProfileRegions(tt.prepared, tt.create, tt.oldProvince, tt.oldCity, tt.oldText, tt.text)
			if province != tt.province || city != tt.city {
				t.Fatalf("regions = (%q, %q), want (%q, %q)", province, city, tt.province, tt.city)
			}
		})
	}
}

// Exercise the real save preprocessing and selection together. Populate only
// the existing legacy cache; canonical input codes require no SQL/name lookup.
func TestPrepareProfileRegionsPairCompatibility(t *testing.T) {
	guangdong := &legacyCMSRegionOption{Id: 440000, Level: 1, Title: "广东省"}
	zhejiang := &legacyCMSRegionOption{Id: 330000, Level: 1, Title: "浙江省"}
	shenzhen := &legacyCMSRegionOption{Id: 440300, Pid: 440000, Level: 2, Title: "深圳市"}
	hangzhou := &legacyCMSRegionOption{Id: 330100, Pid: 330000, Level: 2, Title: "杭州市"}
	index := &legacyCMSRegionIndex{
		provincesByName: map[string]*legacyCMSRegionOption{"广东": guangdong, "浙江": zhejiang},
		citiesByName:    map[string][]*legacyCMSRegionOption{"深圳": {shenzhen}, "杭州": {hangzhou}},
		optionsById:     map[int64]*legacyCMSRegionOption{440000: guangdong, 330000: zhejiang, 440300: shenzhen, 330100: hangzhou},
	}
	legacyCMSRegionIndexCache.Lock()
	previous := legacyCMSRegionIndexCache.index
	legacyCMSRegionIndexCache.index = index
	legacyCMSRegionIndexCache.Unlock()
	t.Cleanup(func() {
		legacyCMSRegionIndexCache.Lock()
		legacyCMSRegionIndexCache.index = previous
		legacyCMSRegionIndexCache.Unlock()
	})
	tests := []struct {
		name, province, city, text, wantProvince, wantCity string
		failed                                             bool
	}{
		{name: "只改杭州回显广东", province: "440000", city: "330100", text: "资料正文", wantProvince: "330000", wantCity: "330100"},
		{name: "正文深圳改杭州", province: "440000", city: "440300", text: "所在城市：杭州", wantProvince: "330000", wantCity: "330100"},
		{name: "只改浙江回显深圳", province: "330000", city: "440300", text: "资料正文", wantProvince: "330000"},
		{name: "海外无匹配保留", province: "440000", city: "440300", text: "所在城市：海外", wantProvince: "440000", wantCity: "440300"},
		{name: "超长省份不清空", province: strings.Repeat("北", profileRegionMaxRunes+1), city: "440300", text: "资料正文", wantProvince: "440000", wantCity: "440300", failed: true},
		{name: "超长城市不清空", province: "440000", city: strings.Repeat("北", profileRegionMaxRunes+1), text: "资料正文", wantProvince: "440000", wantCity: "440300", failed: true},
		{name: "未知城市无法校验则保留", province: "440000", city: "999999", text: "资料正文", wantProvince: "440000", wantCity: "440300", failed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared := prepareProfileRegions(context.Background(), 602651, tt.text, tt.province, tt.city)
			if prepared.explicitFailed != tt.failed {
				t.Fatalf("explicitFailed = %v, want %v", prepared.explicitFailed, tt.failed)
			}
			province, city := selectProfileRegions(prepared, false, "440000", "440300", "旧正文", tt.text)
			if province != tt.wantProvince || city != tt.wantCity {
				t.Fatalf("regions = (%q, %q), want (%q, %q)", province, city, tt.wantProvince, tt.wantCity)
			}
		})
	}
}
