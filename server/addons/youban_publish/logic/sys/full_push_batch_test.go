package sys

import "testing"

func TestFullPushOperationUsesUnifiedPrefix(t *testing.T) {
	batchNo := newFullPushBatchNo(12, 345)
	if batchNo != "full_push:12:345" {
		t.Fatalf("unexpected batch no: %s", batchNo)
	}
	operationNo := fullPushTaskOperationNo(batchNo, 678)
	if operationNo != "full_push:12:345:678" {
		t.Fatalf("unexpected operation no: %s", operationNo)
	}
	if fullPushOperationBatchKey(operationNo) != batchNo {
		t.Fatalf("operation batch key mismatch: %s", fullPushOperationBatchKey(operationNo))
	}
	if publishSuccessRecordAction(operationNo) != publishSuccessTypeFull {
		t.Fatalf("operation was not classified as full push: %s", operationNo)
	}
}
