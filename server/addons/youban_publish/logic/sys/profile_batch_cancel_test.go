package sys

import "testing"

func TestAdminBatchTextOperationNo(t *testing.T) {
	operationNo, err := adminBatchTextOperationNo(12, "1786000000000-a1b2c3d4", 345)
	if err != nil {
		t.Fatal(err)
	}
	if operationNo != "batchtext:12:1786000000000-a1b2c3d4:profile:345" {
		t.Fatalf("unexpected operation no: %s", operationNo)
	}
	if !isManualProfilePublishOperation(operationNo) {
		t.Fatal("batch text publish must use manual republish cleanup")
	}
}

func TestAdminBatchTextOperationNoWithoutBatch(t *testing.T) {
	operationNo, err := adminBatchTextOperationNo(12, "", 345)
	if err != nil {
		t.Fatal(err)
	}
	if operationNo != "" {
		t.Fatalf("expected default operation no, got %s", operationNo)
	}
}

func TestNormalizeAdminBatchTextId(t *testing.T) {
	valid := []string{"1786000000000-a1b2c3d4", "abcdefgh", "ABCDEF12-3456"}
	for _, value := range valid {
		if _, err := normalizeAdminBatchTextId(value); err != nil {
			t.Fatalf("expected %q valid: %v", value, err)
		}
	}
	invalid := []string{"short", "has_underscore", "has:colon", "contains%wildcard", "含中文字符123456"}
	for _, value := range invalid {
		if _, err := normalizeAdminBatchTextId(value); err == nil {
			t.Fatalf("expected %q invalid", value)
		}
	}
}
