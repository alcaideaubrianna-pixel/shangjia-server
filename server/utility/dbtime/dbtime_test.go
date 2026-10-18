package dbtime

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestFromTimestampDriverLocation(t *testing.T) {
	want := time.Date(2026, 10, 16, 15, 20, 0, 123456000, Location())
	for _, zone := range []*time.Location{time.UTC, Location(), time.FixedZone("other", -7*3600)} {
		t.Run(zone.String(), func(t *testing.T) {
			input := gtime.NewFromTime(time.Date(2026, 10, 16, 15, 20, 0, 123456000, zone))
			before := input.Time
			got := FromTimestamp(input)
			if !got.Time.Equal(want) || got.Time.Location() != Location() {
				t.Fatalf("got=%s want=%s", got.Time, want)
			}
			if !FromTimestamp(got).Time.Equal(want) || input.Time != before {
				t.Fatal("conversion must be idempotent and must not mutate input")
			}
		})
	}
}

func TestFromTimestampDeadline(t *testing.T) {
	deadline := FromTimestamp(gtime.NewFromTime(time.Date(2026, 10, 16, 15, 20, 0, 0, time.UTC))).Time
	for _, zone := range []*time.Location{time.UTC, Location()} {
		for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			now := deadline.Add(delta).In(zone)
			if got, want := deadline.After(now), delta < 0; got != want {
				t.Fatalf("zone=%s delta=%s future=%t want=%t", zone, delta, got, want)
			}
		}
	}
}

func TestFromTimestampEmptyAndAbsoluteTime(t *testing.T) {
	if FromTimestamp(nil) != nil || !FromTime(time.Time{}).IsZero() || !FromTimestamp(gtime.NewFromTime(time.Time{})).IsZero() {
		t.Fatal("nil and zero must retain their meaning")
	}
	// 绝对时间只用 In 转换时区，不能误调用 FromTimestamp 重解释钟面。
	absolute := time.Unix(1792135200, 123)
	if !absolute.In(Location()).Equal(absolute) {
		t.Fatal("display zone conversion changed the instant")
	}
}
