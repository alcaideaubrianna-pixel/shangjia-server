package sysin

import (
	"context"
	"testing"
)

func TestAntiScanSegmentInpRequiresMediaId(t *testing.T) {
	if err := (&AntiScanSegmentInp{}).Filter(context.Background()); err == nil {
		t.Fatal("expected empty media id to be rejected")
	}
	if err := (&AntiScanSegmentInp{MediaId: 42}).Filter(context.Background()); err != nil {
		t.Fatalf("expected valid media id, got %v", err)
	}
}
