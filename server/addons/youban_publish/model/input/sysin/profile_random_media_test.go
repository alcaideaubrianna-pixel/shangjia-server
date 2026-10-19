package sysin

import "testing"

func TestProfileRandomMediaPolicy(t *testing.T) {
	on := true
	policy := ProfileRandomMediaPolicy{Enabled: &on}
	if err := policy.Filter(); err != nil || policy.Count != 5 {
		t.Fatalf("manual default: %+v %v", policy, err)
	}
	policy.Count = -1
	if policy.Filter() == nil {
		t.Fatal("negative count accepted")
	}
	policy.Count = 20
	if err := policy.Filter(); err != nil {
		t.Fatal("count is independent of album chunk limit", err)
	}
}
