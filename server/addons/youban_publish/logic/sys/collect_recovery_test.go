package sys

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestFairCollectRecoverySourceRowsRotatesWindows(t *testing.T) {
	rows := make(gdb.Result, 0, 157)
	for id := 1; id <= 157; id++ {
		rows = append(rows, gdb.Record{"source_id": gvar.New(id)})
	}

	first := fairCollectRecoverySourceRows(rows, 100, 0)
	second := fairCollectRecoverySourceRows(rows, 100, 1)
	if len(first) != 100 || len(second) != 100 {
		t.Fatalf("window sizes = %d and %d, want 100", len(first), len(second))
	}
	seen := make(map[int64]bool, len(rows))
	for _, row := range append(first, second...) {
		seen[row["source_id"].Int64()] = true
	}
	if len(seen) != len(rows) {
		t.Fatalf("two windows covered %d sources, want %d", len(seen), len(rows))
	}
}

func TestFairCollectRecoverySourceRowsKeepsShortInput(t *testing.T) {
	rows := gdb.Result{{"source_id": gvar.New(1)}, {"source_id": gvar.New(2)}}
	got := fairCollectRecoverySourceRows(rows, 100, 1)
	if len(got) != len(rows) || got[0]["source_id"].Int() != 1 || got[1]["source_id"].Int() != 2 {
		t.Fatalf("fairCollectRecoverySourceRows() = %#v, want original rows", got)
	}
}

func TestCollectRecoveryPerSourceLimit(t *testing.T) {
	tests := []struct {
		name        string
		totalLimit  int
		sourceCount int
		want        int
	}{
		{name: "all candidates get one slot", totalLimit: 100, sourceCount: 100, want: 1},
		{name: "quota shared across sources", totalLimit: 100, sourceCount: 25, want: 4},
		{name: "quota capped for few sources", totalLimit: 100, sourceCount: 2, want: 10},
		{name: "more sources than slots", totalLimit: 3, sourceCount: 10, want: 1},
		{name: "invalid total limit", totalLimit: 0, sourceCount: 10, want: 1},
		{name: "invalid source count", totalLimit: 100, sourceCount: 0, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collectRecoveryPerSourceLimit(tt.totalLimit, tt.sourceCount); got != tt.want {
				t.Fatalf("collectRecoveryPerSourceLimit(%d, %d) = %d, want %d", tt.totalLimit, tt.sourceCount, got, tt.want)
			}
		})
	}
}
