package sys

import "testing"

func TestIsRegionCode(t *testing.T) {
	for _, value := range []string{"410000", "010000", " 410100 "} {
		if !isRegionCode(value) {
			t.Errorf("expected %q to be a region code", value)
		}
	}
	for _, value := range []string{"河南", "41000", "4100000", "41010a"} {
		if isRegionCode(value) {
			t.Errorf("expected %q not to be a region code", value)
		}
	}
}

func TestCodeFiltersUseExactValue(t *testing.T) {
	if got := provinceFilterValuesWithContext(nil, "410000"); len(got) != 1 || got[0] != "410000" {
		t.Fatalf("unexpected province filter: %#v", got)
	}
	if got := cityFilterValues("410000", "410100"); len(got) != 1 || got[0] != "410100" {
		t.Fatalf("unexpected city filter: %#v", got)
	}
}
