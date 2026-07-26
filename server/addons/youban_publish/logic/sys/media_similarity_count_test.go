package sys

import (
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMediaPHashLshProfileCountSQLUsesDatabaseDistanceAndDistinctProfile(t *testing.T) {
	query, args, err := mediaPHashLshProfileCountSQL(
		"9e7a659c12876c8d",
		mediaSimilarDefaultThreshold,
		[]mediaPHashBucketScopePart{{TenantId: 7, AccountIds: []int64{8, 110}}},
		"video",
		208721,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"COUNT(DISTINCT candidate.profile_id)", "bit_count(", "candidate.profile_id", "t.deleted_at IS NULL"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("count query missing %q", expected)
		}
	}
	if len(args) == 0 || args[len(args)-2] != "9e7a659c12876c8d" || args[len(args)-1] != mediaSimilarDefaultThreshold {
		t.Fatalf("unexpected trailing count args: %#v", args)
	}
}

func TestMediaPHashLshProfileCountSQLSplitsTenantPartitions(t *testing.T) {
	query, _, err := mediaPHashLshProfileCountSQL(
		"9e7a659c12876c8d",
		mediaSimilarDefaultThreshold,
		[]mediaPHashBucketScopePart{
			{TenantId: 6, AccountIds: []int64{7}},
			{TenantId: 7, AccountIds: []int64{8, 110}},
		},
		"image",
		208721,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(query, "SELECT b.media_id"); got != mediaPHashLshBlockCount*2 {
		t.Fatalf("LSH branch count = %d, want %d", got, mediaPHashLshBlockCount*2)
	}
}

func TestNormalizeMediaSimilarListInputUsesAccurateDefaultThreshold(t *testing.T) {
	in := &sysin.MediaSimilarListInp{}
	normalizeMediaSimilarListInput(in)
	if in.Threshold != mediaSimilarDefaultThreshold {
		t.Fatalf("default threshold = %d, want %d", in.Threshold, mediaSimilarDefaultThreshold)
	}
}
