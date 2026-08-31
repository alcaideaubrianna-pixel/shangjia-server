package fix

import (
	"testing"

	"hotgo/internal/library/profileextractor"
)

func TestProfileAgeBackfillParserScope(t *testing.T) {
	tests := []struct {
		text string
		age  int
	}{
		{"年龄：23\n身高: 176", 23},
		{"Age: 24 Height: 168", 24},
		{"25岁，身高170", 25},
		{"年龄：[05年]身高：[165 cm]", 0},
		{"Age：06 Height：/163", 0},
	}
	for _, test := range tests {
		if got := profileextractor.Parse(test.text).Age; got != test.age {
			t.Fatalf("text %q age %d, want %d", test.text, got, test.age)
		}
	}
}
