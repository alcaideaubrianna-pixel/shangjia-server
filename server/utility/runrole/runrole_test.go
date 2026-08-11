package runrole

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "all", values: []string{"web,all"}, want: []string{All}},
		{name: "three roles", values: []string{"runtime", "worker|web"}, want: []string{Runtime, Web, Worker}},
		{name: "aliases", values: []string{"account,scheduler,push-worker"}, want: []string{Runtime, Worker}},
		{name: "invalid", values: []string{"unknown"}, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalize(test.values); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalize() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNormalizeLegacy(t *testing.T) {
	want := []string{Runtime, Worker}
	if got := normalizeLegacy([]string{"account,scheduler,media-worker"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLegacy() = %v, want %v", got, want)
	}
}
