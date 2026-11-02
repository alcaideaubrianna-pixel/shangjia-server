package youban_publish

import (
	"context"
	"testing"

	"hotgo/utility/runrole"
)

func TestPublishRuntimeShouldStart(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{name: "collector", role: runrole.CollectorWorker, want: true},
		{name: "background", role: runrole.BackgroundWorker, want: true},
		{name: "web", role: runrole.Web, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runrole.Set(test.role)
			t.Cleanup(func() { runrole.Set() })
			if got := publishRuntimeShouldStart(context.Background()); got != test.want {
				t.Fatalf("publishRuntimeShouldStart() = %t, want %t", got, test.want)
			}
		})
	}
}
