package location

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hotgo/internal/model/entity"
)

// A preloaded in-memory directory keeps this API test independent of SQL,
// database configuration, and any production connection.
func TestRegionRecordsCachedValueCopy(t *testing.T) {
	directory := &regionDirectory{byCode: map[string]*entity.SysProvinces{
		"30":  {Id: 30, Pid: 20, Level: 3, Title: "区"},
		"10":  {Id: 10, Level: 1, Title: "省"},
		"20":  {Id: 20, Pid: 10, Level: 2, Title: "市"},
		"nil": nil,
	}}
	regionDirectoryCache.Lock()
	previous, expiresAt := regionDirectoryCache.value, regionDirectoryCache.expiresAt
	regionDirectoryCache.value = directory
	regionDirectoryCache.expiresAt = time.Now().Add(time.Hour)
	regionDirectoryCache.Unlock()
	t.Cleanup(func() {
		regionDirectoryCache.Lock()
		regionDirectoryCache.value, regionDirectoryCache.expiresAt = previous, expiresAt
		regionDirectoryCache.Unlock()
	})
	want := []RegionRecord{
		{Id: 10, Level: 1, Title: "省"},
		{Id: 20, Pid: 10, Level: 2, Title: "市"},
		{Id: 30, Pid: 20, Level: 3, Title: "区"},
	}
	first, err := RegionRecords(context.Background())
	if err != nil || !reflect.DeepEqual(first, want) {
		t.Fatalf("RegionRecords() = %+v, %v; want %+v", first, err, want)
	}
	first[0] = RegionRecord{Id: 999, Pid: 999, Level: 9, Title: "mutated"}
	first[1].Title = "mutated"
	first = append(first, RegionRecord{Id: 1000})
	again, err := RegionRecords(context.Background())
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatalf("caller mutation changed cached records: %+v, %v", again, err)
	}
	if directory.byCode["10"].Id != 10 || directory.byCode["20"].Title != "市" {
		t.Fatal("returned records exposed cached entity pointers")
	}
}

func TestRegionRecordsEmptyDirectory(t *testing.T) {
	directory := &regionDirectory{byCode: map[string]*entity.SysProvinces{}}
	if got := directory.records(); got == nil || len(got) != 0 {
		t.Fatalf("empty directory records = %+v, want empty value slice", got)
	}
}
