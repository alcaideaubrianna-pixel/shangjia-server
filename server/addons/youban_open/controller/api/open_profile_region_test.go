package api

import "testing"

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
