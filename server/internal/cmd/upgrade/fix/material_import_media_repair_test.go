package fix

import "testing"

func TestParseMaterialImportRepairGroupIDs(t *testing.T) {
	ids, err := ParseMaterialImportRepairGroupIDs("5049,5047,5049")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != 5047 || ids[1] != 5049 {
		t.Fatalf("ids=%v, want [5047 5049]", ids)
	}
}

func TestParseMaterialImportRepairGroupIDsRejectsInvalidID(t *testing.T) {
	if _, err := ParseMaterialImportRepairGroupIDs("5047,nope"); err == nil {
		t.Fatal("expected invalid group id error")
	}
}
