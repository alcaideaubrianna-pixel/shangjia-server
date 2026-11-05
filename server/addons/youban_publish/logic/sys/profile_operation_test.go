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

func TestProfileOperationClaimKeySerializesAllLifecycleOperationsToOneProfileLock(t *testing.T) {
	base := profileOperationClaimKey(7, 42, profileOperationPublish)
	for _, operation := range []profileOperationType{
		profileOperationCreate,
		profileOperationEdit,
		profileOperationUnpublish,
		profileOperationDelete,
		profileOperationUnpublishDelete,
		profileOperationRepublish,
	} {
		if got := profileOperationClaimKey(7, 42, operation); got != base {
			t.Fatalf("operation %q used a separate profile lock: %q != %q", operation, got, base)
		}
	}
	if got := profileOperationClaimKey(7, 43, profileOperationPublish); got == base {
		t.Fatal("different profile IDs must not share a lifecycle lock")
	}
}
