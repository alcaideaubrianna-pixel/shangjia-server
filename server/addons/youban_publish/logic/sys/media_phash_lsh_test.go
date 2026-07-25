package sys

import "testing"

func TestMediaPHashLshNeighborhood(t *testing.T) {
	if got := len(mediaPHashLshNeighborhood(0x1234, 0)); got != 1 {
		t.Fatalf("radius 0 neighborhood size = %d, want 1", got)
	}
	if got := len(mediaPHashLshNeighborhood(0x1234, 3)); got != 697 {
		t.Fatalf("radius 3 neighborhood size = %d, want 697", got)
	}
}

func TestMediaPHashLshCells(t *testing.T) {
	cells := mediaPHashLshCells("a2995926ca37967a", 12)
	if got := len(cells); got != 4*697 {
		t.Fatalf("LSH query cell count = %d, want %d", got, 4*697)
	}
}
