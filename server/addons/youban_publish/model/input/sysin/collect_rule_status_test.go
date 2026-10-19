package sysin

import "testing"

func TestCollectRuleCannotBeDisabledByLegacyClients(t *testing.T) {
	for _, status := range []int{-1, 0, 1, 2} {
		in := &CollectRuleSaveInp{Name: "采集规则", TargetChannelIds: []int64{1}, Status: status}
		if err := in.Filter(t.Context()); err != nil {
			t.Fatal(err)
		}
		if in.Status != 1 {
			t.Fatalf("legacy status %d disabled the rule: %d", status, in.Status)
		}
	}
}
