package profilesearch

import "testing"

func TestIdentifier(t *testing.T) {
	for input, want := range map[string]string{
		"A73854":           "A73854",
		" kq7528863 ":      "KQ7528863",
		"资料编号：fnur8829266": "FNUR8829266",
	} {
		got, ok := Identifier(input)
		if !ok || got != want {
			t.Fatalf("Identifier(%q) = (%q, %v), want (%q, true)", input, got, ok, want)
		}
	}
	for _, input := range []string{"成都", "高颜值", "ABC", "001", "A 73854"} {
		if got, ok := Identifier(input); ok {
			t.Fatalf("Identifier(%q) = (%q, true), want no identifier", input, got)
		}
	}
}
