package sys

import "testing"

func TestProfileOperationTypeFromNo(t *testing.T) {
	cases := map[string]profileOperationType{
		"profile:1:2":    profileOperationPublish,
		"collect:9":      profileOperationPublish,
		"edit:3:4":       profileOperationEdit,
		"republish:3":    profileOperationRepublish,
		"ai-republish:3": profileOperationRepublish,
		"down:3:4":       profileOperationUnpublish,
		"unknown:3":      profileOperationPublish,
	}
	for operationNo, want := range cases {
		if got := profileOperationTypeFromNo(operationNo); got != want {
			t.Fatalf("operation %q got %q want %q", operationNo, got, want)
		}
	}
}

func TestProfileOperationClaimIsDatabaseOwned(t *testing.T) {
	claimed, err := claimProfileOperation(nil, 7, 42, profileOperationPublish)
	if err != nil || !claimed {
		t.Fatalf("database-owned operation claim should not depend on Redis: claimed=%t err=%v", claimed, err)
	}
}
