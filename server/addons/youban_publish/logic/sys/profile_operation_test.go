package sys

import "testing"

func TestProfileOperationTypeFromNo(t *testing.T) {
	cases := map[string]profileOperationType{
		"profile:1:2": profileOperationPublish,
		"collect:9":   profileOperationPublish,
		"edit:3:4":    profileOperationEdit,
		"republish:3": profileOperationRepublish,
		"down:3:4":    profileOperationUnpublish,
		"unknown:3":   profileOperationPublish,
	}
	for operationNo, want := range cases {
		if got := profileOperationTypeFromNo(operationNo); got != want {
			t.Fatalf("operation %q got %q want %q", operationNo, got, want)
		}
	}
}
