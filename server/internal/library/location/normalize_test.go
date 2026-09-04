package location

import (
	"testing"

	"hotgo/internal/model/entity"
)

func TestRegionDirectoryNormalize(t *testing.T) {
	directory := &regionDirectory{
		byCode: map[string]*entity.SysProvinces{
			"110000": {Id: 110000, Level: 1, Title: "北京市"},
			"110100": {Id: 110100, Pid: 110000, Level: 2, Title: "北京市"},
			"440000": {Id: 440000, Level: 1, Title: "广东省"},
			"440100": {Id: 440100, Pid: 440000, Level: 2, Title: "广州市"},
		},
		provincesByName: map[string][]*entity.SysProvinces{
			"北京": {{Id: 110000, Level: 1, Title: "北京市"}},
		},
		citiesByName: map[string][]*entity.SysProvinces{
			"北京": {{Id: 110100, Pid: 110000, Level: 2, Title: "北京市"}},
			"广州": {{Id: 440100, Pid: 440000, Level: 2, Title: "广州市"}},
		},
	}
	province, changed := directory.normalizeProvince("北京")
	city, cityChanged := directory.normalizeCity("北京市", province)
	if province != "110000" || city != "110100" || !changed || !cityChanged {
		t.Fatalf("unexpected normalized region: province=%q city=%q", province, city)
	}
	if overseas, changed := directory.normalizeProvince("韩国"); overseas != "韩国" || changed {
		t.Fatalf("overseas region must be preserved: %q", overseas)
	}
	province, city, _ = directory.normalize("广州", "")
	if province != "440000" || city != "440100" {
		t.Fatalf("city-only province field was not inferred: province=%q city=%q", province, city)
	}
}
