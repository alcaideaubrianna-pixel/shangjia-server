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

// Four 16-bit blocks guarantee that a distance below 16 has at least
// one block within radius 3. Album threshold 14 must keep the indexed path.
func TestMediaPHashLshAlbumThresholdRecall(t *testing.T) {
	query := "0000000000000000"
	candidate := "000f000f00070007" // block distances 4+4+3+3 = 14
	targets := mediaPHashLshBucketValues(candidate)
	for _, threshold := range []int{12, 13, 14, 15} {
		cells := mediaPHashLshCells(query, threshold)
		if len(cells) != 4*697 {
			t.Fatalf("threshold %d bypasses indexed LSH", threshold)
		}
		if threshold == 14 {
			found := false
			for _, cell := range cells {
				for _, target := range targets {
					if cell == target {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("distance-14 candidate lost")
			}
		}
	}
	if len(mediaPHashLshCells(query, 16)) != 0 {
		t.Fatal("radius 3 cannot guarantee recall at distance 16")
	}
}
