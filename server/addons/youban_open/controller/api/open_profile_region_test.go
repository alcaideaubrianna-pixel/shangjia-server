package api

import (
	"testing"

	"hotgo/addons/youban_open/api/api/open"
)

func TestNormalizeProvinceCodes(t *testing.T) {
	codes, err := normalizeProvinceCodes("410000", "310000,410000, 330000")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"410000", "310000", "330000"}
	if len(codes) != len(want) {
		t.Fatalf("got %#v, want %#v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("got %#v, want %#v", codes, want)
		}
	}
}

func TestNormalizeProvinceCodesRejectsLegacyNames(t *testing.T) {
	if _, err := normalizeProvinceCodes("", "河南,410000"); err == nil {
		t.Fatal("expected province name to be rejected")
	}
}

func TestOpenProfileRequestIncludesKeyword(t *testing.T) {
	in := openProfileListInput(&open.ListReq{Keyword: " KQ7528863 "})
	if in.Keyword != "KQ7528863" {
		t.Fatalf("keyword = %q, want KQ7528863", in.Keyword)
	}
}
