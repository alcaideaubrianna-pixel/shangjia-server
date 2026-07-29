package sys

import "testing"

func TestMaterialImportLatestSourceMessageID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "sorted ids", value: "151,152,153,154,155", want: 155},
		{name: "unsorted ids", value: "155,151,154", want: 155},
		{name: "invalid values", value: "abc,0,-1,156", want: 156},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := materialImportLatestSourceMessageID(test.value); got != test.want {
				t.Fatalf("latest source message id = %d, want %d", got, test.want)
			}
		})
	}
}

func TestMaterialImportVerifyMessageMustBeContinuous(t *testing.T) {
	if got := materialImportLatestSourceMessageID("151,152,153,154,155"); got+1 != 156 {
		t.Fatal("message 156 should be continuous with the profile group")
	}
	if got := materialImportLatestSourceMessageID("151,152,153,154,155"); got+1 == 125 {
		t.Fatal("message 125 must not be attached to the profile group")
	}
}

func TestMaterialImportHasVerifyMedia(t *testing.T) {
	if !materialImportHasVerifyMedia(`[{"type":"video","purpose":"verify"}]`) {
		t.Fatal("expected verify media to be detected")
	}
	if materialImportHasVerifyMedia(`[{"type":"video","purpose":"display"}]`) {
		t.Fatal("display media must not be treated as verify media")
	}
}

func TestMaterialImportVerifyUnitIsContinuous(t *testing.T) {
	if !materialImportVerifyUnitIsContinuous("151,152,153,154,155", []int{156}) {
		t.Fatal("message 156 should be accepted as the continuous verify message")
	}
	if materialImportVerifyUnitIsContinuous("151,152,153,154,155", []int{125}) {
		t.Fatal("message 125 must not be attached to the profile group")
	}
	if !materialImportVerifyUnitIsContinuous("151,152,153,154,155", []int{156, 157}) {
		t.Fatal("a grouped verify unit should attach as one continuous group")
	}
	if materialImportVerifyUnitIsContinuous("151,152,153,154,155", []int{156, 158}) {
		t.Fatal("a grouped verify unit with a gap must not attach")
	}
}
