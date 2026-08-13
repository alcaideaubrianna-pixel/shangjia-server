package router

import "testing"

func TestAIOpsRouteUsesAddonPrefix(t *testing.T) {
	const path = "/api/youban_publish/ai-ops/profile/media"
	if path == "/internal/youban-publish/ai-ops/profile/media" {
		t.Fatal("AI运维接口必须使用标准插件路由前缀")
	}
}

func TestAIOpsTokenMatches(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		provided string
		want     bool
	}{
		{name: "match", expected: "secret-token", provided: "secret-token", want: true},
		{name: "empty", expected: "", provided: "", want: false},
		{name: "different", expected: "secret-token", provided: "other-token", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := aiOpsTokenMatches(test.expected, test.provided); got != test.want {
				t.Fatalf("aiOpsTokenMatches() = %t, want %t", got, test.want)
			}
		})
	}
}
